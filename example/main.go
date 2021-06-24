package main

import (
	"context"
	"fmt"
	"log"

	"github.com/josebalius/go-liveshare"
)

func main() {
	liveShare, err := liveshare.New(
		liveshare.WithWorkspaceID("..."),
		liveshare.WithToken("..."),
	)
	if err != nil {
		log.Fatal(fmt.Errorf("error creating liveshare: %v", err))
	}

	if err := liveShare.Connect(context.Background()); err != nil {
		log.Fatal(fmt.Errorf("error connecting to liveshare: %v", err))
	}
}
