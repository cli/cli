package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/github/gh-cli/command"
	"github.com/github/gh-cli/update"
)

func main() {
	updateMessageChan := make(chan *string)
	go updateInBackground(updateMessageChan)

	if cmd, err := command.RootCmd.ExecuteC(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		_, isFlagError := err.(command.FlagError)
		if isFlagError || strings.HasPrefix(err.Error(), "unknown command ") {
			fmt.Fprintln(os.Stderr, cmd.UsageString())
		}
		os.Exit(1)
	}

	updateMessage := <-updateMessageChan
	if updateMessage != nil {
		fmt.Fprintf(os.Stderr, *updateMessage)
	}
}

func updateInBackground(updateMessageChan chan *string) {
	if os.Getenv("APP_ENV") != "production" {
		updateMessageChan <- nil
		return
	}

	client, err := command.BasicClient()
	if err != nil {
		updateMessageChan <- nil
		return
	}

	updateMessageChan <- update.UpdateMessage(client)
}
