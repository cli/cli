package codespace

import (
	"context"
	"errors"
	"testing"

	"github.com/cli/cli/v2/internal/codespaces/portforwarder"
	"github.com/microsoft/dev-tunnels/go/tunnels"
	"github.com/stretchr/testify/require"
)

// stubPortForwarder embeds the interface so that only ListPorts needs implementing.
type stubPortForwarder struct {
	portforwarder.PortForwarder
	ports []*tunnels.TunnelPort
	err   error
}

func (s stubPortForwarder) ListPorts(ctx context.Context) ([]*tunnels.TunnelPort, error) {
	return s.ports, s.err
}

func TestJupyterWebForwardingURL(t *testing.T) {
	const forwardingURI = "https://ideal-space-fishstick-abc123-8888.app.github.dev/"

	forwardedPort := func() []*tunnels.TunnelPort {
		return []*tunnels.TunnelPort{
			{PortNumber: 8888, PortForwardingURIs: []string{forwardingURI}},
		}
	}

	// portWithVisibility builds a forwarded port carrying the access control entries
	// that Dev Tunnels uses for the given visibility.
	portWithVisibility := func(visibility string) []*tunnels.TunnelPort {
		var entry tunnels.TunnelAccessControlEntry
		switch visibility {
		case portforwarder.PublicPortVisibility:
			entry = tunnels.TunnelAccessControlEntry{
				Type:     tunnels.TunnelAccessControlEntryTypeAnonymous,
				Subjects: []string{},
				Scopes:   []string{string(tunnels.TunnelAccessScopeConnect)},
			}
		case portforwarder.OrgPortVisibility:
			entry = tunnels.TunnelAccessControlEntry{
				Type:     tunnels.TunnelAccessControlEntryTypeOrganizations,
				Scopes:   []string{string(tunnels.TunnelAccessScopeConnect)},
				Provider: string(tunnels.TunnelAuthenticationSchemeGitHub),
			}
		default:
			t.Fatalf("unsupported visibility %q", visibility)
		}

		return []*tunnels.TunnelPort{{
			PortNumber:         8888,
			PortForwardingURIs: []string{forwardingURI},
			AccessControl:      &tunnels.TunnelAccessControl{Entries: []tunnels.TunnelAccessControlEntry{entry}},
		}}
	}

	tests := []struct {
		name       string
		ports      []*tunnels.TunnelPort
		listErr    error
		serverPort int
		serverURL  string
		want       string
		wantErr    string
	}{
		{
			name:       "preserves path and token from the reported URL",
			ports:      forwardedPort(),
			serverPort: 8888,
			serverURL:  "http://localhost:8888/lab?token=abc",
			want:       "https://ideal-space-fishstick-abc123-8888.app.github.dev/lab?token=abc",
		},
		{
			name:       "reported origin is ignored",
			ports:      forwardedPort(),
			serverPort: 8888,
			serverURL:  "http://127.0.0.1:9999/anything?x=1",
			want:       "https://ideal-space-fishstick-abc123-8888.app.github.dev/anything?x=1",
		},
		{
			name:       "token containing the port digits is left intact",
			ports:      forwardedPort(),
			serverPort: 8888,
			serverURL:  "http://localhost:8888/?token=8888zz",
			want:       "https://ideal-space-fishstick-abc123-8888.app.github.dev/?token=8888zz",
		},
		{
			name:       "reported URL without a path",
			ports:      forwardedPort(),
			serverPort: 8888,
			serverURL:  "http://localhost:8888?token=abc",
			want:       "https://ideal-space-fishstick-abc123-8888.app.github.dev/?token=abc",
		},
		{
			name:       "fragment is preserved",
			ports:      forwardedPort(),
			serverPort: 8888,
			serverURL:  "http://localhost:8888/lab?token=abc#/tree",
			want:       "https://ideal-space-fishstick-abc123-8888.app.github.dev/lab?token=abc#/tree",
		},
		{
			name:       "no port matching the reported port",
			ports:      forwardedPort(),
			serverPort: 9999,
			serverURL:  "http://localhost:9999/?token=abc",
			wantErr:    "no web forwarding URL available for JupyterLab port 9999",
		},
		{
			name:       "matching port has no web forwarding URI",
			ports:      []*tunnels.TunnelPort{{PortNumber: 8888}},
			serverPort: 8888,
			serverURL:  "http://localhost:8888/?token=abc",
			wantErr:    "no web forwarding URL available for JupyterLab port 8888",
		},
		{
			name:       "listing ports fails",
			listErr:    errors.New("boom"),
			serverPort: 8888,
			serverURL:  "http://localhost:8888/?token=abc",
			wantErr:    "failed to list forwarded ports: boom",
		},
		{
			name:       "unparseable reported URL",
			ports:      forwardedPort(),
			serverPort: 8888,
			serverURL:  "http://[::1",
			wantErr:    "failed to parse JupyterLab server URL",
		},
		{
			name:       "public port is refused",
			ports:      portWithVisibility(portforwarder.PublicPortVisibility),
			serverPort: 8888,
			serverURL:  "http://localhost:8888/lab?token=abc",
			wantErr:    "JupyterLab port 8888 is public rather than private",
		},
		{
			name:       "org visible port is refused",
			ports:      portWithVisibility(portforwarder.OrgPortVisibility),
			serverPort: 8888,
			serverURL:  "http://localhost:8888/lab?token=abc",
			wantErr:    "JupyterLab port 8888 is org rather than private",
		},
		{
			name: "denied anonymous entry is still private",
			ports: []*tunnels.TunnelPort{{
				PortNumber:         8888,
				PortForwardingURIs: []string{forwardingURI},
				AccessControl: &tunnels.TunnelAccessControl{
					Entries: []tunnels.TunnelAccessControlEntry{{
						Type:   tunnels.TunnelAccessControlEntryTypeAnonymous,
						IsDeny: true,
					}},
				},
			}},
			serverPort: 8888,
			serverURL:  "http://localhost:8888/lab?token=abc",
			want:       "https://ideal-space-fishstick-abc123-8888.app.github.dev/lab?token=abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fwd := stubPortForwarder{ports: tt.ports, err: tt.listErr}

			got, err := jupyterWebForwardingURL(context.Background(), fwd, tt.serverPort, tt.serverURL)

			if tt.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
