package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/github/gh-cli/command"
)

func main() {
	if cmd, err := command.RootCmd.ExecuteC(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		_, isFlagError := err.(command.FlagError)
		if isFlagError || strings.HasPrefix(err.Error(), "unknown command ") {
			fmt.Fprintln(os.Stderr, cmd.UsageString())
		}
		os.Exit(1)
	}
}
