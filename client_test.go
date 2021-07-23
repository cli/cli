package liveshare

import (
	"context"
	"crypto/tls"
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

func TestClientJoin(t *testing.T) {
	sessionToken := "session-token"
	joinWorkspace := func(req *jsonrpc2.Request) (interface{}, error) {
		return 1, nil
	}

	server, err := livesharetest.NewServer(
		livesharetest.WithPassword(sessionToken),
		livesharetest.WithService("workspace.joinWorkspace", joinWorkspace),
	)
	if err != nil {
		t.Errorf("error creating liveshare server: %v", err)
	}
	defer server.Close()

	ctx := context.Background()
	connection := Connection{
		SessionID:     "session-id",
		SessionToken:  sessionToken,
		RelaySAS:      "relay-sas",
		RelayEndpoint: "sb" + strings.TrimPrefix(server.URL(), "https"),
	}

	tlsConfig := WithTLSConfig(&tls.Config{InsecureSkipVerify: true})
	client, err := NewClient(WithConnection(connection), tlsConfig)
	if err != nil {
		t.Errorf("error creating new client: %v", err)
	}

	clientErr := make(chan error)
	go func() {
		if err := client.Join(ctx); err != nil {
			clientErr <- fmt.Errorf("error joining client: %v", err)
			return
		}

		ctx.Done()
	}()

	select {
	case err := <-server.Err():
		t.Errorf("error from server: %v", err)
	case err := <-clientErr:
		t.Errorf("error from client: %v", err)
	case <-ctx.Done():
		return
	}
}
