package api

import (
	"net/http"
	"os/exec"
	"path/filepath"
	"testing"

	ghconfig "github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/identity"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIdentityGatewayFallbackRespectsDestinationEnvironmentCredential(t *testing.T) {
	for _, tt := range []struct{ name, environment, expected string }{
		{"uncredentialed gateway", "", "alice-token"},
		{"gateway owns an environment credential", "destination-token", "destination-token"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Given a configured gateway without registered users and an optional destination credential
			isolateIdentityAPI(t)
			cfg, _ := ghconfig.NewIsolatedTestConfig(t, `
hosts:
  github.com:
    api_host: gateway.example
    user: alice
    users:
      alice:
        oauth_token: alice-token
`)
			home := t.TempDir()
			require.NoError(t, exec.Command("git", "init", "-q", home).Run())
			t.Chdir(home)
			t.Chdir(home)
			t.Setenv("HOME", home)
			t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
			t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
			t.Setenv("GH_ENTERPRISE_TOKEN", tt.environment)
			t.Setenv("GIT_SSH_COMMAND", "")
			t.Setenv("GIT_SSH", "")
			s, err := identity.Open()
			require.NoError(t, err)
			require.NoError(t, s.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
			require.NoError(t, s.Bind(identity.Binding{Identity: "work", Directory: home}, "/path/to/gh"))
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("GET", "repos/company/project"), func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "gateway.example", req.URL.Host)
				assert.Equal(t, "token "+tt.expected, req.Header.Get("Authorization"))
				return httpmock.StringResponse("{}")(req)
			})
			client := &http.Client{Transport: AddAuthTokenHeaderWithRefresh(reg, cfg.Authentication())}

			// When a request reaches the gateway
			response, err := client.Get("https://gateway.example/repos/company/project")

			// Then canonical identity fallback only supplies otherwise missing destination credentials
			require.NoError(t, err)
			response.Body.Close()
		})
	}
}
