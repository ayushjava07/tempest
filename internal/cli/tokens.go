package cli

func tokensCmd() *Command {
	return &Command{
		Name:     "tokens",
		Summary:  "manage API tokens",
		Children: []*Command{},
	}
}
