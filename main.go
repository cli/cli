package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/github/gh-cli/command"
	"github.com/github/gh-cli/update"
)

func main() {
	alertMsgChan := make(chan *string)
	go updateInBackground(alertMsgChan)

	if cmd, err := command.RootCmd.ExecuteC(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		_, isFlagError := err.(command.FlagError)
		if isFlagError || strings.HasPrefix(err.Error(), "unknown command ") {
			fmt.Fprintln(os.Stderr, cmd.UsageString())
		}
		os.Exit(1)
	}

	alertMsg := <-alertMsgChan
	if alertMsg != nil {
		fmt.Fprintf(os.Stderr, *alertMsg)
	}
}

func updateInBackground(alertMsgChan chan *string) {
	client, err := command.BasicClient()
	if err != nil {
		alertMsgChan <- nil
		return
	}

	alertMsg := update.UpdateMessage(client)
	alertMsgChan <- alertMsg
}
