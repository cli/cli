package codespaces

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/github/ghcs/internal/api"
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

// ConnectToLiveshare creates a Live Share client and joins the Live Share session.
// It will start the Codespace if it is not already running, it will time out after 60 seconds if fails to start.
func ConnectToLiveshare(ctx context.Context, log logger, apiClient *api.API, userLogin, token string, codespace *api.Codespace) (*liveshare.Session, error) {
	var startedCodespace bool
	if codespace.Environment.State != api.CodespaceEnvironmentStateAvailable {
		startedCodespace = true
		log.Print("Starting your codespace...")
		if err := apiClient.StartCodespace(ctx, token, codespace); err != nil {
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

	lsclient, err := liveshare.NewClient(
		liveshare.WithConnection(liveshare.Connection{
			SessionID:     codespace.Environment.Connection.SessionID,
			SessionToken:  codespace.Environment.Connection.SessionToken,
			RelaySAS:      codespace.Environment.Connection.RelaySAS,
			RelayEndpoint: codespace.Environment.Connection.RelayEndpoint,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("error creating Live Share client: %w", err)
	}

	return lsclient.JoinWorkspace(ctx)
}

type apiClient interface {
	CreateCodespace(ctx context.Context, user *api.User, repo *api.Repository, machine, branch, location string) (*api.Codespace, error)
	GetCodespaceToken(ctx context.Context, userLogin, codespaceName string) (string, error)
	GetCodespace(ctx context.Context, token, userLogin, codespaceName string) (*api.Codespace, error)
}

// ProvisionParams are the required parameters for provisioning a Codespace.
type ProvisionParams struct {
	User                      *api.User
	Repository                *api.Repository
	Branch, Machine, Location string
}

// Provision creates a codespace with the given parameters and handles polling in the case
// of initial creation failures.
func Provision(ctx context.Context, log logger, client apiClient, params *ProvisionParams) (*api.Codespace, error) {
	codespace, err := client.CreateCodespace(
		ctx, params.User, params.Repository, params.Machine, params.Branch, params.Location,
	)
	if err != nil {
		// This error is returned by the API when the initial creation fails with a retryable error.
		// A retryable error means that GitHub will retry to re-create Codespace and clients should poll
		// the API and attempt to fetch the Codespace for the next two minutes.
		if err == api.ErrCreateAsyncRetry {
			log.Print("Switching to async provisioning...")

			pollTimeout := 2 * time.Minute
			pollInterval := 1 * time.Second
			codespace, err = pollForCodespace(ctx, client, log, pollTimeout, pollInterval, params.User.Login, codespace.Name)
			log.Print("\n")

			if err != nil {
				return nil, fmt.Errorf("error creating codespace with async provisioning: %s: %w", codespace.Name, err)
			}
		}

		return nil, err
	}

	return codespace, nil
}

// pollForCodespace polls the Codespaces GET endpoint on a given interval for a specified duration.
// If it succeeds at fetching the codespace, we consider the codespace provisioned.
func pollForCodespace(ctx context.Context, client apiClient, log logger, duration, interval time.Duration, user, name string) (*api.Codespace, error) {
	ctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			log.Print(".")
			token, err := client.GetCodespaceToken(ctx, user, name)
			if err != nil {
				if err == api.ErrNotProvisioned {
					// Do nothing. We expect this to fail until the codespace is provisioned
					continue
				}

				return nil, fmt.Errorf("failed to get codespace token: %w", err)
			}

			return client.GetCodespace(ctx, token, user, name)
		}
	}
}
