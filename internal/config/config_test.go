package config_test

import (
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/stretchr/testify/require"

	ghConfig "github.com/cli/go-gh/v2/pkg/config"
)

func TestGetNonExistentKey(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("GH_CONFIG_DIR", tempDir)

	// Given we have no top level configuration
	cfg, err := config.NewConfig()
	require.NoError(t, err)

	// When we get a key that has no value
	val, err := cfg.Get("", "non-existent-key")

	// Then it returns an error and the value is empty
	var keyNotFoundError *ghConfig.KeyNotFoundError
	require.ErrorAs(t, err, &keyNotFoundError)
	require.Empty(t, val)
}

func TestGetNonExistentHostSpecificKey(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("GH_CONFIG_DIR", tempDir)

	// Given have no top level configuration
	cfg, err := config.NewConfig()
	require.NoError(t, err)

	// When we get a key for a host that has no value
	val, err := cfg.Get("non-existent-host", "non-existent-key")

	// Then it returns an error and the value is empty
	var keyNotFoundError *ghConfig.KeyNotFoundError
	require.ErrorAs(t, err, &keyNotFoundError)
	require.Empty(t, val)
}

func TestGetExistingTopLevelKey(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("GH_CONFIG_DIR", tempDir)

	// Given have a top level config entry
	cfg, err := config.NewConfig()
	require.NoError(t, err)
	cfg.Set("", "top-level-key", "top-level-value")

	// When we get that key
	val, err := cfg.Get("non-existent-host", "top-level-key")

	// Then it returns successfully with the correct value
	require.NoError(t, err)
	require.Equal(t, "top-level-value", val)
}

func TestGetExistingHostSpecificKey(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("GH_CONFIG_DIR", tempDir)

	// Given have a host specific config entry
	cfg, err := config.NewConfig()
	require.NoError(t, err)
	cfg.Set("github.com", "host-specific-key", "host-specific-value")

	// When we get that key
	val, err := cfg.Get("github.com", "host-specific-key")

	// Then it returns successfully with the correct value
	require.NoError(t, err)
	require.Equal(t, "host-specific-value", val)
}

func TestGetHostnameSpecificKeyFallsBackToTopLevel(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("GH_CONFIG_DIR", tempDir)

	// Given have a top level config entry
	cfg, err := config.NewConfig()
	require.NoError(t, err)
	cfg.Set("", "key", "value")

	// When we get that key on a specific host
	val, err := cfg.Get("github.com", "key")

	// Then it returns successfully, falling back to the top level config
	require.NoError(t, err)
	require.Equal(t, "value", val)
}
