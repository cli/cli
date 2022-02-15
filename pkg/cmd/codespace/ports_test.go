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

type joinWorkspaceResult struct {
	SessionNumber int `json:"sessionNumber"`
}

func TestPortsUpdateVisibility(t *testing.T) {
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

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case port := <-ch:
				testServer.WriteToObjectStream(rpcMessage{
					Method: "sharingSucceeded",
					Params: portData{
						Port:       int(port),
						ChangeKind: portChangeKindUpdate,
					},
				})
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

	err = a.UpdatePortVisibility(ctx, "codespace-name", []string{"80:80", "9999:9999"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
