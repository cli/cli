package grpc

// gRPC client implementation to be able to connect to the gRPC server and perform the following operations:
// - Start a remote JupyterLab server

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/cli/cli/v2/internal/codespaces/grpc/jupyter"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const (
	connectionTimeout = 5 * time.Second
	requestTimeout    = 30 * time.Second
)

type Client struct {
	conn          *grpc.ClientConn
	token         string
	listener      net.Listener
	jupyterClient jupyter.JupyterServerHostClient
}

func NewClient() *Client {
	return &Client{}
}

// Connects to the gRPC server on the given port
func (g *Client) Connect(ctx context.Context, listener net.Listener, port int, token string) error {
	// Attempt to connect to the given port
	conn, err := grpc.Dial(fmt.Sprintf("127.0.0.1:%d", port), grpc.WithTimeout(connectionTimeout), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		return err
	}

	g.conn = conn
	g.token = token
	g.listener = listener
	g.jupyterClient = jupyter.NewJupyterServerHostClient(conn)

	return nil
}

// Closes the gRPC connection
func (g *Client) Close() error {
	// Closing the local listener effectively closes the gRPC connection
	if err := g.listener.Close(); err != nil {
		g.conn.Close() // If we fail to close the listener, explicitly close the gRPC connection and ignore any error
		return fmt.Errorf("failed to close local tcp port listener: %w", err)
	}

	return nil
}

// Appends the authentication token to the gRPC context
func (g *Client) appendMetadata(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "Authorization", "Bearer "+g.token)
}

// Starts a remote JupyterLab server to allow the user to connect to the codespace via JupyterLab in their browser
func (g *Client) StartJupyterServer() (port int, serverUrl string, err error) {
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

	port, err = strconv.Atoi(response.Port)
	if err != nil {
		return 0, "", fmt.Errorf("failed to parse JupyterLab port: %w", err)
	}

	return port, response.ServerUrl, err
}
