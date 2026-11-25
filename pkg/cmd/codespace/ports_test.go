package codespace

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/internal/codespaces/connection"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListPorts(t *testing.T) {
	ctx := t.Context()

	mockApi := GetMockApi(false)
	ios, _, _, _ := iostreams.Test()

	a := &App{
		io:        ios,
		apiClient: mockApi,
	}

	selector := &CodespaceSelector{api: a.apiClient, codespaceName: "codespace-name"}
	err := a.ListPorts(ctx, selector, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestListPorts_escapesControlSequencesInLabel(t *testing.T) {
	tests := []struct {
		name string
		tty  bool
	}{
		{name: "tty", tty: true},
		{name: "not tty", tty: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockApi := GetMockApi(false)
			mockApi.GetCodespaceRepositoryContentsFunc = func(ctx context.Context, codespace *api.Codespace, path string) ([]byte, error) {
				return []byte(`{"portsAttributes":{"0":{"label":"web \u001b]0;title\u0007end"}}}`), nil
			}
			ios, _, stdout, _ := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)

			a := &App{
				io:        ios,
				apiClient: mockApi,
			}

			selector := &CodespaceSelector{api: a.apiClient, codespaceName: "codespace-name"}
			err := a.ListPorts(t.Context(), selector, nil)
			require.NoError(t, err)

			assert.Contains(t, stdout.String(), `web ^[]0;title^Gend`)
			assert.NotContains(t, stdout.String(), "\x1b]")
			assert.NotContains(t, stdout.String(), "\a")
		})
	}
}

func TestPortsUpdateVisibilitySuccess(t *testing.T) {
	portVisibilities := []portVisibility{
		{
			number:     80,
			visibility: "org",
		},
		{
			number:     9999,
			visibility: "public",
		},
	}

	err := runUpdateVisibilityTest(t, portVisibilities, true)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPortsUpdateVisibilityFailure(t *testing.T) {
	portVisibilities := []portVisibility{
		{
			number:     9999,
			visibility: "public",
		},
		{
			number:     80,
			visibility: "org",
		},
	}

	err := runUpdateVisibilityTest(t, portVisibilities, false)
	if err == nil {
		t.Fatalf("runUpdateVisibilityTest succeeded unexpectedly")
	}
}

func runUpdateVisibilityTest(t *testing.T, portVisibilities []portVisibility, allowOrgPorts bool) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockApi := GetMockApi(allowOrgPorts)
	ios, _, _, _ := iostreams.Test()

	a := &App{
		io:        ios,
		apiClient: mockApi,
	}

	var portArgs []string
	for _, pv := range portVisibilities {
		portArgs = append(portArgs, fmt.Sprintf("%d:%s", pv.number, pv.visibility))
	}

	selector := &CodespaceSelector{api: a.apiClient, codespaceName: "codespace-name"}

	return a.UpdatePortVisibility(ctx, selector, portArgs)
}

func TestPendingOperationDisallowsListPorts(t *testing.T) {
	app := testingPortsApp()
	selector := &CodespaceSelector{api: app.apiClient, codespaceName: "disabledCodespace"}

	if err := app.ListPorts(context.Background(), selector, nil); err != nil {
		if err.Error() != "codespace is disabled while it has a pending operation: Some pending operation" {
			t.Errorf("expected pending operation error, but got: %v", err)
		}
	} else {
		t.Error("expected pending operation error, but got nothing")
	}
}

func TestPendingOperationDisallowsUpdatePortVisibility(t *testing.T) {
	app := testingPortsApp()
	selector := &CodespaceSelector{api: app.apiClient, codespaceName: "disabledCodespace"}

	if err := app.UpdatePortVisibility(context.Background(), selector, nil); err != nil {
		if err.Error() != "codespace is disabled while it has a pending operation: Some pending operation" {
			t.Errorf("expected pending operation error, but got: %v", err)
		}
	} else {
		t.Error("expected pending operation error, but got nothing")
	}
}

func TestPendingOperationDisallowsForwardPorts(t *testing.T) {
	app := testingPortsApp()
	selector := &CodespaceSelector{api: app.apiClient, codespaceName: "disabledCodespace"}

	if err := app.ForwardPorts(context.Background(), selector, nil, false); err != nil {
		if err.Error() != "codespace is disabled while it has a pending operation: Some pending operation" {
			t.Errorf("expected pending operation error, but got: %v", err)
		}
	} else {
		t.Error("expected pending operation error, but got nothing")
	}
}

func GetMockApi(allowOrgPorts bool) *apiClientMock {
	return &apiClientMock{
		GetCodespaceFunc: func(ctx context.Context, codespaceName string, includeConnection bool) (*api.Codespace, error) {
			allowedPortPrivacySettings := []string{"public", "private"}
			if allowOrgPorts {
				allowedPortPrivacySettings = append(allowedPortPrivacySettings, "org")
			}

			return &api.Codespace{
				Name:  "codespace-name",
				State: api.CodespaceStateAvailable,
				Connection: api.CodespaceConnection{
					TunnelProperties: api.TunnelProperties{
						ConnectAccessToken:     "tunnel access-token",
						ManagePortsAccessToken: "manage-ports-token",
						ServiceUri:             "http://global.rel.tunnels.api.visualstudio.com/",
						TunnelId:               "tunnel-id",
						ClusterId:              "usw2",
						Domain:                 "domain.com",
					},
				},
				RuntimeConstraints: api.RuntimeConstraints{
					AllowedPortPrivacySettings: allowedPortPrivacySettings,
				},
			}, nil
		},
		StartCodespaceFunc: func(ctx context.Context, codespaceName string) error {
			return nil
		},
		GetCodespaceRepositoryContentsFunc: func(ctx context.Context, codespace *api.Codespace, path string) ([]byte, error) {
			return nil, nil
		},
		ExternalHTTPClientFunc: func() (*http.Client, error) {
			return connection.NewMockHttpClient()
		},
	}
}

func testingPortsApp() *App {
	disabledCodespace := &api.Codespace{
		Name:                           "disabledCodespace",
		PendingOperation:               true,
		PendingOperationDisabledReason: "Some pending operation",
	}
	apiMock := &apiClientMock{
		GetCodespaceFunc: func(_ context.Context, name string, _ bool) (*api.Codespace, error) {
			if name == "disabledCodespace" {
				return disabledCodespace, nil
			}
			return nil, nil
		},
	}

	ios, _, _, _ := iostreams.Test()

	return NewApp(ios, nil, apiMock, nil, nil)
}
