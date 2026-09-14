// Package gh provides types that represent the domain of the CLI application.
//
// For example, the CLI expects to be able to get and set user configuration in order to perform its functionality,
// so the Config interface is defined here, though the concrete implementation lives elsewhere. Though the current
// implementation of config writes to certain files on disk, that is an implementation detail compared to the contract
// laid out in the interface here.
//
// Currently this package is in an early state but we could imagine other domain concepts living here for interacting
// with git or GitHub.
package gh

import (
	"errors"
	"time"

	o "github.com/cli/cli/v2/pkg/option"
	ghConfig "github.com/cli/go-gh/v2/pkg/config"
)

type ConfigSource string

const (
	ConfigDefaultProvided ConfigSource = "default"
	ConfigUserProvided    ConfigSource = "user"
)

type ConfigEntry struct {
	Value  string
	Source ConfigSource
}

// A Config implements persistent storage and modification of application configuration.
//
//go:generate moq -rm -pkg ghmock -out mock/config.go . Config
type Config interface {
	// GetOrDefault provides primitive access for fetching configuration values, optionally scoped by host.
	GetOrDefault(hostname string, key string) o.Option[ConfigEntry]
	// Set provides primitive access for setting configuration values, optionally scoped by host.
	Set(hostname string, key string, value string)

	// AccessibleColors returns the configured accessible_colors setting, optionally scoped by host.
	AccessibleColors(hostname string) ConfigEntry
	// AccessiblePrompter returns the configured accessible_prompter setting, optionally scoped by host.
	AccessiblePrompter(hostname string) ConfigEntry
	// Browser returns the configured browser, optionally scoped by host.
	Browser(hostname string) ConfigEntry
	// Clipboard returns the configured clipboard setting, ignoring host scoping since clipboard is a global setting.
	Clipboard() ConfigEntry
	// ColorLabels returns the configured color_label setting, optionally scoped by host.
	ColorLabels(hostname string) ConfigEntry
	// Editor returns the configured editor, optionally scoped by host.
	Editor(hostname string) ConfigEntry
	// GitProtocol returns the configured git protocol, optionally scoped by host.
	GitProtocol(hostname string) ConfigEntry
	// HTTPUnixSocket returns the configured HTTP unix socket, optionally scoped by host.
	HTTPUnixSocket(hostname string) ConfigEntry
	// Pager returns the configured Pager, optionally scoped by host.
	Pager(hostname string) ConfigEntry
	// Prompt returns the configured prompt, optionally scoped by host.
	Prompt(hostname string) ConfigEntry
	// PreferEditorPrompt returns the configured editor-based prompt, optionally scoped by host.
	PreferEditorPrompt(hostname string) ConfigEntry
	// Spinner returns the configured spinner setting, optionally scoped by host.
	Spinner(hostname string) ConfigEntry
	// Telemetry returns the configured telemetry setting, ignoring host scoping since telemetry is a global setting.
	Telemetry() ConfigEntry

	// Aliases provides persistent storage and modification of command aliases.
	Aliases() AliasConfig

	// Authentication provides persistent storage and modification of authentication configuration.
	Authentication() AuthConfig

	// CacheDir returns the directory where the cacheable artifacts can be persisted.
	CacheDir() string

	// Migrate applies a migration to the configuration.
	Migrate(Migration) error

	// Version returns the current schema version of the configuration.
	Version() o.Option[string]

	// Write persists modifications to the configuration.
	Write() error
}

// Migration is the interface that config migrations must implement.
//
// Migrations will receive a copy of the config, and should modify that copy
// as necessary. After migration has completed, the modified config contents
// will be used.
//
// The calling code is expected to verify that the current version of the config
// matches the PreVersion of the migration before calling Do, and will set the
// config version to the PostVersion after the migration has completed successfully.
//
//go:generate moq -rm  -pkg ghmock -out mock/migration.go . Migration
type Migration interface {
	// PreVersion is the required config version for this to be applied
	PreVersion() string
	// PostVersion is the config version that must be applied after migration
	PostVersion() string
	// Do is expected to apply any necessary changes to the config in place
	Do(*ghConfig.Config) error
}

// TokenType is the kind of credential a token is, and its value is the prefix
// that identifies it. The zero value covers a token gh does not recognise,
// including an empty one.
//
// See the [token formats] GitHub documents.
//
// [token formats]: https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/about-authentication-to-github#githubs-token-formats
type TokenType string

