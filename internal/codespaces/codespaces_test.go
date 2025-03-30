package codespaces

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/cli/cli/v2/internal/codespaces/api"
)

func init() {
	// Set the backoff to 0 for testing so that they run quickly
	codespaceStatePollingBackoff = backoff.NewConstantBackOff(time.Second * 0)
}

// This is just enough to trick `connectionReady`
var readyCodespace = &api.Codespace{
	State: api.CodespaceStateAvailable,
	Connection: api.CodespaceConnection{
		TunnelProperties: api.TunnelProperties{
			ConnectAccessToken:     "test",
			ManagePortsAccessToken: "test",
			ServiceUri:             "test",
			TunnelId:               "test",
			ClusterId:              "test",
			Domain:                 "test",
		},
	},
}

func TestWaitUntilCodespaceConnectionReady_WhenAlreadyReady(t *testing.T) {
	t.Parallel()

	apiClient := &mockApiClient{}
	result, err := waitUntilCodespaceConnectionReady(context.Background(), &mockProgressIndicator{}, apiClient, readyCodespace)
	if err != nil {
		t.Fatalf("Expected nil error, but was %v", err)
	}
	if result.State != api.CodespaceStateAvailable {
		t.Fatalf("Expected final state to be %s, but was %s", api.CodespaceStateAvailable, result.State)
	}
}

func TestWaitUntilCodespaceConnectionReady_PollsApi(t *testing.T) {
	t.Parallel()

	apiClient := &mockApiClient{
		onGetCodespace: func() (*api.Codespace, error) {
			return readyCodespace, nil
		},
	}
	result, err := waitUntilCodespaceConnectionReady(context.Background(), &mockProgressIndicator{}, apiClient, &api.Codespace{State: api.CodespaceStateStarting})

	if err != nil {
		t.Fatalf("Expected nil error, but was %v", err)
	}
	if result.State != api.CodespaceStateAvailable {
		t.Fatalf("Expected final state to be %s, but was %s", api.CodespaceStateAvailable, result.State)
	}
}

func TestWaitUntilCodespaceConnectionReady_StartsCodespace(t *testing.T) {
	t.Parallel()

	codespace := &api.Codespace{State: api.CodespaceStateShutdown}

	apiClient := &mockApiClient{
		onGetCodespace: func() (*api.Codespace, error) {
			return codespace, nil
		},
		onStartCodespace: func() error {
			*codespace = *readyCodespace
			return nil
		},
	}
	result, err := waitUntilCodespaceConnectionReady(context.Background(), &mockProgressIndicator{}, apiClient, codespace)
	if err != nil {
		t.Fatalf("Expected nil error, but was %v", err)
	}
	if result.State != api.CodespaceStateAvailable {
		t.Fatalf("Expected final state to be %s, but was %s", api.CodespaceStateAvailable, result.State)
	}
}

func TestWaitUntilCodespaceConnectionReady_PollsCodespaceUntilReady(t *testing.T) {
	t.Parallel()

	codespace := &api.Codespace{State: api.CodespaceStateShutdown}
	hasPolled := false

	apiClient := &mockApiClient{
		onGetCodespace: func() (*api.Codespace, error) {
			if hasPolled {
				*codespace = *readyCodespace
			}

			hasPolled = true

			return codespace, nil
		},
		onStartCodespace: func() error {
			codespace.State = api.CodespaceStateStarting
			return nil
		},
	}
	result, err := waitUntilCodespaceConnectionReady(context.Background(), &mockProgressIndicator{}, apiClient, codespace)
	if err != nil {
		t.Fatalf("Expected nil error, but was %v", err)
	}
	if result.State != api.CodespaceStateAvailable {
		t.Fatalf("Expected final state to be %s, but was %s", api.CodespaceStateAvailable, result.State)
	}
}

func TestWaitUntilCodespaceConnectionReady_WaitsForShutdownBeforeStarting(t *testing.T) {
	t.Parallel()

	codespace := &api.Codespace{State: api.CodespaceStateShuttingDown}

	apiClient := &mockApiClient{
		onGetCodespace: func() (*api.Codespace, error) {
			// Make sure that we poll at least once before going to shutdown
			if codespace.State == api.CodespaceStateShuttingDown {
				codespace.State = api.CodespaceStateShutdown
			}
			return codespace, nil
		},
		onStartCodespace: func() error {
			if codespace.State != api.CodespaceStateShutdown {
				t.Fatalf("Codespace started from non-shutdown state: %s", codespace.State)
			}
			*codespace = *readyCodespace
			return nil
		},
	}
	result, err := waitUntilCodespaceConnectionReady(context.Background(), &mockProgressIndicator{}, apiClient, codespace)
	if err != nil {
		t.Fatalf("Expected nil error, but was %v", err)
	}
	if result.State != api.CodespaceStateAvailable {
		t.Fatalf("Expected final state to be %s, but was %s", api.CodespaceStateAvailable, result.State)
	}
}

func TestUntilCodespaceConnectionReady_DoesntStartTwice(t *testing.T) {
	t.Parallel()

	codespace := &api.Codespace{State: api.CodespaceStateShutdown}
	didStart := false
	didPollAfterStart := false

	apiClient := &mockApiClient{
		onGetCodespace: func() (*api.Codespace, error) {
			// Make sure that we are in shutdown state for one poll after starting to make sure we don't try to start again
			if didPollAfterStart {
				*codespace = *readyCodespace
			}

			if didStart {
				didPollAfterStart = true
			}

			return codespace, nil
		},
		onStartCodespace: func() error {
			if didStart {
				t.Fatal("Should not start multiple times")
			}
			didStart = true
			return nil
		},
	}
	result, err := waitUntilCodespaceConnectionReady(context.Background(), &mockProgressIndicator{}, apiClient, codespace)
	if err != nil {
		t.Fatalf("Expected nil error, but was %v", err)
	}
	if result.State != api.CodespaceStateAvailable {
		t.Fatalf("Expected final state to be %s, but was %s", api.CodespaceStateAvailable, result.State)
	}
}

type mockApiClient struct {
	onStartCodespace func() error
	onGetCodespace   func() (*api.Codespace, error)
}

func (m *mockApiClient) StartCodespace(ctx context.Context, name string) error {
	if m.onStartCodespace == nil {
		panic("onStartCodespace not set and StartCodespace was called")
	}

	return m.onStartCodespace()
}

func (m *mockApiClient) GetCodespace(ctx context.Context, name string, includeConnection bool) (*api.Codespace, error) {
	if m.onGetCodespace == nil {
		panic("onGetCodespace not set and GetCodespace was called")
	}

	return m.onGetCodespace()
}

func (m *mockApiClient) HTTPClient() (*http.Client, error) {
	panic("Not implemented")
}

type mockProgressIndicator struct{}

func (m *mockProgressIndicator) StartProgressIndicatorWithLabel(s string) {}
func (m *mockProgressIndicator) StopProgressIndicator()                   {}
