package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/github/gh-cli/command"
	"github.com/github/gh-cli/update"
	"github.com/github/gh-cli/utils"
	"github.com/mattn/go-isatty"
	"github.com/mgutz/ansi"
)

var updaterEnabled = ""

func main() {
	currentVersion := command.Version
	updateMessageChan := make(chan *update.ReleaseInfo)
	go func() {
		rel, _ := checkForUpdate(currentVersion)
		updateMessageChan <- rel
	}()

	if cmd, err := command.RootCmd.ExecuteC(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		_, isFlagError := err.(command.FlagError)
		if isFlagError || strings.HasPrefix(err.Error(), "unknown command ") {
			fmt.Fprintln(os.Stderr, cmd.UsageString())
		}
		os.Exit(1)
	}

	newRelease := <-updateMessageChan
	if newRelease != nil {
		msg := fmt.Sprintf(`A new release of gh is available: %s → %s
Release notes: %s`, currentVersion, newRelease.Version, newRelease.URL)
		stderr := utils.NewColorable(os.Stderr)
		fmt.Fprintf(stderr, "\n\n%s\n\n", ansi.Color(msg, "cyan"))
	}
}

func shouldCheckForUpdate() bool {
	errFd := os.Stderr.Fd()
	return updaterEnabled != "" && (isatty.IsTerminal(errFd) || isatty.IsCygwinTerminal(errFd))
}

func checkForUpdate(currentVersion string) (*update.ReleaseInfo, error) {
	if !shouldCheckForUpdate() {
		return nil, nil
	}

	client, err := command.BasicClient()
	if err != nil {
		return nil, err
	}

	repo := updaterEnabled
	return update.CheckForUpdate(client, repo, currentVersion)
}
