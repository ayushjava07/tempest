package cli

import (
	"fmt"
)

func adminCmd() *Command {
	return &Command{
		Name:    "admin",
		Summary: "administrative operations",
		Children: []*Command{
			adminTokenCmd(),
			adminQueueCmd(),
			adminCacheCmd(),
		},
	}
}

func adminTokenCmd() *Command {
	return &Command{
		Name:    "token",
		Summary: "manage API tokens",
		Children: []*Command{
			adminTokenCreateCmd(),
			adminTokenListCmd(),
			adminTokenDeleteCmd(),
		},
	}
}

func adminTokenCreateCmd() *Command {
	return &Command{
		Name:    "create",
		Summary: "create a new API token",
		Run: func(ctx *Context) error {
			role := "reader"
			ns := "default"
			for i, arg := range ctx.Args {
				if arg == "--role" && i+1 < len(ctx.Args) {
					role = ctx.Args[i+1]
				}
				if arg == "--namespace" && i+1 < len(ctx.Args) {
					ns = ctx.Args[i+1]
				}
			}
			if role != "reader" && role != "operator" && role != "admin" {
				return fmt.Errorf("invalid role %q: must be reader, operator, or admin", role)
			}
			ctx.Printf("created token for namespace=%s role=%s\n", ns, role)
			return nil
		},
	}
}

func adminTokenListCmd() *Command {
	return &Command{
		Name:    "list",
		Summary: "list API tokens",
		Run: func(ctx *Context) error {
			ctx.Printf("ID\tROLE\tNAMESPACE\tLABEL\n")
			return nil
		},
	}
}

func adminTokenDeleteCmd() *Command {
	return &Command{
		Name:    "delete",
		Summary: "delete an API token",
		Run: func(ctx *Context) error {
			if len(ctx.Args) < 1 {
				return fmt.Errorf("usage: tempest admin token delete <token-id>")
			}
			ctx.Printf("deleted token %s\n", ctx.Args[0])
			return nil
		},
	}
}

func adminQueueCmd() *Command {
	return &Command{
		Name:    "queue",
		Summary: "inspect and manage the work queue",
		Children: []*Command{
			adminQueueListCmd(),
			adminQueueRequeueCmd(),
		},
	}
}

func adminQueueListCmd() *Command {
	return &Command{
		Name:    "list",
		Summary: "list queued items",
		Run: func(ctx *Context) error {
			ctx.Printf("RUN_ID\tSTEP_ID\tATTEMPT\tVISIBLE_AT\n")
			return nil
		},
	}
}

func adminQueueRequeueCmd() *Command {
	return &Command{
		Name:    "requeue",
		Summary: "requeue a stuck item",
		Run: func(ctx *Context) error {
			if len(ctx.Args) < 2 {
				return fmt.Errorf("usage: tempest admin queue requeue <run-id> <step-id>")
			}
			ctx.Printf("requeued %s/%s\n", ctx.Args[0], ctx.Args[1])
			return nil
		},
	}
}

func adminCacheCmd() *Command {
	return &Command{
		Name:    "cache",
		Summary: "inspect cache state",
		Run: func(ctx *Context) error {
			ctx.Printf("Cache Stats:\n")
			ctx.Printf("  Hits:   0\n")
			ctx.Printf("  Misses: 0\n")
			ctx.Printf("  Size:   0\n")
			return nil
		},
	}
}
