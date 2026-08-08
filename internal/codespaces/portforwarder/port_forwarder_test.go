package portforwarder

import (
	"context"
	"testing"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/internal/codespaces/connection"
	"github.com/microsoft/dev-tunnels/go/tunnels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
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
	require.NoError(t, err)

	// Call the function being tested
	conn, err := connection.NewCodespaceConnection(ctx, codespace, httpClient)
	require.NoError(t, err)

	// Create the new port forwarder
	portForwarder, err := NewPortForwarder(ctx, conn)
	require.NoError(t, err)
	require.NotNil(t, portForwarder)
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
			assert.Equal(t, test.expected, visibility)
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
			assert.Equal(t, test.expected, isInternal)
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
	require.NoError(t, err)

	connection, err := connection.NewCodespaceConnection(t.Context(), codespace, httpClient)
	require.NoError(t, err)

	fwd, err := NewPortForwarder(t.Context(), connection)
	require.NoError(t, err)

	// When we forward a port without an existing one to use for a protocol, it should default to HTTP.
	err = fwd.ForwardPort(t.Context(), ForwardPortOpts{
		Port: 1337,
	})
	require.NoError(t, err)

	ports, err := fwd.ListPorts(t.Context())
	require.NoError(t, err)
	require.Len(t, ports, 1)
	assert.Equal(t, string(tunnels.TunnelProtocolHttp), ports[0].Protocol)
}

func TestConcurrentForwardPortDoesNotRace(t *testing.T) {
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

	tunnelPorts := map[int]tunnels.TunnelPort{}

	httpClient, err := connection.NewMockHttpClient(
		connection.WithSpecificPorts(tunnelPorts),
	)
	require.NoError(t, err)

	conn, err := connection.NewCodespaceConnection(t.Context(), codespace, httpClient)
	require.NoError(t, err)

	// Forward multiple ports concurrently from the same connection,
	// mirroring what ForwardPorts does in ports.go.
	group, ctx := errgroup.WithContext(t.Context())
	for port := 3000; port < 3010; port++ {
		fwd, err := NewPortForwarder(ctx, conn)
		require.NoError(t, err)

		group.Go(func() error {
			return fwd.ForwardPort(ctx, ForwardPortOpts{
				Port: port,
			})
		})
	}

	require.NoError(t, group.Wait())
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
	require.NoError(t, err)

	connection, err := connection.NewCodespaceConnection(t.Context(), codespace, httpClient)
	require.NoError(t, err)

	fwd, err := NewPortForwarder(t.Context(), connection)
	require.NoError(t, err)

	// When we forward a port, it would typically default to HTTP, to which the mock server would respond with a 400,
	// but it should respect the existing port's protocol and forward it as HTTPS.
	err = fwd.ForwardPort(t.Context(), ForwardPortOpts{
		Port: 1337,
	})
	require.NoError(t, err)

	ports, err := fwd.ListPorts(t.Context())
	require.NoError(t, err)
	require.Len(t, ports, 1)
	assert.Equal(t, string(tunnels.TunnelProtocolHttps), ports[0].Protocol)
}
