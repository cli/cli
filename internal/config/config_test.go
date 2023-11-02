package config

import (
	"testing"

	"github.com/stretchr/testify/require"

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

func TestGetNonExistentKey(t *testing.T) {
	// Given we have no top level configuration
	cfg := newTestConfig()

	// When we get a key that has no value
	val, err := cfg.Get("", "non-existent-key")

	// Then it returns an error and the value is empty
	var keyNotFoundError *ghConfig.KeyNotFoundError
	require.ErrorAs(t, err, &keyNotFoundError)
	require.Empty(t, val)
}

func TestGetNonExistentHostSpecificKey(t *testing.T) {
	// Given have no top level configuration
	cfg := newTestConfig()

	// When we get a key for a host that has no value
	val, err := cfg.Get("non-existent-host", "non-existent-key")

	// Then it returns an error and the value is empty
	var keyNotFoundError *ghConfig.KeyNotFoundError
	require.ErrorAs(t, err, &keyNotFoundError)
	require.Empty(t, val)
}

func TestGetExistingTopLevelKey(t *testing.T) {
	// Given have a top level config entry
	cfg := newTestConfig()
	cfg.Set("", "top-level-key", "top-level-value")

	// When we get that key
	val, err := cfg.Get("non-existent-host", "top-level-key")

	// Then it returns successfully with the correct value
	require.NoError(t, err)
	require.Equal(t, "top-level-value", val)
}

func TestGetExistingHostSpecificKey(t *testing.T) {
	// Given have a host specific config entry
	cfg := newTestConfig()
	cfg.Set("github.com", "host-specific-key", "host-specific-value")

	// When we get that key
	val, err := cfg.Get("github.com", "host-specific-key")

	// Then it returns successfully with the correct value
	require.NoError(t, err)
	require.Equal(t, "host-specific-value", val)
}

func TestGetHostnameSpecificKeyFallsBackToTopLevel(t *testing.T) {
	// Given have a top level config entry
	cfg := newTestConfig()
	cfg.Set("", "key", "value")

	// When we get that key on a specific host
	val, err := cfg.Get("github.com", "key")

	// Then it returns successfully, falling back to the top level config
	require.NoError(t, err)
	require.Equal(t, "value", val)
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
			val, err := cfg.GetOrDefault("", tt.key)

			// Then it returns the default value
			require.NoError(t, err)
			require.Equal(t, tt.expectedDefault, val)
		})
	}
}

func TestGetOrDefaultExistingKey(t *testing.T) {
	// Given have a top level config entry
	cfg := newTestConfig()
	cfg.Set("", gitProtocolKey, "ssh")

	// When we get that key
	val, err := cfg.GetOrDefault("", gitProtocolKey)

	// Then it returns successfully with the correct value, and doesn't fall back
	// to the default
	require.NoError(t, err)
	require.Equal(t, "ssh", val)
}

func TestGetOrDefaultNotFoundAndNoDefault(t *testing.T) {
	// Given have no configuration
	cfg := newTestConfig()

	// When we get a non-existent-key that has no default
	val, err := cfg.GetOrDefault("", "non-existent-key")

	// Then it returns an error and the value is empty
	var keyNotFoundError *ghConfig.KeyNotFoundError
	require.ErrorAs(t, err, &keyNotFoundError)
	require.Empty(t, val)
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
