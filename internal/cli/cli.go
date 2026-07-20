package cli

import (
	"context"
	"slices"

	"github.com/urfave/cli/v3"
)

// Version is set via ldflags at build time.
var Version = "dev"

func Run(ctx context.Context, args []string) error {
	if slices.Contains(args, "--no-color") {
		useColor = false
	}

	app := &cli.Command{
		Name:    "mirage",
		Usage:   "generate and run schema migrations from Go structs",
		Version: Version,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "no-color",
				Usage: "disable colorized output",
			},
		},
		Commands: []*cli.Command{
			cmdInit(),
			cmdGenerate(),
			cmdMigrate(),
			cmdRollback(),
			cmdStatus(),
			cmdCreate(),
			cmdReset(),
			cmdValidate(),
		},
	}

	return app.Run(ctx, args)
}
