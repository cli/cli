package codespaces

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cli/cli/v2/internal/codespaces/codespace"
	"github.com/cli/cli/v2/pkg/liveshare"
)

type logger interface {
	Print(v ...interface{}) (int, error)
	Println(v ...interface{}) (int, error)
}

func connectionReady(cs *codespace.Codespace) bool {
	return cs.Environment.Connection.SessionID != "" &&
		cs.Environment.Connection.SessionToken != "" &&
		cs.Environment.Connection.RelayEndpoint != "" &&
		cs.Environment.Connection.RelaySAS != "" &&
		cs.Environment.State == codespace.EnvironmentStateAvailable
}

type apiClient interface {
	GetCodespace(ctx context.Context, token, user, name string) (*codespace.Codespace, error)
	GetCodespaceToken(ctx context.Context, user, codespace string) (string, error)
	StartCodespace(ctx context.Context, name string) error
}

// ConnectToLiveshare waits for a Codespace to become running,
// and connects to it using a Live Share session.
func ConnectToLiveshare(ctx context.Context, log logger, apiClient apiClient, userLogin, token string, cs *codespace.Codespace) (*liveshare.Session, error) {
	var startedCodespace bool
	if cs.Environment.State != codespace.EnvironmentStateAvailable {
		startedCodespace = true
		log.Print("Starting your codespace...")
		if err := apiClient.StartCodespace(ctx, cs.Name); err != nil {
			return nil, fmt.Errorf("error starting codespace: %w", err)
		}
	}

	for retries := 0; !connectionReady(cs); retries++ {
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
		cs, err = apiClient.GetCodespace(ctx, token, userLogin, cs.Name)
		if err != nil {
			return nil, fmt.Errorf("error getting codespace: %w", err)
		}
	}

	if startedCodespace {
		fmt.Print("\n")
	}

	log.Println("Connecting to your codespace...")

	return liveshare.Connect(ctx, liveshare.Options{
		SessionID:      cs.Environment.Connection.SessionID,
		SessionToken:   cs.Environment.Connection.SessionToken,
		RelaySAS:       cs.Environment.Connection.RelaySAS,
		RelayEndpoint:  cs.Environment.Connection.RelayEndpoint,
		HostPublicKeys: cs.Environment.Connection.HostPublicKeys,
	})
}
