package portforwarder

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/cli/cli/v2/internal/codespaces/connection"
	"github.com/microsoft/dev-tunnels/go/tunnels"
)

const (
	githubSubjectId      = "1"
	InternalPortTag      = "InternalPort"
	UserForwardedPortTag = "UserForwardedPort"
)

const (
	PrivatePortVisibility = "private"
	OrgPortVisibility     = "org"
	PublicPortVisibility  = "public"
)

const (
	trafficTypeInput  = "input"
	trafficTypeOutput = "output"
)

type ForwardPortOpts struct {
	Port       int
	Internal   bool
	KeepAlive  bool
	Visibility string
}

type CodespacesPortForwarder struct {
	connection      connection.CodespaceConnection
	keepAliveReason chan string
}

type PortForwarder interface {
	ForwardPortToListener(ctx context.Context, opts ForwardPortOpts, listener *net.TCPListener) error
	ForwardPort(ctx context.Context, opts ForwardPortOpts) error
	ConnectToForwardedPort(ctx context.Context, conn io.ReadWriteCloser, opts ForwardPortOpts) error
	ListPorts(ctx context.Context) ([]*tunnels.TunnelPort, error)
	UpdatePortVisibility(ctx context.Context, remotePort int, visibility string) error
	KeepAlive(reason string)
	GetKeepAliveReason() string
	Close() error
}

// NewPortForwarder returns a new PortForwarder for the specified codespace.
func NewPortForwarder(ctx context.Context, codespaceConnection *connection.CodespaceConnection) (fwd PortForwarder, err error) {
	return &CodespacesPortForwarder{
		connection:      *codespaceConnection,
		keepAliveReason: make(chan string, 1),
	}, nil
}

// ForwardPortToListener forwards the specified port to the given TCP listener.
func (fwd *CodespacesPortForwarder) ForwardPortToListener(ctx context.Context, opts ForwardPortOpts, listener *net.TCPListener) error {
	err := fwd.ForwardPort(ctx, opts)
	if err != nil {
		return fmt.Errorf("error forwarding port: %w", err)
	}

	done := make(chan error)
	go func() {
		// Convert the port number to a uint16
		port, err := convertIntToUint16(opts.Port)
		if err != nil {
			done <- fmt.Errorf("error converting port: %w", err)
			return
		}

		// Ensure the port is forwarded before connecting
		err = fwd.connection.TunnelClient.WaitForForwardedPort(ctx, port)
		if err != nil {
			done <- fmt.Errorf("wait for forwarded port failed: %v", err)
			return
		}

		// Connect to the forwarded port
		err = fwd.connectListenerToForwardedPort(ctx, opts, listener)
		if err != nil {
			done <- fmt.Errorf("connect to forwarded port failed: %v", err)
		}
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("error connecting to tunnel: %w", err)
		}
		return nil
	case <-ctx.Done():
		return nil
	}
}

// ForwardPort informs the host that we would like to forward the given port.
func (fwd *CodespacesPortForwarder) ForwardPort(ctx context.Context, opts ForwardPortOpts) error {
	// Convert the port number to a uint16
	port, err := convertIntToUint16(opts.Port)
	if err != nil {
		return fmt.Errorf("error converting port: %w", err)
	}

	tunnelPort := tunnels.NewTunnelPort(port, "", "", tunnels.TunnelProtocolHttp)

	// If no visibility is provided, Dev Tunnels will use the default (private)
	if opts.Visibility != "" {
		// Check if the requested visibility is allowed
		allowed := false
		for _, allowedVisibility := range fwd.connection.AllowedPortPrivacySettings {
			if allowedVisibility == opts.Visibility {
				allowed = true
				break
			}
		}

		// If the requested visibility is not allowed, return an error
		if !allowed {
			return fmt.Errorf("visibility %s is not allowed", opts.Visibility)
		}

		accessControlEntries := visibilityToAccessControlEntries(opts.Visibility)
		if len(accessControlEntries) > 0 {
			tunnelPort.AccessControl = &tunnels.TunnelAccessControl{
				Entries: accessControlEntries,
			}
		}
	}

	// Tag the port as internal or user forwarded so we know if it needs to be shown in the UI
	if opts.Internal {
		tunnelPort.Tags = []string{InternalPortTag}
	} else {
		tunnelPort.Tags = []string{UserForwardedPortTag}
	}

	// Create the tunnel port
	_, err = fwd.connection.TunnelManager.CreateTunnelPort(ctx, fwd.connection.Tunnel, tunnelPort, fwd.connection.Options)
	if err != nil && !strings.Contains(err.Error(), "409") {
		return fmt.Errorf("create tunnel port failed: %v", err)
	}

	// Connect to the tunnel
	err = fwd.connection.Connect(ctx)
	if err != nil {
		return fmt.Errorf("connect failed: %v", err)
	}

	// Inform the host that we've forwarded the port locally
	err = fwd.connection.TunnelClient.RefreshPorts(ctx)
	if err != nil {
		return fmt.Errorf("refresh ports failed: %v", err)
	}

	return nil
}

