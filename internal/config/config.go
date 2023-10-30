package config

import (
	"os"
	"path/filepath"

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
)

// This interface describes interacting with some persistent configuration for gh.
//
//go:generate moq -rm -out config_mock.go . Config
type Config interface {
	GetOrDefault(string, string) (string, error)
	Set(string, string, string)
	Write() error

	Aliases() *AliasConfig
	Authentication() *AuthConfig
	Browser(string) string
	Editor(string) string
	GitProtocol(string) string
	HTTPUnixSocket(string) string
	Pager(string) string
	Prompt(string) string
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
	// TODO: This is _extremely_ knowledgable about the implementation of TokenFromEnvOrConfig
	// It has to use a hostname that is not going to be found in the hosts so that it
	// can guarantee that tokens will only be returned from a set env var.
	// Discussed here, but maybe worth revisiting: https://github.com/cli/cli/pull/7169#discussion_r1136979033
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
	return keyring.Get(keyringServiceName(hostname), "")
}

// User will retrieve the username for the logged in user at the given hostname.
func (c *AuthConfig) User(hostname string) (string, error) {
	return c.cfg.Get([]string{hostsKey, hostname, "user"})
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
		if setErr = keyring.Set(keyringServiceName(hostname), "", token); setErr == nil {
			// Clean up the previous oauth_token from the config file.
			_ = c.cfg.Remove([]string{hostsKey, hostname, oauthTokenKey})
		}
	}
	insecureStorageUsed := false
	if !secureStorage || setErr != nil {
		c.cfg.Set([]string{hostsKey, hostname, oauthTokenKey}, token)
		insecureStorageUsed = true
	}

	c.cfg.Set([]string{hostsKey, hostname, "user"}, username)

	if gitProtocol != "" {
		c.cfg.Set([]string{hostsKey, hostname, gitProtocolKey}, gitProtocol)
	}
	return insecureStorageUsed, ghConfig.Write(c.cfg)
}

// Logout will remove user, git protocol, and auth token for the given hostname.
// It will remove the auth token from the encrypted storage if it exists there.
func (c *AuthConfig) Logout(hostname string) error {
	_ = c.cfg.Remove([]string{hostsKey, hostname})
	_ = keyring.Delete(keyringServiceName(hostname), "")
	return ghConfig.Write(c.cfg)
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

const defaultConfigStr = `
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
