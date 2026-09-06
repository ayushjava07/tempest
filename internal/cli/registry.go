package cli

func registryCmd() *Command {
	return &Command{
		Name:     "registry",
		Summary:  "manage workflow registry",
		Children: []*Command{},
	}
}
