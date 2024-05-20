package test

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/cli/cli/v2/internal/codespaces/portforwarder"
	"github.com/microsoft/dev-tunnels/go/tunnels"
)

type PortForwarder struct{}

// Close implements portforwarder.PortForwarder.
func (PortForwarder) Close() error {
	return nil
}

// ConnectToForwardedPort implements portforwarder.PortForwarder.
func (PortForwarder) ConnectToForwardedPort(ctx context.Context, conn io.ReadWriteCloser, opts portforwarder.ForwardPortOpts) error {
	panic("unimplemented")
}

// ForwardPort implements portforwarder.PortForwarder.
func (PortForwarder) ForwardPort(ctx context.Context, opts portforwarder.ForwardPortOpts) error {
	panic("unimplemented")
}

// GetKeepAliveReason implements portforwarder.PortForwarder.
func (PortForwarder) GetKeepAliveReason() string {
	panic("unimplemented")
}

// KeepAlive implements portforwarder.PortForwarder.
func (PortForwarder) KeepAlive(reason string) {
	panic("unimplemented")
}

// ForwardPortToListener implements portforwarder.PortForwarder.
func (PortForwarder) ForwardPortToListener(ctx context.Context, opts portforwarder.ForwardPortOpts, listener *net.TCPListener) error {
	// Start forwarding the port locally
	hostConn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", opts.Port))
	if err != nil {
		return err
	}

	// Accept the connection from the listener
	listenerConn, err := listener.Accept()
	if err != nil {
		return err
	}

	// Copy data between the two connections
	go func() {
		_, _ = io.Copy(hostConn, listenerConn)
		hostConn.Close()
	}()
	go func() {
		_, _ = io.Copy(listenerConn, hostConn)
		listenerConn.Close()
	}()

	// ForwardPortToListener typically blocks until the context is cancelled so we need to do the same
	<-ctx.Done()

	return nil
}

// ListPorts implements portforwarder.PortForwarder.
func (PortForwarder) ListPorts(ctx context.Context) ([]*tunnels.TunnelPort, error) {
	panic("unimplemented")
}

// UpdatePortVisibility implements portforwarder.PortForwarder.
func (PortForwarder) UpdatePortVisibility(ctx context.Context, remotePort int, visibility string) error {
	panic("unimplemented")
}
