package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/justblue/mirage/internal/scanner"
)

func cmdCreate() *cli.Command {
	return &cli.Command{
		Name:  "create",
		Usage: "create an empty migration file",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "dir", Value: "./migrations", Usage: "migrations directory"},
			&cli.StringFlag{Name: "message", Aliases: []string{"m"}, Usage: "migration description"},
			&cli.BoolFlag{Name: "sequential-naming", Usage: "use sequential version numbers instead of timestamps"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			dir := cmd.String("dir")

			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("creating migrations directory: %w", err)
			}

			msg := cmd.String("message")

			ts := time.Now().UTC().Format("20060102150405")
			if cmd.Bool("sequential-naming") {
				ts = nextSequentialVersion(dir)
			}
			safeName := strings.ToLower(scanner.SnakeCase(msg))
			fileName := fmt.Sprintf("V%s_%s.sql", ts, safeName)
			filePath := filepath.Join(dir, fileName)

			var content string
			if msg != "" {
				content = fmt.Sprintf(`-- Migration: V%s_%s
-- Created manually at %s
-- Message: %s

-- +migrate Up

-- Write your Up SQL here

-- +migrate Down

-- Write your Down SQL here
`, ts, safeName, time.Now().UTC().Format(time.RFC3339), msg)
			} else {
				content = fmt.Sprintf(`-- Migration: V%s_%s
-- Created manually at %s

-- +migrate Up

-- Write your Up SQL here

-- +migrate Down

-- Write your Down SQL here
`, ts, safeName, time.Now().UTC().Format(time.RFC3339))
			}

			if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
				return fmt.Errorf("writing migration: %w", err)
			}

			success(fmt.Sprintf("Created: %s", filePath))
			return nil
		},
	}
}
