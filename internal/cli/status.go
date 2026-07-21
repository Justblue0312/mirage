package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justblue/mirage/internal/diff"
	"github.com/justblue/mirage/internal/introspect"
	"github.com/justblue/mirage/internal/runner"
	"github.com/justblue/mirage/internal/schema"
)

func cmdStatus() *cli.Command {
	return &cli.Command{
		Name:  "status",
		Usage: "show applied and pending migrations",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "db", Usage: "database connection string (optional, shows file-only status if omitted)"},
			&cli.StringFlag{Name: "dir", Value: "./migrations", Usage: "migrations directory"},
			&cli.StringFlag{Name: "format", Value: "table", Usage: "table|json"},
			&cli.BoolFlag{Name: "check-drift", Usage: "compare the live database schema against the last-applied snapshot and report any manual changes (requires --db)"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			dir := cmd.String("dir")
			connStr := cmd.String("db")
			checkDrift := cmd.Bool("check-drift")

			if checkDrift && connStr == "" {
				return fmt.Errorf("--check-drift requires --db to connect to the database")
			}

			files, err := runner.ScanMigrations(dir)
			if err != nil {
				return fmt.Errorf("scanning: %w", err)
			}

			type StatusEntry struct {
				Version   string `json:"version"`
				Name      string `json:"name"`
				AppliedAt string `json:"applied_at"`
				State     string `json:"state"`
			}

			var entries []StatusEntry

			if connStr != "" {
				pool, err := pgxpool.New(ctx, connStr)
				if err != nil {
					return fmt.Errorf("connecting: %w", err)
				}
				defer pool.Close()

				d := getDialect()
				rows, err := pool.Query(ctx, d.ListMigrationsSQL())
				if err != nil {
					return fmt.Errorf("querying: %w", err)
				}
				defer rows.Close()

				applied := make(map[string]StatusEntry)
				for rows.Next() {
					var e StatusEntry
					if err := rows.Scan(&e.Version, &e.Name, &e.AppliedAt, &e.State); err != nil {
						return err
					}
					applied[e.Version] = e
				}

				for _, f := range files {
					if a, ok := applied[f.Version]; ok {
						entries = append(entries, a)
					} else {
						entries = append(entries, StatusEntry{
							Version:   f.Version,
							Name:      f.Name,
							AppliedAt: "(pending)",
							State:     "-",
						})
					}
				}

				if checkDrift {
					return runDriftCheck(ctx, pool)
				}
			} else {
				for _, f := range files {
					entries = append(entries, StatusEntry{
						Version:   f.Version,
						Name:      f.Name,
						AppliedAt: "(pending)",
						State:     "-",
					})
				}
			}

			if cmd.String("format") == "json" {
				data, err := json.MarshalIndent(entries, "", "  ")
				if err != nil {
					return fmt.Errorf("marshaling status to JSON: %w", err)
				}
				fmt.Println(string(data))
				return nil
			}

			header("Migration Status")
			fmt.Println()
			fmt.Printf("  %-16s %-40s %-22s %s\n", "VERSION", "NAME", "APPLIED AT", "STATE")
			fmt.Printf("  %s\n", strings.Repeat("─", 86))
			for _, e := range entries {
				stateIcon := "○"
				if e.State == "applied" {
					stateIcon = "✓"
				}
				if !useColor {
					stateIcon = "o"
					if e.State == "applied" {
						stateIcon = "x"
					}
				}
				fmt.Printf("  %s %-16s %-40s %-22s %s\n", stateIcon, e.Version, e.Name, e.AppliedAt, e.State)
			}
			fmt.Println()

			if connStr == "" {
				dim("(file-only mode — no database connection)")
			}

			return nil
		},
	}
}

