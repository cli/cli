package codespaces

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/github/ghcs/api"
	"github.com/github/go-liveshare"
)

type logger interface {
	Print(v ...interface{}) (int, error)
	Println(v ...interface{}) (int, error)
}

func connectionReady(codespace *api.Codespace) bool {
	return codespace.Environment.Connection.SessionID != "" &&
		codespace.Environment.Connection.SessionToken != "" &&
		codespace.Environment.Connection.RelayEndpoint != "" &&
		codespace.Environment.Connection.RelaySAS != "" &&
		codespace.Environment.State == api.CodespaceEnvironmentStateAvailable
}

func ConnectToLiveshare(ctx context.Context, log logger, apiClient *api.API, userLogin, token string, codespace *api.Codespace) (*liveshare.Session, error) {
	var startedCodespace bool
	if codespace.Environment.State != api.CodespaceEnvironmentStateAvailable {
		startedCodespace = true
		log.Print("Starting your codespace...")
		if err := apiClient.StartCodespace(ctx, token, codespace); err != nil {
			return nil, fmt.Errorf("error starting codespace: %v", err)
		}
	}

	for retries := 0; !connectionReady(codespace); retries++ {
		if retries > 1 {
			if retries%2 == 0 {
				log.Print(".")
			}

			time.Sleep(1 * time.Second)
		}

		if retries == 30 {
			return nil, errors.New("timed out while waiting for the codespace to start")
		}

		var err error
		codespace, err = apiClient.GetCodespace(ctx, token, userLogin, codespace.Name)
		if err != nil {
			return nil, fmt.Errorf("error getting codespace: %v", err)
		}
	}

	if startedCodespace {
		fmt.Print("\n")
	}

	log.Println("Connecting to your codespace...")

	lsclient, err := liveshare.NewClient(
		liveshare.WithConnection(liveshare.Connection{
			SessionID:     codespace.Environment.Connection.SessionID,
			SessionToken:  codespace.Environment.Connection.SessionToken,
			RelaySAS:      codespace.Environment.Connection.RelaySAS,
			RelayEndpoint: codespace.Environment.Connection.RelayEndpoint,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("error creating Live Share client: %v", err)
	}

	return lsclient.JoinWorkspace(ctx)
}
