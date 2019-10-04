package main

import (
	"os"

	"github.com/github/gh-cli/command"
)

func main() {
	err := command.RootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
