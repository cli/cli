package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/keyring"
	o "github.com/cli/cli/v2/pkg/option"
	ghauth "github.com/cli/go-gh/v2/pkg/auth"
	ghConfig "github.com/cli/go-gh/v2/pkg/config"
)

const (
	aliasesKey            = "aliases"
	browserKey            = "browser"
	editorKey             = "editor"
	gitProtocolKey        = "git_protocol"
	hostsKey              = "hosts"
	httpUnixSocketKey     = "http_unix_socket"
	oauthTokenKey         = "oauth_token"
	pagerKey              = "pager"
	promptKey             = "prompt"
	preferEditorPromptKey = "prefer_editor_prompt"
	userKey               = "user"
	usersKey              = "users"
	versionKey            = "version"
)

func NewConfig() (gh.Config, error) {
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

func (c *cfg) get(hostname, key string) o.Option[string] {
	if hostname != "" {
		val, err := c.cfg.Get([]string{hostsKey, hostname, key})
		if err == nil {
			return o.Some(val)
		}
	}

	val, err := c.cfg.Get([]string{key})
	if err == nil {
		return o.Some(val)
	}

	return o.None[string]()
}

func (c *cfg) GetOrDefault(hostname, key string) o.Option[gh.ConfigEntry] {
	if val := c.get(hostname, key); val.IsSome() {
		// Map the Option[string] to Option[gh.ConfigEntry] with a source of ConfigUserProvided
		return o.Map(val, toConfigEntry(gh.ConfigUserProvided))
	}

	if defaultVal := defaultFor(key); defaultVal.IsSome() {
		// Map the Option[string] to Option[gh.ConfigEntry] with a source of ConfigDefaultProvided
		return o.Map(defaultVal, toConfigEntry(gh.ConfigDefaultProvided))
	}

	return o.None[gh.ConfigEntry]()
}

// toConfigEntry is a helper function to convert a string value to a ConfigEntry with a given source.
//
// It's a bit of FP style but it allows us to map an Option[string] to Option[gh.ConfigEntry] without
// unwrapping the it and rewrapping it.
func toConfigEntry(source gh.ConfigSource) func(val string) gh.ConfigEntry {
	return func(val string) gh.ConfigEntry {
		return gh.ConfigEntry{Value: val, Source: source}
	}
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

func (c *cfg) Aliases() gh.AliasConfig {
	return &AliasConfig{cfg: c.cfg}
}

func (c *cfg) Authentication() gh.AuthConfig {
	return &AuthConfig{cfg: c.cfg}
}

func (c *cfg) Browser(hostname string) gh.ConfigEntry {
	// Intentionally panic if there is no user provided value or default value (which would be a programmer error)
	return c.GetOrDefault(hostname, browserKey).Unwrap()
}

func (c *cfg) Editor(hostname string) gh.ConfigEntry {
	// Intentionally panic if there is no user provided value or default value (which would be a programmer error)
	return c.GetOrDefault(hostname, editorKey).Unwrap()
}

func (c *cfg) GitProtocol(hostname string) gh.ConfigEntry {
	// Intentionally panic if there is no user provided value or default value (which would be a programmer error)
	return c.GetOrDefault(hostname, gitProtocolKey).Unwrap()
}

func (c *cfg) HTTPUnixSocket(hostname string) gh.ConfigEntry {
	// Intentionally panic if there is no user provided value or default value (which would be a programmer error)
	return c.GetOrDefault(hostname, httpUnixSocketKey).Unwrap()
}

func (c *cfg) Pager(hostname string) gh.ConfigEntry {
	// Intentionally panic if there is no user provided value or default value (which would be a programmer error)
	return c.GetOrDefault(hostname, pagerKey).Unwrap()
}

func (c *cfg) Prompt(hostname string) gh.ConfigEntry {
	// Intentionally panic if there is no user provided value or default value (which would be a programmer error)
	return c.GetOrDefault(hostname, promptKey).Unwrap()
}

func (c *cfg) PreferEditorPrompt(hostname string) gh.ConfigEntry {
	// Intentionally panic if there is no user provided value or default value (which would be a programmer error)
	return c.GetOrDefault(hostname, preferEditorPromptKey).Unwrap()
}

func (c *cfg) Version() o.Option[string] {
	return c.get("", versionKey)
}

func (c *cfg) Migrate(m gh.Migration) error {
	// If there is no version entry we must never have applied a migration, and the following conditional logic
	// handles the version as an empty string correctly.
	version := c.Version().UnwrapOrZero()

	// If migration has already occurred then do not attempt to migrate again.
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

func (c *cfg) CacheDir() string {
	return ghConfig.CacheDir()
}

func defaultFor(key string) o.Option[string] {
	for _, co := range Options {
		if co.Key == key {
			return o.Some(co.DefaultValue)
		}
	}
	return o.None[string]()
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
	token, source := ghauth.TokenFromEnvOrConfig(hostname)
	if token == "" {
		var err error
		token, err = c.TokenFromKeyring(hostname)
		if err == nil {
			source = "keyring"
		}
	}
	return token, source
}

// HasActiveToken returns true when a token for the hostname is present.
func (c *AuthConfig) HasActiveToken(hostname string) bool {
	token, _ := c.ActiveToken(hostname)
	return token != ""
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
	// TODO: This is _extremely_ knowledgeable about the implementation of TokenFromEnvOrConfig
	// It has to use a hostname that is not going to be found in the hosts so that it
	// can guarantee that tokens will only be returned from a set env var.
	// Discussed here, but maybe worth revisiting: https://github.com/cli/cli/pull/7169#discussion_r1136979033
	token, _ := ghauth.TokenFromEnvOrConfig(hostname)
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
	return ghauth.KnownHosts()
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
	return ghauth.DefaultHost()
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
# The default config file, auto-generated by gh. Run 'gh environment' to learn more about
# environment variables respected by gh and their precedence.

# The current version of the config schema
version: 1
# What protocol to use when performing git operations. Supported values: ssh, https
git_protocol: https
# What editor gh should run when creating issues, pull requests, etc. If blank, will refer to environment.
editor:
# When to interactively prompt. This is a global config that cannot be overridden by hostname. Supported values: enabled, disabled
prompt: enabled
# Preference for editor-based interactive prompting. This is a global config that cannot be overridden by hostname. Supported values: enabled, disabled
prefer_editor_prompt: disabled
# A pager program to send command output to, e.g. "less". If blank, will refer to environment. Set the value to "cat" to disable the pager.
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
	CurrentValue  func(c gh.Config, hostname string) string
}

var Options = []ConfigOption{
	{
		Key:           gitProtocolKey,
		Description:   "the protocol to use for git clone and push operations",
		DefaultValue:  "https",
		AllowedValues: []string{"https", "ssh"},
		CurrentValue: func(c gh.Config, hostname string) string {
			return c.GitProtocol(hostname).Value
		},
	},
	{
		Key:          editorKey,
		Description:  "the text editor program to use for authoring text",
		DefaultValue: "",
		CurrentValue: func(c gh.Config, hostname string) string {
			return c.Editor(hostname).Value
		},
	},
	{
		Key:           promptKey,
		Description:   "toggle interactive prompting in the terminal",
		DefaultValue:  "enabled",
		AllowedValues: []string{"enabled", "disabled"},
		CurrentValue: func(c gh.Config, hostname string) string {
			return c.Prompt(hostname).Value
		},
	},
	{
		Key:           preferEditorPromptKey,
		Description:   "toggle preference for editor-based interactive prompting in the terminal",
		DefaultValue:  "disabled",
		AllowedValues: []string{"enabled", "disabled"},
		CurrentValue: func(c gh.Config, hostname string) string {
			return c.PreferEditorPrompt(hostname).Value
		},
	},
	{
		Key:          pagerKey,
		Description:  "the terminal pager program to send standard output to",
		DefaultValue: "",
		CurrentValue: func(c gh.Config, hostname string) string {
			return c.Pager(hostname).Value
		},
	},
	{
		Key:          httpUnixSocketKey,
		Description:  "the path to a Unix socket through which to make an HTTP connection",
		DefaultValue: "",
		CurrentValue: func(c gh.Config, hostname string) string {
			return c.HTTPUnixSocket(hostname).Value
		},
	},
	{
		Key:          browserKey,
		Description:  "the web browser to use for opening URLs",
		DefaultValue: "",
		CurrentValue: func(c gh.Config, hostname string) string {
			return c.Browser(hostname).Value
		},
	},
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
