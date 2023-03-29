package codespace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/liveshare"
	livesharetest "github.com/cli/cli/v2/pkg/liveshare/test"
	"github.com/sourcegraph/jsonrpc2"
)

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

	eventResponses := []string{
		"serverSharing.sharingSucceeded",
		"serverSharing.sharingSucceeded",
	}

	portsData := []liveshare.PortNotification{
		{
			Success:    true,
			Port:       80,
			ChangeKind: liveshare.PortChangeKindUpdate,
		},
		{
			Success:    true,
			Port:       9999,
			ChangeKind: liveshare.PortChangeKindUpdate,
		},
	}

	err := runUpdateVisibilityTest(t, portVisibilities, eventResponses, portsData)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPortsUpdateVisibilityFailure403(t *testing.T) {
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

	eventResponses := []string{
		"serverSharing.sharingSucceeded",
		"serverSharing.sharingFailed",
	}

	portsData := []liveshare.PortNotification{
		{
			Success:    true,
			Port:       80,
			ChangeKind: liveshare.PortChangeKindUpdate,
		},
		{
			Success:     false,
			Port:        9999,
			ChangeKind:  liveshare.PortChangeKindUpdate,
			ErrorDetail: "test error",
			StatusCode:  403,
		},
	}

	err := runUpdateVisibilityTest(t, portVisibilities, eventResponses, portsData)
	if err == nil {
		t.Fatalf("runUpdateVisibilityTest succeeded unexpectedly")
	}

	if errors.Unwrap(err) != errUpdatePortVisibilityForbidden {
		t.Errorf("expected: %v, got: %v", errUpdatePortVisibilityForbidden, errors.Unwrap(err))
	}
}

func TestPortsUpdateVisibilityFailure(t *testing.T) {
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

	eventResponses := []string{
		"serverSharing.sharingSucceeded",
		"serverSharing.sharingFailed",
	}

	portsData := []liveshare.PortNotification{
		{
			Success:    true,
			Port:       80,
			ChangeKind: liveshare.PortChangeKindUpdate,
		},
		{
			Success:     false,
			Port:        9999,
			ChangeKind:  liveshare.PortChangeKindUpdate,
			ErrorDetail: "test error",
		},
	}

	err := runUpdateVisibilityTest(t, portVisibilities, eventResponses, portsData)
	if err == nil {
		t.Fatalf("runUpdateVisibilityTest succeeded unexpectedly")
	}

	var expectedErr *ErrUpdatingPortVisibility
	if !errors.As(err, &expectedErr) {
		t.Errorf("expected: %v, got: %v", expectedErr, err)
	}
}

type joinWorkspaceResult struct {
	SessionNumber int `json:"sessionNumber"`
}

func runUpdateVisibilityTest(t *testing.T, portVisibilities []portVisibility, eventResponses []string, portsData []liveshare.PortNotification) error {
	t.Helper()
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("fails intermittently in CI: https://github.com/cli/cli/issues/5663")
	}

	joinWorkspace := func(conn *jsonrpc2.Conn, req *jsonrpc2.Request) (interface{}, error) {
		return joinWorkspaceResult{1}, nil
	}
	const sessionToken = "session-token"

	ch := make(chan *jsonrpc2.Conn, 1)
	updateSharedVisibility := func(conn *jsonrpc2.Conn, rpcReq *jsonrpc2.Request) (interface{}, error) {
		ch <- conn
		return nil, nil
	}
	testServer, err := livesharetest.NewServer(
		livesharetest.WithNonSecure(),
		livesharetest.WithPassword(sessionToken),
		livesharetest.WithService("workspace.joinWorkspace", joinWorkspace),
		livesharetest.WithService("serverSharing.updateSharedServerPrivacy", updateSharedVisibility),
	)
	if err != nil {
		return fmt.Errorf("unable to create test server: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		for i, pd := range portsData {
			select {
			case <-ctx.Done():
				return
			case conn := <-ch:
				_, _ = conn.DispatchCall(ctx, eventResponses[i], pd, nil)
			}
		}
	}()

	mockApi := &apiClientMock{
		GetCodespaceFunc: func(ctx context.Context, codespaceName string, includeConnection bool) (*api.Codespace, error) {
			return &api.Codespace{
				Name:  "codespace-name",
				State: api.CodespaceStateAvailable,
				Connection: api.CodespaceConnection{
					SessionID:      "session-id",
					SessionToken:   sessionToken,
					RelayEndpoint:  testServer.URL(),
					RelaySAS:       "relay-sas",
					HostPublicKeys: []string{livesharetest.SSHPublicKey},
				},
			}, nil
		},
	}

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

func TestPendingOperationDisallowsUpdatePortVisability(t *testing.T) {
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

	if err := app.ForwardPorts(context.Background(), selector, nil); err != nil {
		if err.Error() != "codespace is disabled while it has a pending operation: Some pending operation" {
			t.Errorf("expected pending operation error, but got: %v", err)
		}
	} else {
		t.Error("expected pending operation error, but got nothing")
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
