package grpctunnel

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/cli/cli/v2/internal/codespaces"
	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/internal/codespaces/connection"
	"github.com/cli/cli/v2/internal/codespaces/portforwarder"
	"github.com/cli/cli/v2/internal/codespaces/rpc"
)

type client interface {
	GetCodespace(ctx context.Context, name string, includeConnection bool) (*api.Codespace, error)
	StartCodespace(ctx context.Context, name string) error
	HTTPClient() (*http.Client, error)
}

// withProgresss declares an abstract interface for any struct that can provide a
// progress indicator.
type withProgresss interface {
	StartProgressIndicatorWithLabel(s string)
	StopProgressIndicator()
}

// invoker is an interface that allows us to keep a nilInvoker in case the
// tunnel creation is unsuccessful, which simplifies things like closing it,
// etc. without having to deal with the invoker being nil.
type invoker interface {
	Close() error
	User() string
	Destination(profile string) string
	ForwardOpts() portforwarder.ForwardPortOpts
	KeepAlive()
}

type nilInvoker struct{}

func (n *nilInvoker) Close() error {
	return nil
}
func (n *nilInvoker) User() string {
	return ""
}
func (n *nilInvoker) Destination(profile string) string {
	return ""
}
func (n *nilInvoker) ForwardOpts() portforwarder.ForwardPortOpts {
	return portforwarder.ForwardPortOpts{}
}
func (n *nilInvoker) KeepAlive() {}

// Tunnel abstracts the process of connecting to a codespace to create a
// tunnel on it through gRPC in order to run commands on it, like ssh or scp.
//
// This is done through 3 different structs that collaborate:
// - The connection to the codespace itself.
// - The port forwarder that we use to make grpc calls to the codespace through that connection.
// - The invoker that makes the actual grpc calls in order to run commands.
//
// In other words, this is a wrapper around the invoker to simplify its setup.
type Tunnel struct {
	fwd    portforwarder.PortForwarder
	conn   *connection.CodespaceConnection
	app    withProgresss
	port   int
	closed chan error
	invoker
}

func New(ctx context.Context, app withProgresss, client client, codespace *api.Codespace) (*Tunnel, error) {
	conn, err := codespaces.GetCodespaceConnection(ctx, app, client, codespace)
	if err != nil {
		return nil, fmt.Errorf("error connecting to codespace: %w", err)
	}

	fwd, err := portforwarder.NewPortForwarder(ctx, conn)
	if err != nil {
		return nil, fmt.Errorf("failed to create port forwarder: %w", err)
	}

	return &Tunnel{
		fwd:     fwd,
		conn:    conn,
		app:     app,
		invoker: &nilInvoker{},
		closed:  make(chan error, 1),
		port:    0,
	}, nil
}

// This allows us to mock rpc.CreateInvoker in tests.
var createInvoker = rpc.CreateInvoker

func (t *Tunnel) ConnectWithOptions(ctx context.Context, opts rpc.StartSSHServerOptions) error {
	invoker, err := createInvoker(ctx, t.fwd)
	if err != nil {
		return fmt.Errorf("error connecting to codespace: %w", err)
	}

	port, user, err := invoker.StartSSHServerWithOptions(ctx, opts)
	if err != nil {
		return fmt.Errorf("error getting ssh server details: %w", err)
	}

	t.invoker = &sshInvoker{port: port, user: user, Invoker: invoker}
	return nil
}

func (t *Tunnel) Connect(ctx context.Context) error {
	return t.ConnectWithOptions(ctx, rpc.StartSSHServerOptions{})
}

func (t *Tunnel) ProxyStdio(ctx context.Context) error {
	stdio := &rwio{os.Stdin, os.Stdout}

	// Forward the port
	if err := t.fwd.ForwardPort(ctx, t.ForwardOpts()); err != nil {
		return fmt.Errorf("failed to forward port: %w", err)
	}

	// Connect to the forwarded port
	if err := t.fwd.ConnectToForwardedPort(ctx, stdio, t.ForwardOpts()); err != nil {
		return fmt.Errorf("failed to connect to forwarded port: %w", err)
	}

	return nil
}

func (t *Tunnel) Forward(ctx context.Context, port int) error {
	// Ensure local port is listening before client (Shell) connects.  Unless the
	// user specifies a server port,  it will 0 and thus the client will pick a
	// random port.
	listen, port, err := codespaces.ListenTCP(port, false)
	if err != nil {
		return err
	}

	// We need to keep the port for 2 reasons:
	// 1. If the user didn't select one, this will be random.
	// 2. We need to give this to the invoker later on on `Run`
	t.port = port

	go func() {
		// Once the forwarder has finished we don't need the listener anymore, so we
		// close it.
		defer listen.Close()
		t.closed <- t.fwd.ForwardPortToListener(ctx, t.ForwardOpts(), listen)
	}()

	return nil
}

func (t *Tunnel) Run(profile string, cmd func(string, int) error) error {
	closed := make(chan error, 1)
	go func() {
		closed <- cmd(t.Destination(profile), t.port)
	}()

	select {
	case err := <-t.closed:
		return fmt.Errorf("tunnel closed: %w", err)
	case err := <-closed:
		if err != nil {
			return fmt.Errorf("shell closed: %w", err)
		}
		return nil // success
	}
}

func (t *Tunnel) RunWithKeepAlive(profile string, cmd func(string, int) error) error {
	// We need to keep the invoker alive while the command is running.
	t.KeepAlive()
	return t.Run(profile, cmd)
}

func (s *Tunnel) Close() error {
	if err := s.invoker.Close(); err != nil {
		return fmt.Errorf("error closing invoker: %w", err)
	}
	if err := s.fwd.Close(); err != nil {
		return fmt.Errorf("error closing port forwarder: %w", err)
	}
	if err := s.conn.Close(); err != nil {
		return fmt.Errorf("error closing codespace connection: %w", err)
	}
	return nil
}

// sshInvoker wraps the invoker itself with the results of using it to start an
// SSH server. That way the data that is needed for it to work is kept together
// and easier to use.
type sshInvoker struct {
	port int
	user string
	rpc.Invoker
}

func (c *sshInvoker) Destination(profile string) string {
	if profile != "" {
		return profile
	}

	return fmt.Sprintf("%s@localhost", c.user)
}

func (c *sshInvoker) ForwardOpts() portforwarder.ForwardPortOpts {
	return portforwarder.ForwardPortOpts{
		Port:      c.port,
		Internal:  true,
		KeepAlive: true,
	}
}

func (c *sshInvoker) User() string {
	return c.user
}
