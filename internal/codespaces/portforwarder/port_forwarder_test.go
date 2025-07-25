package portforwarder

import (
	"context"
	"testing"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/internal/codespaces/connection"
	"github.com/microsoft/dev-tunnels/go/tunnels"
)

func TestNewPortForwarder(t *testing.T) {
	ctx := context.Background()

	// Create a mock codespace
	codespace := &api.Codespace{
		Connection: api.CodespaceConnection{
			TunnelProperties: api.TunnelProperties{
				ConnectAccessToken:     "connect-token",
				ManagePortsAccessToken: "manage-ports-token",
				ServiceUri:             "http://global.rel.tunnels.api.visualstudio.com/",
				TunnelId:               "tunnel-id",
				ClusterId:              "usw2",
				Domain:                 "domain.com",
			},
		},
		RuntimeConstraints: api.RuntimeConstraints{
			AllowedPortPrivacySettings: []string{"public", "private"},
		},
	}

	// Create the mock HTTP client
	httpClient, err := connection.NewMockHttpClient()
	if err != nil {
		t.Fatalf("NewHttpClient returned an error: %v", err)
	}

	// Call the function being tested
	conn, err := connection.NewCodespaceConnection(ctx, codespace, httpClient)
	if err != nil {
		t.Fatalf("NewCodespaceConnection returned an error: %v", err)
	}

	// Create the new port forwarder
	portForwarder, err := NewPortForwarder(ctx, conn)
	if err != nil {
		t.Fatalf("NewPortForwarder returned an error: %v", err)
	}

	// Check that the port forwarder was created successfully
	if portForwarder == nil {
		t.Fatal("NewPortForwarder returned nil")
	}
}

func TestAccessControlEntriesToVisibility(t *testing.T) {
	publicAccessControlEntry := []tunnels.TunnelAccessControlEntry{{
		Type: tunnels.TunnelAccessControlEntryTypeAnonymous,
	}}
	orgAccessControlEntry := []tunnels.TunnelAccessControlEntry{{
		Provider: string(tunnels.TunnelAuthenticationSchemeGitHub),
	}}
	privateAccessControlEntry := []tunnels.TunnelAccessControlEntry{}
	orgIsDenyAccessControlEntry := []tunnels.TunnelAccessControlEntry{{
		Provider: string(tunnels.TunnelAuthenticationSchemeGitHub),
		IsDeny:   true,
	}}

	tests := []struct {
		name                 string
		accessControlEntries []tunnels.TunnelAccessControlEntry
		expected             string
	}{
		{
			name:                 "public",
			accessControlEntries: publicAccessControlEntry,
			expected:             PublicPortVisibility,
		},
		{
			name:                 "org",
			accessControlEntries: orgAccessControlEntry,
			expected:             OrgPortVisibility,
		},
		{
			name:                 "private",
			accessControlEntries: privateAccessControlEntry,
			expected:             PrivatePortVisibility,
		},
		{
			name:                 "orgIsDeny",
			accessControlEntries: orgIsDenyAccessControlEntry,
			expected:             PrivatePortVisibility,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			visibility := AccessControlEntriesToVisibility(test.accessControlEntries)
			if visibility != test.expected {
				t.Errorf("expected %q, got %q", test.expected, visibility)
			}
		})
	}
}

func TestIsInternalPort(t *testing.T) {
	internalPort := &tunnels.TunnelPort{
		Labels: []string{"InternalPort"},
	}
	userForwardedPort := &tunnels.TunnelPort{
		Labels: []string{"UserForwardedPort"},
	}

	tests := []struct {
		name     string
		port     *tunnels.TunnelPort
		expected bool
	}{
		{
			name:     "internal",
			port:     internalPort,
			expected: true,
		},
		{
			name:     "user-forwarded",
			port:     userForwardedPort,
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			isInternal := IsInternalPort(test.port)
			if isInternal != test.expected {
				t.Errorf("expected %v, got %v", test.expected, isInternal)
			}
		})
	}
}

