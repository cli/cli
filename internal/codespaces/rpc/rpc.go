package rpc

import (
	"context"
	"fmt"

	"github.com/cli/cli/v2/internal/codespaces/rpc/grpc"
	"github.com/cli/cli/v2/pkg/liveshare"
)

// Helper function to connect to the GRPC server in the codespace
func connectToGRPCServer(ctx context.Context, session *liveshare.Session, token string) (*grpc.Client, error) {
	ctx, cancel := context.WithTimeout(ctx, grpc.ConnectionTimeout)
	defer cancel()

	client, err := grpc.Connect(ctx, session, token)
	if err != nil {
		return nil, fmt.Errorf("error connecting to internal server: %w", err)
	}

	return client, nil
}

func RebuildContainer(ctx context.Context, session *liveshare.Session, full bool) error {
	return session.RebuildContainer(ctx, full)
}

func StartSSHServer(ctx context.Context, session *liveshare.Session) (int, string, error) {
	return session.StartSSHServer(ctx)
}

func StartSSHServerWithOptions(ctx context.Context, session *liveshare.Session, options liveshare.StartSSHServerOptions) (int, string, error) {
	return session.StartSSHServerWithOptions(ctx, options)
}

func StartJupyterServer(ctx context.Context, session *liveshare.Session) (int, string, error) {
	client, err := connectToGRPCServer(ctx, session, "")
	ctx, cancel := context.WithTimeout(ctx, grpc.RequestTimeout)
	defer cancel()

	serverPort, serverUrl, err := client.StartJupyterServer(ctx)
	if err != nil {
		return 0, "", fmt.Errorf("failed to start JupyterLab server: %w", err)
	}

	return serverPort, serverUrl, nil
}