// runDriftCheck loads the last-applied snapshot, introspects the live
// database, diffs them, and prints the result.
func runDriftCheck(ctx context.Context, pool *pgxpool.Pool) error {
	d := getDialect()
	r := runner.New(d)

	if err := r.EnsureTracker(ctx, pool); err != nil {
		return fmt.Errorf("ensuring tracker: %w", err)
	}

	snapshot, err := r.LoadSnapshot(ctx, pool)
	if err != nil {
		return fmt.Errorf("loading snapshot: %w", err)
	}
	if len(snapshot) == 0 {
		dim("No snapshot found in schema_migrations — run mirage generate first")
		return nil
	}

	var snapshotSchema schema.Package
	if err := json.Unmarshal(snapshot, &snapshotSchema); err != nil {
		return fmt.Errorf("parsing snapshot: %w", err)
	}

	// Determine search path for introspection.
	searchPath := determineSearchPath(ctx, pool)

	fmt.Printf("Checking for schema drift against last-applied snapshot...\n\n")

	liveSchema, err := introspect.FromLiveDatabase(ctx, pool, searchPath)
	if err != nil {
		return fmt.Errorf("introspecting live database: %w", err)
	}

	events := diff.Diff(&snapshotSchema, liveSchema)
	if len(events) == 0 {
		if useColor {
			fmt.Printf("  ✓ No drift detected — live database matches the last-applied snapshot.\n")
		} else {
			fmt.Printf("  x No drift detected — live database matches the last-applied snapshot.\n")
		}
		return nil
	}

	warn("Drift detected — the live database does not match the last-applied snapshot:\n")
	for _, e := range events {
		fmt.Printf("  • %s\n", formatDriftEvent(e))
	}
	fmt.Println()
	dim("These changes were not made through mirage generate/migrate.")
	dim("If intentional, revert them or capture them with: mirage generate --source ./models")

	return nil
}

// determineSearchPath queries the database for the current search_path.
func determineSearchPath(ctx context.Context, pool *pgxpool.Pool) string {
	var searchPath string
	err := pool.QueryRow(ctx, `SHOW search_path`).Scan(&searchPath)
	if err != nil {
		return "public"
	}
	// Strip quotes and "public" prefix if present, fall back to "public".
	searchPath = strings.Trim(searchPath, "\"")
	if searchPath == "" {
		return "public"
	}
	return searchPath
}

