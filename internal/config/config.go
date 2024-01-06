package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/cli/cli/v2/internal/keyring"
	ghAuth "github.com/cli/go-gh/v2/pkg/auth"
	ghConfig "github.com/cli/go-gh/v2/pkg/config"
)

const (
	aliasesKey        = "aliases"
	browserKey        = "browser"
	editorKey         = "editor"
	gitProtocolKey    = "git_protocol"
	hostsKey          = "hosts"
	httpUnixSocketKey = "http_unix_socket"
	oauthTokenKey     = "oauth_token"
	pagerKey          = "pager"
	promptKey         = "prompt"
	userKey           = "user"
	usersKey          = "users"
	versionKey        = "version"
)

// This interface describes interacting with some persistent configuration for gh.
//
//go:generate moq -rm -out config_mock.go . Config
type Config interface {
	GetOrDefault(string, string) (string, error)
	Set(string, string, string)
	Write() error
	Migrate(Migration) error

	Aliases() *AliasConfig
	Authentication() *AuthConfig
	Browser(string) string
	Editor(string) string
	GitProtocol(string) string
	HTTPUnixSocket(string) string
	Pager(string) string
	Prompt(string) string
	Version() string
}

// Migration is the interace that config migrations must implement.
//
// Migrations will receive a copy of the config, and should modify that copy
// as necessary. After migration has completed, the modified config contents
// will be used.
//
// The calling code is expected to verify that the current version of the config
// matches the PreVersion of the migration before calling Do, and will set the
// config version to the PostVersion after the migration has completed successfully.
//
//go:generate moq -rm -out migration_mock.go . Migration
type Migration interface {
	// PreVersion is the required config version for this to be applied
	PreVersion() string
	// PostVersion is the config version that must be applied after migration
	PostVersion() string
	// Do is expected to apply any necessary changes to the config in place
	Do(*ghConfig.Config) error
}

func NewConfig() (Config, error) {
	c, err := ghConfig.Read(fallbackConfig())
	if err != nil {
		return nil, err
	}
	return &cfg{c}, nil
}

// Implements Config interface
type cfg struct {
	cfg *ghConfig.Config
}

func (c *cfg) Get(hostname, key string) (string, error) {
	if hostname != "" {
		val, err := c.cfg.Get([]string{hostsKey, hostname, key})
		if err == nil {
			return val, err
		}
	}

	return c.cfg.Get([]string{key})
}

func (c *cfg) GetOrDefault(hostname, key string) (string, error) {
	val, err := c.Get(hostname, key)
	if err == nil {
		return val, err
	}

	if val, ok := defaultFor(key); ok {
		return val, nil
	}

	return val, err
}

func (c *cfg) Set(hostname, key, value string) {
	if hostname == "" {
		c.cfg.Set([]string{key}, value)
		return
	}

	c.cfg.Set([]string{hostsKey, hostname, key}, value)

	if user, _ := c.cfg.Get([]string{hostsKey, hostname, userKey}); user != "" {
		c.cfg.Set([]string{hostsKey, hostname, usersKey, user, key}, value)
	}
}

func (c *cfg) Write() error {
	return ghConfig.Write(c.cfg)
}

func (c *cfg) Aliases() *AliasConfig {
	return &AliasConfig{cfg: c.cfg}
}

func (c *cfg) Authentication() *AuthConfig {
	return &AuthConfig{cfg: c.cfg}
}

func (c *cfg) Browser(hostname string) string {
	val, _ := c.GetOrDefault(hostname, browserKey)
	return val
}

func (c *cfg) Editor(hostname string) string {
	val, _ := c.GetOrDefault(hostname, editorKey)
	return val
}

func (c *cfg) GitProtocol(hostname string) string {
	val, _ := c.GetOrDefault(hostname, gitProtocolKey)
	return val
}

func (c *cfg) HTTPUnixSocket(hostname string) string {
	val, _ := c.GetOrDefault(hostname, httpUnixSocketKey)
	return val
}

func (c *cfg) Pager(hostname string) string {
	val, _ := c.GetOrDefault(hostname, pagerKey)
	return val
}

func (c *cfg) Prompt(hostname string) string {
	val, _ := c.GetOrDefault(hostname, promptKey)
	return val
}

func (c *cfg) Version() string {
	val, _ := c.GetOrDefault("", versionKey)
	return val
}

