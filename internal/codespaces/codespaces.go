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

func connectionReady(codespace *api.Codespace) bool {
	return codespace.Environment.Connection.SessionID != "" &&
		codespace.Environment.Connection.SessionToken != "" &&
		codespace.Environment.Connection.RelayEndpoint != "" &&
		codespace.Environment.Connection.RelaySAS != "" &&
		codespace.Environment.State == api.CodespaceEnvironmentStateAvailable
}

type apiClient interface {
	GetCodespace(ctx context.Context, token, user, name string) (*api.Codespace, error)
	GetCodespaceToken(ctx context.Context, user, codespace string) (string, error)
	StartCodespace(ctx context.Context, name string) error
}

// ConnectToLiveshare waits for a Codespace to become running,
// and connects to it using a Live Share session.
func ConnectToLiveshare(ctx context.Context, log logger, apiClient apiClient, userLogin, token string, codespace *api.Codespace) (*liveshare.Session, error) {
	var startedCodespace bool
	if codespace.Environment.State != api.CodespaceEnvironmentStateAvailable {
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
		codespace, err = apiClient.GetCodespace(ctx, token, userLogin, codespace.Name)
		if err != nil {
			return nil, fmt.Errorf("error getting codespace: %w", err)
		}
	}

	if startedCodespace {
		fmt.Print("\n")
	}

	log.Println("Connecting to your codespace...")

	return liveshare.Connect(ctx, liveshare.Options{
		SessionID:      codespace.Environment.Connection.SessionID,
		SessionToken:   codespace.Environment.Connection.SessionToken,
		RelaySAS:       codespace.Environment.Connection.RelaySAS,
		RelayEndpoint:  codespace.Environment.Connection.RelayEndpoint,
		HostPublicKeys: codespace.Environment.Connection.HostPublicKeys,
	})
}
