package cli

import (
	"encoding/json"
	"fmt"
	"os"
)

func workflowsCmd() *Command {
	return &Command{
		Name:     "workflows",
		Summary:  "manage workflow definitions",
		Children: []*Command{
			workflowsListCmd(),
			workflowsGetCmd(),
		},
	}
}

func workflowsListCmd() *Command {
	return &Command{
		Name:    "list",
		Summary: "list workflow definitions",
		Run: func(ctx *Context) error {
			fmt.Fprintln(ctx.Out, "NAME\tVERSION\tSTEPS")
			fmt.Fprintln(ctx.Out, "---\t---\t---")
			return nil
		},
	}
}

func workflowsGetCmd() *Command {
	return &Command{
		Name:    "get",
		Summary: "get a workflow definition by name",
		Run: func(ctx *Context) error {
			if len(ctx.Args) < 1 {
				return fmt.Errorf("usage: tempest workflows get <name>")
			}
			name := ctx.Args[0]
			if ctx.Global.Output == "json" {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{"name": name})
			}
			fmt.Fprintf(ctx.Out, "workflow: %s\n", name)
			return nil
		},
	}
}
