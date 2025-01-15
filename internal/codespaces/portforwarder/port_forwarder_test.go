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
		Tags: []string{"InternalPort"},
	}
	userForwardedPort := &tunnels.TunnelPort{
		Tags: []string{"UserForwardedPort"},
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
