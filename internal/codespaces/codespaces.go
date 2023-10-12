package codespaces

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/internal/codespaces/connection"
)

func connectionReady(codespace *api.Codespace) bool {
	// If the codespace is not available, it is not ready
	if codespace.State != api.CodespaceStateAvailable {
		return false
	}

	return codespace.Connection.TunnelProperties.ConnectAccessToken != "" &&
		codespace.Connection.TunnelProperties.ManagePortsAccessToken != "" &&
		codespace.Connection.TunnelProperties.ServiceUri != "" &&
		codespace.Connection.TunnelProperties.TunnelId != "" &&
		codespace.Connection.TunnelProperties.ClusterId != "" &&
		codespace.Connection.TunnelProperties.Domain != ""
}

type apiClient interface {
	GetCodespace(ctx context.Context, name string, includeConnection bool) (*api.Codespace, error)
	StartCodespace(ctx context.Context, name string) error
	HTTPClient() (*http.Client, error)
}

type progressIndicator interface {
	StartProgressIndicatorWithLabel(s string)
	StopProgressIndicator()
}

type TimeoutError struct {
	message string
}

func (e *TimeoutError) Error() string {
	return e.message
}

// GetCodespaceConnection waits until a codespace is able
// to be connected to and initializes a connection to it.
func GetCodespaceConnection(ctx context.Context, progress progressIndicator, apiClient apiClient, codespace *api.Codespace) (*connection.CodespaceConnection, error) {
	codespace, err := waitUntilCodespaceConnectionReady(ctx, progress, apiClient, codespace)
	if err != nil {
		return nil, err
	}

	progress.StartProgressIndicatorWithLabel("Connecting to codespace")
	defer progress.StopProgressIndicator()

	httpClient, err := apiClient.HTTPClient()
	if err != nil {
		return nil, fmt.Errorf("error getting http client: %w", err)
	}

	return connection.NewCodespaceConnection(ctx, codespace, httpClient)
}

// waitUntilCodespaceConnectionReady waits for a Codespace to be running and is able to be connected to.
func waitUntilCodespaceConnectionReady(ctx context.Context, progress progressIndicator, apiClient apiClient, codespace *api.Codespace) (*api.Codespace, error) {
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

			return &TimeoutError{message: "codespace not ready yet"}
		}, backoff.WithContext(expBackoff, ctx))
		if err != nil {
			var timeoutErr *TimeoutError
			if errors.As(err, &timeoutErr) {
				return nil, errors.New("timed out while waiting for the codespace to start")
			}

			return nil, err
		}
	}

	return codespace, nil
}

// ListenTCP starts a localhost tcp listener on 127.0.0.1 (unless allInterfaces is true) and returns the listener and bound port
func ListenTCP(port int, allInterfaces bool) (*net.TCPListener, int, error) {
	host := "127.0.0.1"
	if allInterfaces {
		host = ""
	}

	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", host, port))
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
