package authflow

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/oauth"
	oauthapi "github.com/cli/oauth/api"
)

// timeNow is the clock used to anchor a server-reported token lifetime to an absolute expiry. Callers capture it just
// before initiating the OAuth exchange, so the derived expiry is measured from an instant at or before the moment the
// server issued the token and is therefore never later than the true expiry. It is a package variable so tests can pin
// the anchor and assert the computed expiry deterministically.
var timeNow = time.Now

// TokenRefresher exchanges a refresh token for a renewed credential against a host's OAuth token endpoint. It performs
// only the token endpoint request and response parsing; deciding when to refresh and serializing concurrent refreshes
// are the caller's responsibility, per the gh.TokenRefresher contract.
type TokenRefresher struct {
	httpClient   *http.Client
	clientID     string
	clientSecret string
}

// NewTokenRefresher builds a TokenRefresher that renews credentials issued to gh's OAuth app. The provided HTTP client
// should be a plain client that sets no auth or other headers, since the token endpoint is reached without an existing
// token.
func NewTokenRefresher(httpClient *http.Client) *TokenRefresher {
	return &TokenRefresher{
		httpClient:   httpClient,
		clientID:     oauthClientID,
		clientSecret: oauthClientSecret,
	}
}

// Refresh exchanges refreshToken for a renewed credential on hostname. A refresh token the server rejects surfaces as
// an error wrapping gh.ErrRefreshTokenInvalid, so callers can distinguish an unusable refresh token from a transient
// failure without depending on the underlying OAuth library. When the server omits a new refresh token, the returned
// credential's RefreshToken is left empty rather than reusing refreshToken, so the response is reported exactly as
// received.
func (r *TokenRefresher) Refresh(refreshToken, hostname string) (gh.RefreshableCredential, error) {
	host, err := oauth.NewGitHubHost(ghinstance.HostPrefix(hostname))
	if err != nil {
		return gh.RefreshableCredential{}, err
	}

	opts := oauth.RefreshOptions{
		Host:         host,
		ClientID:     r.clientID,
		ClientSecret: r.clientSecret,
		RefreshToken: refreshToken,
	}
	// Only set HTTPClient when we actually have one. RefreshOptions.HTTPClient is an interface, so assigning a nil
	// *http.Client would store a non-nil typed nil that defeats the library's own http.DefaultClient fallback and
	// panics when the request is made.
	if r.httpClient != nil {
		opts.HTTPClient = r.httpClient
	}

	// Anchor the token lifetime to the instant just before the exchange. The server reports expires_in relative to
	// when it issues the token, which is at or after this point, so measuring from here never overestimates the
	// expiry. Use UTC so the derived absolute expiry matches the canonical UTC form gh stores and displays.
	requestedAt := timeNow().UTC()
	token, err := oauth.Refresh(opts)
	if err != nil {
		// Translate the library's rejection sentinel to gh's own so callers can classify a rejected refresh token
		// without importing the OAuth library, while preserving the original error in the chain for debugging.
		if errors.Is(err, oauth.ErrRefreshTokenInvalid) {
			return gh.RefreshableCredential{}, fmt.Errorf("%w: %w", gh.ErrRefreshTokenInvalid, err)
		}
		return gh.RefreshableCredential{}, err
	}

	return refreshableCredentialFromAccessToken(token, requestedAt), nil
}

// refreshableCredentialFromAccessToken projects an OAuth access token onto gh.RefreshableCredential. It derives each
// absolute expiry by adding the server-reported lifetime (ExpiresIn) to requestedAt, the instant the exchange was
// initiated, rather than trusting any absolute time from the OAuth library. Because requestedAt is at or before the
// server issued the token, the derived expiry is never later than the true expiry. A zero lifetime maps to a nil
// pointer so the refresh gating treats the credential as non-expiring rather than already expired.
func refreshableCredentialFromAccessToken(token *oauthapi.AccessToken, requestedAt time.Time) gh.RefreshableCredential {
	credential := gh.RefreshableCredential{
		AccessToken:           token.Token,
		RefreshToken:          token.RefreshToken,
		ExpiresIn:             token.ExpiresIn,
		RefreshTokenExpiresIn: token.RefreshTokenExpiresIn,
	}
	if token.ExpiresIn > 0 {
		credential.ExpiresAt = new(requestedAt.Add(time.Duration(token.ExpiresIn) * time.Second))
	}
	if token.RefreshTokenExpiresIn > 0 {
		credential.RefreshTokenExpiresAt = new(requestedAt.Add(time.Duration(token.RefreshTokenExpiresIn) * time.Second))
	}
	return credential
}

// credentialFromAccessToken projects an OAuth access token onto gh.Credential, reusing
// refreshableCredentialFromAccessToken so the expiry mapping lives in one place. requestedAt is the instant the
// exchange was initiated, used to anchor the token lifetime. The runtime-only Source is left unset; the storage layer
// drops it regardless.
func credentialFromAccessToken(token *oauthapi.AccessToken, requestedAt time.Time) gh.Credential {
	rc := refreshableCredentialFromAccessToken(token, requestedAt)
	return gh.Credential{
		Token:                 rc.AccessToken,
		RefreshToken:          rc.RefreshToken,
		ExpiresIn:             rc.ExpiresIn,
		RefreshTokenExpiresIn: rc.RefreshTokenExpiresIn,
		ExpiresAt:             rc.ExpiresAt,
		RefreshTokenExpiresAt: rc.RefreshTokenExpiresAt,
	}
}
