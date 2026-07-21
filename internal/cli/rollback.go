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

func cmdRollback() *cli.Command {
	return &cli.Command{
		Name:      "rollback",
		Usage:     "roll back the last N migrations (default 1)",
		ArgsUsage: "[N]",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "db", Usage: "database connection string"},
			&cli.StringFlag{Name: "dir", Value: "./migrations", Usage: "migrations directory"},
			&cli.BoolFlag{Name: "force", Usage: "skip confirmation prompt"},
			&cli.BoolFlag{Name: "dry-run", Usage: "print down statements without applying"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			n := 1
			if arg := cmd.Args().First(); arg != "" {
				if _, err := fmt.Sscanf(arg, "%d", &n); err != nil {
					return fmt.Errorf("invalid rollback count %q: expected an integer", arg)
				}
			}

			connStr := cmd.String("db")
			if !cmd.IsSet("db") {
				if cfg := cfgDB(); cfg != "" {
					connStr = cfg
				}
			}
			if connStr == "" {
				return fmt.Errorf("--db is required (or set db in mirage.yaml)")
			}

			dir := cmd.String("dir")
			if !cmd.IsSet("dir") {
				if cfg := cfgMigrationsDir(); cfg != "" {
					dir = cfg
				}
			}

			if cmd.Bool("dry-run") {
				// Dry-run: no confirmation needed, just print what would happen.
			} else if n >= 1 && !cmd.Bool("force") {
				if isTTY(os.Stdin.Fd()) {
					fmt.Printf("Roll back %d migration(s)? [y/N] ", n)
					reader := bufio.NewReader(os.Stdin)
					line, err := reader.ReadString('\n')
					if err != nil && line == "" {
						return fmt.Errorf("reading confirmation from stdin: %w", err)
					}
					if strings.TrimSpace(strings.ToLower(line)) != "y" {
						return fmt.Errorf("aborted by user")
					}
				} else {
					// Non-TTY: fail closed — require explicit --force.
					return fmt.Errorf(
						"rollback requires confirmation but no terminal is attached; " +
							"re-run with --force to proceed non-interactively")
				}
			}

			pool, err := pgxpool.New(ctx, connStr)
			if err != nil {
				return fmt.Errorf("connecting: %w", err)
			}
			defer pool.Close()

			r := runner.New(getDialect())
			if cmd.Bool("dry-run") {
				return r.DryRollback(ctx, pool, dir, n)
			}
			return r.Rollback(ctx, pool, dir, n)
		},
	}
}
