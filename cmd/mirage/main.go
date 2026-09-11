package main

import (
	"context"
	"os"

	"github.com/Justblue0312/mirage/internal/cli"
)

func main() {
	if err := cli.Run(context.Background(), os.Args); err != nil {
		// urfave/cli returns errors for invalid flags/args; ensure they are visible
		// even though cli.Run does not print them itself. Use plain Fprintln to
		// avoid color codes when --no-color is set.
		if err.Error() != "" {
			// Use os.Stderr directly; cli's helpers (warn/dim) may be suppressed
			// by --no-color, but errors must always be visible.
			_, _ = os.Stderr.WriteString(err.Error() + "\n")
		}
		os.Exit(1)
	}
}
