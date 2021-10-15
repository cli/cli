package codespaces

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/liveshare"
)

type logger interface {
	Print(v ...interface{}) (int, error)
	Println(v ...interface{}) (int, error)
}

// TODO(josebalius): clean this up once we standardrize
// logging for codespaces
type liveshareLogger interface {
	Println(v ...interface{})
	Printf(f string, v ...interface{})
}

func connectionReady(codespace *api.Codespace) bool {
	return codespace.Connection.SessionID != "" &&
		codespace.Connection.SessionToken != "" &&
		codespace.Connection.RelayEndpoint != "" &&
		codespace.Connection.RelaySAS != "" &&
		codespace.State == api.CodespaceStateAvailable
}

type apiClient interface {
	GetCodespace(ctx context.Context, name string, includeConnection bool) (*api.Codespace, error)
	StartCodespace(ctx context.Context, name string) error
}

// ConnectToLiveshare waits for a Codespace to become running,
// and connects to it using a Live Share session.
func ConnectToLiveshare(ctx context.Context, log logger, sessionLogger liveshareLogger, apiClient apiClient, codespace *api.Codespace) (*liveshare.Session, error) {
	var startedCodespace bool
	if codespace.State != api.CodespaceStateAvailable {
		startedCodespace = true
		log.Print("Starting your codespace...")
		if err := apiClient.StartCodespace(ctx, codespace.Name); err != nil {
			return nil, fmt.Errorf("error starting codespace: %w", err)
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
		codespace, err = apiClient.GetCodespace(ctx, codespace.Name, true)
		if err != nil {
			return nil, fmt.Errorf("error getting codespace: %w", err)
		}
	}

	if startedCodespace {
		fmt.Print("\n")
	}

	log.Println("Connecting to your codespace...")

	return liveshare.Connect(ctx, liveshare.Options{
		ClientName:     "gh",
		SessionID:      codespace.Connection.SessionID,
		SessionToken:   codespace.Connection.SessionToken,
		RelaySAS:       codespace.Connection.RelaySAS,
		RelayEndpoint:  codespace.Connection.RelayEndpoint,
		HostPublicKeys: codespace.Connection.HostPublicKeys,
		Logger:         sessionLogger,
	})
}
