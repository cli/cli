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
	"github.com/cli/cli/v2/pkg/liveshare"
	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const (
	serverConnectionTimeout = 5 * time.Second
	requestTimeout          = 30 * time.Second
	portConnectionTimeout   = 30 * time.Second
)

const (
	codespacesInternalPort        = 16634
	codespacesInternalSessionName = "CodespacesInternal"
)

type Client struct {
	conn          *grpc.ClientConn
	token         string
	listener      net.Listener
	jupyterClient jupyter.JupyterServerHostClient
}

type liveshareSession interface {
	KeepAlive(string)
	OpenStreamingChannel(context.Context, liveshare.ChannelID) (ssh.Channel, error)
	StartSharing(context.Context, string, int) (liveshare.ChannelID, error)
}

// Finds a free port to listen on and creates a new gRPC client that connects to that port
func Connect(ctx context.Context, session liveshareSession, token string) (*Client, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", 0))
	if err != nil {
		return nil, fmt.Errorf("failed to listen to local port over tcp: %w", err)
	}

	// Tunnel the remote gRPC server port to the local port
	localAddress := fmt.Sprintf("127.0.0.1:%d", listener.Addr().(*net.TCPAddr).Port)
	internalTunnelClosed := make(chan error, 1)
	go func() {
		fwd := liveshare.NewPortForwarder(session, codespacesInternalSessionName, codespacesInternalPort, true)
		internalTunnelClosed <- fwd.ForwardToListener(ctx, listener)
	}()

	// Ping the port to ensure that it is fully forwarded before continuing
	connctx, cancel := context.WithTimeout(ctx, portConnectionTimeout)
	defer cancel()
	err = liveshare.WaitForPortConnection(connctx, localAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to local port: %w", err)
	}

	// Attempt to connect to the port
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	}
	ctx, cancel = context.WithTimeout(ctx, serverConnectionTimeout)
	defer cancel()
	conn, err := grpc.DialContext(ctx, localAddress, opts...)
	if err != nil {
		return nil, err
	}

	g := &Client{
		conn:          conn,
		token:         token,
		listener:      listener,
		jupyterClient: jupyter.NewJupyterServerHostClient(conn),
	}

	return g, nil
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
func (g *Client) StartJupyterServer(ctx context.Context) (port int, serverUrl string, err error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
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
