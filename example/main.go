package main

import (
	"context"
	"fmt"
	"log"

	"github.com/josebalius/go-liveshare"
)

func main() {
	liveShare, err := liveshare.New(
		liveshare.WithWorkspaceID(""),
		liveshare.WithToken(""),
	)
	if err != nil {
		log.Fatal(fmt.Errorf("error creating liveshare: %v", err))
	}

	if err := liveShare.Connect(context.Background()); err != nil {
		log.Fatal(fmt.Errorf("error connecting to liveshare: %v", err))
	}

	terminal := liveShare.NewTerminal()

	cmd := terminal.NewCommand(
		"/home/codespace/workspace",
		"docker ps -aq --filter label=Type=codespaces --filter status=running",
	)
	output, err := cmd.Run(context.Background())
	if err != nil {
		log.Fatal(fmt.Errorf("error starting ssh server with liveshare: %v", err))
	}

	fmt.Println(string(output))
}
