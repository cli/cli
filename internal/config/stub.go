package config

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	ghConfig "github.com/cli/go-gh/v2/pkg/config"
)

func NewBlankConfig() *ConfigMock {
	defaultStr := `
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
	return NewFromString(defaultStr)
}

func NewFromString(cfgStr string) *ConfigMock {
	c := ghConfig.ReadFromString(cfgStr)
	cfg := cfg{c}
	mock := &ConfigMock{}
	mock.GetFunc = func(host, key string) (string, error) {
		return cfg.Get(host, key)
	}
	mock.GetOrDefaultFunc = func(host, key string) (string, error) {
		return cfg.GetOrDefault(host, key)
	}
	mock.SetFunc = func(host, key, value string) {
		cfg.Set(host, key, value)
	}
	mock.WriteFunc = func() error {
		return cfg.Write()
	}
	mock.AliasesFunc = func() *AliasConfig {
		return &AliasConfig{cfg: c}
	}
	mock.AuthenticationFunc = func() *AuthConfig {
		return &AuthConfig{
			cfg: c,
			defaultHostOverride: func() (string, string) {
				return "github.com", "default"
			},
			hostsOverride: func() []string {
				keys, _ := c.Keys([]string{"hosts"})
				return keys
			},
			tokenOverride: func(hostname string) (string, string) {
				token, _ := c.Get([]string{hosts, hostname, oauthToken})
				return token, "oauth_token"
			},
		}
	}
	return mock
}

// StubWriteConfig stubs out the filesystem where config file are written.
// It then returns a function that will read in the config files into io.Writers.
// It automatically cleans up environment variables and written files.
func StubWriteConfig(t *testing.T) func(io.Writer, io.Writer) {
	t.Helper()
	tempDir := t.TempDir()
	t.Setenv("GH_CONFIG_DIR", tempDir)
	return func(wc io.Writer, wh io.Writer) {
		config, err := os.Open(filepath.Join(tempDir, "config.yml"))
		if err != nil {
			return
		}
		defer config.Close()
		configData, err := io.ReadAll(config)
		if err != nil {
			return
		}
		_, err = wc.Write(configData)
		if err != nil {
			return
		}

		hosts, err := os.Open(filepath.Join(tempDir, "hosts.yml"))
		if err != nil {
			return
		}
		defer hosts.Close()
		hostsData, err := io.ReadAll(hosts)
		if err != nil {
			return
		}
		_, err = wh.Write(hostsData)
		if err != nil {
			return
		}
	}
}
