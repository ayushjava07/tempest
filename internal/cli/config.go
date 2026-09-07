package cli

import (
	"fmt"
	"os"

	"github.com/tempest-io/tempest/internal/config"
)

func configCmd() *Command {
	return &Command{
		Name:    "config",
		Summary: "manage configuration",
		Children: []*Command{
			configShowCmd(),
			configValidateCmd(),
		},
	}
}

func configShowCmd() *Command {
	return &Command{
		Name:    "show",
		Summary: "show current configuration",
		Run: func(ctx *Context) error {
			cfg, err := config.Load(config.BuildOptions{})
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			ctx.Printf("Server Address:    %s\n", cfg.ServerAddr)
			ctx.Printf("gRPC Address:      %s\n", cfg.ServerGRPCAddr)
			ctx.Printf("Storage Driver:    %s\n", cfg.StorageDriver)
			ctx.Printf("Scheduler Workers: %d\n", cfg.SchedulerWorkers)
			ctx.Printf("Log Level:         %s\n", cfg.LogLevel)
			ctx.Printf("Log Format:        %s\n", cfg.LogFormat)
			ctx.Printf("Dashboard:         %v\n", cfg.DashboardEnabled)
			return nil
		},
	}
}

func configValidateCmd() *Command {
	return &Command{
		Name:    "validate",
		Summary: "validate configuration file",
		Run: func(ctx *Context) error {
			path := ""
			for i, arg := range ctx.Args {
				if arg == "--file" && i+1 < len(ctx.Args) {
					path = ctx.Args[i+1]
				}
			}
			if path == "" {
				path = "tempest.json"
			}
			if _, err := os.Stat(path); os.IsNotExist(err) {
				ctx.Printf("config file %s not found (using defaults)\n", path)
				return nil
			}
			cfg, err := config.Load(config.BuildOptions{FilePath: path})
			if err != nil {
				ctx.Errf("invalid config: %v\n", err)
				return err
			}
			ctx.Printf("config %s is valid\n", path)
			ctx.Printf("  server: %s\n", cfg.ServerAddr)
			return nil
		},
	}
}
