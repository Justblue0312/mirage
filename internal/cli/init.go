package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justblue/mirage/internal/runner"
)

func cmdInit() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "initialise mirage in the current project",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "dir", Value: ".", Usage: "project root"},
			&cli.StringFlag{Name: "db", Usage: "database connection string (creates schema_migrations table)"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			dir := cmd.String("dir")
			migDir := filepath.Join(dir, "migrations")

			if err := os.MkdirAll(migDir, 0755); err != nil {
				return fmt.Errorf("creating migrations directory: %w", err)
			}
			success("Created migrations/")

			connStr := cmd.String("db")
			if !cmd.IsSet("db") {
				if cfg := cfgDB(); cfg != "" {
					connStr = cfg
				}
			}
			if connStr != "" {
				pool, err := pgxpool.New(ctx, connStr)
				if err != nil {
					return fmt.Errorf("connecting to db: %w", err)
				}
				defer pool.Close()

				r := runner.New(getDialect())
				if err := r.EnsureTracker(ctx, pool); err != nil {
					return fmt.Errorf("creating schema_migrations: %w", err)
				}
				success("Created schema_migrations table")
			}

			return nil
		},
	}
}
