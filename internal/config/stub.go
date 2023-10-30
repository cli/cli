package config

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	ghConfig "github.com/cli/go-gh/v2/pkg/config"
)

func NewBlankConfig() *ConfigMock {
	return NewFromString(defaultConfigStr)
}

func NewFromString(cfgStr string) *ConfigMock {
	c := ghConfig.ReadFromString(cfgStr)
	cfg := cfg{c}
	mock := &ConfigMock{}
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
				keys, _ := c.Keys([]string{hostsKey})
				return keys
			},
			tokenOverride: func(hostname string) (string, string) {
				token, _ := c.Get([]string{hostsKey, hostname, oauthTokenKey})
				return token, oauthTokenKey
			},
		}
	}
	mock.BrowserFunc = func(hostname string) string {
		val, _ := cfg.GetOrDefault(hostname, browserKey)
		return val
	}
	mock.EditorFunc = func(hostname string) string {
		val, _ := cfg.GetOrDefault(hostname, editorKey)
		return val
	}
	mock.GitProtocolFunc = func(hostname string) string {
		val, _ := cfg.GetOrDefault(hostname, gitProtocolKey)
		return val
	}
	mock.HTTPUnixSocketFunc = func(hostname string) string {
		val, _ := cfg.GetOrDefault(hostname, httpUnixSocketKey)
		return val
	}
	mock.PagerFunc = func(hostname string) string {
		val, _ := cfg.GetOrDefault(hostname, pagerKey)
		return val
	}
	mock.PromptFunc = func(hostname string) string {
		val, _ := cfg.GetOrDefault(hostname, promptKey)
		return val
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
