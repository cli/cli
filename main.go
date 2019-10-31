package main

import (
	"fmt"
	"os"

	"github.com/github/gh-cli/command"
)

func main() {
	if err := command.RootCmd.Execute(); err != nil {
		if err != command.SilentErr {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}
