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

func makeMockSession(opts ...livesharetest.ServerOption) (*livesharetest.Server, *Session, error) {
	connection := Connection{
		SessionID:    "session-id",
		SessionToken: "session-token",
		RelaySAS:     "relay-sas",
	}
	joinWorkspace := func(req *jsonrpc2.Request) (interface{}, error) {
		return joinWorkspaceResult{1}, nil
	}
	opts = append(
		opts,
		livesharetest.WithPassword(connection.SessionToken),
		livesharetest.WithService("workspace.joinWorkspace", joinWorkspace),
	)
	testServer, err := livesharetest.NewServer(
		opts...,
	)
	connection.RelayEndpoint = "sb" + strings.TrimPrefix(testServer.URL(), "https")
	tlsConfig := WithTLSConfig(&tls.Config{InsecureSkipVerify: true})
	session, err := Connect(context.Background(), WithConnection(connection), tlsConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("error connecting to Live Share: %v", err)
	}
	return testServer, session, nil
}

func TestServerStartSharing(t *testing.T) {
	serverPort, serverProtocol := 2222, "sshd"
	startSharing := func(req *jsonrpc2.Request) (interface{}, error) {
		var args []interface{}
		if err := json.Unmarshal(*req.Params, &args); err != nil {
			return nil, fmt.Errorf("error unmarshaling request: %v", err)
		}
		if len(args) < 3 {
			return nil, errors.New("not enough arguments to start sharing")
		}
		if port, ok := args[0].(float64); !ok {
			return nil, errors.New("port argument is not an int")
		} else if port != float64(serverPort) {
			return nil, errors.New("port does not match serverPort")
		}
		if protocol, ok := args[1].(string); !ok {
			return nil, errors.New("protocol argument is not a string")
		} else if protocol != serverProtocol {
			return nil, errors.New("protocol does not match serverProtocol")
		}
		if browseURL, ok := args[2].(string); !ok {
			return nil, errors.New("browse url is not a string")
		} else if browseURL != fmt.Sprintf("http://localhost:%v", serverPort) {
			return nil, errors.New("browseURL does not match expected")
		}
		return Port{StreamName: "stream-name", StreamCondition: "stream-condition"}, nil
	}
	testServer, session, err := makeMockSession(
		livesharetest.WithService("serverSharing.startSharing", startSharing),
	)
	defer testServer.Close()
	if err != nil {
		t.Errorf("error creating mock session: %v", err)
	}
	ctx := context.Background()

	done := make(chan error)
	go func() {
		streamID, err := session.startSharing(ctx, serverProtocol, serverPort)
		if err != nil {
			done <- fmt.Errorf("error sharing server: %v", err)
		}
		if streamID.name == "" || streamID.condition == "" {
			done <- errors.New("stream name or condition is blank")
		}
		done <- nil
	}()

	select {
	case err := <-testServer.Err():
		t.Errorf("error from server: %v", err)
	case err := <-done:
		if err != nil {
			t.Errorf("error from client: %v", err)
		}
	}
}

func TestServerGetSharedServers(t *testing.T) {
	sharedServer := Port{
		SourcePort:      2222,
		StreamName:      "stream-name",
		StreamCondition: "stream-condition",
	}
	getSharedServers := func(req *jsonrpc2.Request) (interface{}, error) {
		return []*Port{&sharedServer}, nil
	}
	testServer, session, err := makeMockSession(
		livesharetest.WithService("serverSharing.getSharedServers", getSharedServers),
	)
	if err != nil {
		t.Errorf("error creating mock session: %v", err)
	}
	defer testServer.Close()
	ctx := context.Background()
	done := make(chan error)
	go func() {
		ports, err := session.GetSharedServers(ctx)
		if err != nil {
			done <- fmt.Errorf("error getting shared servers: %v", err)
		}
		if len(ports) < 1 {
			done <- errors.New("not enough ports returned")
		}
		if ports[0].SourcePort != sharedServer.SourcePort {
			done <- errors.New("source port does not match")
		}
		if ports[0].StreamName != sharedServer.StreamName {
			done <- errors.New("stream name does not match")
		}
		if ports[0].StreamCondition != sharedServer.StreamCondition {
			done <- errors.New("stream condiion does not match")
		}
		done <- nil
	}()

	select {
	case err := <-testServer.Err():
		t.Errorf("error from server: %v", err)
	case err := <-done:
		if err != nil {
			t.Errorf("error from client: %v", err)
		}
	}
}

func TestServerUpdateSharedVisibility(t *testing.T) {
	updateSharedVisibility := func(rpcReq *jsonrpc2.Request) (interface{}, error) {
		var req []interface{}
		if err := json.Unmarshal(*rpcReq.Params, &req); err != nil {
			return nil, fmt.Errorf("unmarshal req: %v", err)
		}
		if len(req) < 2 {
			return nil, errors.New("request arguments is less than 2")
		}
		if port, ok := req[0].(float64); ok {
			if port != 80.0 {
				return nil, errors.New("port param is not expected value")
			}
		} else {
			return nil, errors.New("port param is not a float64")
		}
		if public, ok := req[1].(bool); ok {
			if public != true {
				return nil, errors.New("pulic param is not expected value")
			}
		} else {
			return nil, errors.New("public param is not a bool")
		}
		return nil, nil
	}
	testServer, session, err := makeMockSession(
		livesharetest.WithService("serverSharing.updateSharedServerVisibility", updateSharedVisibility),
	)
	if err != nil {
		t.Errorf("creating mock session: %v", err)
	}
	defer testServer.Close()
	ctx := context.Background()
	done := make(chan error)
	go func() {
		done <- session.UpdateSharedVisibility(ctx, 80, true)
	}()
	select {
	case err := <-testServer.Err():
		t.Errorf("error from server: %v", err)
	case err := <-done:
		if err != nil {
			t.Errorf("error from client: %v", err)
		}
	}
}
