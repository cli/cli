package codespace

import (
	"context"
	"errors"
	"fmt"
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

	io, _, _, _ := iostreams.Test()

	a := &App{
		io:        io,
		apiClient: mockApi,
	}

	var portArgs []string
	for _, pv := range portVisibilities {
		portArgs = append(portArgs, fmt.Sprintf("%d:%s", pv.number, pv.visibility))
	}

	return a.UpdatePortVisibility(ctx, "codespace-name", portArgs)
}

func TestPendingOperationDisallowsListPorts(t *testing.T) {
	app := testingPortsApp()

	if err := app.ListPorts(context.Background(), "disabledCodespace", nil); err != nil {
		if err.Error() != "codespace is disabled while it has a pending operation: Some pending operation" {
			t.Errorf("expected pending operation error, but got: %v", err)
		}
	} else {
		t.Error("expected pending operation error, but got nothing")
	}
}

func TestPendingOperationDisallowsUpdatePortVisability(t *testing.T) {
	app := testingPortsApp()

	if err := app.UpdatePortVisibility(context.Background(), "disabledCodespace", nil); err != nil {
		if err.Error() != "codespace is disabled while it has a pending operation: Some pending operation" {
			t.Errorf("expected pending operation error, but got: %v", err)
		}
	} else {
		t.Error("expected pending operation error, but got nothing")
	}
}

func TestPendingOperationDisallowsForwardPorts(t *testing.T) {
	app := testingPortsApp()

	if err := app.ForwardPorts(context.Background(), "disabledCodespace", nil); err != nil {
		if err.Error() != "codespace is disabled while it has a pending operation: Some pending operation" {
			t.Errorf("expected pending operation error, but got: %v", err)
		}
	} else {
		t.Error("expected pending operation error, but got nothing")
	}
}

func testingPortsApp() *App {
	user := &api.User{Login: "monalisa"}
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
		GetUserFunc: func(_ context.Context) (*api.User, error) {
			return user, nil
		},
		AuthorizedKeysFunc: func(_ context.Context, _ string) ([]byte, error) {
			return []byte{}, nil
		},
	}

	io, _, _, _ := iostreams.Test()

	return NewApp(io, nil, apiMock, nil)
}