const (
	TokenTypeUnknown        TokenType = ""
	TokenTypeOAuth          TokenType = "gho_"
	TokenTypePersonalAccess TokenType = "ghp_"
	TokenTypeFineGrainedPAT TokenType = "github_pat_"
	TokenTypeUserToServer   TokenType = "ghu_"
	TokenTypeServerToServer TokenType = "ghs_"
	TokenTypeRefresh        TokenType = "ghr_"
)

// TokenTypes lists every recognised credential. TokenTypeUnknown is absent
// because its empty value prefixes every string, so matching against it would
// claim any token.
var TokenTypes = []TokenType{
	TokenTypeOAuth,
	TokenTypePersonalAccess,
	TokenTypeFineGrainedPAT,
	TokenTypeUserToServer,
	TokenTypeServerToServer,
	TokenTypeRefresh,
}

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

// RefreshableCredential is an OAuth token pair that gh can renew, together with the expiration metadata returned by its
// issuer. The expiration fields are optional because a server may issue a non-expiring credential. The ExpiresIn fields
// carry the raw lifetime in seconds as returned by the issuer, alongside the absolute expiry times computed from them.
//
// It is an in-memory value passed across the TokenRefresher boundary; persistence and wire encoding are owned by the
// storage layer, so it deliberately carries no json tags.
type RefreshableCredential struct {
	AccessToken           string
	RefreshToken          string
	ExpiresIn             int
	RefreshTokenExpiresIn int
	ExpiresAt             *time.Time
	RefreshTokenExpiresAt *time.Time
}

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

// Token source values name the origins that gh itself records in a Credential's Source. Tokens supplied through
// environment variables are labeled dynamically by go-gh (for example GH_TOKEN) and are intentionally not enumerated
// here.
const (
	// TokenSourceKeyring indicates the token was read from the system keyring, gh's secure storage.
	TokenSourceKeyring = "keyring"
	// TokenSourceOAuthToken indicates the token was read from the oauth_token entry in the config file.
	TokenSourceOAuthToken = "oauth_token"
	// TokenSourceRefreshableOAuthToken indicates the token was read from the refreshable_oauth_token entry in the
	// config file, which holds a refreshable credential and is kept distinct from the non-expiring oauth_token entry.
	TokenSourceRefreshableOAuthToken = "refreshable_oauth_token"
	// TokenSourceDefault is the placeholder source reported when no token was found.
	TokenSourceDefault = "default"
)

// ErrRefreshTokenInvalid is returned by a TokenRefresher when the OAuth server rejects the refresh token because it is
// invalid, expired, or already used. Callers can test for it with errors.Is to distinguish an unusable refresh token,
// which requires re-authenticating with 'gh auth login', from a transient failure that may succeed on a later retry.
var ErrRefreshTokenInvalid = errors.New("refresh token is invalid or expired")

// TokenRefresher performs the OAuth token refresh request. It knows nothing about when a refresh should happen or about
// in-process or cross-process locking; those concerns belong to the caller. Its only job is to make the refresh request
// for the given refresh token and hostname and parse the response, or the error, back to the caller.
//
//go:generate moq -rm -pkg ghmock -out mock/token_refresher.go . TokenRefresher
type TokenRefresher interface {
	// Refresh exchanges the given refresh token for a renewed credential on the given hostname.
	Refresh(refreshToken string, hostname string) (RefreshableCredential, error)
}

