package identity

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func TestEffectiveAuthenticationUsesConfiguredGatewayMapping(t *testing.T) {
	// Given a selected GitHub account whose API gateway also has saved credentials
	cfg, _ := config.NewIsolatedTestConfig(t, `
hosts:
  github.com:
    api_host: gateway.example
    user: alice
    oauth_token: alice-token
    users:
      alice:
        oauth_token: alice-token
      bob:
        oauth_token: bob-token
  gateway.example:
    user: gateway-user
    oauth_token: gateway-token
    users:
      gateway-user:
        oauth_token: gateway-token
`)
	cfg.Authentication().SetAccountOverride("github.com", "bob")
	var authorization string
	base := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		authorization = request.Header.Get("Authorization")
		return &http.Response{StatusCode: http.StatusOK}, nil
	})
	req, err := http.NewRequest(http.MethodGet, "https://gateway.example/user", nil)
	require.NoError(t, err)

	// When resolving effective authentication for the gateway destination
	_, err = api.AddAuthTokenHeaderWithResolver(base, TokenResolver(cfg.Authentication())).RoundTrip(req)

	// Then the explicit API host mapping determines which host supplies authentication
	require.NoError(t, err)
	assert.Equal(t, "token bob-token", authorization)
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestAccountOverridePreservesRequestTrustBoundaries(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		authorization string
		redirect      bool
		want          string
	}{
		{name: "selected host", url: "https://api.github.com/user", want: "token bob-token"},
		{name: "unrelated host", url: "https://unrelated.example/user"},
		{name: "explicit authorization", url: "https://api.github.com/user", authorization: "Bearer caller-token", want: "Bearer caller-token"},
		{name: "cross-host redirect", url: "https://api.github.com/user", redirect: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given saved accounts, an explicit override, and a request with its own trust context
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
			auth := cfg.Authentication()
			auth.SetAccountOverride("github.com", "bob")
			req, err := http.NewRequest(http.MethodGet, tt.url, nil)
			require.NoError(t, err)
			req.Header.Set("Authorization", tt.authorization)
			if tt.redirect {
				original, err := http.NewRequest(http.MethodGet, "https://other.example/user", nil)
				require.NoError(t, err)
				req.Response = &http.Response{Request: original}
			}
			var authorization string
			base := roundTripFunc(func(request *http.Request) (*http.Response, error) {
				authorization = request.Header.Get("Authorization")
				return &http.Response{StatusCode: http.StatusOK}, nil
			})

			// When authentication is attached for the selected saved account
			_, err = api.AddAuthTokenHeaderWithResolver(base, TokenResolver(auth)).RoundTrip(req)

			// Then only eligible requests get the saved account, without changing persistent selection
			require.NoError(t, err)
			assert.Equal(t, tt.want, authorization)
			assert.Equal(t, tt.authorization, req.Header.Get("Authorization"))
			user, err := auth.ConfiguredUser("github.com")
			require.NoError(t, err)
			assert.Equal(t, "alice", user)
		})
	}
}
