package cli

import (
	"encoding/json"
	"fmt"
	"os"
)

func runCmd() *Command {
	return &Command{
		Name:    "runs",
		Summary: "manage workflow runs",
		Children: []*Command{
			runListCmd(),
			runGetCmd(),
			runSubmitCmd(),
			runCancelCmd(),
			runLogsCmd(),
		},
	}
}

func runListCmd() *Command {
	return &Command{
		Name:    "list",
		Summary: "list runs",
		Run: func(ctx *Context) error {
			fmt.Fprintln(ctx.Out, "ID\tWORKFLOW\tSTATE")
			return nil
		},
	}
}

func runGetCmd() *Command {
	return &Command{
		Name:    "get",
		Summary: "get run details",
		Run: func(ctx *Context) error {
			if len(ctx.Args) < 1 {
				return fmt.Errorf("usage: tempest runs get <run-id>")
			}
			fmt.Fprintf(ctx.Out, "run: %s\n", ctx.Args[0])
			return nil
		},
	}
}

func runSubmitCmd() *Command {
	return &Command{
		Name:    "submit",
		Summary: "submit a new run",
		Run: func(ctx *Context) error {
			if len(ctx.Args) < 1 {
				return fmt.Errorf("usage: tempest runs submit <workflow> [--input=file]")
			}
			workflow := ctx.Args[0]
			inputFile := ""
			for i := 1; i < len(ctx.Args); i++ {
				if ctx.Args[i] == "--input" && i+1 < len(ctx.Args) {
					inputFile = ctx.Args[i+1]
					i++
				}
			}
			var input map[string]any
			if inputFile != "" {
				data, err := os.ReadFile(inputFile)
				if err != nil {
					return fmt.Errorf("read input file: %w", err)
				}
				if err := json.Unmarshal(data, &input); err != nil {
					return fmt.Errorf("parse input: %w", err)
				}
			}
			_ = input
			fmt.Fprintf(ctx.Out, "submitted workflow %s\n", workflow)
			return nil
		},
	}
}

func runCancelCmd() *Command {
	return &Command{
		Name:    "cancel",
		Summary: "cancel a run",
		Run: func(ctx *Context) error {
			if len(ctx.Args) < 1 {
				return fmt.Errorf("usage: tempest runs cancel <run-id>")
			}
			fmt.Fprintf(ctx.Out, "cancelled run %s\n", ctx.Args[0])
			return nil
		},
	}
}

func runLogsCmd() *Command {
	return &Command{
		Name:    "logs",
		Summary: "show run logs/events",
		Run: func(ctx *Context) error {
			if len(ctx.Args) < 1 {
				return fmt.Errorf("usage: tempest runs logs <run-id>")
			}
			fmt.Fprintf(ctx.Out, "logs for run %s\n", ctx.Args[0])
			return nil
		},
	}
}
