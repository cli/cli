package grpc

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"

	grpctest "github.com/cli/cli/v2/internal/codespaces/grpc/test"
)

func startServer(t *testing.T) {
	t.Helper()
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Skip("fails intermittently in CI: https://github.com/cli/cli/issues/5663")
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Start the gRPC server in the background
	go func() {
		err := grpctest.StartServer(ctx)
		if err != nil && err != context.Canceled {
			log.Println(fmt.Errorf("error starting test server: %v", err))
		}
	}()

	// Stop the gRPC server when the test is done
	t.Cleanup(func() {
		cancel()
	})
}

func connect(t *testing.T) (client *Client) {
	t.Helper()

	client, err := Connect(context.Background(), &grpctest.Session{}, "token")
	if err != nil {
		t.Fatalf("error connecting to internal server: %v", err)
	}

	t.Cleanup(func() {
		client.Close()
	})

	return client
}

// Test that the gRPC client returns the correct port and URL when the JupyterLab server starts successfully
func TestStartJupyterServerSuccess(t *testing.T) {
	startServer(t)
	client := connect(t)

	port, url, err := client.StartJupyterServer(context.Background())
	if err != nil {
		t.Fatalf("expected %v, got %v", nil, err)
	}
	if port != grpctest.JupyterPort {
		t.Fatalf("expected %d, got %d", grpctest.JupyterPort, port)
	}
	if url != grpctest.JupyterServerUrl {
		t.Fatalf("expected %s, got %s", grpctest.JupyterServerUrl, url)
	}
}

// Test that the gRPC client returns an error when the JupyterLab server fails to start
func TestStartJupyterServerFailure(t *testing.T) {
	startServer(t)
	client := connect(t)
	grpctest.JupyterMessage = "error message"
	grpctest.JupyterResult = false
	errorMessage := fmt.Sprintf("failed to start JupyterLab: %s", grpctest.JupyterMessage)
	port, url, err := client.StartJupyterServer(context.Background())
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
