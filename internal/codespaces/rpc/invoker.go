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

type Invoker interface {
	Close() error
	StartJupyterServer(ctx context.Context) (int, string, error)
	RebuildContainer(ctx context.Context, full bool) error
	StartSSHServer(ctx context.Context) (int, string, error)
	StartSSHServerWithOptions(ctx context.Context, options liveshare.StartSSHServerOptions) (int, string, error)
}

type invoker struct {
	conn          *grpc.ClientConn
	token         string
	session       liveshare.LiveshareSession
	listener      net.Listener
	jupyterClient jupyter.JupyterServerHostClient
	cancelPF      context.CancelFunc
}

// Connects to the internal RPC server and returns a new invoker for it
func CreateInvoker(ctx context.Context, session liveshare.LiveshareSession, token string) (Invoker, error) {
	ctx, cancel := context.WithTimeout(ctx, ConnectionTimeout)
	defer cancel()

	invoker, err := connect(ctx, session, token)
	if err != nil {
		return nil, fmt.Errorf("error connecting to internal server: %w", err)
	}

	return invoker, nil
}

// Finds a free port to listen on and creates a new RPC invoker that connects to that port
func connect(ctx context.Context, session liveshare.LiveshareSession, token string) (Invoker, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", 0))
	if err != nil {
		return nil, fmt.Errorf("failed to listen to local port over tcp: %w", err)
	}
	localAddress := fmt.Sprintf("127.0.0.1:%d", listener.Addr().(*net.TCPAddr).Port)

	invoker := &invoker{
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
func (i *invoker) Close() error {
	i.cancelPF()

	// Closing the local listener effectively closes the gRPC connection
	if err := i.listener.Close(); err != nil {
		i.conn.Close() // If we fail to close the listener, explicitly close the gRPC connection and ignore any error
		return fmt.Errorf("failed to close local tcp port listener: %w", err)
	}

	return nil
}

// Appends the authentication token to the gRPC context
func (i *invoker) appendMetadata(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "Authorization", "Bearer "+i.token)
}

// Starts a remote JupyterLab server to allow the user to connect to the codespace via JupyterLab in their browser
func (i *invoker) StartJupyterServer(ctx context.Context) (port int, serverUrl string, err error) {
	ctx = i.appendMetadata(ctx)
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	response, err := i.jupyterClient.GetRunningServer(ctx, &jupyter.GetRunningServerRequest{})
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
func (i *invoker) RebuildContainer(ctx context.Context, full bool) error {
	return i.session.RebuildContainer(ctx, full)
}

// Starts a remote SSH server to allow the user to connect to the codespace via SSH
func (i *invoker) StartSSHServer(ctx context.Context) (int, string, error) {
	return i.session.StartSSHServer(ctx)
}

// Starts a remote SSH server to allow the user to connect to the codespace via SSH
func (i *invoker) StartSSHServerWithOptions(ctx context.Context, options liveshare.StartSSHServerOptions) (int, string, error) {
	return i.session.StartSSHServerWithOptions(ctx, options)
}
