package connection

import (
	"context"
	"reflect"
	"testing"

	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/microsoft/dev-tunnels/go/tunnels"
)

func TestNewCodespaceConnection(t *testing.T) {
	ctx := context.Background()

	// Create a mock codespace
	connection := api.CodespaceConnection{
		TunnelProperties: api.TunnelProperties{
			ConnectAccessToken:     "connect-token",
			ManagePortsAccessToken: "manage-ports-token",
			ServiceUri:             "http://global.rel.tunnels.api.visualstudio.com/",
			TunnelId:               "tunnel-id",
			ClusterId:              "usw2",
			Domain:                 "domain.com",
		},
	}
	allowedPortPrivacySettings := []string{"public", "private"}
	codespace := &api.Codespace{
		Connection:         connection,
		RuntimeConstraints: api.RuntimeConstraints{AllowedPortPrivacySettings: allowedPortPrivacySettings},
	}

	// Create the mock HTTP client
	httpClient, err := NewMockHttpClient()
	if err != nil {
		t.Fatalf("NewHttpClient returned an error: %v", err)
	}

	// Create the connection
	conn, err := NewCodespaceConnection(ctx, codespace, httpClient)
	if err != nil {
		t.Fatalf("NewCodespaceConnection returned an error: %v", err)
	}

	// Verify closing before connected doesn't throw
	err = conn.Close()
	if err != nil {
		t.Fatalf("Close returned an error: %v", err)
	}

	// Check that the connection was created successfully
	if conn == nil {
		t.Fatal("NewCodespaceConnection returned nil")
	}

	// Verify that the connection contains the expected tunnel properties
	if conn.tunnelProperties != connection.TunnelProperties {
		t.Fatalf("NewCodespaceConnection returned a connection with unexpected tunnel properties: %+v", conn.tunnelProperties)
	}

	// Verify that the connection contains the expected tunnel
	expectedTunnel := &tunnels.Tunnel{
		AccessTokens: map[tunnels.TunnelAccessScope]string{tunnels.TunnelAccessScopeConnect: connection.TunnelProperties.ConnectAccessToken, tunnels.TunnelAccessScopeManagePorts: connection.TunnelProperties.ManagePortsAccessToken},
		TunnelID:     connection.TunnelProperties.TunnelId,
		ClusterID:    connection.TunnelProperties.ClusterId,
		Domain:       connection.TunnelProperties.Domain,
	}
	if !reflect.DeepEqual(conn.Tunnel, expectedTunnel) {
		t.Fatalf("NewCodespaceConnection returned a connection with unexpected tunnel: %+v", conn.Tunnel)
	}

	// Verify that the connection contains the expected tunnel options
	expectedOptions := &tunnels.TunnelRequestOptions{IncludePorts: true}
	if !reflect.DeepEqual(conn.Options, expectedOptions) {
		t.Fatalf("NewCodespaceConnection returned a connection with unexpected options: %+v", conn.Options)
	}

	// Verify that the connection contains the expected allowed port privacy settings
	if !reflect.DeepEqual(conn.AllowedPortPrivacySettings, allowedPortPrivacySettings) {
		t.Fatalf("NewCodespaceConnection returned a connection with unexpected allowed port privacy settings: %+v", conn.AllowedPortPrivacySettings)
	}
}