// connectListenerToForwardedPort connects to the forwarded port via a local TCP port.
func (fwd *CodespacesPortForwarder) connectListenerToForwardedPort(ctx context.Context, opts ForwardPortOpts, listener *net.TCPListener) (err error) {
	errc := make(chan error, 1)
	sendError := func(err error) {
		// Use non-blocking send, to avoid goroutines getting
		// stuck in case of concurrent or sequential errors.
		select {
		case errc <- err:
		default:
		}
	}
	go func() {
		for {
			conn, err := listener.AcceptTCP()
			if err != nil {
				sendError(err)
				return
			}

			// Connect to the forwarded port in a goroutine so we can accept new connections
			go func() {
				if err := fwd.ConnectToForwardedPort(ctx, conn, opts); err != nil {
					sendError(err)
				}
			}()
		}
	}()

	// Wait for an error or for the context to be cancelled
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		return ctx.Err() // canceled
	}
}

// ConnectToForwardedPort connects to the forwarded port via a given ReadWriteCloser.
// Optionally, it detects traffic over the connection and sends activity signals to the server to keep the codespace from shutting down.
func (fwd *CodespacesPortForwarder) ConnectToForwardedPort(ctx context.Context, conn io.ReadWriteCloser, opts ForwardPortOpts) error {
	// Create a traffic monitor to keep the session alive
	if opts.KeepAlive {
		conn = newTrafficMonitor(conn, fwd)
	}

	// Convert the port number to a uint16
	port, err := convertIntToUint16(opts.Port)
	if err != nil {
		return fmt.Errorf("error converting port: %w", err)
	}

	// Connect to the forwarded port
	err = fwd.connection.TunnelClient.ConnectToForwardedPort(ctx, conn, port)
	if err != nil {
		return fmt.Errorf("error connecting to forwarded port: %w", err)
	}

	return nil
}

// ListPorts fetches the list of ports that are currently forwarded.
func (fwd *CodespacesPortForwarder) ListPorts(ctx context.Context) (ports []*tunnels.TunnelPort, err error) {
	ports, err = fwd.connection.TunnelManager.ListTunnelPorts(ctx, fwd.connection.Tunnel, fwd.connection.Options)
	if err != nil {
		return nil, fmt.Errorf("error listing ports: %w", err)
	}

	return ports, nil
}

// UpdatePortVisibility changes the visibility (private, org, public) of the specified port.
func (fwd *CodespacesPortForwarder) UpdatePortVisibility(ctx context.Context, remotePort int, visibility string) error {
	tunnelPort, err := fwd.connection.TunnelManager.GetTunnelPort(ctx, fwd.connection.Tunnel, remotePort, fwd.connection.Options)
	if err != nil {
		return fmt.Errorf("error getting tunnel port: %w", err)
	}

	// If the port visibility isn't changing, don't do anything
	if AccessControlEntriesToVisibility(tunnelPort.AccessControl.Entries) == visibility {
		return nil
	}

	// Delete the existing tunnel port to update
	err = fwd.connection.TunnelManager.DeleteTunnelPort(ctx, fwd.connection.Tunnel, uint16(remotePort), fwd.connection.Options)
	if err != nil {
		return fmt.Errorf("error deleting tunnel port: %w", err)
	}

	done := make(chan error)
	go func() {
		// Connect to the tunnel
		err = fwd.connection.Connect(ctx)
		if err != nil {
			done <- fmt.Errorf("connect failed: %v", err)
			return
		}

		// Inform the host that we've deleted the port
		err = fwd.connection.TunnelClient.RefreshPorts(ctx)
		if err != nil {
			done <- fmt.Errorf("refresh ports failed: %v", err)
			return
		}

		// Re-forward the port with the updated visibility
		err = fwd.ForwardPort(ctx, ForwardPortOpts{Port: remotePort, Visibility: visibility})
		if err != nil {
			done <- fmt.Errorf("error forwarding port: %w", err)
			return
		}

		done <- nil
	}()

	// Wait for the done channel to be closed
	select {
	case err := <-done:
		if err != nil {
			// If we fail to re-forward the port, we need to forward again with the original visibility so the port is still accessible
			_ = fwd.ForwardPort(ctx, ForwardPortOpts{Port: remotePort, Visibility: AccessControlEntriesToVisibility(tunnelPort.AccessControl.Entries)})

			return fmt.Errorf("error connecting to tunnel: %w", err)
		}

		return nil
	case <-ctx.Done():
		return nil
	}
}

