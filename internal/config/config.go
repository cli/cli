package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cli/cli/v2/internal/keyring"
	ghAuth "github.com/cli/go-gh/v2/pkg/auth"
	ghConfig "github.com/cli/go-gh/v2/pkg/config"
)

const (
	aliases    = "aliases"
	hosts      = "hosts"
	oauthToken = "oauth_token"
)

// This interface describes interacting with some persistent configuration for gh.
//
//go:generate moq -rm -out config_mock.go . Config
type Config interface {
	Get(string, string) (string, error)
	GetOrDefault(string, string) (string, error)
	Set(string, string, string)
	Write() error

	Aliases() *AliasConfig
	Authentication() *AuthConfig

	String() string

	// Migrate takes any migration steps necessary, returning true if there were any
	// otherwise false. In the case of error, it will additionally return an error.
	Migrate() (bool, error)
	Revert() (bool, error)
}

func NewConfig() (Config, error) {
	c, err := ghConfig.Read()
	if err != nil {
		return nil, err
	}
	return &cfg{c}, nil
}

// Implements Config interface
type cfg struct {
	cfg *ghConfig.Config
}

func (c *cfg) String() string {
	return c.cfg.String()
}

func (c *cfg) Migrate() (bool, error) {
	return c.cfg.Migrate()
}

func (c *cfg) Revert() (bool, error) {
	return c.cfg.Revert()
}

func (c *cfg) Get(hostname, key string) (string, error) {
	if hostname != "" {
		val, err := c.cfg.Get([]string{hosts, hostname, key})
		if err == nil {
			return val, err
		}
	}

	return c.cfg.Get([]string{key})
}

func (c *cfg) GetOrDefault(hostname, key string) (string, error) {
	var val string
	var err error
	if hostname != "" {
		val, err = c.cfg.Get([]string{hosts, hostname, key})
		if err == nil {
			return val, err
		}
	}

	val, err = c.cfg.Get([]string{key})
	if err == nil {
		return val, err
	}

	if defaultExists(key) {
		return defaultFor(key), nil
	}

	return val, err
}

