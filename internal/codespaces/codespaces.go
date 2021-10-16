package codespaces

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/liveshare"
)

// connectToLiveshare waits for a Codespace to become running,
// and connects to it using a Live Share session.
func (a *App) connectToLiveshare(ctx context.Context, sessionLogger *log.Logger, apiCodespace *api.Codespace) (session *liveshare.Session, err error) {
	var startedCodespace bool
	cs := codespace{apiCodespace}

	if !cs.running() {
		startedCodespace = true
		a.Print("Starting your codespace...")
		if err := a.apiClient.StartCodespace(ctx, cs.Name); err != nil {
			return nil, fmt.Errorf("error starting codespace: %w", err)
		}
	}

	for retries := 0; !cs.connectionReady(); retries++ {
		if retries > 1 {
			if retries%2 == 0 {
				a.Print(".")
			}

			time.Sleep(1 * time.Second)
		}

		if retries == 30 {
			return nil, errors.New("timed out while waiting for the codespace to start")
		}

		apiCodespace, err = a.apiClient.GetCodespace(ctx, cs.Name, true)
		if err != nil {
			return nil, fmt.Errorf("error getting codespace: %w", err)
		}
		cs = codespace{apiCodespace}
	}

	if startedCodespace {
		a.Print("\n")
	}

	a.Println("Connecting to your codespace...")
	return liveshare.Connect(ctx, liveshare.Options{
		ClientName:     "gh",
		SessionID:      cs.Connection.SessionID,
		SessionToken:   cs.Connection.SessionToken,
		RelaySAS:       cs.Connection.RelaySAS,
		RelayEndpoint:  cs.Connection.RelayEndpoint,
		HostPublicKeys: cs.Connection.HostPublicKeys,
		Logger:         sessionLogger,
	})
}