func TestForwardPortDefaultsToHTTPProtocol(t *testing.T) {
	codespace := &api.Codespace{
		Name:  "codespace-name",
		State: api.CodespaceStateAvailable,
		Connection: api.CodespaceConnection{
			TunnelProperties: api.TunnelProperties{
				ConnectAccessToken:     "tunnel access-token",
				ManagePortsAccessToken: "manage-ports-token",
				ServiceUri:             "http://global.rel.tunnels.api.visualstudio.com/",
				TunnelId:               "tunnel-id",
				ClusterId:              "usw2",
				Domain:                 "domain.com",
			},
		},
		RuntimeConstraints: api.RuntimeConstraints{
			AllowedPortPrivacySettings: []string{"public", "private"},
		},
	}

	// Given there are no forwarded ports.
	tunnelPorts := map[int]tunnels.TunnelPort{}

	httpClient, err := connection.NewMockHttpClient(
		connection.WithSpecificPorts(tunnelPorts),
	)
	if err != nil {
		t.Fatalf("NewMockHttpClient returned an error: %v", err)
	}

	connection, err := connection.NewCodespaceConnection(t.Context(), codespace, httpClient)
	if err != nil {
		t.Fatalf("NewCodespaceConnection returned an error: %v", err)
	}

	fwd, err := NewPortForwarder(t.Context(), connection)
	if err != nil {
		t.Fatalf("NewPortForwarder returned an error: %v", err)
	}

	// When we forward a port without an existing one to use for a protocol, it should default to HTTP.
	if err := fwd.ForwardPort(t.Context(), ForwardPortOpts{
		Port: 1337,
	}); err != nil {
		t.Fatalf("ForwardPort returned an error: %v", err)
	}

	ports, err := fwd.ListPorts(t.Context())
	if err != nil {
		t.Fatalf("ListPorts returned an error: %v", err)
	}

	if len(ports) != 1 {
		t.Fatalf("expected 1 port, got %d", len(ports))
	}

	if ports[0].Protocol != string(tunnels.TunnelProtocolHttp) {
		t.Fatalf("expected port protocol to be http, got %s", ports[0].Protocol)
	}
}

func TestForwardPortRespectsProtocolOfExistingTunneledPorts(t *testing.T) {
	codespace := &api.Codespace{
		Name:  "codespace-name",
		State: api.CodespaceStateAvailable,
		Connection: api.CodespaceConnection{
			TunnelProperties: api.TunnelProperties{
				ConnectAccessToken:     "tunnel access-token",
				ManagePortsAccessToken: "manage-ports-token",
				ServiceUri:             "http://global.rel.tunnels.api.visualstudio.com/",
				TunnelId:               "tunnel-id",
				ClusterId:              "usw2",
				Domain:                 "domain.com",
			},
		},
		RuntimeConstraints: api.RuntimeConstraints{
			AllowedPortPrivacySettings: []string{"public", "private"},
		},
	}

	// Given we already have a port forwarded with an HTTPS protocol.
	tunnelPorts := map[int]tunnels.TunnelPort{
		1337: {
			Protocol: string(tunnels.TunnelProtocolHttps),
			AccessControl: &tunnels.TunnelAccessControl{
				Entries: []tunnels.TunnelAccessControlEntry{},
			},
		},
	}

	httpClient, err := connection.NewMockHttpClient(
		connection.WithSpecificPorts(tunnelPorts),
	)
	if err != nil {
		t.Fatalf("NewMockHttpClient returned an error: %v", err)
	}

	connection, err := connection.NewCodespaceConnection(t.Context(), codespace, httpClient)
	if err != nil {
		t.Fatalf("NewCodespaceConnection returned an error: %v", err)
	}

	fwd, err := NewPortForwarder(t.Context(), connection)
	if err != nil {
		t.Fatalf("NewPortForwarder returned an error: %v", err)
	}

	// When we forward a port, it would typically default to HTTP, to which the mock server would respond with a 400,
	// but it should respect the existing port's protocol and forward it as HTTPS.
	if err := fwd.ForwardPort(t.Context(), ForwardPortOpts{
		Port: 1337,
	}); err != nil {
		t.Fatalf("ForwardPort returned an error: %v", err)
	}

	ports, err := fwd.ListPorts(t.Context())
	if err != nil {
		t.Fatalf("ListPorts returned an error: %v", err)
	}

	if len(ports) != 1 {
		t.Fatalf("expected 1 port, got %d", len(ports))
	}

	if ports[0].Protocol != string(tunnels.TunnelProtocolHttps) {
		t.Fatalf("expected port protocol to be https, got %s", ports[0].Protocol)
	}
}