func (c *cfg) Migrate(m Migration) error {
	version := c.Version()

	// If migration has already occured then do not attempt to migrate again.
	if m.PostVersion() == version {
		return nil
	}

	// If migration is incompatible with current version then return an error.
	if m.PreVersion() != version {
		return fmt.Errorf("failed to migrate as %q pre migration version did not match config version %q", m.PreVersion(), version)
	}

	if err := m.Do(c.cfg); err != nil {
		return fmt.Errorf("failed to migrate config: %s", err)
	}

	c.Set("", versionKey, m.PostVersion())

	// Then write out our migrated config.
	if err := c.Write(); err != nil {
		return fmt.Errorf("failed to write config after migration: %s", err)
	}

	return nil
}

func defaultFor(key string) (string, bool) {
	for _, co := range ConfigOptions() {
		if co.Key == key {
			return co.DefaultValue, true
		}
	}
	return "", false
}

// AuthConfig is used for interacting with some persistent configuration for gh,
// with knowledge on how to access encrypted storage when neccesarry.
// Behavior is scoped to authentication specific tasks.
type AuthConfig struct {
	cfg                 *ghConfig.Config
	defaultHostOverride func() (string, string)
	hostsOverride       func() []string
	tokenOverride       func(string) (string, string)
}

// ActiveToken will retrieve the active auth token for the given hostname,
// searching environment variables, plain text config, and
// lastly encrypted storage.
func (c *AuthConfig) ActiveToken(hostname string) (string, string) {
	if c.tokenOverride != nil {
		return c.tokenOverride(hostname)
	}
	token, source := ghAuth.TokenFromEnvOrConfig(hostname)
	if token == "" {
		var err error
		token, err = c.TokenFromKeyring(hostname)
		if err == nil {
			source = "keyring"
		}
	}
	return token, source
}

// HasEnvToken returns true when a token has been specified in an
// environment variable, else returns false.
func (c *AuthConfig) HasEnvToken() bool {
	// This will check if there are any environment variable
	// authentication tokens set for enterprise hosts.
	// Any non-github.com hostname is fine here
	hostname := "example.com"
	if c.tokenOverride != nil {
		token, _ := c.tokenOverride(hostname)
		if token != "" {
			return true
		}
	}
	// TODO: This is _extremely_ knowledgable about the implementation of TokenFromEnvOrConfig
	// It has to use a hostname that is not going to be found in the hosts so that it
	// can guarantee that tokens will only be returned from a set env var.
	// Discussed here, but maybe worth revisiting: https://github.com/cli/cli/pull/7169#discussion_r1136979033
	token, _ := ghAuth.TokenFromEnvOrConfig(hostname)
	return token != ""
}

// SetActiveToken will override any token resolution and return the given
// token and source for all calls to ActiveToken. Use for testing purposes only.
func (c *AuthConfig) SetActiveToken(token, source string) {
	c.tokenOverride = func(_ string) (string, string) {
		return token, source
	}
}

// TokenFromKeyring will retrieve the auth token for the given hostname,
// only searching in encrypted storage.
func (c *AuthConfig) TokenFromKeyring(hostname string) (string, error) {
	return keyring.Get(keyringServiceName(hostname), "")
}

// TokenFromKeyringForUser will retrieve the auth token for the given hostname
// and username, only searching in encrypted storage.
//
// An empty username will return an error because the potential to return
// the currently active token under surprising cases is just too high to risk
// compared to the utility of having the function being smart.
func (c *AuthConfig) TokenFromKeyringForUser(hostname, username string) (string, error) {
	if username == "" {
		return "", errors.New("username cannot be blank")
	}

	return keyring.Get(keyringServiceName(hostname), username)
}

// ActiveUser will retrieve the username for the active user at the given hostname.
// This will not be accurate if the oauth token is set from an environment variable.
func (c *AuthConfig) ActiveUser(hostname string) (string, error) {
	return c.cfg.Get([]string{hostsKey, hostname, userKey})
}

func (c *AuthConfig) Hosts() []string {
	if c.hostsOverride != nil {
		return c.hostsOverride()
	}
	return ghAuth.KnownHosts()
}

// SetHosts will override any hosts resolution and return the given
// hosts for all calls to Hosts. Use for testing purposes only.
func (c *AuthConfig) SetHosts(hosts []string) {
	c.hostsOverride = func() []string {
		return hosts
	}
}

func (c *AuthConfig) DefaultHost() (string, string) {
	if c.defaultHostOverride != nil {
		return c.defaultHostOverride()
	}
	return ghAuth.DefaultHost()
}

// SetDefaultHost will override any host resolution and return the given
// host and source for all calls to DefaultHost. Use for testing purposes only.
func (c *AuthConfig) SetDefaultHost(host, source string) {
	c.defaultHostOverride = func() (string, string) {
		return host, source
	}
}

