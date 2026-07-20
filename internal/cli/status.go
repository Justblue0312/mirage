package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/justblue/mirage/internal/runner"
)

func cmdStatus() *cli.Command {
	return &cli.Command{
		Name:  "status",
		Usage: "show applied and pending migrations",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "db", Usage: "database connection string (optional, shows file-only status if omitted)"},
			&cli.StringFlag{Name: "dir", Value: "./migrations", Usage: "migrations directory"},
			&cli.StringFlag{Name: "format", Value: "table", Usage: "table|json"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			dir := cmd.String("dir")

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

			connStr := cmd.String("db")

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
					// In no-color mode, use plain ASCII indicators.
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
