package config

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/internal/gh"
	ghmock "github.com/cli/cli/v2/internal/gh/mock"
	"github.com/cli/cli/v2/internal/keyring"
	o "github.com/cli/cli/v2/pkg/option"
	ghConfig "github.com/cli/go-gh/v2/pkg/config"
)

// NewMockConfig returns a mock config populated with gh's default config file.
// See NewMockConfigFromString for when to prefer a mock over NewIsolatedTestConfig.
func NewMockConfig() *ghmock.ConfigMock {
	return NewMockConfigFromString(defaultConfigStr)
}

// NewMockConfigFromString returns a mock config populated from cfgString, for tests
// that need to stub config behaviour by assigning to the mock's function fields.
//
// The mock answers host, token, and default host lookups from cfgString alone, so it
// ignores both the config files on disk and the environment. It never writes anything.
//
// Prefer NewIsolatedTestConfig when the code under test exercises the real config
// implementation, writes config, or reads the auth environment variables directly,
// since none of those go through the mock.
func NewMockConfigFromString(cfgString string) *ghmock.ConfigMock {
	c := ghConfig.ReadFromString(cfgString)
	cfg := cfg{c}
	mock := &ghmock.ConfigMock{}
	mock.GetOrDefaultFunc = func(host, key string) o.Option[gh.ConfigEntry] {
		return cfg.GetOrDefault(host, key)
	}
	mock.SetFunc = func(host, key, value string) {
		cfg.Set(host, key, value)
	}
	mock.WriteFunc = func() error {
		return cfg.Write()
	}
	mock.MigrateFunc = func(m gh.Migration) error {
		return cfg.Migrate(m)
	}
	mock.AliasesFunc = func() gh.AliasConfig {
		return &AliasConfig{cfg: c}
	}
	mock.AuthenticationFunc = func() gh.AuthConfig {
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
	mock.AccessibleColorsFunc = func(hostname string) gh.ConfigEntry {
		return cfg.AccessibleColors(hostname)
	}
	mock.AccessiblePrompterFunc = func(hostname string) gh.ConfigEntry {
		return cfg.AccessiblePrompter(hostname)
	}
	mock.BrowserFunc = func(hostname string) gh.ConfigEntry {
		return cfg.Browser(hostname)
	}
	mock.TelemetryFunc = func() gh.ConfigEntry {
		return cfg.Telemetry()
	}
	mock.ColorLabelsFunc = func(hostname string) gh.ConfigEntry {
		return cfg.ColorLabels(hostname)
	}
	mock.EditorFunc = func(hostname string) gh.ConfigEntry {
		return cfg.Editor(hostname)
	}
	mock.GitProtocolFunc = func(hostname string) gh.ConfigEntry {
		return cfg.GitProtocol(hostname)
	}
	mock.HTTPUnixSocketFunc = func(hostname string) gh.ConfigEntry {
		return cfg.HTTPUnixSocket(hostname)
	}
	mock.PagerFunc = func(hostname string) gh.ConfigEntry {
		return cfg.Pager(hostname)
	}
	mock.PromptFunc = func(hostname string) gh.ConfigEntry {
		return cfg.Prompt(hostname)
	}
	mock.PreferEditorPromptFunc = func(hostname string) gh.ConfigEntry {
		return cfg.PreferEditorPrompt(hostname)
	}
	mock.SpinnerFunc = func(hostname string) gh.ConfigEntry {
		return cfg.Spinner(hostname)
	}
	mock.VersionFunc = func() o.Option[string] {
		return cfg.Version()
	}
	mock.CacheDirFunc = func() string {
		return cfg.CacheDir()
	}
	return mock
}

// NewIsolatedTestConfig returns the real config implementation, built from cfgString
// and isolated from the machine running the tests. Pass "" for a config with no
// content. It also returns a function that reads back anything written to disk.
//
// Use it when the code under test exercises real config behaviour: writing config,
// logging in and out, or reading the auth environment variables directly. Prefer
// NewMockConfigFromString when the test only needs to stub config lookups.
//
// Isolation covers all three places config comes from. It mocks the keyring, replaces
// the ghConfig.Read singleton so each test gets its own config, points GH_CONFIG_DIR at
// a temp dir so writes stay off the real config, and clears the environment variables
// that go-gh consults for authentication and host resolution.
//
// Callers that want one of the auth env vars set should set it after calling this,
// otherwise the value is cleared along with the ambient environment.
func NewIsolatedTestConfig(t *testing.T, cfgString string) (*cfg, func(io.Writer, io.Writer)) {
	keyring.MockInit()

	// go-gh reads these ahead of any stored config, so isolating the config file is
	// not enough on its own. A developer with GH_TOKEN exported, or any CI image that
	// provides one, would otherwise see an authenticated config here and fail tests
	// that assert on the logged out state.
	for _, key := range []string{
		"GH_TOKEN",
		"GITHUB_TOKEN",
		"GH_ENTERPRISE_TOKEN",
		"GITHUB_ENTERPRISE_TOKEN",
		"GH_HOST",
	} {
		t.Setenv(key, "")
	}

	c := ghConfig.ReadFromString(cfgString)
	cfg := cfg{c}

	// The real implementation of config.Read uses a sync.Once
	// to read config files and initialise package level variables
	// that are used from then on.
	//
	// This means that tests can't be isolated from each other, so
	// we swap out the function here to return a new config each time.
	ghConfig.Read = func(_ *ghConfig.Config) (*ghConfig.Config, error) {
		return c, nil
	}

	// The config.Write method isn't defined in the same way as Read to allow
	// the function to be swapped out and it does try to write to disk.
	//
	// We should consider whether it makes sense to change that but in the meantime
	// we can use GH_CONFIG_DIR env var to ensure the tests remain isolated.
	readConfigs := StubWriteConfig(t)

	return &cfg, readConfigs
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
