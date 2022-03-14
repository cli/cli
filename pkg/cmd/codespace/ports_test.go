package codespace

import (
	"context"
	"encoding/json"
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
			Success: true,
			PortUpdate: liveshare.PortUpdate{
				Port:       80,
				ChangeKind: liveshare.PortChangeKindUpdate,
			},
		},
		{
			Success: true,
			PortUpdate: liveshare.PortUpdate{
				Port:       9999,
				ChangeKind: liveshare.PortChangeKindUpdate,
			},
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
			Success: true,
			PortUpdate: liveshare.PortUpdate{
				Port:       80,
				ChangeKind: liveshare.PortChangeKindUpdate,
			},
		},
		{
			Success: false,
			PortUpdate: liveshare.PortUpdate{
				Port:        9999,
				ChangeKind:  liveshare.PortChangeKindUpdate,
				ErrorDetail: "test error",
				StatusCode:  403,
			},
		},
	}

	err := runUpdateVisibilityTest(t, portVisibilities, eventResponses, portsData)
	if err == nil {
		t.Errorf("unexpected error: %v", err)
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
			Success: true,
			PortUpdate: liveshare.PortUpdate{
				Port:       80,
				ChangeKind: liveshare.PortChangeKindUpdate,
			},
		},
		{
			Success: false,
			PortUpdate: liveshare.PortUpdate{
				Port:        9999,
				ChangeKind:  liveshare.PortChangeKindUpdate,
				ErrorDetail: "test error",
			},
		},
	}

	err := runUpdateVisibilityTest(t, portVisibilities, eventResponses, portsData)
	if err == nil {
		t.Errorf("unexpected error: %v", err)
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan *jsonrpc2.Conn, 1)
	updateSharedVisibility := func(conn *jsonrpc2.Conn, rpcReq *jsonrpc2.Request) (interface{}, error) {
		var req []interface{}
		if err := json.Unmarshal(*rpcReq.Params, &req); err != nil {
			return nil, fmt.Errorf("unmarshal req: %w", err)
		}

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

	type rpcMessage struct {
		Method string
		Params liveshare.PortUpdate
	}

	go func() {
		var i int
		for ; ; i++ {
			select {
			case <-ctx.Done():
				return
			case conn := <-ch:
				pd := portsData[i]
				_, _ = conn.DispatchCall(context.Background(), eventResponses[i], pd.PortUpdate, nil)
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

	err = a.UpdatePortVisibility(ctx, "codespace-name", portArgs)

	return err
}
