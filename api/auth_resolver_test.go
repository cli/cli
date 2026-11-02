package api

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuthTokenResolverFailureStopsRequest(t *testing.T) {
	t.Parallel()
	// Given an account selection whose credential cannot be resolved
	want := errors.New("selected account unavailable")
	base := &funcTripper{roundTrip: func(*http.Request) (*http.Response, error) {
		t.Fatal("request must not be sent after credential resolution fails")
		return nil, nil
	}}
	transport := AddAuthTokenHeaderWithResolver(base, func(host string, _ *http.Request) (string, error) {
		require.Equal(t, "github.com", host)
		return "", want
	})
	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/user", nil)
	require.NoError(t, err)

	// When the shared authentication wrapper handles the request
	_, err = transport.RoundTrip(req)

	// Then the resolution error is returned without trying unauthenticated or default credentials
	require.ErrorIs(t, err, want)
}

func TestHTTPClientRejectsAmbiguousTokenResolvers(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		opts HTTPClientOptions
	}{
		{name: "primitive config", opts: HTTPClientOptions{Config: legacyConfig{}}},
		{name: "refreshing config", opts: HTTPClientOptions{RefreshingConfig: tinyConfig{}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Given both a configuration-based source and an explicit token resolver
			tt.opts.TokenForHost = func(string, *http.Request) (string, error) { return "", nil }

			// When constructing a client
			_, err := NewHTTPClient(tt.opts)

			// Then no implicit precedence between the competing sources is chosen
			require.EqualError(t, err, "provide TokenForHost without Config or RefreshingConfig")
		})
	}
}
