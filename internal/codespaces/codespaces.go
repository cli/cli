package codespaces

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/liveshare"
)

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

type progressIndicator interface {
	StartProgressIndicatorWithLabel(s string)
	StopProgressIndicator()
}

type logger interface {
	Println(v ...interface{})
	Printf(f string, v ...interface{})
}

// ConnectToLiveshare waits for a Codespace to become running,
// and connects to it using a Live Share session.
func ConnectToLiveshare(ctx context.Context, progress progressIndicator, sessionLogger logger, apiClient apiClient, codespace *api.Codespace) (*liveshare.Session, error) {
	if codespace.State != api.CodespaceStateAvailable {
		progress.StartProgressIndicatorWithLabel("Starting codespace")
		defer progress.StopProgressIndicator()
		if err := apiClient.StartCodespace(ctx, codespace.Name); err != nil {
			return nil, fmt.Errorf("error starting codespace: %w", err)
		}
	}

	if !connectionReady(codespace) {
		expBackoff := backoff.NewExponentialBackOff()
		expBackoff.Multiplier = 1.1
		expBackoff.MaxInterval = 10 * time.Second
		expBackoff.MaxElapsedTime = 5 * time.Minute

		err := backoff.Retry(func() error {
			var err error
			codespace, err = apiClient.GetCodespace(ctx, codespace.Name, true)
			if err != nil {
				return backoff.Permanent(fmt.Errorf("error getting codespace: %w", err))
			}

			if connectionReady(codespace) {
				return nil
			}

			return errors.New("codespace not ready yet")
		}, backoff.WithContext(expBackoff, ctx))
		if err != nil {
			var permErr *backoff.PermanentError
			if errors.As(err, &permErr) {
				return nil, err
			}

			return nil, errors.New("timed out while waiting for the codespace to start")
		}
	}

	progress.StartProgressIndicatorWithLabel("Connecting to codespace")
	defer progress.StopProgressIndicator()

	return liveshare.Connect(ctx, liveshare.Options{
		SessionID:      codespace.Connection.SessionID,
		SessionToken:   codespace.Connection.SessionToken,
		RelaySAS:       codespace.Connection.RelaySAS,
		RelayEndpoint:  codespace.Connection.RelayEndpoint,
		HostPublicKeys: codespace.Connection.HostPublicKeys,
		Logger:         sessionLogger,
	})
}

// ListenTCP starts a localhost tcp listener and returns the listener and bound port
func ListenTCP(port int) (*net.TCPListener, int, error) {
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to build tcp address: %w", err)
	}
	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to listen to local port over tcp: %w", err)
	}
	port = listener.Addr().(*net.TCPAddr).Port

	return listener, port, nil
}
