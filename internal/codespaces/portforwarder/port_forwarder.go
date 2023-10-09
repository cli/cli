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
	Connect    bool
	Internal   bool
	KeepAlive  bool
	Visibility string
}

type PortForwarder struct {
	connection connection.CodespaceConnection
}

// NewPortForwarder returns a new PortForwarder for the specified codespace.
func NewPortForwarder(ctx context.Context, codespaceConnection *connection.CodespaceConnection) (fwd *PortForwarder, err error) {
	return &PortForwarder{
		connection: *codespaceConnection,
	}, nil
}

// ForwardPortToListener forwards the specified port to the given TCP listener.
func (fwd *PortForwarder) ForwardPortToListener(ctx context.Context, opts ForwardPortOpts, listener *net.TCPListener) error {
	return fwd.ForwardPort(ctx, opts, listener, nil)
}

// ForwardPort forwards the specified port to the given TCP listener or ReadWriteCloser.
func (fwd *PortForwarder) ForwardPort(ctx context.Context, opts ForwardPortOpts, listener *net.TCPListener, conn io.ReadWriteCloser) error {
	// Ensure that the port number is valid before casting it to a uint16
	var portNumber uint16
	if opts.Port >= 0 && opts.Port <= 65535 {
		portNumber = uint16(opts.Port)
	} else {
		return fmt.Errorf("invalid port number: %d", opts.Port)
	}

	tunnelPort := tunnels.NewTunnelPort(portNumber, "", "", tunnels.TunnelProtocolHttps)

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
	_, err := fwd.connection.TunnelManager.CreateTunnelPort(ctx, fwd.connection.Tunnel, tunnelPort, fwd.connection.Options)
	if err != nil && !strings.Contains(err.Error(), "409") {
		return fmt.Errorf("create tunnel port failed: %v", err)
	}

	done := make(chan error)
	go func() {
		// Connect to the tunnel
		err = fwd.connection.TunnelClient.Connect(ctx, "")
		if err != nil {
			done <- fmt.Errorf("connect failed: %v", err)
			return
		}

		// Close the tunnel client when we're done
		defer fwd.connection.TunnelClient.Close()

		// Inform the host that we've forwarded the port locally
		err = fwd.connection.TunnelClient.RefreshPorts(ctx)
		if err != nil {
			done <- fmt.Errorf("refresh ports failed: %v", err)
			return
		}

		// If we don't want to connect to the port, exit early
		if !opts.Connect {
			done <- nil
			return
		}

		// Ensure the port is forwarded before connecting
		err = fwd.connection.TunnelClient.WaitForForwardedPort(ctx, portNumber)
		if err != nil {
			done <- fmt.Errorf("wait for forwarded port failed: %v", err)
			return
		}

		// Connect to the forwarded port
		err = fwd.connectToForwardedPort(ctx, portNumber, opts.KeepAlive, listener, conn)
		if err != nil {
			done <- fmt.Errorf("connect to forwarded port failed: %v", err)
			return
		}

		done <- nil
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

// connectToForwardedPort connects to the forwarded port via a local TCP port or a given ReadWriteCloser.
// Optionally, it detects traffic over the connection and sends activity signals to the server to keep the codespace from shutting down.
func (fwd *PortForwarder) connectToForwardedPort(ctx context.Context, portNumber uint16, keepAlive bool, listener *net.TCPListener, conn io.ReadWriteCloser) (err error) {
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
			// Make a copy of the connection so we can accept new connections before this one closes
			connCopy := conn

			// If a listener is provided, accept a connection from it
			if listener != nil {
				connCopy, err = listener.AcceptTCP()
				if err != nil {
					sendError(err)
					return
				}
			}

			// Create a traffic monitor to keep the session alive
			if keepAlive {
				connCopy = newTrafficMonitor(connCopy, fwd.connection)
			}

			// Connect to the forwarded port in a goroutine so we can accept new connections
			go func() {
				if err := fwd.connection.TunnelClient.ConnectToForwardedPort(ctx, connCopy, portNumber); err != nil {
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

// ListPorts fetches the list of ports that are currently forwarded.
func (fwd *PortForwarder) ListPorts(ctx context.Context) (ports []*tunnels.TunnelPort, err error) {
	ports, err = fwd.connection.TunnelManager.ListTunnelPorts(ctx, fwd.connection.Tunnel, fwd.connection.Options)
	if err != nil {
		return nil, fmt.Errorf("error listing ports: %w", err)
	}

	return ports, nil
}

// UpdatePortVisibility changes the visibility (private, org, public) of the specified port.
func (fwd *PortForwarder) UpdatePortVisibility(ctx context.Context, remotePort int, visibility string) error {
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
		err = fwd.connection.TunnelClient.Connect(ctx, "")
		if err != nil {
			done <- fmt.Errorf("connect failed: %v", err)
			return
		}

		// Close the tunnel client when we're done
		defer fwd.connection.TunnelClient.Close()

		// Inform the host that we've deleted the port
		err = fwd.connection.TunnelClient.RefreshPorts(ctx)
		if err != nil {
			done <- fmt.Errorf("refresh ports failed: %v", err)
			return
		}

		done <- nil
	}()

	// Wait for the done channel to be closed
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("error connecting to tunnel: %w", err)
		}

		// Re-forward the port with the updated visibility
		err = fwd.ForwardPort(ctx, ForwardPortOpts{Port: remotePort, Visibility: visibility}, nil, nil)
		if err != nil {
			return fmt.Errorf("error forwarding port: %w", err)
		}

		return nil
	case <-ctx.Done():
		return nil
	}
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

// trafficMonitor implements io.Reader. It keeps the session alive by notifying
// it of the traffic type during Read operations.
type trafficMonitor struct {
	rwc        io.ReadWriteCloser
	connection connection.CodespaceConnection
}

// newTrafficMonitor returns a trafficMonitor for the specified codespace connection.
// It wraps the provided io.ReaderWriteCloser with its own Read/Write/Close methods.
func newTrafficMonitor(rwc io.ReadWriteCloser, connection connection.CodespaceConnection) *trafficMonitor {
	return &trafficMonitor{rwc, connection}
}

// Read wraps the underlying ReadWriteCloser's Read method and keeps the session alive with the "input" traffic type.
func (t *trafficMonitor) Read(p []byte) (n int, err error) {
	t.connection.KeepAlive(trafficTypeInput)
	return t.rwc.Read(p)
}

// Write wraps the underlying ReadWriteCloser's Write method and keeps the session alive with the "output" traffic type.
func (t *trafficMonitor) Write(p []byte) (n int, err error) {
	t.connection.KeepAlive(trafficTypeOutput)
	return t.rwc.Write(p)
}

// Close closes the underlying ReadWriteCloser.
func (t *trafficMonitor) Close() error {
	return t.rwc.Close()
}
