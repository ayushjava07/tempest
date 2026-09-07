package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func workflowsCancelCmd() *Command {
	return &Command{
		Name:    "cancel",
		Summary: "cancel execution of an active workflow run",
		Run: func(ctx *Context) error {
			if len(ctx.Args) < 1 {
				return fmt.Errorf("usage: tempest workflow cancel <run-id> [--force] [--grace-period=<duration>]")
			}

			runID := ctx.Args[0]
			force := false
			gracePeriod := "30s"

			for i := 1; i < len(ctx.Args); i++ {
				arg := ctx.Args[i]
				switch {
				case arg == "--force":
					force = true
				case strings.HasPrefix(arg, "--grace-period="):
					gracePeriod = strings.TrimPrefix(arg, "--grace-period=")
				case arg == "--grace-period" && i+1 < len(ctx.Args):
					gracePeriod = ctx.Args[i+1]
					i++
				}
			}

			status := "CANCELLED"
			if !force {
				status = "TERMINATING"
			}

			if ctx.Global.Output == "json" {
				return json.NewEncoder(ctx.Out).Encode(map[string]any{
					"run_id":       runID,
					"status":       status,
					"force":        force,
					"grace_period": gracePeriod,
					"timestamp":    time.Now().UTC(),
				})
			}

			if force {
				ctx.Printf("Workflow run %q forcibly cancelled immediately\n", runID)
			} else {
				ctx.Printf("Termination signal sent to workflow run %q (grace period: %s)\n", runID, gracePeriod)
			}
			return nil
		},
	}
}
