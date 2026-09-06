package main

import (
	"os"

	"github.com/tempest-io/tempest/internal/cli"
)

func main() {
	if err := cli.Run(os.Args[1:]); err != nil {
		os.Exit(1)
	}
}
