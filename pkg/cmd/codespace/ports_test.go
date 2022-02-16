package codespace

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/iostreams"
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
		"sharingSucceeded",
		"sharingSucceeded",
	}

	portsData := []portData{
		{
			Port:       80,
			ChangeKind: portChangeKindUpdate,
		},
		{
			Port:       9999,
			ChangeKind: portChangeKindUpdate,
		},
	}

	err := RunUpdateVisibilityTest(t, portVisibilities, eventResponses, portsData)

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
		"sharingSucceeded",
		"sharingFailed",
	}

	portsData := []portData{
		{
			Port:       80,
			ChangeKind: portChangeKindUpdate,
		},
		{
			Port:        9999,
			ChangeKind:  portChangeKindUpdate,
			ErrorDetail: "test error",
			StatusCode:  403,
		},
	}

	err := RunUpdateVisibilityTest(t, portVisibilities, eventResponses, portsData)
	if err == nil {
		t.Errorf("unexpected error: %v", err)
	}

	expectedErr := "error waiting for port 9999 to update to public: organization admin has forbidden this privacy setting"
	if err.Error() != expectedErr {
		t.Errorf("expected: %v, got: %v", expectedErr, err)
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
		"sharingSucceeded",
		"sharingFailed",
	}

	portsData := []portData{
		{
			Port:       80,
			ChangeKind: portChangeKindUpdate,
		},
		{
			Port:        9999,
			ChangeKind:  portChangeKindUpdate,
			ErrorDetail: "test error",
		},
	}

	err := RunUpdateVisibilityTest(t, portVisibilities, eventResponses, portsData)
	if err == nil {
		t.Errorf("unexpected error: %v", err)
	}

	expectedErr := "error waiting for port 9999 to update to public: test error"
	if err.Error() != expectedErr {
		t.Errorf("expected: %v, got: %v", expectedErr, err)
	}
}

type joinWorkspaceResult struct {
	SessionNumber int `json:"sessionNumber"`
}

func RunUpdateVisibilityTest(t *testing.T, portVisibilities []portVisibility, eventResponses []string, portsData []portData) error {
	joinWorkspace := func(req *jsonrpc2.Request) (interface{}, error) {
		return joinWorkspaceResult{1}, nil
	}
	const sessionToken = "session-token"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan float64, 1)
	updateSharedVisibility := func(rpcReq *jsonrpc2.Request) (interface{}, error) {
		var req []interface{}
		if err := json.Unmarshal(*rpcReq.Params, &req); err != nil {
			return nil, fmt.Errorf("unmarshal req: %w", err)
		}

		ch <- req[0].(float64)
		return nil, nil
	}
	testServer, err := livesharetest.NewServer(
		livesharetest.WithNonSecure(),
		livesharetest.WithPassword(sessionToken),
		livesharetest.WithService("workspace.joinWorkspace", joinWorkspace),
		livesharetest.WithService("serverSharing.updateSharedServerPrivacy", updateSharedVisibility),
	)
	if err != nil {
		t.Fatal(err)
	}

	type rpcMessage struct {
		Method string
		Params portData
	}

	for index, pd := range portsData {
		go func(index int, pd portData) {
			for {
				select {
				case <-ctx.Done():
					return
				case <-ch:
					testServer.WriteToObjectStream(rpcMessage{
						Method: eventResponses[index],
						Params: pd,
					})
				}
			}
		}(index, pd)
	}

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
