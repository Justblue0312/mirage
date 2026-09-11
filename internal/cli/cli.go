package cli

import (
	"context"
	"slices"

	"github.com/urfave/cli/v3"

	"github.com/justblue/mirage/internal/config"
)

// Version is set via ldflags at build time.
var Version = "dev"

// appConfig holds the loaded config file, available to all commands.
// A nil value means no config file was found (normal for projects
// that haven't adopted one yet).
var appConfig *config.Config

func Run(ctx context.Context, args []string) error {
	if slices.Contains(args, "--no-color") {
		useColor = false
	}

	cfg, cfgPath, err := config.Load()
	if err != nil {
		return err
	}
	appConfig = cfg

	if cfgPath != "" {
		dim("Using config: " + cfgPath)
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

// cfgDB returns the db connection string from the config, or empty if not set.
func cfgDB() string {
	if appConfig != nil {
		return appConfig.DB
	}
	return ""
}

// cfgSource returns the source directories from the config, or nil.
func cfgSource() []string {
	if appConfig != nil {
		return appConfig.Source
	}
	return nil
}

// cfgMigrationsDir returns the migrations directory from the config, or empty.
func cfgMigrationsDir() string {
	if appConfig != nil {
		return appConfig.MigrationsDir
	}
	return ""
}

// cfgIdempotent returns the idempotent setting from the config.
func cfgIdempotent() bool {
	if appConfig != nil {
		return appConfig.Idempotent
	}
	return false
}

// cfgVerbose returns the verbose setting from the config.
func cfgVerbose() bool {
	if appConfig != nil {
		return appConfig.Verbose
	}
	return false
}
