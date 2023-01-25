package rpc

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"testing"

	"github.com/cli/cli/v2/internal/codespaces/rpc/codespace"
	"github.com/cli/cli/v2/internal/codespaces/rpc/jupyter"
	"github.com/cli/cli/v2/internal/codespaces/rpc/ssh"
	rpctest "github.com/cli/cli/v2/internal/codespaces/rpc/test"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

type mockServer struct {
	jupyter.JupyterServerHostServerMock
	codespace.CodespaceHostServerMock
	ssh.SshServerHostServerMock
}

func newMockServer() *mockServer {
	server := &mockServer{}

	server.CodespaceHostServerMock.NotifyCodespaceOfClientActivityFunc = func(context.Context, *codespace.NotifyCodespaceOfClientActivityRequest) (*codespace.NotifyCodespaceOfClientActivityResponse, error) {
		return &codespace.NotifyCodespaceOfClientActivityResponse{
			Message: "",
			Result:  true,
		}, nil
	}

	return server
}

func runTestServer(ctx context.Context, server *mockServer) error {
	listener, err := net.Listen("tcp", "127.0.0.1:16634")
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	defer listener.Close()

	s := grpc.NewServer()
	jupyter.RegisterJupyterServerHostServer(s, server)
	codespace.RegisterCodespaceHostServer(s, server)
	ssh.RegisterSshServerHostServer(s, server)

	ch := make(chan error, 1)
	go func() { ch <- s.Serve(listener) }()

	select {
	case <-ctx.Done():
		s.Stop()
		<-ch
		return nil
	case err := <-ch:
		return err
	}
}

// Test that the RPC invoker notifies the codespace of client activity on connection
func verifyNotifyCodespaceOfClientActivity(t *testing.T, server *mockServer) {
	calls := server.CodespaceHostServerMock.NotifyCodespaceOfClientActivityCalls()
	if len(calls) == 0 {
		t.Fatalf("no client activity calls")
	}

	for _, call := range calls {
		activities := call.NotifyCodespaceOfClientActivityRequest.GetClientActivities()
		if activities[0] == connectedEventName {
			return
		}
	}

	t.Fatalf("no activity named %s", connectedEventName)
}

// Test that the RPC invoker returns the correct port and URL when the JupyterLab server starts successfully
func TestStartJupyterServerSuccess(t *testing.T) {
	resp := jupyter.GetRunningServerResponse{
		Port:      strconv.Itoa(1234),
		ServerUrl: "http://localhost:1234?token=1234",
		Message:   "",
		Result:    true,
	}

	server := newMockServer()
	server.JupyterServerHostServerMock.GetRunningServerFunc = func(context.Context, *jupyter.GetRunningServerRequest) (*jupyter.GetRunningServerResponse, error) {
		return &resp, nil
	}

	var wg errgroup.Group
	defer wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg.Go(func() error { return runTestServer(ctx, server) })

	invoker, err := CreateInvoker(context.Background(), &rpctest.Session{})
	if err != nil {
		t.Fatalf("error connecting to internal server: %v", err)
	}
	defer invoker.Close()

	port, url, err := invoker.StartJupyterServer(context.Background())
	if err != nil {
		t.Fatalf("expected %v, got %v", nil, err)
	}
	if strconv.Itoa(port) != resp.Port {
		t.Fatalf("expected %s, got %d", resp.Port, port)
	}
	if url != resp.ServerUrl {
		t.Fatalf("expected %s, got %s", resp.ServerUrl, url)
	}

	verifyNotifyCodespaceOfClientActivity(t, server)
}

// Test that the RPC invoker returns an error when the JupyterLab server fails to start
func TestStartJupyterServerFailure(t *testing.T) {
	resp := jupyter.GetRunningServerResponse{
		Port:      strconv.Itoa(1234),
		ServerUrl: "http://localhost:1234?token=1234",
		Message:   "error message",
		Result:    false,
	}

	server := newMockServer()
	server.JupyterServerHostServerMock.GetRunningServerFunc = func(context.Context, *jupyter.GetRunningServerRequest) (*jupyter.GetRunningServerResponse, error) {
		return &resp, nil
	}

	var wg errgroup.Group
	defer wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg.Go(func() error { return runTestServer(ctx, server) })

	invoker, err := CreateInvoker(context.Background(), &rpctest.Session{})
	if err != nil {
		t.Fatalf("error connecting to internal server: %v", err)
	}
	defer invoker.Close()

	errorMessage := fmt.Sprintf("failed to start JupyterLab: %s", resp.Message)
	port, url, err := invoker.StartJupyterServer(context.Background())
	if err.Error() != errorMessage {
		t.Fatalf("expected %v, got %v", errorMessage, err)
	}
	if port != 0 {
		t.Fatalf("expected %d, got %d", 0, port)
	}
	if url != "" {
		t.Fatalf("expected %s, got %s", "", url)
	}

	verifyNotifyCodespaceOfClientActivity(t, server)
}

