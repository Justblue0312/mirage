package cli

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/justblue/mirage/internal/scanner"
	"github.com/justblue/mirage/internal/validate"
)

func cmdValidate() *cli.Command {
	return &cli.Command{
		Name:  "validate",
		Usage: "validate model struct tags",
		Flags: []cli.Flag{
			&cli.StringSliceFlag{Name: "source", Usage: "source dir(s) to scan", Required: true},
			&cli.BoolFlag{Name: "recursive", Aliases: []string{"r"}, Usage: "scan subdirectories recursively"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			dirs := cmd.StringSlice("source")

			s := &scanner.Scanner{SourceDirs: dirs, Recursive: cmd.Bool("recursive")}
			pkg, err := s.Scan()
			if err != nil {
				return fmt.Errorf("scanning: %w", err)
			}

			valErrs := validate.Validate(pkg)
			hasErrors := false
			hasWarnings := false
			errorCount := 0
			warningCount := 0

			for _, e := range valErrs {
				if e.IsWarn() {
					hasWarnings = true
					warningCount++
					warn(fmt.Sprintf("[%s] %s.%s: %s", e.Code, e.Table, e.Column, e.Message))
				} else {
					hasErrors = true
					errorCount++
					errMsg(fmt.Sprintf("[%s] %s.%s: %s", e.Code, e.Table, e.Column, e.Message))
				}
			}

			if hasErrors {
				fmt.Println()
				errMsg(fmt.Sprintf("%d error(s) found. Fix the struct tags above before running mirage generate", errorCount))
				return fmt.Errorf("validation failed")
			}
			if hasWarnings {
				fmt.Println()
				dim(fmt.Sprintf("Validation passed with %d warning(s)", warningCount))
				return nil
			}

			success("Validation passed — no issues found")
			return nil
		},
	}
}
