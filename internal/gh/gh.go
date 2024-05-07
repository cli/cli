package gh

import (
	ghConfig "github.com/cli/go-gh/v2/pkg/config"
)

// This interface describes interacting with some persistent configuration for gh.
//
//go:generate moq -rm -pkg ghmock -out mock/config.go . Config
type Config interface {
	GetOrDefault(string, string) (string, error)
	Set(string, string, string)
	Write() error
	Migrate(Migration) error

	CacheDir() string

	Aliases() AliasConfig
	Authentication() AuthConfig
	Browser(string) string
	Editor(string) string
	GitProtocol(string) string
	HTTPUnixSocket(string) string
	Pager(string) string
	Prompt(string) string
	Version() string
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

// AuthConfig is used for interacting with some persistent configuration for gh,
// with knowledge on how to access encrypted storage when neccesarry.
// Behavior is scoped to authentication specific tasks.
type AuthConfig interface {
	// ActiveToken retrieves the active authentication token for a specified hostname.
	ActiveToken(hostname string) (token string, source string)

	// HasEnvToken checks if an authentication token has been specified in the environment variables.
	HasEnvToken() bool

	// SetActiveToken sets a static token and source that overrides any other token resolution method.
	SetActiveToken(token, source string)

	// TokenFromKeyring retrieves the authentication token for a specified hostname from encrypted storage.
	TokenFromKeyring(hostname string) (token string, err error)

	// TokenFromKeyringForUser retrieves the authentication token for a specified hostname and user from encrypted storage.
	TokenFromKeyringForUser(hostname, username string) (token string, err error)

	// ActiveUser retrieves the username for the active user at the given hostname.
	ActiveUser(hostname string) (username string, err error)

	// Hosts retrieves a list of known hosts.
	Hosts() []string

	// SetHosts sets a static list of hosts that overrides any other hosts resolution method.
	SetHosts(hosts []string)

	// DefaultHost retrieves the default host configuration.
	DefaultHost() (host string, source string)

	// SetDefaultHost sets a static host and source that overrides any other default host resolution method.
	SetDefaultHost(host, source string)

	// Login sets user, git protocol, and auth token configurations for a given hostname.
	Login(hostname, username, token, gitProtocol string, secureStorage bool) (insecureStorageUsed bool, err error)

	// SwitchUser switches the active user for a given hostname.
	SwitchUser(hostname, user string) error

	// Logout removes user and authentication token configurations for a given hostname.
	Logout(hostname, username string) error

	// UsersForHost retrieves a list of users configured for a specific host.
	UsersForHost(hostname string) []string

	// TokenForUser retrieves the authentication token and its source for a specified user and hostname.
	TokenForUser(hostname, user string) (token string, source string, err error)
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