// Login will set user, git protocol, and auth token for the given hostname.
// If the encrypt option is specified it will first try to store the auth token
// in encrypted storage and will fall back to the plain text config file.
func (c *AuthConfig) Login(hostname, username, token, gitProtocol string, secureStorage bool) (bool, error) {
	// In this section we set up the users config
	var setErr error
	if secureStorage {
		// Try to set the token for this user in the encrypted storage for later switching
		setErr = keyring.Set(keyringServiceName(hostname), username, token)
		if setErr == nil {
			// Clean up the previous oauth_token from the config file, if there were one
			_ = c.cfg.Remove([]string{hostsKey, hostname, usersKey, username, oauthTokenKey})
		}
	}
	insecureStorageUsed := false
	if !secureStorage || setErr != nil {
		// And set the oauth token under the user for later switching
		c.cfg.Set([]string{hostsKey, hostname, usersKey, username, oauthTokenKey}, token)
		insecureStorageUsed = true
	}

	if gitProtocol != "" {
		// Set the host level git protocol
		// Although it might be expected that this is handled by switch, git protocol
		// is currently a host level config and not a user level config, so any change
		// will overwrite the protocol for all users on the host.
		c.cfg.Set([]string{hostsKey, hostname, gitProtocolKey}, gitProtocol)
	}

	// Create the username key with an empty value so it will be
	// written even when there are no keys set under it.
	if _, getErr := c.cfg.Get([]string{hostsKey, hostname, usersKey, username}); getErr != nil {
		c.cfg.Set([]string{hostsKey, hostname, usersKey, username}, "")
	}

	// Then we activate the new user
	return insecureStorageUsed, c.activateUser(hostname, username)
}

func (c *AuthConfig) SwitchUser(hostname, user string) error {
	previouslyActiveUser, err := c.ActiveUser(hostname)
	if err != nil {
		return fmt.Errorf("failed to get active user: %s", err)
	}

	previouslyActiveToken, previousSource := c.ActiveToken(hostname)
	if previousSource != "keyring" && previousSource != "oauth_token" {
		return fmt.Errorf("currently active token for %s is from %s", hostname, previousSource)
	}

	err = c.activateUser(hostname, user)
	if err != nil {
		// Given that activateUser can only fail before the config is written, or when writing the config
		// we know for sure that the config has not been written. However, we still should restore it back
		// to its previous clean state just in case something else tries to make use of the config, or tries
		// to write it again.
		if previousSource == "keyring" {
			if setErr := keyring.Set(keyringServiceName(hostname), "", previouslyActiveToken); setErr != nil {
				err = errors.Join(err, setErr)
			}
		}

		if previousSource == "oauth_token" {
			c.cfg.Set([]string{hostsKey, hostname, oauthTokenKey}, previouslyActiveToken)
		}
		c.cfg.Set([]string{hostsKey, hostname, userKey}, previouslyActiveUser)

		return err
	}

	return nil
}

// Logout will remove user, git protocol, and auth token for the given hostname.
// It will remove the auth token from the encrypted storage if it exists there.
func (c *AuthConfig) Logout(hostname, username string) error {
	users := c.UsersForHost(hostname)

	// If there is only one (or zero) users, then we remove the host
	// and unset the keyring tokens.
	if len(users) < 2 {
		_ = c.cfg.Remove([]string{hostsKey, hostname})
		_ = keyring.Delete(keyringServiceName(hostname), "")
		_ = keyring.Delete(keyringServiceName(hostname), username)
		return ghConfig.Write(c.cfg)
	}

	// Otherwise, we remove the user from this host
	_ = c.cfg.Remove([]string{hostsKey, hostname, usersKey, username})

	// This error is ignorable because we already know there is an active user for the host
	activeUser, _ := c.ActiveUser(hostname)

	// If the user we're removing isn't active, then we just write the config
	if activeUser != username {
		return ghConfig.Write(c.cfg)
	}

	// Otherwise we get the first user in the slice that isn't the user we're removing
	switchUserIdx := slices.IndexFunc(users, func(n string) bool {
		return n != username
	})

	// And activate them
	return c.activateUser(hostname, users[switchUserIdx])
}