// KeepAlive accepts a reason that is retained if there is no active reason
// to send to the server.
func (fwd *CodespacesPortForwarder) KeepAlive(reason string) {
	select {
	case fwd.keepAliveReason <- reason:
	default:
		// there is already an active keep alive reason
		// so we can ignore this one
	}
}

// GetKeepAliveReason fetches the keep alive reason from the channel and returns it.
func (fwd *CodespacesPortForwarder) GetKeepAliveReason() string {
	return <-fwd.keepAliveReason
}

// Close closes the port forwarder's tunnel client connection.
func (fwd *CodespacesPortForwarder) Close() error {
	return fwd.connection.Close()
}

// AccessControlEntriesToVisibility converts the access control entries used by Dev Tunnels to a friendly visibility value.
func AccessControlEntriesToVisibility(accessControlEntries []tunnels.TunnelAccessControlEntry) string {
	for _, entry := range accessControlEntries {
		// If we have the anonymous type (and we're not denying it), it's public
		if (entry.Type == tunnels.TunnelAccessControlEntryTypeAnonymous) && (!entry.IsDeny) {
			return PublicPortVisibility
		}

		// If we have the organizations type (and we're not denying it), it's org
		if (entry.Provider == string(tunnels.TunnelAuthenticationSchemeGitHub)) && (!entry.IsDeny) {
			return OrgPortVisibility
		}
	}

	// Else, it's private
	return PrivatePortVisibility
}

// visibilityToAccessControlEntries converts the given visibility to access control entries that can be used by Dev Tunnels.
func visibilityToAccessControlEntries(visibility string) []tunnels.TunnelAccessControlEntry {
	switch visibility {
	case PublicPortVisibility:
		return []tunnels.TunnelAccessControlEntry{{
			Type:     tunnels.TunnelAccessControlEntryTypeAnonymous,
			Subjects: []string{},
			Scopes:   []string{string(tunnels.TunnelAccessScopeConnect)},
		}}
	case OrgPortVisibility:
		return []tunnels.TunnelAccessControlEntry{{
			Type:     tunnels.TunnelAccessControlEntryTypeOrganizations,
			Subjects: []string{githubSubjectId},
			Scopes: []string{
				string(tunnels.TunnelAccessScopeConnect),
			},
			Provider: string(tunnels.TunnelAuthenticationSchemeGitHub),
		}}
	default:
		// The tunnel manager doesn't accept empty access control entries, so we need to return a deny entry
		return []tunnels.TunnelAccessControlEntry{{
			Type:     tunnels.TunnelAccessControlEntryTypeOrganizations,
			Subjects: []string{githubSubjectId},
			Scopes:   []string{},
			IsDeny:   true,
		}}
	}
}

// IsInternalPort returns true if the port is internal.
func IsInternalPort(port *tunnels.TunnelPort) bool {
	for _, tag := range port.Tags {
		if strings.EqualFold(tag, InternalPortTag) {
			return true
		}
	}

	return false
}

// convertIntToUint16 converts the given int to a uint16.
func convertIntToUint16(port int) (uint16, error) {
	var updatedPort uint16
	if port >= 0 && port <= 65535 {
		updatedPort = uint16(port)
	} else {
		return 0, fmt.Errorf("invalid port number: %d", port)
	}

	return updatedPort, nil
}

// trafficMonitor implements io.Reader. It keeps the session alive by notifying
// it of the traffic type during Read operations.
type trafficMonitor struct {
	rwc io.ReadWriteCloser
	fwd PortForwarder
}

// newTrafficMonitor returns a trafficMonitor for the specified codespace connection.
// It wraps the provided io.ReaderWriteCloser with its own Read/Write/Close methods.
func newTrafficMonitor(rwc io.ReadWriteCloser, fwd PortForwarder) *trafficMonitor {
	return &trafficMonitor{rwc, fwd}
}

// Read wraps the underlying ReadWriteCloser's Read method and keeps the session alive with the "input" traffic type.
func (t *trafficMonitor) Read(p []byte) (n int, err error) {
	t.fwd.KeepAlive(trafficTypeInput)
	return t.rwc.Read(p)
}

// Write wraps the underlying ReadWriteCloser's Write method and keeps the session alive with the "output" traffic type.
func (t *trafficMonitor) Write(p []byte) (n int, err error) {
	t.fwd.KeepAlive(trafficTypeOutput)
	return t.rwc.Write(p)
}

// Close closes the underlying ReadWriteCloser.
func (t *trafficMonitor) Close() error {
	return t.rwc.Close()
}
