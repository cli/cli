package identity

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func emailFixture(t *testing.T) (*cmdutil.Factory, *httpmock.Registry) {
	t.Helper()
	f := identityFactory(t)
	t.Chdir(t.TempDir())
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
  enterprise.example:
    users:
      alice:
        oauth_token: enterprise-alice-token
`)
	f.Config = func() (gh.Config, error) { return cfg, nil }
	reg := &httpmock.Registry{}
	t.Cleanup(func() { reg.Verify(t) })
	f.HttpClient = func() (*http.Client, error) {
		return &http.Client{Transport: api.AddAuthTokenHeaderWithRefresh(reg, cfg.Authentication())}, nil
	}
	return f, reg
}

func TestEmailEditResolvesNewAccountOnceAndKeepsGitName(t *testing.T) {
	// Given an existing literal identity with a separately supplied name
	f, reg := emailFixture(t)
	runIdentity(t, f, "create", "work", "--account", "alice", "--git-user-name", "Original Name", "--git-user-email", "old@example.test")
	reg.Register(httpmock.REST("GET", "users/bob"), func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "token bob-token", req.Header.Get("Authorization"))
		return httpmock.StringResponse(`{"login":"bob","id":222}`)(req)
	})

	// When changing the account and explicitly resolving its noreply address, then editing only the name
	runIdentity(t, f, "edit", "work", "--account", "bob", "--git-user-email", "@noreply")
	var first map[string]any
	require.NoError(t, json.Unmarshal([]byte(runIdentity(t, f, "view", "work")), &first))
	runIdentity(t, f, "edit", "work", "--git-user-name", "New Name")
	var second map[string]any
	require.NoError(t, json.Unmarshal([]byte(runIdentity(t, f, "view", "work")), &second))

	// Then the name was not taken from the profile, and subsequent edits don't trigger another lookup
	assert.Equal(t, "Original Name", first["gitUserName"])
	assert.Equal(t, "222+bob@users.noreply.github.com", first["gitUserEmail"])
	assert.Equal(t, first["gitUserEmail"], second["gitUserEmail"])
	assert.Equal(t, "New Name", second["gitUserName"])
}

func TestMissingPublicEmailLeavesExistingIdentityUnchanged(t *testing.T) {
	// Given an existing identity whose public profile has no email
	f, reg := emailFixture(t)
	runIdentity(t, f, "create", "work", "--account", "alice", "--git-user-name", "Alice", "--git-user-email", "old@example.test")
	before := runIdentity(t, f, "view", "work")
	reg.Register(httpmock.REST("GET", "users/alice"), httpmock.StringResponse(`{"login":"alice","id":123,"email":null}`))
	cmd := NewCmdIdentity(f)
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	cmd.SetArgs([]string{"edit", "work", "--git-user-email", "@public"})

	// When the user explicitly asks for public profile email
	err := cmd.Execute()

	// Then no fallback or partial edit occurs, and alternatives are explained
	require.ErrorContains(t, err, "has no public profile email")
	assert.Contains(t, err.Error(), "@noreply")
	assert.Contains(t, err.Error(), "literal address")
	assert.Equal(t, before, runIdentity(t, f, "view", "work"))
}

func TestPublicEmailUsesIdentityEnterpriseHost(t *testing.T) {
	// Given a saved account on a non-default host
	f, reg := emailFixture(t)
	reg.Register(httpmock.REST("GET", "api/v3/users/alice"), func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "enterprise.example", req.URL.Host)
		assert.Equal(t, "token enterprise-alice-token", req.Header.Get("Authorization"))
		return httpmock.StringResponse(`{"login":"alice","email":"alice@company.test"}`)(req)
	})

	// When resolving public email on that host
	runIdentity(t, f, "create", "work", "--hostname", "enterprise.example", "--account", "alice", "--git-user-name", "Alice", "--git-user-email", "@public")

	// Then the host's actual public address is stored
	assert.Contains(t, runIdentity(t, f, "view", "work"), "alice@company.test")
}

func TestEmailSelectorsRejectUnsupportedRequestsWithoutLookup(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{"no Git name", []string{"--git-user-email", "@public"}, "requires --git-user-name"},
		{"unknown source", []string{"--git-user-name", "Alice", "--git-user-email", "@primary"}, "unknown Git email source"},
		{"unsupported noreply host", []string{"--hostname", "enterprise.example", "--git-user-name", "Alice", "--git-user-email", "@noreply"}, "supported only for github.com"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Given an invalid selector request and no registered network responses
			f, _ := emailFixture(t)
			cmd := NewCmdIdentity(f)
			cmd.SilenceErrors, cmd.SilenceUsage = true, true
			cmd.SetArgs(append([]string{"create", "work", "--account", "alice"}, tt.args...))

			// When the request is validated
			err := cmd.Execute()

			// Then it fails before any HTTP lookup or identity write
			require.ErrorContains(t, err, tt.want)
			assert.JSONEq(t, "[]", runIdentity(t, f, "list"))
		})
	}
}

func TestEmailProfileFailuresNeverSaveAnIdentity(t *testing.T) {
	for _, tt := range []struct{ name, body, want string }{
		{"missing ID", `{"login":"alice"}`, "no valid account ID"},
		{"wrong account", `{"login":"bob","id":222}`, "instead of \"alice\""},
		{"invalid response", `not-json`, "read account profile"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Given a profile response that cannot safely identify the requested account's email
			f, reg := emailFixture(t)
			reg.Register(httpmock.REST("GET", "users/alice"), httpmock.StringResponse(tt.body))
			cmd := NewCmdIdentity(f)
			cmd.SilenceErrors, cmd.SilenceUsage = true, true
			cmd.SetArgs([]string{"create", "work", "--account", "alice", "--git-user-name", "Alice", "--git-user-email", "@noreply"})

			// When resolving the address
			err := cmd.Execute()

			// Then there is no partial or guessed identity file
			require.ErrorContains(t, err, tt.want)
			assert.JSONEq(t, "[]", runIdentity(t, f, "list"))
		})
	}
}

func TestCreateResolvesEmailSelectorUsingIdentityAccount(t *testing.T) {
	for _, tt := range []struct{ selector, email string }{
		{"@public", "alice@example.test"},
		{"@noreply", "123+alice@users.noreply.github.com"},
	} {
		t.Run(tt.selector, func(t *testing.T) {
			// Given Alice's saved credential while another account and environment token are active
			f := identityFactory(t)
			t.Chdir(t.TempDir())
			cfg, _ := config.NewIsolatedTestConfig(t, `
hosts:
  github.com:
    user: bob
    oauth_token: bob-token
    users:
      alice:
        oauth_token: alice-token
      bob:
        oauth_token: bob-token
`)
			t.Setenv("GH_TOKEN", "unrelated-environment-token")
			f.Config = func() (gh.Config, error) { return cfg, nil }
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(httpmock.REST("GET", "users/alice"), func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "api.github.com", req.URL.Host)
				assert.Equal(t, "token alice-token", req.Header.Get("Authorization"))
				return httpmock.StringResponse(`{"login":"alice","id":123,"name":"Profile Name","email":"alice@example.test"}`)(req)
			})
			f.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: api.AddAuthTokenHeaderWithRefresh(reg, cfg.Authentication())}, nil
			}

			// When creating an identity with an explicitly chosen email source
			runIdentity(t, f, "create", "work", "--account", "alice", "--git-user-name", "Chosen Name", "--git-user-email", tt.selector)
			var got map[string]any
			require.NoError(t, json.Unmarshal([]byte(runIdentity(t, f, "view", "work")), &got))

			// Then the resolved literal email is saved once, without changing the supplied Git name
			assert.Equal(t, tt.email, got["gitUserEmail"])
			assert.Equal(t, "Chosen Name", got["gitUserName"])
			assert.Equal(t, "alice", got["account"])
		})
	}
}