func (c *cfg) Set(hostname, key, value string) {
	if hostname == "" {
		c.cfg.Set([]string{key}, value)
		return
	}
	c.cfg.Set([]string{hosts, hostname, key}, value)
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

func defaultFor(key string) string {
	for _, co := range configOptions {
		if co.Key == key {
			return co.DefaultValue
		}
	}
	return ""
}

func defaultExists(key string) bool {
	for _, co := range configOptions {
		if co.Key == key {
			return true
		}
	}
	return false
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

// Token will retrieve the auth token for the given hostname,
// searching environment variables, plain text config, and
// lastly encrypted storage.
func (c *AuthConfig) Token(hostname string) (string, string) {
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
	token, _ := ghAuth.TokenFromEnvOrConfig(hostname)
	return token != ""
}

// SetToken will override any token resolution and return the given
// token and source for all calls to Token. Use for testing purposes only.
func (c *AuthConfig) SetToken(token, source string) {
	c.tokenOverride = func(_ string) (string, string) {
		return token, source
	}
}

// TokenFromKeyring will retrieve the auth token for the given hostname,
// only searching in encrypted storage.
func (c *AuthConfig) TokenFromKeyring(hostname string) (string, error) {
	activeUser, err := c.User(hostname)
	if err != nil {
		return "", fmt.Errorf("failed to get the active user from the hosts config for %s: %v", hostname, err)
	}

	return keyring.Get(keyringServiceName(hostname), activeUser)
}

// User will retrieve the username for the logged in user at the given hostname.
func (c *AuthConfig) User(hostname string) (string, error) {
	return c.cfg.Get([]string{hosts, hostname, "active_user"})
}

// GitProtocol will retrieve the git protocol for the logged in user at the given hostname.
// If none is set it will return the default value.
func (c *AuthConfig) GitProtocol(hostname string) (string, error) {
	activeUser, err := c.User(hostname)
	if err != nil {
		return "", fmt.Errorf("failed to get the active user from the hosts config for %s: %v", hostname, err)
	}

	key := "git_protocol"
	val, err := c.cfg.Get([]string{hosts, hostname, "users", activeUser, key})
	if err == nil {
		return val, err
	}
	return defaultFor(key), nil
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
	var setErr error
	if secureStorage {
		// TODO: What does providing the username to this keyring call actually do?
		if setErr = keyring.Set(keyringServiceName(hostname), username, token); setErr == nil {
			// Clean up the previous oauth_token from the config file.
			_ = c.cfg.Remove([]string{hosts, hostname, "users", username, oauthToken})
		}
	}
	insecureStorageUsed := false
	if !secureStorage || setErr != nil {
		c.cfg.Set([]string{hosts, hostname, "users", username, oauthToken}, token)
		insecureStorageUsed = true
	}

	if username != "" {
		// Set the user to be active, since we're logging in.
		c.cfg.Set([]string{hosts, hostname, "active_user"}, username)
	}

	// TODO: Can gitProtocol ever be set without username?
	if gitProtocol != "" {
		c.cfg.Set([]string{hosts, hostname, "users", username, "git_protocol"}, gitProtocol)
	}
	return insecureStorageUsed, ghConfig.Write(c.cfg)
}

// Logout will remove user, git protocol, and auth token for the given hostname.
// It will remove the auth token from the encrypted storage if it exists there.
// TODO: This needs to take a username, or if not, then it needs to remove the currently
// active user
func (c *AuthConfig) Logout(hostname string) error {
	if hostname == "" {
		return nil
	}

	// Get all the logged in users for this host, that way we can set one to active if there is one,
	// otherwise delete the entire host.
	users, err := c.cfg.Keys([]string{hosts, hostname, "users"})
	if err != nil {
		return fmt.Errorf("failed to get the active user from the hosts config for %s: %v", hostname, err)
	}

	// Get the active user in order to delete the entry for that user, or to get their username
	// so that we can delete their entry for the keyring. Probably this should use the .User() method TODO
	activeUser, err := c.User(hostname)
	if err != nil {
		return fmt.Errorf("failed to get the active user from the hosts config for %s: %v", hostname, err)
	}

	// Remove the keyring entry for this user
	_ = keyring.Delete(keyringServiceName(hostname), activeUser)

	// If we remove the active user from the list of users and there are none left, then
	// we should remove the entire host entry.
	users = remove(users, activeUser)
	if len(users) == 0 {
		_ = c.cfg.Remove([]string{hosts, hostname})
		return ghConfig.Write(c.cfg)
	}

	// If we have multiple entries then we should delete that one user from config,
	// and pick another to set as the active user.
	_ = c.cfg.Remove([]string{hosts, hostname, "users", activeUser})
	c.cfg.Set([]string{hosts, hostname, "active_user"}, users[0])

	return ghConfig.Write(c.cfg)
}

func keyringServiceName(hostname string) string {
	return "gh:" + hostname
}

type AliasConfig struct {
	cfg *ghConfig.Config
}

func (a *AliasConfig) Get(alias string) (string, error) {
	return a.cfg.Get([]string{aliases, alias})
}

func (a *AliasConfig) Add(alias, expansion string) {
	a.cfg.Set([]string{aliases, alias}, expansion)
}

func (a *AliasConfig) Delete(alias string) error {
	return a.cfg.Remove([]string{aliases, alias})
}

func (a *AliasConfig) All() map[string]string {
	out := map[string]string{}
	keys, err := a.cfg.Keys([]string{aliases})
	if err != nil {
		return out
	}
	for _, key := range keys {
		val, _ := a.cfg.Get([]string{aliases, key})
		out[key] = val
	}
	return out
}

type ConfigOption struct {
	Key           string
	Description   string
	DefaultValue  string
	AllowedValues []string
}

var configOptions = []ConfigOption{
	{
		Key:           "git_protocol",
		Description:   "the protocol to use for git clone and push operations",
		DefaultValue:  "https",
		AllowedValues: []string{"https", "ssh"},
	},
	{
		Key:          "editor",
		Description:  "the text editor program to use for authoring text",
		DefaultValue: "",
	},
	{
		Key:           "prompt",
		Description:   "toggle interactive prompting in the terminal",
		DefaultValue:  "enabled",
		AllowedValues: []string{"enabled", "disabled"},
	},
	{
		Key:          "pager",
		Description:  "the terminal pager program to send standard output to",
		DefaultValue: "",
	},
	{
		Key:          "http_unix_socket",
		Description:  "the path to a Unix socket through which to make an HTTP connection",
		DefaultValue: "",
	},
	{
		Key:          "browser",
		Description:  "the web browser to use for opening URLs",
		DefaultValue: "",
	},
}

func ConfigOptions() []ConfigOption {
	return configOptions
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

func remove[T comparable](l []T, item T) []T {
	out := make([]T, 0)
	for _, element := range l {
		if element != item {
			out = append(out, element)
		}
	}
	return out
}
