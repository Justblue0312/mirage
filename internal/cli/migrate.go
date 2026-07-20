package cli

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justblue/mirage/internal/runner"
)

func cmdMigrate() *cli.Command {
	return &cli.Command{
		Name:  "migrate",
		Usage: "apply pending migrations to the database",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "db", Usage: "database connection string", Required: true},
			&cli.StringFlag{Name: "dir", Value: "./migrations", Usage: "migrations directory"},
			&cli.BoolFlag{Name: "dry-run", Usage: "print pending migrations without applying"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			connStr := cmd.String("db")
			dir := cmd.String("dir")

			pool, err := pgxpool.New(ctx, connStr)
			if err != nil {
				return fmt.Errorf("connecting: %w", err)
			}
			defer pool.Close()

			r := runner.New(getDialect())
			if cmd.Bool("dry-run") {
				return r.DryMigrate(ctx, pool, dir)
			}
			return r.Migrate(ctx, pool, dir)
		},
	}
}
