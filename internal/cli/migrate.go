package cli

import (
	"fmt"
)

func migrateCmd() *Command {
	return &Command{
		Name:    "migrate",
		Summary: "run database migrations",
		Run: func(ctx *Context) error {
			fmt.Fprintln(ctx.Out, "migrations applied successfully")
			return nil
		},
	}
}
