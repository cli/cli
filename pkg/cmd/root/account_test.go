package root

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/api"
	ghcontext "github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/identity"
	"github.com/cli/cli/v2/internal/telemetry"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func accountCommand(t *testing.T, cfg gh.Config, reg *httpmock.Registry, remotes ...*ghcontext.Remote) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	ios, _, stdout, stderr := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios, Config: func() (gh.Config, error) { return cfg, nil },
		Browser: &browser.Stub{},
		Remotes: func() (ghcontext.Remotes, error) { return remotes, nil },
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: api.AddAuthTokenHeaderWithResolver(reg, identity.TokenResolver(cfg.Authentication()))}, nil
		},
		ExtensionManager: &extensions.ExtensionManagerMock{ListFunc: func() []extensions.Extension { return nil }},
	}
	cmd, err := NewCmdRoot(f, &telemetry.NoOpService{}, "", "")
	require.NoError(t, err)
	return cmd, stdout, stderr
}

func isolateAccountCommand(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Chdir(home)
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GH_CONFIG_DIR", filepath.Join(home, "gh"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, key := range []string{
		"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN",
		"GH_HOST", "GH_REPO", "GIT_DIR", "GIT_WORK_TREE", "GIT_SSH", "GIT_SSH_COMMAND",
		"GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS", "GH_IDENTITY_ACCOUNT", "GH_IDENTITY_HOST",
		"GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "SSH_AUTH_SOCK",
		"GIT_SSH_VARIANT", "GH_IDENTITY_SSH_COMMAND", "GH_IDENTITY_SSH_VARIANT",
	} {
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}
}

func TestAccountOverrideWithoutIdentitiesIsInvocationLocal(t *testing.T) {
	// Given two saved accounts and no identities or bindings
	isolateAccountCommand(t)
	cfg, _ := config.NewIsolatedTestConfig(t, `
hosts:
  github.com:
    user: alice
    oauth_token: alice-token
    users:
      alice:
        oauth_token: alice-token
      bob:
        oauth_token: bob-token
`)
	t.Setenv("GH_TOKEN", "environment-token-that-must-not-be-used")
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.GraphQL("RepositoryInfo"), func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "api.github.com", req.URL.Host)
		assert.Equal(t, "token bob-token", req.Header.Get("Authorization"))
		return httpmock.StringResponse(`{"data":{"repository":{"name":"project"}}}`)(req)
	})
	cmd, stdout, stderr := accountCommand(t, cfg, reg)
	cmd.SetArgs([]string{"repo", "view", "company/project", "--account", "bob", "--json", "name"})

	// When an API command explicitly selects bob
	require.NoError(t, cmd.Execute(), "%s", stderr.String())

	// Then the request uses bob without enrolling in identities or changing the saved active account
	assert.JSONEq(t, `{"name":"project"}`, stdout.String())
	assert.Empty(t, stderr.String())
	assert.NoDirExists(t, filepath.Join(os.Getenv("GH_CONFIG_DIR"), "identities"))
	entry, ok := cfg.GetOrDefault("github.com", "user").Value()
	require.True(t, ok, "saved active account must still exist")
	assert.Equal(t, "alice", entry.Value)
	t.Setenv("GH_TOKEN", "")
	reg.Register(httpmock.GraphQL("RepositoryInfo"), func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "token alice-token", req.Header.Get("Authorization"))
		return httpmock.StringResponse(`{"data":{"repository":{"name":"project"}}}`)(req)
	})
	next, _, nextErr := accountCommand(t, cfg, reg)
	next.SetArgs([]string{"repo", "view", "company/project", "--json", "name"})
	require.NoError(t, next.Execute(), "%s", nextErr.String())
}

func TestAccountOverrideUsesExplicitRepositoryHost(t *testing.T) {
	// Given the same login saved separately on GitHub.com and an enterprise host
	isolateAccountCommand(t)
	cfg, _ := config.NewIsolatedTestConfig(t, `
hosts:
  github.com:
    user: bob
    oauth_token: public-token
    users:
      bob:
        oauth_token: public-token
  enterprise.example:
    user: alice
    oauth_token: enterprise-alice-token
    users:
      bob:
        oauth_token: enterprise-bob-token
`)
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.GraphQL("RepositoryInfo"), func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "enterprise.example", req.URL.Host)
		assert.Equal(t, "token enterprise-bob-token", req.Header.Get("Authorization"))
		return httpmock.StringResponse(`{"data":{"repository":{"name":"project"}}}`)(req)
	})
	cmd, stdout, stderr := accountCommand(t, cfg, reg)
	cmd.SetArgs([]string{"repo", "view", "enterprise.example/company/project", "--account", "bob", "--json", "name"})

	// When selecting bob for a repository on the non-default host
	require.NoError(t, cmd.Execute(), "%s", stderr.String())

	// Then both repository lookup and authentication use that host, not GitHub.com
	assert.JSONEq(t, `{"name":"project"}`, stdout.String())
}

func TestAccountOverrideCannotBeSilentlyIgnoredByCommandFamilyHooks(t *testing.T) {
	// Given a command family that has its own persistent pre-run hook
	isolateAccountCommand(t)
	cfg, _ := config.NewIsolatedTestConfig(t, "")
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	cmd, stdout, _ := accountCommand(t, cfg, reg)
	cmd.SetArgs([]string{"skill", "list", "--account", "missing"})

	// When an account that is not saved is explicitly requested
	err := cmd.Execute()

	// Then the override is rejected, rather than silently running with ordinary authentication
	require.ErrorContains(t, err, `account "missing" is not saved`)
	assert.Empty(t, stdout.String())
}

func TestAccountOverrideSearchUsesDefaultHostNotCheckoutHost(t *testing.T) {
	// Given a saved account and a search query while inside an unrelated enterprise checkout
	isolateAccountCommand(t)
	cfg, _ := config.NewIsolatedTestConfig(t, accountGitConfig)
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(httpmock.REST("GET", "search/repositories"), func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "api.github.com", req.URL.Host)
		assert.Equal(t, "token fixture-bob-token", req.Header.Get("Authorization"))
		return httpmock.StringResponse(`{"total_count":1,"items":[{"name":"project"}]}`)(req)
	})
	cmd, stdout, stderr := accountCommand(t, cfg, reg, &ghcontext.Remote{
		Remote: &git.Remote{Name: "origin"},
		Repo:   ghrepo.NewWithHost("company", "project", "enterprise.example"),
	})
	cmd.SetArgs([]string{"search", "repos", "topic:cli", "--account", "bob", "--json", "name", "--limit", "1"})

	// When searching with an explicit account
	require.NoError(t, cmd.Execute(), "%s", stderr.String())

	// Then neither the checkout nor the query changes the search command's authentication host
	assert.JSONEq(t, `[{"name":"project"}]`, stdout.String())
}