func (c *AuthConfig) activateUser(hostname, user string) error {
	// We first need to idempotently clear out any set tokens for the host
	_ = keyring.Delete(keyringServiceName(hostname), "")
	_ = c.cfg.Remove([]string{hostsKey, hostname, oauthTokenKey})

	// Then we'll move the keyring token or insecure token as necessary, only one of the
	// following branches should be true.

	// If there is a token in the secure keyring for the user, move it to the active slot
	var tokenSwitched bool
	if token, err := keyring.Get(keyringServiceName(hostname), user); err == nil {
		if err = keyring.Set(keyringServiceName(hostname), "", token); err != nil {
			return fmt.Errorf("failed to move active token in keyring: %v", err)
		}
		tokenSwitched = true
	}

	// If there is a token in the insecure config for the user, move it to the active field
	if token, err := c.cfg.Get([]string{hostsKey, hostname, usersKey, user, oauthTokenKey}); err == nil {
		c.cfg.Set([]string{hostsKey, hostname, oauthTokenKey}, token)
		tokenSwitched = true
	}

	if !tokenSwitched {
		return fmt.Errorf("no token found for %s", user)
	}

	// Then we'll update the active user for the host
	c.cfg.Set([]string{hostsKey, hostname, userKey}, user)

	return ghConfig.Write(c.cfg)
}

func (c *AuthConfig) UsersForHost(hostname string) []string {
	users, err := c.cfg.Keys([]string{hostsKey, hostname, usersKey})
	if err != nil {
		return nil
	}

	return users
}

func (c *AuthConfig) TokenForUser(hostname, user string) (string, string, error) {
	if token, err := keyring.Get(keyringServiceName(hostname), user); err == nil {
		return token, "keyring", nil
	}

	if token, err := c.cfg.Get([]string{hostsKey, hostname, usersKey, user, oauthTokenKey}); err == nil {
		return token, "oauth_token", nil
	}

	return "", "default", fmt.Errorf("no token found for '%s'", user)
}

func keyringServiceName(hostname string) string {
	return "gh:" + hostname
}

type AliasConfig struct {
	cfg *ghConfig.Config
}

func (a *AliasConfig) Get(alias string) (string, error) {
	return a.cfg.Get([]string{aliasesKey, alias})
}

func (a *AliasConfig) Add(alias, expansion string) {
	a.cfg.Set([]string{aliasesKey, alias}, expansion)
}

func (a *AliasConfig) Delete(alias string) error {
	return a.cfg.Remove([]string{aliasesKey, alias})
}

func (a *AliasConfig) All() map[string]string {
	out := map[string]string{}
	keys, err := a.cfg.Keys([]string{aliasesKey})
	if err != nil {
		return out
	}
	for _, key := range keys {
		val, _ := a.cfg.Get([]string{aliasesKey, key})
		out[key] = val
	}
	return out
}

func fallbackConfig() *ghConfig.Config {
	return ghConfig.ReadFromString(defaultConfigStr)
}

// The schema version in here should match the PostVersion of whatever the
// last migration we decided to run is. Therefore, if we run a new migration,
// this should be bumped.
const defaultConfigStr = `
# The current version of the config schema
version: 1
# What protocol to use when performing git operations. Supported values: ssh, https
git_protocol: https
# What editor gh should run when creating issues, pull requests, etc. If blank, will refer to environment.
editor:
# When to interactively prompt. This is a global config that cannot be overridden by hostname. Supported values: enabled, disabled
prompt: enabled
# A pager program to send command output to, e.g. "less". Set the value to "cat" to disable the pager.
pager:
# Aliases allow you to create nicknames for gh commands
aliases:
  co: pr checkout
# The path to a unix socket through which send HTTP connections. If blank, HTTP traffic will be handled by net/http.DefaultTransport.
http_unix_socket:
# What web browser gh should use when opening URLs. If blank, will refer to environment.
browser:
`

type ConfigOption struct {
	Key           string
	Description   string
	DefaultValue  string
	AllowedValues []string
}

func ConfigOptions() []ConfigOption {
	return []ConfigOption{
		{
			Key:           gitProtocolKey,
			Description:   "the protocol to use for git clone and push operations",
			DefaultValue:  "https",
			AllowedValues: []string{"https", "ssh"},
		},
		{
			Key:          editorKey,
			Description:  "the text editor program to use for authoring text",
			DefaultValue: "",
		},
		{
			Key:           promptKey,
			Description:   "toggle interactive prompting in the terminal",
			DefaultValue:  "enabled",
			AllowedValues: []string{"enabled", "disabled"},
		},
		{
			Key:          pagerKey,
			Description:  "the terminal pager program to send standard output to",
			DefaultValue: "",
		},
		{
			Key:          httpUnixSocketKey,
			Description:  "the path to a Unix socket through which to make an HTTP connection",
			DefaultValue: "",
		},
		{
			Key:          browserKey,
			Description:  "the web browser to use for opening URLs",
			DefaultValue: "",
		},
	}
}

func HomeDirPath(subdir string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	newPath := filepath.Join(homeDir, subdir)
	return newPath, nil
}

func StateDir() string {
	return ghConfig.StateDir()
}

func DataDir() string {
	return ghConfig.DataDir()
}

func ConfigDir() string {
	return ghConfig.ConfigDir()
}