// Test that the RPC invoker doesn't throw an error when requesting an incremental rebuild
func TestRebuildContainerIncremental(t *testing.T) {
	resp := codespace.RebuildContainerResponse{
		RebuildContainer: true,
	}

	server := newMockServer()
	server.RebuildContainerAsyncFunc = func(context.Context, *codespace.RebuildContainerRequest) (*codespace.RebuildContainerResponse, error) {
		return &resp, nil
	}

	var wg errgroup.Group
	defer wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg.Go(func() error { return runTestServer(ctx, server) })

	invoker, err := CreateInvoker(context.Background(), &rpctest.Session{})
	if err != nil {
		t.Fatalf("error connecting to internal server: %v", err)
	}
	defer invoker.Close()

	err = invoker.RebuildContainer(context.Background(), false)
	if err != nil {
		t.Fatalf("expected %v, got %v", nil, err)
	}

	verifyNotifyCodespaceOfClientActivity(t, server)
}

// Test that the RPC invoker doesn't throw an error when requesting a full rebuild
func TestRebuildContainerFull(t *testing.T) {
	resp := codespace.RebuildContainerResponse{
		RebuildContainer: true,
	}

	server := newMockServer()
	server.RebuildContainerAsyncFunc = func(context.Context, *codespace.RebuildContainerRequest) (*codespace.RebuildContainerResponse, error) {
		return &resp, nil
	}

	var wg errgroup.Group
	defer wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg.Go(func() error { return runTestServer(ctx, server) })

	invoker, err := CreateInvoker(context.Background(), &rpctest.Session{})
	if err != nil {
		t.Fatalf("error connecting to internal server: %v", err)
	}
	defer invoker.Close()

	err = invoker.RebuildContainer(context.Background(), true)
	if err != nil {
		t.Fatalf("expected %v, got %v", nil, err)
	}

	verifyNotifyCodespaceOfClientActivity(t, server)
}

// Test that the RPC invoker throws an error when the rebuild fails
func TestRebuildContainerFailure(t *testing.T) {
	resp := codespace.RebuildContainerResponse{
		RebuildContainer: false,
	}

	server := newMockServer()
	server.RebuildContainerAsyncFunc = func(context.Context, *codespace.RebuildContainerRequest) (*codespace.RebuildContainerResponse, error) {
		return &resp, nil
	}

	var wg errgroup.Group
	defer wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg.Go(func() error { return runTestServer(ctx, server) })

	invoker, err := CreateInvoker(context.Background(), &rpctest.Session{})
	if err != nil {
		t.Fatalf("error connecting to internal server: %v", err)
	}
	defer invoker.Close()

	errorMessage := "couldn't rebuild codespace"
	err = invoker.RebuildContainer(context.Background(), true)
	if err.Error() != errorMessage {
		t.Fatalf("expected %v, got %v", errorMessage, err)
	}
}

// Test that the RPC invoker returns the correct port and user when the SSH server starts successfully
func TestStartSSHServerSuccess(t *testing.T) {
	resp := ssh.StartRemoteServerResponse{
		ServerPort: strconv.Itoa(1234),
		User:       "test",
		Message:    "",
		Result:     true,
	}

	server := newMockServer()
	server.StartRemoteServerAsyncFunc = func(context.Context, *ssh.StartRemoteServerRequest) (*ssh.StartRemoteServerResponse, error) {
		return &resp, nil
	}

	var wg errgroup.Group
	defer wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg.Go(func() error { return runTestServer(ctx, server) })

	invoker, err := CreateInvoker(context.Background(), &rpctest.Session{})
	if err != nil {
		t.Fatalf("error connecting to internal server: %v", err)
	}
	defer invoker.Close()

	port, user, err := invoker.StartSSHServer(context.Background())
	if err != nil {
		t.Fatalf("expected %v, got %v", nil, err)
	}
	if strconv.Itoa(port) != resp.ServerPort {
		t.Fatalf("expected %s, got %d", resp.ServerPort, port)
	}
	if user != resp.User {
		t.Fatalf("expected %s, got %s", resp.User, user)
	}

	verifyNotifyCodespaceOfClientActivity(t, server)
}

// Test that the RPC invoker returns an error when the SSH server fails to start
func TestStartSSHServerFailure(t *testing.T) {
	resp := ssh.StartRemoteServerResponse{
		ServerPort: strconv.Itoa(1234),
		User:       "test",
		Message:    "error message",
		Result:     false,
	}

	server := newMockServer()
	server.StartRemoteServerAsyncFunc = func(context.Context, *ssh.StartRemoteServerRequest) (*ssh.StartRemoteServerResponse, error) {
		return &resp, nil
	}

	var wg errgroup.Group
	defer wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg.Go(func() error { return runTestServer(ctx, server) })

	invoker, err := CreateInvoker(context.Background(), &rpctest.Session{})
	if err != nil {
		t.Fatalf("error connecting to internal server: %v", err)
	}
	defer invoker.Close()

	errorMessage := fmt.Sprintf("failed to start SSH server: %s", resp.Message)
	port, user, err := invoker.StartSSHServer(context.Background())
	if err.Error() != errorMessage {
		t.Fatalf("expected %v, got %v", errorMessage, err)
	}
	if port != 0 {
		t.Fatalf("expected %d, got %d", 0, port)
	}
	if user != "" {
		t.Fatalf("expected %s, got %s", "", user)
	}
}
