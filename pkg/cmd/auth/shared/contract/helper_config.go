package contract

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/auth/shared"
	"github.com/stretchr/testify/require"
)

// This HelperConfig contract exist to ensure that any HelperConfig implementation conforms to this behaviour.
// This is useful because we can swap in fake implementations for testing, rather than requiring our tests to be
// isolated from git.
//
// See for example, TestAuthenticatingGitCredentials for LoginFlow.
type HelperConfig struct {
	NewHelperConfig func(t *testing.T) shared.HelperConfig
	ConfigureHelper func(t *testing.T, hostname string)
}

func (contract HelperConfig) Test(t *testing.T) {
	t.Run("when there are no credential helpers, configures gh for repo and gist host", func(t *testing.T) {
		hc := contract.NewHelperConfig(t)
		require.NoError(t, hc.ConfigureOurs("github.com"))

		repoHelper, err := hc.ConfiguredHelper("github.com")
		require.NoError(t, err)
		require.True(t, repoHelper.IsConfigured(), "expected our helper to be configured")
		require.True(t, repoHelper.IsOurs(), "expected the helper to be ours but was %q", repoHelper.Cmd)

		gistHelper, err := hc.ConfiguredHelper("gist.github.com")
		require.NoError(t, err)
		require.True(t, gistHelper.IsConfigured(), "expected our helper to be configured")
		require.True(t, gistHelper.IsOurs(), "expected the helper to be ours but was %q", gistHelper.Cmd)
	})

	t.Run("when there is a global credential helper, it should be configured but not ours", func(t *testing.T) {
		hc := contract.NewHelperConfig(t)
		contract.ConfigureHelper(t, "credential.helper")

		helper, err := hc.ConfiguredHelper("github.com")
		require.NoError(t, err)
		require.True(t, helper.IsConfigured(), "expected helper to be configured")
		require.False(t, helper.IsOurs(), "expected the helper not to be ours but was %q", helper.Cmd)
	})

	t.Run("when there is a host credential helper, it should be configured but not ours", func(t *testing.T) {
		hc := contract.NewHelperConfig(t)
		contract.ConfigureHelper(t, "credential.https://github.com.helper")

		helper, err := hc.ConfiguredHelper("github.com")
		require.NoError(t, err)
		require.True(t, helper.IsConfigured(), "expected helper to be configured")
		require.False(t, helper.IsOurs(), "expected the helper not to be ours but was %q", helper.Cmd)
	})

	t.Run("returns non configured helper when no helpers are configured", func(t *testing.T) {
		hc := contract.NewHelperConfig(t)

		helper, err := hc.ConfiguredHelper("github.com")
		require.NoError(t, err)
		require.False(t, helper.IsConfigured(), "expected no helper to be configured")
		require.False(t, helper.IsOurs(), "expected the helper not to be ours but was %q", helper.Cmd)
	})
}
