package create

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/identity"
	authuse "github.com/cli/cli/v2/pkg/cmd/auth/use"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateWithoutOwnerUsesDirectoryIdentityOutsideRepository(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("persistent identities are not supported on Windows")
	}
	// Given a directory choice made with bare auth use, outside any repository
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GH_CONFIG_DIR", filepath.Join(home, "gh"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, key := range []string{"GH_TOKEN", "GITHUB_TOKEN", "GH_HOST", "GIT_DIR", "GIT_WORK_TREE", "GIT_SSH", "GIT_SSH_COMMAND", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS"} {
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}
	t.Chdir(t.TempDir())
	store, err := identity.Open()
	require.NoError(t, err)
	require.NoError(t, store.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
	cfg := config.NewMockConfigFromString("hosts:\n  github.com:\n    user: bob\n    oauth_token: globally-active-token\n    users:\n      alice:\n        oauth_token: alice-token\n      bob:\n")
	ios, _, stdout, stderr := iostreams.Test()
	registry := &httpmock.Registry{}
	defer registry.Verify(t)
	registry.Register(httpmock.GraphQL("mutation RepositoryCreate"), func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "api.github.com", req.URL.Host)
		assert.Equal(t, "token alice-token", req.Header.Get("Authorization"))
		var payload struct {
			Variables struct {
				Input map[string]any `json:"input"`
			} `json:"variables"`
		}
		require.NoError(t, json.NewDecoder(req.Body).Decode(&payload))
		assert.Equal(t, "example", payload.Variables.Input["name"])
		assert.NotContains(t, payload.Variables.Input, "ownerId")
		return httpmock.StringResponse(`{"data":{"createRepository":{"repository":{
			"name":"example","owner":{"login":"alice"},"url":"https://github.com/alice/example"
		}}}}`)(req)
	})
	f := &cmdutil.Factory{
		IOStreams: ios, ExecutablePath: "/path/to/gh",
		Config: func() (gh.Config, error) { return cfg, nil },
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: api.AddAuthTokenHeaderWithResolver(registry, identity.TokenResolver(cfg.Authentication()))}, nil
		},
	}
	use := authuse.NewCmdUse(f, nil)
	use.SetArgs([]string{"work"})
	require.NoError(t, use.Execute(), "%s", stderr.String())

	// When creating a repository without supplying an owner
	cmd := NewCmdCreate(f, nil)
	cmd.Flags().BoolP("help", "x", false, "")
	cmd.SetArgs([]string{"example", "--private"})
	require.NoError(t, cmd.Execute(), "%s", stderr.String())

	// Then the server receives the chosen account's credential and gh reports that account's repository
	assert.Equal(t, "https://github.com/alice/example\n", stdout.String())
}
