package liveshare

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	livesharetest "github.com/cli/cli/v2/pkg/liveshare/test"
	"github.com/sourcegraph/jsonrpc2"
)

func TestConnect(t *testing.T) {
	opts := Options{
		SessionID:      "session-id",
		SessionToken:   "session-token",
		RelaySAS:       "relay-sas",
		HostPublicKeys: []string{livesharetest.SSHPublicKey},
		Logger:         newMockLogger(),
	}
	joinWorkspace := func(conn *jsonrpc2.Conn, req *jsonrpc2.Request) (interface{}, error) {
		var joinWorkspaceReq joinWorkspaceArgs
		if err := json.Unmarshal(*req.Params, &joinWorkspaceReq); err != nil {
			return nil, fmt.Errorf("error unmarshaling req: %w", err)
		}
		if joinWorkspaceReq.ID != opts.SessionID {
			return nil, errors.New("connection session id does not match")
		}
		if joinWorkspaceReq.ConnectionMode != "local" {
			return nil, errors.New("connection mode is not local")
		}
		if joinWorkspaceReq.JoiningUserSessionToken != opts.SessionToken {
			return nil, errors.New("connection user token does not match")
		}
		if joinWorkspaceReq.ClientCapabilities.IsNonInteractive != false {
			return nil, errors.New("non interactive is not false")
		}
		return joinWorkspaceResult{1}, nil
	}

	server, err := livesharetest.NewServer(
		livesharetest.WithPassword(opts.SessionToken),
		livesharetest.WithService("workspace.joinWorkspace", joinWorkspace),
		livesharetest.WithRelaySAS(opts.RelaySAS),
	)
	if err != nil {
		t.Errorf("error creating Live Share server: %v", err)
	}
	defer server.Close()
	opts.RelayEndpoint = "sb" + strings.TrimPrefix(server.URL(), "https")

	ctx := context.Background()

	opts.TLSConfig = &tls.Config{InsecureSkipVerify: true}

	done := make(chan error)
	go func() {
		_, err := Connect(ctx, opts) // ignore session
		done <- err
	}()

	select {
	case err := <-server.Err():
		t.Errorf("error from server: %v", err)
	case err := <-done:
		if err != nil {
			t.Errorf("error from client: %v", err)
		}
	}
}
