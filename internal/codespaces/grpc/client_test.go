package grpc

import (
	"context"
	"fmt"
	"testing"

	"github.com/cli/cli/v2/internal/codespaces/grpc/test"
)

func TestMain(m *testing.M) {
	// Start the gRPC server in the background
	go func() {
		err := test.StartServer()
		if err != nil {
			panic(err)
		}
	}()

	m.Run()
}

func connect(t *testing.T) (ctx context.Context, client *Client) {
	t.Helper()
	ctx = context.Background()
	client, err := Connect(ctx, &test.Session{}, "token")
	if err != nil {
		t.Fatalf("error connecting to internal server: %v", err)
	}

	t.Cleanup(func() {
		client.Close()
	})

	return ctx, client
}

// Test that the gRPC client returns the correct port and URL when the JupyterLab server starts successfully
func TestStartJupyterServerSuccess(t *testing.T) {
	ctx, client := connect(t)
	port, url, err := client.StartJupyterServer(ctx)
	if err != nil {
		t.Fatalf("expected %v, got %v", nil, err)
	}
	if port != test.JupyterPort {
		t.Fatalf("expected %d, got %d", test.JupyterPort, port)
	}
	if url != test.JupyterServerUrl {
		t.Fatalf("expected %s, got %s", test.JupyterServerUrl, url)
	}
}

// Test that the gRPC client returns an error when the JupyterLab server fails to start
func TestStartJupyterServerFailure(t *testing.T) {
	ctx, client := connect(t)
	test.JupyterMessage = "error message"
	test.JupyterResult = false
	errorMessage := fmt.Sprintf("failed to start JupyterLab: %s", test.JupyterMessage)
	port, url, err := client.StartJupyterServer(ctx)
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
