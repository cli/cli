package identity_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIUsesOwnerIdentityOutsideRepository(t *testing.T) {
	// Given an owner binding and a separately saved account credential
	cfg, s := isolatedAccounts(t)
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, s.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
	require.NoError(t, s.Bind(identity.Binding{Identity: "work", Owner: "company"}, "/path/to/gh"))
	var authorization string
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		authorization = req.Header.Get("Authorization")
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})
	client := &http.Client{Transport: api.AddAuthTokenHeaderWithResolver(base, identity.TokenResolver(cfg.Authentication()))}

	// When an API request targets that owner's repository outside any checkout
	response, err := client.Get("https://api.github.com/repos/company/project")

	// Then the owner identity's account authenticates the request
	require.NoError(t, err)
	response.Body.Close()
	assert.Equal(t, "token alice-token", authorization)
}

func TestOwnerIdentityRefusesEnvironmentToken(t *testing.T) {
	// Given a bound identity and an environment token of unknown account
	cfg, s := isolatedAccounts(t)
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, s.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
	require.NoError(t, s.Bind(identity.Binding{Identity: "work", Owner: "company"}, "/path/to/gh"))
	t.Setenv("GH_TOKEN", "unknown-account-token")
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatal("a conflicting identity must fail before any network request")
		return nil, nil
	})
	client := &http.Client{Transport: api.AddAuthTokenHeaderWithResolver(base, identity.TokenResolver(cfg.Authentication()))}

	// When the request would otherwise use the bound identity
	_, err := client.Get("https://api.github.com/repos/company/project")

	// Then it refuses the override rather than claiming account agreement
	require.ErrorContains(t, err, "GH_TOKEN overrides saved accounts")
}

func TestGraphQLLiteralOwnerUsesOwnerIdentity(t *testing.T) {
	// Given an owner binding and a GraphQL query spelling its owner literally
	cfg, s := isolatedAccounts(t)
	t.Chdir(t.TempDir())
	require.NoError(t, s.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
	require.NoError(t, s.Bind(identity.Binding{Identity: "work", Owner: "company"}, "/path/to/gh"))
	var authorization string
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		authorization = req.Header.Get("Authorization")
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})
	client := &http.Client{Transport: api.AddAuthTokenHeaderWithResolver(base, identity.TokenResolver(cfg.Authentication()))}

	// When that query is sent outside a repository
	response, err := client.Post("https://api.github.com/graphql", "application/json",
		strings.NewReader(`{"query":"{ repository(owner: \"company\", name: \"project\") { id } }"}`))

	// Then the owner identity, not the global default, authenticates it
	require.NoError(t, err)
	response.Body.Close()
	assert.Equal(t, "token alice-token", authorization)
}
