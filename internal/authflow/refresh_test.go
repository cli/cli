package authflow

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/oauth"
	"github.com/stretchr/testify/require"
)

// stubRoundTripper captures the outgoing request and returns a canned response, so the refresher can be exercised
// against the real oauth.Refresh code path without a network call.
type stubRoundTripper struct {
	lastReq  *http.Request
	lastBody string
	status   int
	body     string
}

func (s *stubRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	s.lastReq = req
	if req.Body != nil {
		data, _ := io.ReadAll(req.Body)
		s.lastBody = string(data)
	}
	status := s.status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"application/x-www-form-urlencoded; charset=utf-8"}},
		Body:       io.NopCloser(strings.NewReader(s.body)),
		Request:    req,
	}, nil
}

func TestTokenRefresherRefresh(t *testing.T) {
	// Pin a non-UTC clock so the test proves the derived expiry is normalized to UTC rather than inheriting the
	// local zone of the anchor.
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	fixedNow := time.Date(2026, 1, 2, 3, 4, 5, 0, loc)
	timeNow = func() time.Time { return fixedNow }
	t.Cleanup(func() { timeNow = time.Now })

	stub := &stubRoundTripper{
		body: "access_token=gho_NEW&refresh_token=ghr_NEW&token_type=bearer&scope=repo&expires_in=28800&refresh_token_expires_in=15897600",
	}
	refresher := NewTokenRefresher(&http.Client{Transport: stub})

	credential, err := refresher.Refresh("ghr_OLD", "github.com")
	require.NoError(t, err)

	// The renewed token pair is mapped through verbatim.
	require.Equal(t, "gho_NEW", credential.AccessToken)
	require.Equal(t, "ghr_NEW", credential.RefreshToken)
	require.Equal(t, 28800, credential.ExpiresIn)
	require.Equal(t, 15897600, credential.RefreshTokenExpiresIn)
	// Absolute expiries are derived from the server lifetimes, anchored at the instant the exchange was initiated,
	// and expressed in UTC so they match the canonical form gh stores and displays.
	require.NotNil(t, credential.ExpiresAt)
	require.Equal(t, time.UTC, credential.ExpiresAt.Location())
	require.Equal(t, fixedNow.Add(28800*time.Second).UTC(), *credential.ExpiresAt)
	require.NotNil(t, credential.RefreshTokenExpiresAt)
	require.Equal(t, time.UTC, credential.RefreshTokenExpiresAt.Location())
	require.Equal(t, fixedNow.Add(15897600*time.Second).UTC(), *credential.RefreshTokenExpiresAt)

	// The request is a refresh_token grant carrying the supplied refresh token, sent to the host's token endpoint.
	require.Equal(t, "https://github.com/login/oauth/access_token", stub.lastReq.URL.String())
	values, err := url.ParseQuery(stub.lastBody)
	require.NoError(t, err)
	require.Equal(t, "refresh_token", values.Get("grant_type"))
	require.Equal(t, "ghr_OLD", values.Get("refresh_token"))
	require.Equal(t, oauthClientID, values.Get("client_id"))
}

func TestTokenRefresherRefreshNoExpiry(t *testing.T) {
	stub := &stubRoundTripper{
		body: "access_token=gho_NEW&refresh_token=ghr_NEW&token_type=bearer&scope=repo",
	}
	refresher := NewTokenRefresher(&http.Client{Transport: stub})

	credential, err := refresher.Refresh("ghr_OLD", "github.com")
	require.NoError(t, err)

	require.Equal(t, "gho_NEW", credential.AccessToken)
	require.Equal(t, "ghr_NEW", credential.RefreshToken)
	// An omitted expiry is mapped to a nil pointer so refresh gating treats the token as non-expiring.
	require.Equal(t, 0, credential.ExpiresIn)
	require.Nil(t, credential.ExpiresAt)
	require.Nil(t, credential.RefreshTokenExpiresAt)
}

func TestTokenRefresherRefreshOmittedRefreshToken(t *testing.T) {
	stub := &stubRoundTripper{
		body: "access_token=gho_NEW&token_type=bearer&scope=repo&expires_in=28800",
	}
	refresher := NewTokenRefresher(&http.Client{Transport: stub})

	credential, err := refresher.Refresh("ghr_OLD", "github.com")
	require.NoError(t, err)

	// When the server omits a refresh token we report it as received rather than reusing the old one.
	require.Equal(t, "gho_NEW", credential.AccessToken)
	require.Empty(t, credential.RefreshToken)
}

func TestTokenRefresherRefreshRejected(t *testing.T) {
	stub := &stubRoundTripper{
		body: "error=bad_refresh_token&error_description=The+refresh+token+passed+is+incorrect+or+expired.",
	}
	refresher := NewTokenRefresher(&http.Client{Transport: stub})

	_, err := refresher.Refresh("ghr_OLD", "github.com")
	require.Error(t, err)
	// The gh sentinel lets a caller classify an unusable refresh token without importing the OAuth library, while the
	// original library error is preserved in the chain.
	require.ErrorIs(t, err, gh.ErrRefreshTokenInvalid)
	require.ErrorIs(t, err, oauth.ErrRefreshTokenInvalid)
}
