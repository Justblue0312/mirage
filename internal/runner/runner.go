package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justblue/mirage/internal/checksum"
	"github.com/justblue/mirage/internal/dialect"
)

const (
	colorReset = "\033[0m"
	colorGreen = "\033[32m"
	colorCyan  = "\033[36m"
)

var migrationRe = regexp.MustCompile(`^V(\d+)_(.+)\.sql$`)

// canonVersion normalizes a migration version string into a canonical form
// used consistently for all in-memory map keys across the runner and tracker.
// File-derived versions are raw filename digits (e.g. "00001"), while the
// tracker stores the version column as an integer and re-pads it (e.g. the
// "%014d" form "00000000000001", or the unpadded "1" from a string scan).
// canonVersion strips leading zeros and repads to the canonical %014d form so
// these sources compare equal. Non-numeric input (e.g. semantic versions) is
// returned unchanged so callers that key on such values don't break.
func canonVersion(v string) string {
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return v
	}
	return fmt.Sprintf("%014d", n)
}

// migrationLockKey is a fixed, arbitrary advisory-lock key identifying
// "a mirage migration is in progress against this database". Any process
// calling Migrate or Rollback against the same database blocks here until
// whichever other process (if any) holds it finishes and releases it.
const migrationLockKey int64 = 0x6D69726167650001 // "mirage" + a version byte, as a readable-in-hex constant

// withMigrationLock acquires a session-level Postgres advisory lock on a
// single connection checked out from pool, runs fn while holding it, and
// always releases the lock and returns the connection to the pool
// afterward -- including on panic/early-return paths, via defer.
func withMigrationLock(ctx context.Context, pool *pgxpool.Pool, fn func(ctx context.Context) error) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquiring connection for migration lock: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockKey); err != nil {
		return fmt.Errorf("acquiring migration lock: %w", err)
	}
	defer func() {
		// Best-effort: if this fails, the lock is still released when the
		// connection above is returned to the pool and eventually closed,
		// or immediately if this session ends -- pg_advisory_lock is
		// strictly session-scoped and cannot leak past the connection.
		_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLockKey)
	}()

	return fn(ctx)
}

type Runner struct {
	dialect dialect.TrackerDialect
}

func New(d dialect.TrackerDialect) *Runner {
	return &Runner{dialect: d}
}

func (r *Runner) Migrate(ctx context.Context, pool *pgxpool.Pool, migrationsDir string) error {
	return withMigrationLock(ctx, pool, func(ctx context.Context) error {
		return r.migrateLocked(ctx, pool, migrationsDir)
	})
}

