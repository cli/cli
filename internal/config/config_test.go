package config

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cli/cli/v2/internal/gh"
	ghConfig "github.com/cli/go-gh/v2/pkg/config"
)

func newTestConfig() *cfg {
	return &cfg{
		cfg: ghConfig.ReadFromString(""),
	}
}

func TestNewConfigProvidesFallback(t *testing.T) {
	var spiedCfg *ghConfig.Config
	ghConfig.Read = func(fallback *ghConfig.Config) (*ghConfig.Config, error) {
		spiedCfg = fallback
		return fallback, nil
	}
	_, err := NewConfig()
	require.NoError(t, err)
	requireKeyWithValue(t, spiedCfg, []string{versionKey}, "1")
	requireKeyWithValue(t, spiedCfg, []string{gitProtocolKey}, "https")
	requireKeyWithValue(t, spiedCfg, []string{editorKey}, "")
	requireKeyWithValue(t, spiedCfg, []string{promptKey}, "enabled")
	requireKeyWithValue(t, spiedCfg, []string{pagerKey}, "")
	requireKeyWithValue(t, spiedCfg, []string{aliasesKey, "co"}, "pr checkout")
	requireKeyWithValue(t, spiedCfg, []string{httpUnixSocketKey}, "")
	requireKeyWithValue(t, spiedCfg, []string{browserKey}, "")
}

func TestGetOrDefaultApplicationDefaults(t *testing.T) {
	tests := []struct {
		key             string
		expectedDefault string
	}{
		{gitProtocolKey, "https"},
		{editorKey, ""},
		{promptKey, "enabled"},
		{pagerKey, ""},
		{httpUnixSocketKey, ""},
		{browserKey, ""},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			// Given we have no top level configuration
			cfg := newTestConfig()

			// When we get a key that has no value, but has a default
			optionalEntry := cfg.GetOrDefault("", tt.key)

			// Then there is an entry with the default value, and source set as default
			entry := optionalEntry.Expect(fmt.Sprintf("expected there to be a value for %s", tt.key))
			require.Equal(t, tt.expectedDefault, entry.Value)
			require.Equal(t, gh.ConfigDefaultProvided, entry.Source)
		})
	}
}

func TestGetOrDefaultNonExistentKey(t *testing.T) {
	// Given we have no top level configuration
	cfg := newTestConfig()

	// When we get a key that has no value
	optionalEntry := cfg.GetOrDefault("", "non-existent-key")

	// Then it returns a None variant
	require.True(t, optionalEntry.IsNone(), "expected there to be no value")
}

func TestGetOrDefaultNonExistentHostSpecificKey(t *testing.T) {
	// Given have no top level configuration
	cfg := newTestConfig()

	// When we get a key for a host that has no value
	optionalEntry := cfg.GetOrDefault("non-existent-host", "non-existent-key")

	// Then it returns a None variant
	require.True(t, optionalEntry.IsNone(), "expected there to be no value")
}

func TestGetOrDefaultExistingTopLevelKey(t *testing.T) {
	// Given have a top level config entry
	cfg := newTestConfig()
	cfg.Set("", "top-level-key", "top-level-value")

	// When we get that key
	optionalEntry := cfg.GetOrDefault("non-existent-host", "top-level-key")

	// Then it returns a Some variant containing the correct value and a source of user
	entry := optionalEntry.Expect("expected there to be a value")
	require.Equal(t, "top-level-value", entry.Value)
	require.Equal(t, gh.ConfigUserProvided, entry.Source)
}

func TestGetOrDefaultExistingHostSpecificKey(t *testing.T) {
	// Given have a host specific config entry
	cfg := newTestConfig()
	cfg.Set("github.com", "host-specific-key", "host-specific-value")

	// When we get that key
	optionalEntry := cfg.GetOrDefault("github.com", "host-specific-key")

	// Then it returns a Some variant containing the correct value and a source of user
	entry := optionalEntry.Expect("expected there to be a value")
	require.Equal(t, "host-specific-value", entry.Value)
	require.Equal(t, gh.ConfigUserProvided, entry.Source)
}

func TestGetOrDefaultHostnameSpecificKeyFallsBackToTopLevel(t *testing.T) {
	// Given have a top level config entry
	cfg := newTestConfig()
	cfg.Set("", "key", "value")

	// When we get that key on a specific host
	optionalEntry := cfg.GetOrDefault("github.com", "key")

	// Then it returns a Some variant containing the correct value by falling back
	// to the top level config, with a source of user
	entry := optionalEntry.Expect("expected there to be a value")
	require.Equal(t, "value", entry.Value)
	require.Equal(t, gh.ConfigUserProvided, entry.Source)
}

func TestFallbackConfig(t *testing.T) {
	cfg := fallbackConfig()
	requireKeyWithValue(t, cfg, []string{gitProtocolKey}, "https")
	requireKeyWithValue(t, cfg, []string{editorKey}, "")
	requireKeyWithValue(t, cfg, []string{promptKey}, "enabled")
	requireKeyWithValue(t, cfg, []string{pagerKey}, "")
	requireKeyWithValue(t, cfg, []string{aliasesKey, "co"}, "pr checkout")
	requireKeyWithValue(t, cfg, []string{httpUnixSocketKey}, "")
	requireKeyWithValue(t, cfg, []string{browserKey}, "")
	requireNoKey(t, cfg, []string{"unknown"})
}

func TestSetTopLevelKey(t *testing.T) {
	c := newTestConfig()
	host := ""
	key := "top-level-key"
	val := "top-level-value"
	c.Set(host, key, val)
	requireKeyWithValue(t, c.cfg, []string{key}, val)
}

func TestSetHostSpecificKey(t *testing.T) {
	c := newTestConfig()
	host := "github.com"
	key := "host-level-key"
	val := "host-level-value"
	c.Set(host, key, val)
	requireKeyWithValue(t, c.cfg, []string{hostsKey, host, key}, val)
}

func TestSetUserSpecificKey(t *testing.T) {
	c := newTestConfig()
	host := "github.com"
	user := "test-user"
	c.cfg.Set([]string{hostsKey, host, userKey}, user)

	key := "host-level-key"
	val := "host-level-value"
	c.Set(host, key, val)
	requireKeyWithValue(t, c.cfg, []string{hostsKey, host, key}, val)
	requireKeyWithValue(t, c.cfg, []string{hostsKey, host, usersKey, user, key}, val)
}

func TestSetUserSpecificKeyNoUserPresent(t *testing.T) {
	c := newTestConfig()
	host := "github.com"
	key := "host-level-key"
	val := "host-level-value"
	c.Set(host, key, val)
	requireKeyWithValue(t, c.cfg, []string{hostsKey, host, key}, val)
	requireNoKey(t, c.cfg, []string{hostsKey, host, usersKey})
}
