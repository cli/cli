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

	// Browser returns the configured browser, optionally scoped by host.
	Browser(hostname string) ConfigEntry
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

// AuthConfig is used for interacting with some persistent configuration for gh,
// with knowledge on how to access encrypted storage when neccesarry.
// Behavior is scoped to authentication specific tasks.
type AuthConfig interface {
	// HasActiveToken returns true when a token for the hostname is present.
	HasActiveToken(hostname string) bool

	// ActiveToken will retrieve the active auth token for the given hostname, searching environment variables,
	// general configuration, and finally encrypted storage.
	ActiveToken(hostname string) (token string, source string)

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

	// DefaultHost retrieves the default host.
	DefaultHost() (host string, source string)

	// Login will set user, git protocol, and auth token for the given hostname.
	//
	// If the encrypt option is specified it will first try to store the auth token
	// in encrypted storage and will fall back to the general insecure configuration.
	Login(hostname, username, token, gitProtocol string, secureStorage bool) (insecureStorageUsed bool, err error)

	// SwitchUser switches the active user for a given hostname.
	SwitchUser(hostname, user string) error

	// Logout will remove user, git protocol, and auth token for the given hostname.
	// It will remove the auth token from the encrypted storage if it exists there.
	Logout(hostname, username string) error

	// UsersForHost retrieves a list of users configured for a specific host.
	UsersForHost(hostname string) []string

	// TokenForUser retrieves the authentication token and its source for a specified user and hostname.
	TokenForUser(hostname, user string) (token string, source string, err error)

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