func (r *Runner) migrateLocked(ctx context.Context, pool *pgxpool.Pool, migrationsDir string) error {
	if err := r.EnsureTracker(ctx, pool); err != nil {
		return fmt.Errorf("ensuring tracker: %w", err)
	}

	files, err := r.scanMigrations(migrationsDir)
	if err != nil {
		return fmt.Errorf("scanning migrations: %w", err)
	}

	applied, err := r.getApplied(ctx, pool)
	if err != nil {
		return fmt.Errorf("getting applied: %w", err)
	}

	appliedChecksums, err := r.getAppliedChecksums(ctx, pool)
	if err != nil {
		return fmt.Errorf("getting applied checksums: %w", err)
	}

	var pending []migrationFile
	for _, f := range files {
		if !applied[canonVersion(f.version)] {
			pending = append(pending, f)
		}
	}

	// Re-verify checksums of migrations that were already applied in a prior
	// run. This catches post-apply tampering: if someone edited a migration
	// file after it was recorded as applied, the file's current checksum will
	// no longer match what we stored, and we must refuse to proceed rather
	// than silently migrate "on top of" a drifted history.
	if err := r.verifyAppliedChecksums(files, applied, appliedChecksums); err != nil {
		return err
	}

	for _, f := range pending {
		if stored, ok := appliedChecksums[canonVersion(f.version)]; ok && stored != "" && stored != f.checksum {
			return fmt.Errorf("checksum mismatch for V%s_%s: file=%s db=%s — migration file may have been modified after being applied", f.version, f.name, f.checksum, stored)
		}
	}

	if len(pending) == 0 {
		return nil
	}

	for _, f := range pending {
		content, err := os.ReadFile(f.path)
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", f.version, err)
		}

		upBlock, err := parseUpBlock(string(content))
		if err != nil {
			return fmt.Errorf("parsing migration %s: %w", f.version, err)
		}

		stmts := splitStatements(upBlock)

		start := time.Now()

		if r.dialect.SupportsTransactionalDDL() {
			tx, err := pool.Begin(ctx)
			if err != nil {
				return fmt.Errorf("migration %s: %w", f.version, err)
			}

			for _, stmt := range stmts {
				stmt = strings.TrimSpace(stmt)
				if stmt == "" || strings.HasPrefix(stmt, "--") {
					continue
				}
				if _, err := tx.Exec(ctx, stmt); err != nil {
					_ = tx.Rollback(ctx) // best-effort: the Exec/recordApplied error above is what we return
					r.recordFailed(ctx, pool, f.version, f.name)
					return fmt.Errorf("migration %s: %w", f.version, err)
				}
			}

			if err := r.recordApplied(ctx, tx, f.version, f.name, f.checksum, nil); err != nil {
				_ = tx.Rollback(ctx) // best-effort: the Exec/recordApplied error above is what we return
				return fmt.Errorf("migration %s: %w", f.version, err)
			}

			if err := tx.Commit(ctx); err != nil {
				return fmt.Errorf("migration %s: %w", f.version, err)
			}
		} else {
			for _, stmt := range stmts {
				stmt = strings.TrimSpace(stmt)
				if stmt == "" || strings.HasPrefix(stmt, "--") {
					continue
				}
				if _, err := pool.Exec(ctx, stmt); err != nil {
					r.recordFailed(ctx, pool, f.version, f.name)
					return fmt.Errorf("migration %s: %w", f.version, err)
				}
			}
			if err := r.recordApplied(ctx, pool, f.version, f.name, f.checksum, nil); err != nil {
				return fmt.Errorf("migration %s: %w", f.version, err)
			}
		}

		elapsed := time.Since(start)
		fmt.Printf("  %s Applying V%s_%s ... %sOK%s (%v)\n", colorCyan, f.version, f.name, colorGreen, colorReset, elapsed)
	}

	fmt.Printf("\n%s✓%s %d migration(s) applied successfully\n", colorGreen, colorReset, len(pending))
	return nil
}

func (r *Runner) Rollback(ctx context.Context, pool *pgxpool.Pool, migrationsDir string, n int) error {
	return withMigrationLock(ctx, pool, func(ctx context.Context) error {
		return r.rollbackLocked(ctx, pool, migrationsDir, n)
	})
}