// formatDriftEvent produces a human-readable description of a diff event.
func formatDriftEvent(e diff.DeltaEvent) string {
	switch e.Kind {
	case diff.TableAdded:
		return fmt.Sprintf("table %q exists in database, absent from snapshot", e.Table)
	case diff.TableDropped:
		return fmt.Sprintf("table %q exists in snapshot, absent from database", e.Table)
	case diff.ColumnAdded:
		return fmt.Sprintf("%s.%s: column present in database, absent from snapshot", e.Table, e.Column)
	case diff.ColumnDropped:
		return fmt.Sprintf("%s.%s: column present in snapshot, absent from database", e.Table, e.Column)
	case diff.ColumnTypeChanged:
		return fmt.Sprintf("%s.%s: type changed from %q (snapshot) to %q (database)", e.Table, e.Column, e.OldValue, e.NewValue)
	case diff.ColumnNullChanged:
		return fmt.Sprintf("%s.%s: nullable changed from %q (snapshot) to %q (database)", e.Table, e.Column, e.OldValue, e.NewValue)
	case diff.ColumnDefaultChanged:
		return fmt.Sprintf("%s.%s: default changed from %q (snapshot) to %q (database)", e.Table, e.Column, e.OldValue, e.NewValue)
	case diff.IndexAdded:
		return fmt.Sprintf("index %q exists in database, absent from snapshot", e.ConstraintName)
	case diff.IndexDropped:
		return fmt.Sprintf("index %q exists in snapshot, absent from database", e.ConstraintName)
	case diff.UniqueAdded:
		return fmt.Sprintf("unique constraint %q exists in database, absent from snapshot", e.ConstraintName)
	case diff.UniqueDropped:
		return fmt.Sprintf("unique constraint %q exists in snapshot, absent from database", e.ConstraintName)
	case diff.FKAdded:
		return fmt.Sprintf("foreign key %q exists in database, absent from snapshot", e.ConstraintName)
	case diff.FKDropped:
		return fmt.Sprintf("foreign key %q exists in snapshot, absent from database", e.ConstraintName)
	case diff.CheckAdded:
		return fmt.Sprintf("check constraint %q exists in database, absent from snapshot", e.ConstraintName)
	case diff.CheckDropped:
		return fmt.Sprintf("check constraint %q exists in snapshot, absent from database", e.ConstraintName)
	case diff.PKChanged:
		return fmt.Sprintf("%s: primary key changed", e.Table)
	case diff.EnumAdded:
		return fmt.Sprintf("enum %q exists in database, absent from snapshot", e.Enum)
	case diff.EnumDropped:
		return fmt.Sprintf("enum %q exists in snapshot, absent from database", e.Enum)
	case diff.EnumValueAdded:
		return fmt.Sprintf("enum %q: value %q added in database", e.Enum, e.NewValue)
	case diff.EnumValueDropped:
		return fmt.Sprintf("enum %q: value %q dropped from database", e.Enum, e.OldValue)
	case diff.ExtensionAdded:
		return fmt.Sprintf("extension %q exists in database, absent from snapshot", e.ConstraintName)
	case diff.ExtensionDropped:
		return fmt.Sprintf("extension %q exists in snapshot, absent from database", e.ConstraintName)
	case diff.FunctionAdded:
		return fmt.Sprintf("function %q exists in database, absent from snapshot", e.Table)
	case diff.FunctionDropped:
		return fmt.Sprintf("function %q exists in snapshot, absent from database", e.Table)
	case diff.FunctionBodyChanged:
		return fmt.Sprintf("function %q: body changed", e.Table)
	case diff.ViewAdded:
		return fmt.Sprintf("view %q exists in database, absent from snapshot", e.Table)
	case diff.ViewDropped:
		return fmt.Sprintf("view %q exists in snapshot, absent from database", e.Table)
	case diff.ViewQueryChanged:
		return fmt.Sprintf("view %q: query changed", e.Table)
	case diff.MaterializedViewAdded:
		return fmt.Sprintf("materialized view %q exists in database, absent from snapshot", e.Table)
	case diff.MaterializedViewDropped:
		return fmt.Sprintf("materialized view %q exists in snapshot, absent from database", e.Table)
	case diff.MaterializedViewQueryChanged:
		return fmt.Sprintf("materialized view %q: query changed", e.Table)
	case diff.TriggerAdded:
		return fmt.Sprintf("trigger %q exists in database, absent from snapshot", e.ConstraintName)
	case diff.TriggerDropped:
		return fmt.Sprintf("trigger %q exists in snapshot, absent from database", e.ConstraintName)
	case diff.ProcedureAdded:
		return fmt.Sprintf("procedure %q exists in database, absent from snapshot", e.Table)
	case diff.ProcedureDropped:
		return fmt.Sprintf("procedure %q exists in snapshot, absent from database", e.Table)
	case diff.ProcedureBodyChanged:
		return fmt.Sprintf("procedure %q: body changed", e.Table)
	case diff.GrantAdded:
		return fmt.Sprintf("grant %q exists in database, absent from snapshot", e.ConstraintName)
	case diff.GrantDropped:
		return fmt.Sprintf("grant %q exists in snapshot, absent from database", e.ConstraintName)
	case diff.PolicyAdded:
		return fmt.Sprintf("policy %q exists in database, absent from snapshot", e.ConstraintName)
	case diff.PolicyDropped:
		return fmt.Sprintf("policy %q exists in snapshot, absent from database", e.ConstraintName)
	case diff.PolicyChanged:
		return fmt.Sprintf("policy %q: changed", e.ConstraintName)
	case diff.CommentChanged:
		return fmt.Sprintf("%s: comment changed", e.Table)
	default:
		return fmt.Sprintf("%s %s.%s: %s → %s", e.Kind, e.Table, e.Column, e.OldValue, e.NewValue)
	}
}
