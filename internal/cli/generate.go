package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justblue/mirage/internal/dialect/postgres"
	"github.com/justblue/mirage/internal/diff"
	"github.com/justblue/mirage/internal/generator"
	"github.com/justblue/mirage/internal/runner"
	"github.com/justblue/mirage/internal/scanner"
	"github.com/justblue/mirage/internal/schema"
	"github.com/justblue/mirage/internal/validate"
)

// generateOptions holds the already-parsed inputs for the generate pipeline.
// It decouples the core scan→validate→diff→generate flow from urfave/cli flag
// parsing so the pipeline can be unit-tested directly.
type generateOptions struct {
	SourceDirs   []string
	Recursive    bool
	ConnString   string
	ResetState   bool
	Force        bool
	Message      string
	Sequential   bool
	MigrationDir string
	Idempotent   bool

	// ConfirmDestructive is called when destructive changes are detected and
	// Force is false. Returning true proceeds; false aborts. When nil, the
	// pipeline fails closed on destructive changes (as a non-interactive run).
	ConfirmDestructive func() (bool, error)
	// ConfirmRename is called for each detected column-rename hint (unless
	// Force is set, which auto-confirms). Returning true collapses the
	// drop+add pair into a rename. When nil, renames are left as drop+add.
	ConfirmRename func(hint diff.RenameHint) (bool, error)
}

// generateResult carries the outcome of a successful pipeline run.
type generateResult struct {
	// File is the generated migration, or nil if there were no changes.
	File *generator.MigrationFile
	// NoChanges is true when models matched the saved state.
	NoChanges bool
	// Warnings collects non-fatal diagnostics for the caller to surface.
	Warnings []string
	// NewSnapshot is the marshalled snapshot of the new package to persist,
	// or nil when there is no database to save to.
	NewSnapshot []byte
}

// generatePipeline runs the scan→validate→diff→generate flow against the given
// options. It performs no terminal I/O or file writes itself; callers handle
// prompting (via the Confirm* hooks), printing, and persistence. The pool, when
// non-nil, is used only to load the previous snapshot for diffing.
func generatePipeline(ctx context.Context, opts generateOptions, pool *pgxpool.Pool) (*generateResult, error) {
	res := &generateResult{}

	if len(opts.SourceDirs) == 0 {
		return nil, fmt.Errorf("at least one source directory is required")
	}

	s := &scanner.Scanner{SourceDirs: opts.SourceDirs, Recursive: opts.Recursive}
	pkg, err := s.Scan()
	if err != nil {
		return nil, fmt.Errorf("scanning: %w", err)
	}

	valErrs := validate.Validate(pkg)
	for _, e := range valErrs {
		if !e.IsError() {
			res.Warnings = append(res.Warnings, fmt.Sprintf("[%s] %s: %s", e.Code, e.Table, e.Message))
		}
	}
	if countErrors(valErrs) > 0 {
		for _, e := range valErrs {
			if e.IsError() {
				return nil, fmt.Errorf("[%s] %s: %s", e.Code, e.Table, e.Message)
			}
		}
		return nil, fmt.Errorf("validation failed with %d error(s)", countErrors(valErrs))
	}

	var oldTree schema.Package
	if pool != nil {
		r := runner.New(getDialect())
		if err := r.EnsureTracker(ctx, pool); err != nil {
			return nil, fmt.Errorf("ensuring tracker: %w", err)
		}
		snapshot, serr := r.LoadSnapshot(ctx, pool)
		if serr == nil && len(snapshot) > 0 {
			if err := json.Unmarshal(snapshot, &oldTree); err != nil {
				return nil, fmt.Errorf("parsing snapshot: %w", err)
			}
		}
	}

	if opts.ResetState {
		oldTree = schema.Package{}
		res.Warnings = append(res.Warnings, "--reset-state: DISCARDED all saved migration history; the next migration will treat the entire schema as new")
	}

	if diff.PackagesEqual(&oldTree, pkg) && !opts.Force {
		res.NoChanges = true
		return res, nil
	}

	events := diff.Diff(&oldTree, pkg)
	if len(events) == 0 {
		res.NoChanges = true
		return res, nil
	}

	var colAdds []validate.ColumnAddition
	for _, e := range events {
		if e.Kind == diff.ColumnAdded {
			colAdds = append(colAdds, validate.ColumnAddition{Table: e.Table, Column: e.Column, NewColumn: e.NewColumn})
		}
	}
	existingTables := make(map[string]*schema.Table, len(oldTree.Tables))
	for i := range oldTree.Tables {
		existingTables[oldTree.Tables[i].Name] = &oldTree.Tables[i]
	}
	for _, w := range validate.ValidateNotNullNoDefault(colAdds, existingTables) {
		res.Warnings = append(res.Warnings, fmt.Sprintf("[%s] %s", w.Code, w.Message))
	}

	var confirmedRenames []diff.RenameHint
	for _, h := range diff.DetectColumnRenames(&oldTree, pkg) {
		res.Warnings = append(res.Warnings, fmt.Sprintf("[rename_hint] column %q on table %q may have been renamed to %q", h.OldColumn, h.Table, h.NewColumn))
		switch {
		case opts.Force:
			confirmedRenames = append(confirmedRenames, h)
		case opts.ConfirmRename != nil:
			ok, cerr := opts.ConfirmRename(h)
			if cerr != nil {
				return nil, cerr
			}
			if ok {
				confirmedRenames = append(confirmedRenames, h)
			}
		}
	}
	if len(confirmedRenames) > 0 {
		events = diff.ApplyRenameHints(events, confirmedRenames)
	}

	hasDestructive := false
	for _, e := range events {
		if e.Severity == diff.Destructive {
			hasDestructive = true
			break
		}
	}
	if hasDestructive && !opts.Force {
		if opts.ConfirmDestructive == nil {
			return nil, fmt.Errorf(
				"destructive changes detected and no terminal is attached to confirm; " +
					"re-run with --force to proceed non-interactively, or run generate " +
					"interactively to confirm")
		}
		ok, cerr := opts.ConfirmDestructive()
		if cerr != nil {
			return nil, cerr
		}
		if !ok {
			return nil, fmt.Errorf("aborted by user")
		}
	}

	gen := generator.New(getDialect(), events, &oldTree, pkg)
	mf, err := gen.Generate()
	if err != nil {
		return nil, fmt.Errorf("generating: %w", err)
	}

	if opts.Sequential {
		mf.Version = nextSequentialVersion(opts.MigrationDir)
	}
	if opts.Message != "" {
		mf.Message = opts.Message
		mf.Description = strings.ToLower(strings.ReplaceAll(opts.Message, " ", "_"))
	}

	res.File = mf

	if pool != nil {
		snapshotData, err := json.Marshal(pkg)
		if err != nil {
			return nil, fmt.Errorf("marshaling snapshot: %w", err)
		}
		res.NewSnapshot = snapshotData
	}

	return res, nil
}