func (r *Runner) rollbackLocked(ctx context.Context, pool *pgxpool.Pool, migrationsDir string, n int) error {
	if err := r.EnsureTracker(ctx, pool); err != nil {
		return fmt.Errorf("ensuring tracker: %w", err)
	}

	rolledBack, err := r.getRecentApplied(ctx, pool, n)
	if err != nil {
		return fmt.Errorf("getting applied: %w", err)
	}

	if len(rolledBack) == 0 {
		return nil
	}

	files, err := r.scanMigrations(migrationsDir)
	if err != nil {
		return fmt.Errorf("scanning migrations: %w", err)
	}

	fileMap := make(map[string]migrationFile)
	for _, f := range files {
		fileMap[canonVersion(f.version)] = f
	}

	for i := len(rolledBack) - 1; i >= 0; i-- {
		row := rolledBack[i]

		f, ok := fileMap[canonVersion(row.version)]
		if !ok {
			return fmt.Errorf("migration file not found for version %s", row.version)
		}

		content, err := os.ReadFile(f.path)
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", f.version, err)
		}

		downBlock, err := parseDownBlock(string(content))
		if err != nil {
			return fmt.Errorf("parsing migration %s: %w", f.version, err)
		}

		stmts := splitStatements(downBlock)

		start := time.Now()

		if r.dialect.SupportsTransactionalDDL() {
			tx, err := pool.Begin(ctx)
			if err != nil {
				return fmt.Errorf("rollback %s: %w", f.version, err)
			}

			for _, stmt := range stmts {
				stmt = strings.TrimSpace(stmt)
				if stmt == "" || strings.HasPrefix(stmt, "--") {
					continue
				}
				if _, err := tx.Exec(ctx, stmt); err != nil {
					_ = tx.Rollback(ctx) // best-effort: the Exec/recordApplied error above is what we return
					return fmt.Errorf("rollback %s: %w", f.version, err)
				}
			}

			if _, err := tx.Exec(ctx, r.dialect.RecordRolledBackSQL(), row.version); err != nil {
				_ = tx.Rollback(ctx) // best-effort: the Exec/recordApplied error above is what we return
				return fmt.Errorf("rollback %s: %w", f.version, err)
			}

			if err := tx.Commit(ctx); err != nil {
				return fmt.Errorf("rollback %s: %w", f.version, err)
			}
		} else {
			for _, stmt := range stmts {
				stmt = strings.TrimSpace(stmt)
				if stmt == "" || strings.HasPrefix(stmt, "--") {
					continue
				}
				if _, err := pool.Exec(ctx, stmt); err != nil {
					return fmt.Errorf("rollback %s: %w", f.version, err)
				}
			}
			if _, err := pool.Exec(ctx, r.dialect.RecordRolledBackSQL(), row.version); err != nil {
				return fmt.Errorf("rollback %s: %w", f.version, err)
			}
		}

		elapsed := time.Since(start)
		fmt.Printf("  %s Rolling back V%s_%s ... %sOK%s (%v)\n", colorCyan, f.version, f.name, colorGreen, colorReset, elapsed)
	}

	fmt.Printf("\n%s✓%s %d migration(s) rolled back\n", colorGreen, colorReset, len(rolledBack))
	return nil
}

// verifyAppliedChecksums re-checks the on-disk files of migrations that were
// already recorded as applied against the checksums stored when they were
// applied. A mismatch means the file was modified after it was applied
// (post-apply tampering or a history drift), and we refuse to continue.
// Pending (never-applied) files are not checked here -- they are validated by
// the caller's own pending-checksum loop.
func (r *Runner) verifyAppliedChecksums(files []migrationFile, applied map[string]bool, storedChecksums map[string]string) error {
	pathByVersion := make(map[string]string, len(files))
	for _, f := range files {
		pathByVersion[canonVersion(f.version)] = f.path
	}

	for version, stored := range storedChecksums {
		if !applied[version] {
			continue
		}
		path, ok := pathByVersion[version]
		if !ok {
			// Applied in the DB but no longer present on disk: nothing to
			// compare against. Leave it to the apply path / operator to
			// surface; we don't fail closed here because the missing file
			// cannot have been "tampered with" in a way that matters.
			continue
		}
		computed, err := checksumFromFile(path)
		if err != nil {
			return fmt.Errorf("recomputing checksum for applied migration V%s: %w", version, err)
		}
		if stored != "" && stored != computed {
			return fmt.Errorf("checksum mismatch for already-applied V%s: current-file=%s db=%s — migration file was modified after being applied; refusing to proceed", version, computed, stored)
		}
	}
	return nil
}

