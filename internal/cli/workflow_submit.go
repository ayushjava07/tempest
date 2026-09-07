package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tempest-io/tempest/internal/analyzer"
)

func workflowsSubmitCmd() *Command {
	return &Command{
		Name:    "submit",
		Summary: "submit a workflow definition file for execution",
		Run: func(ctx *Context) error {
			if len(ctx.Args) < 1 {
				return fmt.Errorf("usage: tempest workflow submit <file.json> [--param key=val] [--dry-run]")
			}

			filePath := ctx.Args[0]
			dryRun := false
			params := make(map[string]string)

			for i := 1; i < len(ctx.Args); i++ {
				arg := ctx.Args[i]
				switch {
				case arg == "--dry-run":
					dryRun = true
				case strings.HasPrefix(arg, "--param="):
					kv := strings.SplitN(strings.TrimPrefix(arg, "--param="), "=", 2)
					if len(kv) == 2 {
						params[kv[0]] = kv[1]
					}
				case arg == "--param" && i+1 < len(ctx.Args):
					kv := strings.SplitN(ctx.Args[i+1], "=", 2)
					if len(kv) == 2 {
						params[kv[0]] = kv[1]
					}
					i++
				}
			}

			data, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read workflow definition %s: %w", filePath, err)
			}

			// Validate JSON structure
			var ast analyzer.WorkflowAST
			if err := json.Unmarshal(data, &ast); err != nil {
				return fmt.Errorf("invalid workflow JSON syntax in %s: %w", filePath, err)
			}

			if ast.WorkflowID == "" {
				ast.WorkflowID = strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
			}

			// Apply runtime params
			if ast.Parameters == nil {
				ast.Parameters = make(map[string]string)
			}
			for k, v := range params {
				ast.Parameters[k] = v
			}

			// Run static analysis
			linter := analyzer.New()
			linter.Register(analyzer.UnreachableStepRule{})
			linter.Register(analyzer.MissingDependencyRule{})
			linter.Register(analyzer.CyclicDependencyRule{})
			linter.Register(analyzer.MissingParameterRule{})

			diags := linter.Analyze(&ast)
			if analyzer.HasErrors(diags) {
				ctx.Errf("Workflow definition validation failed with %d issues:\n", len(diags))
				for _, d := range diags {
					ctx.Errf("  - %s\n", d.String())
				}
				return fmt.Errorf("validation error: definition contains critical rule violations")
			}

			if dryRun {
				ctx.Printf("Workflow %q validated successfully (dry-run, %d steps, 0 errors)\n", ast.WorkflowID, len(ast.Steps))
				return nil
			}

			runID := fmt.Sprintf("run-%s-%d", ast.WorkflowID, time.Now().Unix())
			if ctx.Global.Output == "json" {
				return json.NewEncoder(ctx.Out).Encode(map[string]any{
					"workflow_id": ast.WorkflowID,
					"run_id":      runID,
					"status":      "SUBMITTED",
					"steps_count": len(ast.Steps),
				})
			}

			ctx.Printf("Workflow %q submitted successfully\n", ast.WorkflowID)
			ctx.Printf("Run ID: %s\n", runID)
			ctx.Printf("Steps:  %d\n", len(ast.Steps))
			return nil
		},
	}
}