func cmdGenerate() *cli.Command {
	return &cli.Command{
		Name:  "generate",
		Usage: "scan structs, diff state, and generate migration files",
		Flags: []cli.Flag{
			&cli.StringSliceFlag{Name: "source", Usage: "source dir(s) to scan"},
			&cli.StringFlag{Name: "migrations-dir", Aliases: []string{"dir"}, Value: "./migrations", Usage: "migrations output directory"},
			&cli.StringFlag{Name: "db", Usage: "database connection string (loads/saves snapshot state)"},
			&cli.StringFlag{Name: "message", Aliases: []string{"m"}, Usage: "migration description"},
			&cli.BoolFlag{Name: "recursive", Aliases: []string{"r"}, Usage: "scan subdirectories recursively"},
			&cli.BoolFlag{Name: "dry-run", Usage: "print DDL; do not write files"},
			&cli.BoolFlag{Name: "force", Usage: "skip destructive-change prompt and allow regeneration from scratch"},
			&cli.BoolFlag{Name: "reset-state", Usage: "discard saved state and regenerate migration from scratch"},
			&cli.BoolFlag{Name: "sequential-naming", Usage: "use sequential version numbers (00001, 00002, ...) instead of timestamps"},
			&cli.BoolFlag{Name: "verbose", Aliases: []string{"v"}, Usage: "verbose diagnostic output"},
			&cli.BoolFlag{Name: "idempotent", Usage: "wrap up statements in DO blocks / IF NOT EXISTS for safe re-runs"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			dirs := cmd.StringSlice("source")
			if len(dirs) == 0 {
				return fmt.Errorf("--source is required (e.g. --source ./internal/models)")
			}

			migrationsDir := cmd.String("migrations-dir")
			dryRun := cmd.Bool("dry-run")
			verbose := cmd.Bool("verbose")

			if err := os.MkdirAll(migrationsDir, 0755); err != nil {
				return fmt.Errorf("creating migrations directory: %w", err)
			}

			var pool *pgxpool.Pool
			if connStr := cmd.String("db"); connStr != "" {
				var err error
				pool, err = pgxpool.New(ctx, connStr)
				if err != nil {
					return fmt.Errorf("connecting to db: %w", err)
				}
				defer pool.Close()
			} else if verbose {
				dim("No --db flag, treating as first run")
			}

			opts := generateOptions{
				SourceDirs:         dirs,
				Recursive:          cmd.Bool("recursive"),
				ConnString:         cmd.String("db"),
				ResetState:         cmd.Bool("reset-state"),
				Force:              cmd.Bool("force"),
				Message:            cmd.String("message"),
				Sequential:         cmd.Bool("sequential-naming"),
				MigrationDir:       migrationsDir,
				Idempotent:         cmd.Bool("idempotent"),
				ConfirmRename:      makeRenameConfirmer(dryRun),
				ConfirmDestructive: makeDestructiveConfirmer(dryRun),
			}

			res, err := generatePipeline(ctx, opts, pool)
			if err != nil {
				return err
			}

			for _, w := range res.Warnings {
				warn(w)
			}

			if res.NoChanges {
				info("No changes detected — models match saved state")
				return nil
			}

			mf := res.File

			if opts.Idempotent {
				d := postgres.New()
				mf.UpStatements = d.WrapIdempotent(mf.UpStatements)
			}

			if dryRun {
				header("Dry Run — Up Statements")
				for i, s := range mf.UpStatements {
					fmt.Printf("  %d. %s\n", i+1, s)
				}
				return nil
			}

			fileName := fmt.Sprintf("V%s_%s.sql", mf.Version, mf.Description)
			filePath := filepath.Join(migrationsDir, fileName)

			content := formatMigrationFile(mf)
			if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
				return fmt.Errorf("writing migration: %w", err)
			}

			if pool != nil && res.NewSnapshot != nil {
				r := runner.New(getDialect())
				if err := r.SaveSnapshot(ctx, pool, mf.Version, mf.Description, res.NewSnapshot); err != nil {
					return fmt.Errorf("saving snapshot: %w", err)
				}
				if verbose {
					dim(fmt.Sprintf("Saved snapshot to database for version %s", mf.Version))
				}
			}

			header("Migration Generated")
			success(fmt.Sprintf("File: %s", filePath))
			info(fmt.Sprintf("Up:   %d statements", len(mf.UpStatements)))
			info(fmt.Sprintf("Down: %d statements", len(mf.DownStatements)))
			if mf.HasDestructive {
				warn("Destructive changes detected in this migration")
			}

			return nil
		},
	}
}

