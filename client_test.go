package liveshare

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	livesharetest "github.com/github/go-liveshare/test"
	"github.com/sourcegraph/jsonrpc2"
)

func TestNewClient(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Errorf("error creating new client: %v", err)
	}
	if client == nil {
		t.Error("client is nil")
	}
}

func TestNewClientValidConnection(t *testing.T) {
	connection := Connection{"1", "2", "3", "4"}

	client, err := NewClient(WithConnection(connection))
	if err != nil {
		t.Errorf("error creating new client: %v", err)
	}
	if client == nil {
		t.Error("client is nil")
	}
}

func TestNewClientWithInvalidConnection(t *testing.T) {
	connection := Connection{}

	if _, err := NewClient(WithConnection(connection)); err == nil {
		t.Error("err is nil")
	}
}

func TestJoinSession(t *testing.T) {
	connection := Connection{
		SessionID:    "session-id",
		SessionToken: "session-token",
		RelaySAS:     "relay-sas",
	}
	joinWorkspace := func(req *jsonrpc2.Request) (interface{}, error) {
		var joinWorkspaceReq joinWorkspaceArgs
		if err := json.Unmarshal(*req.Params, &joinWorkspaceReq); err != nil {
			return nil, fmt.Errorf("error unmarshaling req: %v", err)
		}
		if joinWorkspaceReq.ID != connection.SessionID {
			return nil, errors.New("connection session id does not match")
		}
		if joinWorkspaceReq.ConnectionMode != "local" {
			return nil, errors.New("connection mode is not local")
		}
		if joinWorkspaceReq.JoiningUserSessionToken != connection.SessionToken {
			return nil, errors.New("connection user token does not match")
		}
		if joinWorkspaceReq.ClientCapabilities.IsNonInteractive != false {
			return nil, errors.New("non interactive is not false")
		}
		return joinWorkspaceResult{1}, nil
	}

	server, err := livesharetest.NewServer(
		livesharetest.WithPassword(connection.SessionToken),
		livesharetest.WithService("workspace.joinWorkspace", joinWorkspace),
		livesharetest.WithRelaySAS(connection.RelaySAS),
	)
	if err != nil {
		t.Errorf("error creating Live Share server: %v", err)
	}
	defer server.Close()
	connection.RelayEndpoint = "sb" + strings.TrimPrefix(server.URL(), "https")

	ctx := context.Background()

	tlsConfig := WithTLSConfig(&tls.Config{InsecureSkipVerify: true})
	client, err := NewClient(WithConnection(connection), tlsConfig)
	if err != nil {
		t.Errorf("error creating new client: %v", err)
	}

	done := make(chan error)
	go func() {
		session, err := client.JoinWorkspace(ctx)
		if err != nil {
			done <- fmt.Errorf("error joining workspace: %v", err)
			return
		}
		_ = session

		done <- nil
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