// DryMigrate prints the pending migrations without applying them.
func (r *Runner) DryMigrate(ctx context.Context, pool *pgxpool.Pool, migrationsDir string) error {
	if err := r.EnsureTracker(ctx, pool); err != nil {
		return fmt.Errorf("ensuring tracker: %w", err)
	}

	files, err := r.scanMigrations(migrationsDir)
	if err != nil {
		return fmt.Errorf("scanning migrations: %w", err)
	}

	applied, err := r.getApplied(ctx, pool)
	if err != nil {
		return fmt.Errorf("getting applied: %w", err)
	}

	appliedChecksums, err := r.getAppliedChecksums(ctx, pool)
	if err != nil {
		return fmt.Errorf("getting applied checksums: %w", err)
	}

	// Re-verify already-applied migrations even in dry-run, so tampering is
	// surfaced before anyone relies on the printed plan.
	if err := r.verifyAppliedChecksums(files, applied, appliedChecksums); err != nil {
		return err
	}

	var pending []migrationFile
	for _, f := range files {
		if !applied[canonVersion(f.version)] {
			pending = append(pending, f)
		}
	}

	if len(pending) == 0 {
		fmt.Println("No pending migrations.")
		return nil
	}

	fmt.Printf("Pending migrations (%d):\n\n", len(pending))
	for _, f := range pending {
		content, err := os.ReadFile(f.path)
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", f.version, err)
		}
		upBlock, err := parseUpBlock(string(content))
		if err != nil {
			return fmt.Errorf("parsing migration %s: %w", f.version, err)
		}

		fmt.Printf("  %s V%s_%s\n", colorCyan, f.version, f.name)
		for _, line := range strings.Split(strings.TrimSpace(upBlock), "\n") {
			fmt.Printf("    %s\n", line)
		}
		fmt.Println()
	}
	return nil
}

// DryRollback prints the down statements for the last N migrations without applying them.
func (r *Runner) DryRollback(ctx context.Context, pool *pgxpool.Pool, migrationsDir string, n int) error {
	if err := r.EnsureTracker(ctx, pool); err != nil {
		return fmt.Errorf("ensuring tracker: %w", err)
	}

	rolledBack, err := r.getRecentApplied(ctx, pool, n)
	if err != nil {
		return fmt.Errorf("getting applied: %w", err)
	}

	if len(rolledBack) == 0 {
		fmt.Println("No applied migrations to roll back.")
		return nil
	}

	files, err := r.scanMigrations(migrationsDir)
	if err != nil {
		return fmt.Errorf("scanning migrations: %w", err)
	}

	fileMap := make(map[string]migrationFile)
	for _, f := range files {
		fileMap[canonVersion(f.version)] = f
	}

	fmt.Printf("Migrations to roll back (%d):\n\n", len(rolledBack))
	for i := len(rolledBack) - 1; i >= 0; i-- {
		row := rolledBack[i]
		f, ok := fileMap[canonVersion(row.version)]
		if !ok {
			return fmt.Errorf("migration file not found for version %s", row.version)
		}
		content, err := os.ReadFile(f.path)
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", f.version, err)
		}
		downBlock, err := parseDownBlock(string(content))
		if err != nil {
			return fmt.Errorf("parsing migration %s: %w", f.version, err)
		}

		fmt.Printf("  %s V%s_%s\n", colorCyan, f.version, f.name)
		for _, line := range strings.Split(strings.TrimSpace(downBlock), "\n") {
			fmt.Printf("    %s\n", line)
		}
		fmt.Println()
	}
	return nil
}

// MigrationInfo describes a single migration file discovered on disk. It is the
// canonical, exported representation used by both the runner and the CLI so
// that migration-filename parsing rules stay consistent across commands.
type MigrationInfo struct {
	Version  string
	Name     string
	Path     string
	Checksum string
}

type migrationFile struct {
	version  string
	name     string
	path     string
	checksum string
}

// ScanMigrations discovers migration files in dir using the canonical
// `V{digits}_{description}.sql` naming rule, computing each file's checksum.
// Results are sorted by version. This is the single source of truth for
// migration-file parsing; CLI commands must use it rather than reimplementing
// their own looser parser.
func ScanMigrations(dir string) ([]MigrationInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []MigrationInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := migrationRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		path := filepath.Join(dir, e.Name())
		cs, err := checksumFromFile(path)
		if err != nil {
			return nil, fmt.Errorf("computing checksum for %s: %w", e.Name(), err)
		}
		files = append(files, MigrationInfo{
			Version:  m[1],
			Name:     m[2],
			Path:     path,
			Checksum: cs,
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Version < files[j].Version
	})

	return files, nil
}