// makeRenameConfirmer returns a ConfirmRename hook that prompts interactively
// when a TTY is attached, and otherwise declines (preserving the conservative
// drop+add behavior). In dry-run mode it always declines since nothing is
// written.
func makeRenameConfirmer(dryRun bool) func(diff.RenameHint) (bool, error) {
	if dryRun {
		return nil
	}
	if !isTTY(os.Stdin.Fd()) {
		return nil
	}
	return func(h diff.RenameHint) (bool, error) {
		fmt.Printf("Treat drop of %q + add of %q on table %q as a RENAME (preserves data)? [y/N] ", h.OldColumn, h.NewColumn, h.Table)
		reader := bufio.NewReader(os.Stdin)
		line, rerr := reader.ReadString('\n')
		if rerr != nil && line == "" {
			return false, fmt.Errorf("reading rename confirmation from stdin: %w", rerr)
		}
		return strings.TrimSpace(strings.ToLower(line)) == "y", nil
	}
}

// makeDestructiveConfirmer returns a ConfirmDestructive hook. In dry-run mode
// nothing is written, so destructive changes are auto-approved. With a TTY it
// prompts; without one it returns nil so the pipeline fails closed.
func makeDestructiveConfirmer(dryRun bool) func() (bool, error) {
	if dryRun {
		return func() (bool, error) { return true, nil }
	}
	if !isTTY(os.Stdin.Fd()) {
		return nil
	}
	return func() (bool, error) {
		fmt.Print("WARNING: destructive changes detected. Continue? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		line, rerr := reader.ReadString('\n')
		if rerr != nil && line == "" {
			return false, fmt.Errorf("reading confirmation from stdin: %w", rerr)
		}
		return strings.TrimSpace(strings.ToLower(line)) == "y", nil
	}
}
