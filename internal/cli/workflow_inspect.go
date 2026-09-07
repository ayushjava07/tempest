package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	ansiReset  = "\033[0m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiRed    = "\033[31m"
	ansiCyan   = "\033[36m"
	ansiBold   = "\033[1m"
)

type inspectStep struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Status   string        `json:"status"`
	Duration time.Duration `json:"duration"`
}

type inspectRunInfo struct {
	RunID      string        `json:"run_id"`
	WorkflowID string        `json:"workflow_id"`
	Status     string        `json:"status"`
	StartedAt  time.Time     `json:"started_at"`
	Duration   time.Duration `json:"duration"`
	Steps      []inspectStep `json:"steps"`
}

func workflowsInspectCmd() *Command {
	return &Command{
		Name:    "inspect",
		Summary: "inspect workflow execution status with timeline visualization",
		Run: func(ctx *Context) error {
			if len(ctx.Args) < 1 {
				return fmt.Errorf("usage: tempest workflow inspect <run-id> [--no-color]")
			}

			runID := ctx.Args[0]
			noColor := false
			for _, arg := range ctx.Args[1:] {
				if arg == "--no-color" {
					noColor = true
				}
			}

			// Mock run data for inspect command
			now := time.Now().UTC()
			info := inspectRunInfo{
				RunID:      runID,
				WorkflowID: "order-fulfillment",
				Status:     "COMPLETED",
				StartedAt:  now.Add(-250 * time.Millisecond),
				Duration:   250 * time.Millisecond,
				Steps: []inspectStep{
					{ID: "step-validate", Name: "Validate Order", Status: "SUCCESS", Duration: 40 * time.Millisecond},
					{ID: "step-charge", Name: "Charge Payment", Status: "SUCCESS", Duration: 120 * time.Millisecond},
					{ID: "step-fulfill", Name: "Fulfill Inventory", Status: "SUCCESS", Duration: 65 * time.Millisecond},
					{ID: "step-notify", Name: "Notify Customer", Status: "SUCCESS", Duration: 25 * time.Millisecond},
				},
			}

			if ctx.Global.Output == "json" {
				return json.NewEncoder(ctx.Out).Encode(info)
			}

			// Render ANSI formatted summary
			green := ansiGreen
			cyan := ansiCyan
			bold := ansiBold
			reset := ansiReset
			if noColor {
				green, cyan, bold, reset = "", "", "", ""
			}

			ctx.Printf("%sWorkflow Run Details:%s\n", bold, reset)
			ctx.Printf("  Run ID:    %s%s%s\n", cyan, info.RunID, reset)
			ctx.Printf("  Workflow:  %s\n", info.WorkflowID)
			ctx.Printf("  Status:    %s%s%s\n", green, info.Status, reset)
			ctx.Printf("  Duration:  %s\n", info.Duration)
			ctx.Printf("\n%sStep Execution Timeline:%s\n", bold, reset)

			for i, step := range info.Steps {
				connector := "├──"
				if i == len(info.Steps)-1 {
					connector = "└──"
				}

				icon := fmt.Sprintf("%s[✓]%s", green, reset)
				if noColor {
					icon = "[OK]"
				}

				padding := 20 - len(step.ID)
				if padding < 1 {
					padding = 1
				}
				padStr := strings.Repeat(" ", padding)

				ctx.Printf("  %s %s %s%s (%s) %10s\n",
					connector, icon, step.ID, padStr, step.Name, step.Duration)
			}

			return nil
		},
	}
}
