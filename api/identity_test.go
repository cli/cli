package api

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	ghconfig "github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/identity"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefreshAwareAuthenticationUsesBoundIdentity(t *testing.T) {
	isolateIdentityAPI(t)
	// Given an owner binding and an otherwise globally active account
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GH_CONFIG_DIR", filepath.Join(home, "gh"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GIT_SSH_COMMAND", "")
	t.Setenv("GIT_SSH", "")
	store, err := identity.Open()
	require.NoError(t, err)
	require.NoError(t, store.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
	require.NoError(t, store.Bind(identity.Binding{Identity: "work", Owner: "company"}, "/path/to/gh"))
	registry := &httpmock.Registry{}
	defer registry.Verify(t)
	registry.Register(httpmock.REST("GET", "repos/company/project"), func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "token github.com/alice-token", req.Header.Get("Authorization"))
		return httpmock.JSONResponse(map[string]string{"name": "project"})(req)
	})
	cfg := ghconfig.NewMockConfigFromString(`
hosts:
  github.com:
    user: bob
    oauth_token: globally-active-token
    users:
      alice:
        oauth_token: github.com/alice-token
`)
	client := &http.Client{Transport: AddAuthTokenHeaderWithResolver(registry, identity.TokenResolver(cfg.Authentication()))}

	// When a client uses gh's common authentication transport
	response, err := client.Get("https://api.github.com/repos/company/project")

	// Then identity selection happens before the legacy active-token fallback
	require.NoError(t, err)
	response.Body.Close()
}

func TestCloneRepositoryLookupUsesOwnerIdentity(t *testing.T) {
	isolateIdentityAPI(t)
	// Given an owner binding while the globally active credential belongs elsewhere
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GH_CONFIG_DIR", filepath.Join(home, "gh"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GIT_SSH_COMMAND", "")
	t.Setenv("GIT_SSH", "")
	store, err := identity.Open()
	require.NoError(t, err)
	require.NoError(t, store.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
	require.NoError(t, store.Bind(identity.Binding{Identity: "work", Owner: "company"}, "/path/to/gh"))
	registry := &httpmock.Registry{}
	defer registry.Verify(t)
	registry.Register(httpmock.GraphQL("query RepositoryInfo"), func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "token github.com/alice-token", req.Header.Get("Authorization"))
		return httpmock.StringResponse(`{"data":{"repository":{"name":"project","owner":{"login":"company"}}}}`)(req)
	})
	cfg := ghconfig.NewMockConfigFromString(`
hosts:
  github.com:
    user: bob
    oauth_token: globally-active-token
    users:
      alice:
        oauth_token: github.com/alice-token
`)
	client := NewClientFromHTTP(&http.Client{Transport: AddAuthTokenHeaderWithResolver(registry, identity.TokenResolver(cfg.Authentication()))})

	// When gh repo clone performs its actual GraphQL repository lookup
	repo, err := GitHubRepo(client, ghrepo.New("company", "project"))

	// Then the shared client preserves owner context and selects the bound account
	require.NoError(t, err)
	assert.Equal(t, "project", repo.RepoName())
}

func TestPersistentIdentityUsesCanonicalGatewayCredentials(t *testing.T) {
	isolateIdentityAPI(t)
	// Given an independently authenticated destination that is also configured as another host's API gateway
	cfg, _ := ghconfig.NewIsolatedTestConfig(t, `
hosts:
  github.com:
    api_host: gateway.example
    user: alice
    users:
      alice:
        oauth_token: alice-token
  gateway.example:
    user: bob
    oauth_token: bob-token
    users:
      bob:
        oauth_token: bob-token
`)
	home := t.TempDir()
	t.Chdir(home)
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_SSH_COMMAND", "")
	t.Setenv("GIT_SSH", "")
	t.Setenv("GIT_SSH_VARIANT", "")
	store, err := identity.Open()
	require.NoError(t, err)
	require.NoError(t, store.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
	require.NoError(t, store.Bind(identity.Binding{Identity: "work", Owner: "company"}, "/path/to/gh"))
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", "repos/company/project"), func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "gateway.example", req.URL.Host)
		assert.Equal(t, "token alice-token", req.Header.Get("Authorization"))
		return httpmock.StringResponse("{}")(req)
	})
	client := &http.Client{Transport: AddAuthTokenHeaderWithResolver(reg, identity.TokenResolver(cfg.Authentication()))}

	// When requesting the independently authenticated API destination
	response, err := client.Get("https://gateway.example/repos/company/project")

	// Then the explicit API mapping selects the canonical host's persistent identity
	require.NoError(t, err)
	response.Body.Close()
}

func isolateIdentityAPI(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("persistent identities are not supported on Windows")
	}
	t.Chdir(t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, name := range []string{"GIT_CONFIG", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_SYSTEM", "GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_SSH_VARIANT"} {
		t.Setenv(name, "")
		require.NoError(t, os.Unsetenv(name))
	}
}
