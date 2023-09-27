package portforwarder

import (
	"context"
	"fmt"
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

type PortForwarder struct {
	connection connection.CodespaceConnection
}

// NewPortForwarder returns a new PortForwarder for the specified codespace.
func NewPortForwarder(ctx context.Context, codespaceConnection *connection.CodespaceConnection) (fwd *PortForwarder, err error) {
	return &PortForwarder{
		connection: *codespaceConnection,
	}, nil
}

// ForwardAndConnectToPort forwards a port and connects to it via a local TCP port.
func (fwd *PortForwarder) ForwardAndConnectToPort(ctx context.Context, remotePort uint16, listen *net.TCPListener, keepAlive bool, internal bool) error {
	return fwd.ForwardPort(ctx, remotePort, listen, keepAlive, true, internal, "")
}

// ForwardPort forwards a port and optionally connects to it via a local TCP port.
func (fwd *PortForwarder) ForwardPort(ctx context.Context, remotePort uint16, listen *net.TCPListener, keepAlive bool, connect bool, internal bool, visibility string) error {
	tunnelPort := tunnels.NewTunnelPort(remotePort, "", "", tunnels.TunnelProtocolHttp)

	// If no visibility is provided, Dev Tunnels will use the default (private)
	if visibility != "" {
		// Check if the requested visibility is allowed
		allowed := false
		for _, allowedVisibility := range fwd.connection.AllowedPortPrivacySettings {
			if allowedVisibility == visibility {
				allowed = true
				break
			}
		}

		// If the requested visibility is not allowed, return an error
		if !allowed {
			return fmt.Errorf("visibility %s is not allowed", visibility)
		}

		accessControlEntries := visibilityToAccessControlEntries(visibility)
		if len(accessControlEntries) > 0 {
			tunnelPort.AccessControl = &tunnels.TunnelAccessControl{
				Entries: accessControlEntries,
			}
		}
	}

	// Tag the port as internal or user forwarded so we know if it needs to be shown in the UI
	if internal {
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

		// Inform the host that we've forwarded the port locally
		err = fwd.connection.TunnelClient.RefreshPorts(ctx)
		if err != nil {
			done <- fmt.Errorf("refresh ports failed: %v", err)
			return
		}

		// If we don't want to connect to the port, exit early
		if !connect {
			done <- nil
			return
		}

		// Ensure the port is forwarded before connecting
		err = fwd.connection.TunnelClient.WaitForForwardedPort(ctx, remotePort)
		if err != nil {
			done <- fmt.Errorf("wait for forwarded port failed: %v", err)
			return
		}

		// Connect to the forwarded port via a local TCP port
		err = fwd.connection.TunnelClient.ConnectToForwardedPort(ctx, listen, remotePort)
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
		err = fwd.ForwardPort(ctx, uint16(remotePort), nil, false, false, false, visibility)
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
