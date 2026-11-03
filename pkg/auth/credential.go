// Package auth defines the public authentication credential types that gh exposes to consumers of the api package.
//
// These types describe a resolved token and, when the token is refreshable, its OAuth refresh metadata. They carry only
// standard-library types so callers outside github.com/cli/cli/v2 can depend on them. The machinery that performs token
// refresh remains internal to gh; this package deliberately exposes the credential shape alone.
package auth

import "time"

// RefreshStatus describes the outcome of attempting to refresh an authentication token.
type RefreshStatus string

const (
	// RefreshStatusDone indicates that the token was refreshed.
	RefreshStatusDone RefreshStatus = "refresh-done"
	// RefreshStatusInapplicable indicates that the token cannot be refreshed.
	RefreshStatusInapplicable RefreshStatus = "refresh-inapplicable"
	// RefreshStatusFailed indicates that the token refresh failed for a transient reason, such as a network or
	// storage error, so a later retry may succeed.
	RefreshStatusFailed RefreshStatus = "refresh-failed"
	// RefreshStatusExpired indicates that the refresh token itself was rejected by the server, so refreshing cannot
	// succeed and the user must re-authenticate.
	RefreshStatusExpired RefreshStatus = "refresh-expired"
	// RefreshStatusUnnecessary indicates that the token does not need to be refreshed.
	RefreshStatusUnnecessary RefreshStatus = "refresh-unnecessary"
)

// Credential is a resolved authentication token together with its source and, when the token is refreshable, the OAuth
// refresh metadata. It is a single universal shape so a caller can inspect a resolved token without needing to know in
// advance whether it is refreshable: Token and Source are always meaningful, while the refresh fields are zero for a
// non-refreshable token.
type Credential struct {
	// Source identifies where the token came from, such as an environment variable, the config file, or the keyring.
	Source string
	// Token is the token gh would present to the API. For a refreshable credential it is the current access token.
	Token string
	// RefreshToken is the OAuth refresh token, set only for a refreshable credential.
	RefreshToken string
	// ExpiresIn is the access token lifetime in seconds as reported by the issuer, or zero when unknown.
	ExpiresIn int
	// RefreshTokenExpiresIn is the refresh token lifetime in seconds as reported by the issuer, or zero when unknown.
	RefreshTokenExpiresIn int
	// ExpiresAt is the absolute access token expiry, or nil when unknown or non-expiring.
	ExpiresAt *time.Time
	// RefreshTokenExpiresAt is the absolute refresh token expiry, or nil when unknown or non-expiring.
	RefreshTokenExpiresAt *time.Time
}

// IsRefreshable reports whether the credential carries a refresh token and can therefore be renewed by gh.
func (c Credential) IsRefreshable() bool {
	return c.RefreshToken != ""
}
