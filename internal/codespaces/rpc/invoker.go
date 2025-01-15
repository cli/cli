package rpc

// gRPC client implementation to be able to connect to the gRPC server and perform the following operations:
// - Start a remote JupyterLab server

import (
	"context"
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/codespaces/portforwarder"
	"github.com/cli/cli/v2/internal/codespaces/rpc/codespace"
	"github.com/cli/cli/v2/internal/codespaces/rpc/jupyter"
	"github.com/cli/cli/v2/internal/codespaces/rpc/ssh"
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
	clientName                    = "gh"
	connectedEventName            = "connected"
	keepAliveEventName            = "keepAlive"
)

type StartSSHServerOptions struct {
	UserPublicKeyFile string
}

type Invoker interface {
	Close() error
	StartJupyterServer(ctx context.Context) (int, string, error)
	RebuildContainer(ctx context.Context, full bool) error
	StartSSHServer(ctx context.Context) (int, string, error)
	StartSSHServerWithOptions(ctx context.Context, options StartSSHServerOptions) (int, string, error)
	KeepAlive()
}

type invoker struct {
	conn              *grpc.ClientConn
	fwd               portforwarder.PortForwarder
	listener          net.Listener
	jupyterClient     jupyter.JupyterServerHostClient
	codespaceClient   codespace.CodespaceHostClient
	sshClient         ssh.SshServerHostClient
	cancelPF          context.CancelFunc
	keepAliveOverride bool
}

// Connects to the internal RPC server and returns a new invoker for it
func CreateInvoker(ctx context.Context, fwd portforwarder.PortForwarder) (Invoker, error) {
	ctx, cancel := context.WithTimeout(ctx, ConnectionTimeout)
	defer cancel()

	invoker, err := connect(ctx, fwd)
	if err != nil {
		return nil, fmt.Errorf("error connecting to internal server: %w", err)
	}

	return invoker, nil
}

// Finds a free port to listen on and creates a new RPC invoker that connects to that port
func connect(ctx context.Context, fwd portforwarder.PortForwarder) (Invoker, error) {
	listener, err := listenTCP()
	if err != nil {
		return nil, err
	}
	localAddress := listener.Addr().String()

	invoker := &invoker{
		fwd:      fwd,
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
		// Start forwarding the port locally
		opts := portforwarder.ForwardPortOpts{
			Port:     codespacesInternalPort,
			Internal: true,
		}
		ch <- fwd.ForwardPortToListener(pfctx, opts, listener)
	}()

	var conn *grpc.ClientConn
	go func() {
		// Attempt to connect to the port
		opts := []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		}
		conn, err = grpc.NewClient(localAddress, opts...)
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
	invoker.codespaceClient = codespace.NewCodespaceHostClient(conn)
	invoker.sshClient = ssh.NewSshServerHostClient(conn)

	// Send initial connection heartbeat (no need to throw if we fail to get a response from the server)
	_ = invoker.notifyCodespaceOfClientActivity(ctx, connectedEventName)

	// Start the activity heatbeats
	go invoker.heartbeat(pfctx, 1*time.Minute)

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
	return metadata.AppendToOutgoingContext(ctx, "Authorization", "Bearer token")
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
	ctx = i.appendMetadata(ctx)
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	// If full is true, we want to pass false to the RPC call to indicate that we want to do a full rebuild
	incremental := !full
	response, err := i.codespaceClient.RebuildContainerAsync(ctx, &codespace.RebuildContainerRequest{Incremental: &incremental})
	if err != nil {
		return fmt.Errorf("failed to invoke rebuild RPC: %w", err)
	}

	if !response.RebuildContainer {
		return fmt.Errorf("couldn't rebuild codespace")
	}

	return nil
}

// Starts a remote SSH server to allow the user to connect to the codespace via SSH
func (i *invoker) StartSSHServer(ctx context.Context) (int, string, error) {
	return i.StartSSHServerWithOptions(ctx, StartSSHServerOptions{})
}

// Starts a remote SSH server to allow the user to connect to the codespace via SSH
func (i *invoker) StartSSHServerWithOptions(ctx context.Context, options StartSSHServerOptions) (int, string, error) {
	ctx = i.appendMetadata(ctx)
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	userPublicKey := ""
	if options.UserPublicKeyFile != "" {
		publicKeyBytes, err := os.ReadFile(options.UserPublicKeyFile)
		if err != nil {
			return 0, "", fmt.Errorf("failed to read public key file: %w", err)
		}

		userPublicKey = strings.TrimSpace(string(publicKeyBytes))
	}

	response, err := i.sshClient.StartRemoteServerAsync(ctx, &ssh.StartRemoteServerRequest{UserPublicKey: userPublicKey})
	if err != nil {
		return 0, "", fmt.Errorf("failed to invoke SSH RPC: %w", err)
	}

	if !response.Result {
		return 0, "", fmt.Errorf("failed to start SSH server: %s", response.Message)
	}

	port, err := strconv.Atoi(response.ServerPort)
	if err != nil {
		return 0, "", fmt.Errorf("failed to parse SSH server port: %w", err)
	}

	if !isUsernameValid(response.User) {
		return 0, "", fmt.Errorf("invalid username: %s", response.User)
	}
	return port, response.User, nil
}

func listenTCP() (*net.TCPListener, error) {
	// We will end up using this same address to connect, so specify the IP also or the connect will fail
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to build tcp address: %w", err)
	}
	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen to local port over tcp: %w", err)
	}

	return listener, nil
}

// KeepAlive sets a flag to continuously send activity signals to
// the codespace even if there is no other activity (e.g. stdio)
func (i *invoker) KeepAlive() {
	i.keepAliveOverride = true
}

// Periodically check whether there is a reason to keep the connection alive, and if so, notify the codespace to do so
func (i *invoker) heartbeat(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reason := ""

			// If the keep alive override flag is set, we don't need to check for activity on the forwarder
			// Otherwise, grab the reason from the forwarder
			if i.keepAliveOverride {
				reason = keepAliveEventName
			} else {
				reason = i.fwd.GetKeepAliveReason()
			}
			_ = i.notifyCodespaceOfClientActivity(ctx, reason)
		}
	}
}

func (i *invoker) notifyCodespaceOfClientActivity(ctx context.Context, activity string) error {
	ctx = i.appendMetadata(ctx)
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	_, err := i.codespaceClient.NotifyCodespaceOfClientActivity(ctx, &codespace.NotifyCodespaceOfClientActivityRequest{ClientId: clientName, ClientActivities: []string{activity}})
	if err != nil {
		return fmt.Errorf("failed to invoke notify RPC: %w", err)
	}

	return nil
}

func isUsernameValid(username string) bool {
	// assuming valid usernames are alphanumeric, with these special characters allowed: . _ -
	var validUsernamePattern = `^[a-zA-Z0-9_][-.a-zA-Z0-9_]*$`
	re := regexp.MustCompile(validUsernamePattern)
	return re.MatchString(username)
}
