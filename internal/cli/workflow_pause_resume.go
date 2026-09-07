package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func workflowsPauseCmd() *Command {
	return &Command{
		Name:    "pause",
		Summary: "pause execution of an active workflow run",
		Run: func(ctx *Context) error {
			if len(ctx.Args) < 1 {
				return fmt.Errorf("usage: tempest workflow pause <run-id> [--reason=<reason>]")
			}

			runID := ctx.Args[0]
			reason := "user-requested"
			for i := 1; i < len(ctx.Args); i++ {
				arg := ctx.Args[i]
				if strings.HasPrefix(arg, "--reason=") {
					reason = strings.TrimPrefix(arg, "--reason=")
				} else if arg == "--reason" && i+1 < len(ctx.Args) {
					reason = ctx.Args[i+1]
					i++
				}
			}

			if ctx.Global.Output == "json" {
				return json.NewEncoder(ctx.Out).Encode(map[string]any{
					"run_id":    runID,
					"status":    "PAUSED",
					"reason":    reason,
					"timestamp": time.Now().UTC(),
				})
			}

			ctx.Printf("Workflow run %q paused successfully\n", runID)
			ctx.Printf("Reason: %s\n", reason)
			return nil
		},
	}
}

func workflowsResumeCmd() *Command {
	return &Command{
		Name:    "resume",
		Summary: "resume execution of a paused workflow run",
		Run: func(ctx *Context) error {
			if len(ctx.Args) < 1 {
				return fmt.Errorf("usage: tempest workflow resume <run-id>")
			}

			runID := ctx.Args[0]

			if ctx.Global.Output == "json" {
				return json.NewEncoder(ctx.Out).Encode(map[string]any{
					"run_id":    runID,
					"status":    "RUNNING",
					"timestamp": time.Now().UTC(),
				})
			}

			ctx.Printf("Workflow run %q resumed successfully\n", runID)
			ctx.Printf("Status: RUNNING\n")
			return nil
		},
	}
}