// AuthConfig is used for interacting with some persistent configuration for gh,
// with knowledge on how to access encrypted storage when necessary.
// Behavior is scoped to authentication specific tasks.
type AuthConfig interface {
	// HasActiveToken returns true when a token for the hostname is present.
	HasActiveToken(hostname string) bool

	// ActiveTokenWithRefresh retrieves the active credential for the given hostname and, when it is refreshable and
	// its access token is expiring, renews and persists it. It may return a non-empty credential with an error, such
	// as when refreshing fails. The caller decides whether to use the returned credential.
	ActiveTokenWithRefresh(hostname string) (credential Credential, refreshResult RefreshStatus, err error)

	// ActiveToken retrieves the active credential for the given hostname, searching environment variables, general
	// configuration, and finally encrypted storage. It returns the last stored token as-is, including the current
	// access token of a refreshable credential, and never contacts the token endpoint to refresh it. A caller that
	// needs a token valid for API calls should use ActiveTokenWithRefresh instead.
	ActiveToken(hostname string) Credential

	// ActiveTokenType reports what kind of credential the active token for the
	// hostname is, so a caller can decide whether it will do without handling
	// the token itself.
	ActiveTokenType(hostname string) TokenType

	// HasEnvToken returns true when a token has been specified in an environment variable, else returns false.
	HasEnvToken() bool

	// TokenFromKeyring will retrieve the auth token for the given hostname, only searching in encrypted storage.
	TokenFromKeyring(hostname string) (token string, err error)

	// TokenFromKeyringForUser will retrieve the auth token for the given hostname and username, only searching
	// in encrypted storage.
	//
	// An empty username will return an error because the potential to return the currently active token under
	// surprising cases is just too high to risk compared to the utility of having the function being smart.
	TokenFromKeyringForUser(hostname, username string) (token string, err error)

	// ActiveUser will retrieve the username for the active user at the given hostname.
	//
	// This will not be accurate if the oauth token is set from an environment variable.
	ActiveUser(hostname string) (username string, err error)

	// Hosts retrieves a list of known hosts.
	Hosts() []string

	// APIHostForHost returns the api_host configured for host, reporting false
	// when the host has no api_host set. See config.AuthConfig.APIHostForHost.
	APIHostForHost(host string) (apiHost string, found bool)

	// HostForAPIHost returns the known host whose api_host is the given hostname,
	// reporting false when no host claims it. See config.AuthConfig.HostForAPIHost.
	HostForAPIHost(apiHost string) (host string, found bool)

	// DefaultHost retrieves the default host.
	DefaultHost() (host string, source string)

	// Login will set user, git protocol, and auth token for the given hostname.
	//
	// If the encrypt option is specified it will first try to store the auth token
	// in encrypted storage and will fall back to the general insecure configuration.
	Login(hostname, username, token, gitProtocol string, secureStorage bool) (insecureStorageUsed bool, err error)

	// LoginRefreshable stores a refreshable credential for the given user, sets the git protocol, and marks the user
	// active for the hostname. Like Login it prefers encrypted storage and falls back to the insecure configuration,
	// reporting the fallback via the returned bool. Any non-expiring token stored for the same user is removed so it
	// cannot outrank the refreshable record.
	LoginRefreshable(hostname, username string, credential Credential, gitProtocol string, secureStorage bool) (insecureStorageUsed bool, err error)

	// SwitchUser switches the active user for a given hostname.
	SwitchUser(hostname, user string) error

	// Logout will remove user, git protocol, and auth token for the given hostname.
	// It will remove the auth token from the encrypted storage if it exists there.
	Logout(hostname, username string) error

	// UsersForHost retrieves a list of users configured for a specific host.
	UsersForHost(hostname string) []string

	// TokenForUserWithRefresh retrieves the credential for a specified user and hostname and, when it is refreshable
	// and its access token is expiring, renews and persists it. It may return a non-empty credential with an error,
	// such as when refreshing fails. The caller decides whether to use the returned credential.
	TokenForUserWithRefresh(hostname, username string) (credential Credential, refreshResult RefreshStatus, err error)

	// TokenForUser retrieves the credential for a specified user and hostname. Like ActiveToken it returns the last
	// stored token as-is, including the current access token of a refreshable credential, and never refreshes it; use
	// TokenForUserWithRefresh when the token must be valid for API calls.
	TokenForUser(hostname, user string) (credential Credential, err error)

	// SetTokenRefresher supplies the refresher used to renew tokens. It is provided after construction because the
	// refresher depends on the plain HTTP client, which is only available once the command factory has assembled its
	// dependencies.
	SetTokenRefresher(refresher TokenRefresher)

	// The following methods are only for testing and that is a design smell we should consider fixing.

	// SetActiveToken will override any token resolution and return the given token and source for all calls to
	// ActiveToken.
	// Use for testing purposes only.
	SetActiveToken(token, source string)

	// SetHosts will override any hosts resolution and return the given hosts for all calls to Hosts.
	// Use for testing purposes only.
	SetHosts(hosts []string)

	// SetDefaultHost will override any host resolution and return the given host and source for all calls to
	// DefaultHost.
	// Use for testing purposes only.
	SetDefaultHost(host, source string)
}

// AliasConfig defines an interface for managing command aliases.
type AliasConfig interface {
	// Get retrieves the expansion for a specified alias.
	Get(alias string) (expansion string, err error)

	// Add adds a new alias with the specified expansion.
	Add(alias, expansion string)

	// Delete removes an alias.
	Delete(alias string) error

	// All returns a map of all aliases to their corresponding expansions.
	All() map[string]string
}
