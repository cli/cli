package liveshare

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	livesharetest "github.com/cli/cli/v2/pkg/liveshare/test"
	"github.com/sourcegraph/jsonrpc2"
)

const mockClientName = "liveshare-client"

func makeMockSession(opts ...livesharetest.ServerOption) (*livesharetest.Server, *Session, error) {
	joinWorkspace := func(req *jsonrpc2.Request) (interface{}, error) {
		return joinWorkspaceResult{1}, nil
	}
	const sessionToken = "session-token"
	opts = append(
		opts,
		livesharetest.WithPassword(sessionToken),
		livesharetest.WithService("workspace.joinWorkspace", joinWorkspace),
	)
	testServer, err := livesharetest.NewServer(opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating server: %w", err)
	}

	session, err := Connect(context.Background(), Options{
		ClientName:     mockClientName,
		SessionID:      "session-id",
		SessionToken:   sessionToken,
		RelayEndpoint:  "sb" + strings.TrimPrefix(testServer.URL(), "https"),
		RelaySAS:       "relay-sas",
		HostPublicKeys: []string{livesharetest.SSHPublicKey},
		TLSConfig:      &tls.Config{InsecureSkipVerify: true},
		Logger:         newMockLogger(),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("error connecting to Live Share: %w", err)
	}
	return testServer, session, nil
}

func TestServerStartSharing(t *testing.T) {
	serverPort, serverProtocol := 2222, "sshd"
	startSharing := func(req *jsonrpc2.Request) (interface{}, error) {
		var args []interface{}
		if err := json.Unmarshal(*req.Params, &args); err != nil {
			return nil, fmt.Errorf("error unmarshaling request: %w", err)
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
		} else if browseURL != fmt.Sprintf("http://localhost:%d", serverPort) {
			return nil, errors.New("browseURL does not match expected")
		}
		return Port{StreamName: "stream-name", StreamCondition: "stream-condition"}, nil
	}
	testServer, session, err := makeMockSession(
		livesharetest.WithService("serverSharing.startSharing", startSharing),
	)
	defer testServer.Close() //nolint:staticcheck // httptest.Server does not return errors on Close()

	if err != nil {
		t.Errorf("error creating mock session: %v", err)
	}
	ctx := context.Background()

	done := make(chan error)
	go func() {
		streamID, err := session.startSharing(ctx, serverProtocol, serverPort)
		if err != nil {
			done <- fmt.Errorf("error sharing server: %w", err)
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
			done <- fmt.Errorf("error getting shared servers: %w", err)
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

func TestServerUpdateSharedServerPrivacy(t *testing.T) {
	updateSharedVisibility := func(rpcReq *jsonrpc2.Request) (interface{}, error) {
		var req []interface{}
		if err := json.Unmarshal(*rpcReq.Params, &req); err != nil {
			return nil, fmt.Errorf("unmarshal req: %w", err)
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
		if privacy, ok := req[1].(string); ok {
			if privacy != "public" {
				return nil, fmt.Errorf("expected privacy param to be public but got %q", privacy)
			}
		} else {
			return nil, fmt.Errorf("expected privacy param to be a bool but go %T", req[1])
		}
		return nil, nil
	}
	testServer, session, err := makeMockSession(
		livesharetest.WithService("serverSharing.updateSharedServerPrivacy", updateSharedVisibility),
	)
	if err != nil {
		t.Errorf("creating mock session: %v", err)
	}
	defer testServer.Close()
	ctx := context.Background()
	done := make(chan error)
	go func() {
		done <- session.UpdateSharedServerPrivacy(ctx, 80, "public")
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

func TestInvalidHostKey(t *testing.T) {
	joinWorkspace := func(req *jsonrpc2.Request) (interface{}, error) {
		return joinWorkspaceResult{1}, nil
	}
	const sessionToken = "session-token"
	opts := []livesharetest.ServerOption{
		livesharetest.WithPassword(sessionToken),
		livesharetest.WithService("workspace.joinWorkspace", joinWorkspace),
	}
	testServer, err := livesharetest.NewServer(opts...)
	if err != nil {
		t.Errorf("error creating server: %v", err)
	}
	_, err = Connect(context.Background(), Options{
		SessionID:      "session-id",
		SessionToken:   sessionToken,
		RelayEndpoint:  "sb" + strings.TrimPrefix(testServer.URL(), "https"),
		RelaySAS:       "relay-sas",
		HostPublicKeys: []string{},
		TLSConfig:      &tls.Config{InsecureSkipVerify: true},
	})
	if err == nil {
		t.Error("expected invalid host key error, got: nil")
	}
}

func TestKeepAliveNonBlocking(t *testing.T) {
	session := &Session{keepAliveReason: make(chan string, 1)}
	for i := 0; i < 2; i++ {
		session.keepAlive("io")
	}

	// if keepAlive blocks, we'll never reach this and timeout the test
	// timing out
}

func TestNotifyHostOfActivity(t *testing.T) {
	notifyHostOfActivity := func(rpcReq *jsonrpc2.Request) (interface{}, error) {
		var req []interface{}
		if err := json.Unmarshal(*rpcReq.Params, &req); err != nil {
			return nil, fmt.Errorf("unmarshal req: %w", err)
		}
		if len(req) < 2 {
			return nil, errors.New("request arguments is less than 2")
		}

		if clientName, ok := req[0].(string); ok {
			if clientName != mockClientName {
				return nil, fmt.Errorf(
					"unexpected clientName param, expected: %q, got: %q", mockClientName, clientName,
				)
			}
		} else {
			return nil, errors.New("clientName param is not a string")
		}

		if acs, ok := req[1].([]interface{}); ok {
			if fmt.Sprintf("%s", acs) != "[input]" {
				return nil, fmt.Errorf("unexpected activities param, expected: [input], got: %s", acs)
			}
		} else {
			return nil, errors.New("activities param is not a slice")
		}

		return nil, nil
	}
	svc := livesharetest.WithService(
		"ICodespaceHostService.notifyCodespaceOfClientActivity", notifyHostOfActivity,
	)
	testServer, session, err := makeMockSession(svc)
	if err != nil {
		t.Errorf("creating mock session: %w", err)
	}
	defer testServer.Close()
	ctx := context.Background()
	done := make(chan error)
	go func() {
		done <- session.notifyHostOfActivity(ctx, "input")
	}()
	select {
	case err := <-testServer.Err():
		t.Errorf("error from server: %w", err)
	case err := <-done:
		if err != nil {
			t.Errorf("error from client: %w", err)
		}
	}
}

func TestSessionHeartbeat(t *testing.T) {
	var (
		requestsMu sync.Mutex
		requests   int
	)
	notifyHostOfActivity := func(rpcReq *jsonrpc2.Request) (interface{}, error) {
		requestsMu.Lock()
		requests++
		requestsMu.Unlock()

		var req []interface{}
		if err := json.Unmarshal(*rpcReq.Params, &req); err != nil {
			return nil, fmt.Errorf("unmarshal req: %w", err)
		}
		if len(req) < 2 {
			return nil, errors.New("request arguments is less than 2")
		}

		if clientName, ok := req[0].(string); ok {
			if clientName != mockClientName {
				return nil, fmt.Errorf(
					"unexpected clientName param, expected: %q, got: %q", mockClientName, clientName,
				)
			}
		} else {
			return nil, errors.New("clientName param is not a string")
		}

		if acs, ok := req[1].([]interface{}); ok {
			if fmt.Sprintf("%s", acs) != "[input]" {
				return nil, fmt.Errorf("unexpected activities param, expected: [input], got: %s", acs)
			}
		} else {
			return nil, errors.New("activities param is not a slice")
		}

		return nil, nil
	}
	svc := livesharetest.WithService(
		"ICodespaceHostService.notifyCodespaceOfClientActivity", notifyHostOfActivity,
	)
	testServer, session, err := makeMockSession(svc)
	if err != nil {
		t.Errorf("creating mock session: %w", err)
	}
	defer testServer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})

	logger := newMockLogger()
	session.logger = logger

	go session.heartbeat(ctx, 50*time.Millisecond)
	go func() {
		session.keepAlive("input")
		<-time.Tick(200 * time.Millisecond)
		session.keepAlive("input")
		<-time.Tick(100 * time.Millisecond)
		done <- struct{}{}
	}()

	select {
	case err := <-testServer.Err():
		t.Errorf("error from server: %w", err)
	case <-done:
		activityCount := strings.Count(logger.String(), "input")
		if activityCount != 2 {
			t.Errorf("unexpected number of activities, expected: 2, got: %d", activityCount)
		}

		requestsMu.Lock()
		rc := requests
		requestsMu.Unlock()
		if rc != 2 {
			t.Errorf("unexpected number of requests, expected: 2, got: %d", requests)
		}
		return
	}
}

type mockLogger struct {
	sync.Mutex
	buf *bytes.Buffer
}

func newMockLogger() *mockLogger {
	return &mockLogger{buf: new(bytes.Buffer)}
}

func (m *mockLogger) Printf(format string, v ...interface{}) {
	m.Lock()
	defer m.Unlock()
	m.buf.WriteString(fmt.Sprintf(format, v...))
}

func (m *mockLogger) Println(v ...interface{}) {
	m.Lock()
	defer m.Unlock()
	m.buf.WriteString(fmt.Sprintln(v...))
}

func (m *mockLogger) String() string {
	m.Lock()
	defer m.Unlock()
	return m.buf.String()
}
