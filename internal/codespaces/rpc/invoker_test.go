package rpc

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"

	rpctest "github.com/cli/cli/v2/internal/codespaces/rpc/test"
)

func startServer(t *testing.T) {
	t.Helper()
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("fails intermittently in CI: https://github.com/cli/cli/issues/5663")
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start the gRPC server in the background
	go func() {
		err := rpctest.StartServer(ctx)
		if err != nil && err != context.Canceled {
			log.Println(fmt.Errorf("error starting test server: %v", err))
		}
	}()

	// Stop the gRPC server when the test is done
	t.Cleanup(func() {
		cancel()
	})
}

func createTestInvoker(t *testing.T) Invoker {
	t.Helper()

	// Clear the stored client activity
	rpctest.NotifyReceivedActivity = ""

	invoker, err := CreateInvoker(context.Background(), &rpctest.Session{})
	if err != nil {
		t.Fatalf("error connecting to internal server: %v", err)
	}

	t.Cleanup(func() {
		testNotifyCodespaceOfClientActivity(t)
		invoker.Close()
	})

	return invoker
}

// Test that the RPC invoker notifies the codespace of client activity on connection
func testNotifyCodespaceOfClientActivity(t *testing.T) {
	if rpctest.NotifyReceivedActivity != connectedEventName {
		t.Fatalf("expected %s, got %s", connectedEventName, rpctest.NotifyMessage)
	}
}

// Test that the RPC invoker returns the correct port and URL when the JupyterLab server starts successfully
func TestStartJupyterServerSuccess(t *testing.T) {
	startServer(t)
	invoker := createTestInvoker(t)
	port, url, err := invoker.StartJupyterServer(context.Background())
	if err != nil {
		t.Fatalf("expected %v, got %v", nil, err)
	}
	if port != rpctest.JupyterPort {
		t.Fatalf("expected %d, got %d", rpctest.JupyterPort, port)
	}
	if url != rpctest.JupyterServerUrl {
		t.Fatalf("expected %s, got %s", rpctest.JupyterServerUrl, url)
	}
}

// Test that the RPC invoker returns an error when the JupyterLab server fails to start
func TestStartJupyterServerFailure(t *testing.T) {
	startServer(t)
	invoker := createTestInvoker(t)
	rpctest.JupyterMessage = "error message"
	rpctest.JupyterResult = false
	errorMessage := fmt.Sprintf("failed to start JupyterLab: %s", rpctest.JupyterMessage)
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
}

// Test that the RPC invoker doesn't throw an error when requesting an incremental rebuild
func TestRebuildContainerIncremental(t *testing.T) {
	startServer(t)
	invoker := createTestInvoker(t)
	err := invoker.RebuildContainer(context.Background(), false)
	if err != nil {
		t.Fatalf("expected %v, got %v", nil, err)
	}
}

// Test that the RPC invoker doesn't throw an error when requesting a full rebuild
func TestRebuildContainerFull(t *testing.T) {
	startServer(t)
	invoker := createTestInvoker(t)
	err := invoker.RebuildContainer(context.Background(), true)
	if err != nil {
		t.Fatalf("expected %v, got %v", nil, err)
	}
}

// Test that the RPC invoker throws an error when the rebuild fails
func TestRebuildContainerFailure(t *testing.T) {
	startServer(t)
	invoker := createTestInvoker(t)
	rpctest.RebuildContainer = false
	errorMessage := "couldn't rebuild codespace"
	err := invoker.RebuildContainer(context.Background(), true)
	if err.Error() != errorMessage {
		t.Fatalf("expected %v, got %v", errorMessage, err)
	}
}

// Test that the RPC invoker returns the correct port and user when the SSH server starts successfully
func TestStartSSHServerSuccess(t *testing.T) {
	startServer(t)
	invoker := createTestInvoker(t)
	port, user, err := invoker.StartSSHServer(context.Background())
	if err != nil {
		t.Fatalf("expected %v, got %v", nil, err)
	}
	if port != rpctest.SshServerPort {
		t.Fatalf("expected %d, got %d", rpctest.SshServerPort, port)
	}
	if user != rpctest.SshUser {
		t.Fatalf("expected %s, got %s", rpctest.SshUser, user)
	}
}

// Test that the RPC invoker returns an error when the SSH server fails to start
func TestStartSSHServerFailure(t *testing.T) {
	startServer(t)
	invoker := createTestInvoker(t)
	rpctest.SshMessage = "error message"
	rpctest.SshResult = false
	errorMessage := fmt.Sprintf("failed to start SSH server: %s", rpctest.SshMessage)
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
