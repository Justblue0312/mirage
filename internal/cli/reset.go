package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justblue/mirage/internal/runner"
)

func cmdReset() *cli.Command {
	return &cli.Command{
		Name:  "reset",
		Usage: "clear all migration records from the database",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "db", Usage: "database connection string"},
			&cli.BoolFlag{Name: "force", Usage: "skip confirmation"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			connStr := cmd.String("db")
			if !cmd.IsSet("db") {
				if cfg := cfgDB(); cfg != "" {
					connStr = cfg
				}
			}
			if connStr == "" {
				return fmt.Errorf("--db is required (or set db in mirage.yaml)")
			}
			if !cmd.Bool("force") {
				if isTTY(os.Stdin.Fd()) {
					fmt.Print("This will delete all schema_migrations rows. Type 'yes' to confirm: ")
					reader := bufio.NewReader(os.Stdin)
					line, err := reader.ReadString('\n')
					if err != nil && line == "" {
						return fmt.Errorf("reading confirmation from stdin: %w", err)
					}
					if strings.TrimSpace(strings.ToLower(line)) != "yes" {
						return fmt.Errorf("aborted by user")
					}
				} else {
					// Non-TTY: fail closed — require explicit --force.
					return fmt.Errorf(
						"reset requires confirmation but no terminal is attached; " +
							"re-run with --force to proceed non-interactively")
				}
			}

			pool, err := pgxpool.New(ctx, connStr)
			if err != nil {
				return fmt.Errorf("connecting: %w", err)
			}
			defer pool.Close()

			d := getDialect()
			r := runner.New(d)
			if err := r.EnsureTracker(ctx, pool); err != nil {
				return fmt.Errorf("ensuring tracker: %w", err)
			}
			if _, err := pool.Exec(ctx, d.ResetMigrationsSQL()); err != nil {
				return fmt.Errorf("truncating: %w", err)
			}

			success("Reset complete.")
			return nil
		},
	}
}
