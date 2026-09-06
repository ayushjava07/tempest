package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
)

type Command struct {
	Name     string
	Summary  string
	Children []*Command
	Run      func(ctx *Context) error
}

type Context struct {
	Args    []string
	Out     io.Writer
	Err     io.Writer
	Global  *GlobalFlags
	Command *Command
}

type GlobalFlags struct {
	Server string
	Output string
	Token  string
	NS     string
}

func (c *Context) Printf(format string, args ...any) {
	fmt.Fprintf(c.Out, format, args...)
}

func (c *Context) Println(args ...any) {
	fmt.Fprintln(c.Out, args...)
}

func (c *Context) Errf(format string, args ...any) {
	fmt.Fprintf(c.Err, format, args...)
}

func NewRoot() *Command {
	return &Command{
		Name:    "tempest",
		Summary: "distributed workflow orchestration",
	}
}

func Run(args []string) error {
	return runWith(args, os.Stdout, os.Stderr)
}

func runWith(args []string, stdout, stderr io.Writer) error {
	root := NewRoot()
	registerCommands(root)
	gf := &GlobalFlags{}
	remaining := parseGlobalFlags(args, gf)
	cmd, path, err := resolve(root, remaining)
	if err != nil {
		return err
	}
	ctx := &Context{
		Args:    remaining[len(path):],
		Out:     stdout,
		Err:     stderr,
		Global:  gf,
		Command: cmd,
	}
	if cmd.Run != nil {
		return cmd.Run(ctx)
	}
	if len(cmd.Children) > 0 {
		return fmt.Errorf("usage: tempest %s <command>", strings.Join(path, " "))
	}
	return fmt.Errorf("unknown command: %s", remaining[0])
}

func parseGlobalFlags(args []string, gf *GlobalFlags) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--server" && i+1 < len(args):
			gf.Server = args[i+1]
			i++
		case args[i] == "--output" && i+1 < len(args):
			gf.Output = args[i+1]
			i++
		case args[i] == "--token" && i+1 < len(args):
			gf.Token = args[i+1]
			i++
		case args[i] == "--namespace" && i+1 < len(args):
			gf.NS = args[i+1]
			i++
		default:
			out = append(out, args[i])
		}
	}
	return out
}

func resolve(cmd *Command, args []string) (*Command, []string, error) {
	if len(args) == 0 {
		return cmd, nil, nil
	}
	for _, child := range cmd.Children {
		if child.Name == args[0] {
			return resolve(child, args[1:])
		}
	}
	return cmd, nil, nil
}

func registerCommands(root *Command) {
	root.Children = []*Command{
		serverCmd(),
		workflowsCmd(),
		runCmd(),
		migrateCmd(),
		versionCmd(),
	}
}

func versionCmd() *Command {
	return &Command{
		Name:    "version",
		Summary: "print version information",
		Run: func(ctx *Context) error {
			ctx.Println("tempest v0.1.0")
			return nil
		},
	}
}