func (r *Runner) scanMigrations(dir string) ([]migrationFile, error) {
	infos, err := ScanMigrations(dir)
	if err != nil {
		return nil, err
	}

	files := make([]migrationFile, len(infos))
	for i, info := range infos {
		files[i] = migrationFile{
			version:  info.Version,
			name:     info.Name,
			path:     info.Path,
			checksum: info.Checksum,
		}
	}
	return files, nil
}

func parseUpBlock(content string) (string, error) {
	upMarker := "-- +migrate Up"
	downMarker := "-- +migrate Down"

	var upStart int
	var downStart int
	lines := strings.Split(content, "\n")

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == upMarker && upStart == 0 {
			upStart = i + 1
		} else if trimmed == downMarker && upStart > 0 {
			downStart = i
			break
		}
	}

	if upStart == 0 {
		return "", fmt.Errorf("missing up marker")
	}

	if downStart == 0 {
		return strings.TrimSpace(strings.Join(lines[upStart:], "\n")), nil
	}

	return strings.TrimSpace(strings.Join(lines[upStart:downStart], "\n")), nil
}

func parseDownBlock(content string) (string, error) {
	downMarker := "-- +migrate Down"

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == downMarker {
			return strings.TrimSpace(strings.Join(lines[i+1:], "\n")), nil
		}
	}

	return "", fmt.Errorf("missing down marker")
}

func checksumFromFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	upBlock, err := parseUpBlock(string(content))
	if err != nil {
		return "", err
	}
	stmts := splitStatements(upBlock)
	return checksum.Compute(stmts), nil
}

func splitStatements(block string) []string {
	lines := strings.Split(block, "\n")
	var filtered []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		filtered = append(filtered, line)
	}
	block = strings.Join(filtered, "\n")

	var stmts []string
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	inDollarQuote := false
	dollarTag := ""
	escaped := false

	runes := []rune(block)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]

		if escaped {
			current.WriteRune(ch)
			escaped = false
			continue
		}

		// Dollar-quoted string handling: $tag$ ... $tag$
		// Detect the opening tag when not inside any other quote context.
		if !inSingleQuote && !inDoubleQuote && !inDollarQuote && ch == '$' {
			if tag := dollarOpenTag(string(runes[i:]), ch); tag != "" {
				inDollarQuote = true
				dollarTag = tag
				current.WriteString(tag)
				i += len([]rune(tag)) - 1
				continue
			}
		}

		if inDollarQuote {
			current.WriteRune(ch)
			if ch == '$' && strings.HasSuffix(current.String(), dollarTag) {
				inDollarQuote = false
				dollarTag = ""
			}
			continue
		}

		switch ch {
		case '\\':
			escaped = true
			current.WriteRune(ch)
			continue
		case '\'':
			if !inDoubleQuote {
				inSingleQuote = !inSingleQuote
			}
		case '"':
			if !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
			}
		case ';':
			if !inSingleQuote && !inDoubleQuote {
				trimmed := strings.TrimSpace(current.String())
				if trimmed != "" {
					stmts = append(stmts, trimmed+";")
				}
				current.Reset()
				continue
			}
		}

		current.WriteRune(ch)
	}

	trimmed := strings.TrimSpace(current.String())
	if trimmed != "" {
		stmts = append(stmts, trimmed+";")
	}

	return stmts
}

// dollarOpenTag inspects the rune slice starting at the given dollar sign and
// returns the full opening tag (e.g. "$function$" or "$$") if it is a valid
// PostgreSQL dollar-quote opening tag, or "" otherwise. A valid tag is a '$'
// followed by zero or more letters/digits/underscores and a closing '$'.
func dollarOpenTag(s string, _ rune) string {
	if len(s) < 2 || s[0] != '$' {
		return ""
	}
	end := 1
	for end < len(s) && s[end] != '$' {
		c := s[end]
		if !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') &&
			!(c >= '0' && c <= '9') && c != '_' {
			return ""
		}
		end++
	}
	if end >= len(s) {
		return ""
	}
	return s[:end+1]
}
