package grpc

// gRPC client implementation to be able to connect to the gRPC server and perform the following operations:
// - Start a remote JupyterLab server

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/cli/cli/v2/pkg/grpc/jupyter"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const (
	requestTimeout = 30 * time.Second
)

type GrpcClient struct {
	conn          *grpc.ClientConn
	token         string
	jupyterClient jupyter.JupyterServerHostClient
}

func New() *GrpcClient {
	return &GrpcClient{}
}

// Connects to the gRPC server on the given port
func (g *GrpcClient) Connect(ctx context.Context, port int, token string) error {
	// Attempt to connect to the given port
	conn, err := grpc.Dial(fmt.Sprintf("localhost:%d", port), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())

	if err != nil {
		return fmt.Errorf("Failed to connect to the internal server on port %d", port)
	}

	g.conn = conn
	g.token = token
	g.jupyterClient = jupyter.NewJupyterServerHostClient(conn)

	return nil
}

// Appends the authentication token to the gRPC context
func (g *GrpcClient) appendMetadata(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "Authorization", "Bearer "+g.token)
}

// Starts a remote JupyterLab server to allow the user to connect to the codespace via JupyterLab in their browser
func (g *GrpcClient) GetRunningServer() (int, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	ctx = g.appendMetadata(ctx)
	defer cancel()

	response, err := g.jupyterClient.GetRunningServer(ctx, &jupyter.GetRunningServerRequest{})

	if err != nil {
		return 0, "", fmt.Errorf("failed to invoke JupyterLab RPC: %w", err)
	}

	if !response.Result {
		return 0, "", fmt.Errorf("failed to start JupyterLab: %s", response.Message)
	}

	port, err := strconv.Atoi(response.Port)

	if err != nil {
		return 0, "", fmt.Errorf("failed to parse JupyterLab port: %w", err)
	}

	return port, response.ServerUrl, err
}
