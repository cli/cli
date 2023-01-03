package rpc

// gRPC client implementation to be able to connect to the gRPC server and perform the following operations:
// - Start a remote JupyterLab server

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/cli/cli/v2/internal/codespaces/rpc/jupyter"
	"github.com/cli/cli/v2/pkg/liveshare"
	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const (
	ConnectionTimeout = 5 * time.Second
	requestTimeout    = 30 * time.Second
)

const (
	codespacesInternalPort        = 16634
	codespacesInternalSessionName = "CodespacesInternal"
)

type liveshareSession interface {
	Close() error
	GetSharedServers(context.Context) ([]*liveshare.Port, error)
	KeepAlive(string)
	OpenStreamingChannel(context.Context, liveshare.ChannelID) (ssh.Channel, error)
	StartSharing(context.Context, string, int) (liveshare.ChannelID, error)
	StartSSHServer(context.Context) (int, string, error)
	StartSSHServerWithOptions(context.Context, liveshare.StartSSHServerOptions) (int, string, error)
	RebuildContainer(context.Context, bool) error
}

type Invoker struct {
	conn          *grpc.ClientConn
	token         string
	session       liveshareSession
	listener      net.Listener
	jupyterClient jupyter.JupyterServerHostClient
	cancelPF      context.CancelFunc
}

// Finds a free port to listen on and creates a new gRPC client that connects to that port
func Connect(ctx context.Context, session liveshareSession, token string) (*Invoker, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", 0))
	if err != nil {
		return nil, fmt.Errorf("failed to listen to local port over tcp: %w", err)
	}
	localAddress := fmt.Sprintf("127.0.0.1:%d", listener.Addr().(*net.TCPAddr).Port)

	invoker := &Invoker{
		token:    token,
		session:  session,
		listener: listener,
	}

	// Create a cancelable context to be able to cancel background tasks
	// if we encounter an error while connecting to the gRPC server
	connectctx, cancel := context.WithCancel(context.Background())
	defer func() {
		if err != nil {
			cancel()
		}
	}()

	ch := make(chan error, 2) // Buffered channel to ensure we don't block on the goroutine

	// Ensure we close the port forwarder if we encounter an error
	// or once the gRPC connection is closed. pfcancel is retained
	// to close the PF whenever we close the gRPC connection.
	pfctx, pfcancel := context.WithCancel(connectctx)
	invoker.cancelPF = pfcancel

	// Tunnel the remote gRPC server port to the local port
	go func() {
		fwd := liveshare.NewPortForwarder(session, codespacesInternalSessionName, codespacesInternalPort, true)
		ch <- fwd.ForwardToListener(pfctx, listener)
	}()

	var conn *grpc.ClientConn
	go func() {
		// Attempt to connect to the port
		opts := []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
		}
		conn, err = grpc.DialContext(connectctx, localAddress, opts...)
		ch <- err // nil if we successfully connected
	}()

	// Wait for the connection to be established or for the context to be cancelled
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-ch:
		if err != nil {
			return nil, err
		}
	}

	invoker.conn = conn
	invoker.jupyterClient = jupyter.NewJupyterServerHostClient(conn)

	return invoker, nil
}

// Closes the gRPC connection
func (g *Invoker) Close() error {
	g.cancelPF()

	// Closing the local listener effectively closes the gRPC connection
	if err := g.listener.Close(); err != nil {
		g.conn.Close() // If we fail to close the listener, explicitly close the gRPC connection and ignore any error
		return fmt.Errorf("failed to close local tcp port listener: %w", err)
	}

	return nil
}

// Appends the authentication token to the gRPC context
func (g *Invoker) appendMetadata(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "Authorization", "Bearer "+g.token)
}

// Starts a remote JupyterLab server to allow the user to connect to the codespace via JupyterLab in their browser
func (g *Invoker) StartJupyterServer(ctx context.Context) (port int, serverUrl string, err error) {
	ctx = g.appendMetadata(ctx)
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
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

// Rebuilds the container using cached layers by default or from scratch if full is true
func (g *Invoker) RebuildContainer(ctx context.Context, full bool) error {
	return g.session.RebuildContainer(ctx, full)
}

// Starts a remote SSH server to allow the user to connect to the codespace via SSH
func (g *Invoker) StartSSHServer(ctx context.Context) (int, string, error) {
	return g.session.StartSSHServer(ctx)
}

// Starts a remote SSH server to allow the user to connect to the codespace via SSH
func (g *Invoker) StartSSHServerWithOptions(ctx context.Context, options liveshare.StartSSHServerOptions) (int, string, error) {
	return g.session.StartSSHServerWithOptions(ctx, options)
}
