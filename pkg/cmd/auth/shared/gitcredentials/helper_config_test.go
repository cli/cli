package gitcredentials_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared/gitcredentials"
	"github.com/stretchr/testify/require"
)

func withIsolatedGitConfig(t *testing.T) {
	t.Helper()

	// https://git-scm.com/docs/git-config#ENVIRONMENT
	// Set the global git config to a temporary file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, ".gitconfig")
	t.Setenv("GIT_CONFIG_GLOBAL", configFile)

	// And disable git reading the system config
	t.Setenv("GIT_CONFIG_NOSYSTEM", "true")
}

func configureTestCredentialHelper(t *testing.T, key string) {
	t.Helper()

	gc := &git.Client{}
	cmd, err := gc.Command(context.Background(), "config", "--global", "--add", key, "test-helper")
	require.NoError(t, err)
	require.NoError(t, cmd.Run())
}

func TestConfigureOursNoPreexistingHelpersConfigured(t *testing.T) {
	// Given there are no credential helpers configured
	withIsolatedGitConfig(t)

	// When we configure ourselves as the git credential helper
	hc := gitcredentials.HelperConfig{
		SelfExecutablePath: "/path/to/gh",
		GitClient:          &git.Client{},
	}
	require.NoError(t, hc.ConfigureOurs("github.com"))

	// Then the git config should be updated to use our credential helper for both repos and gists
	repoHelper, err := hc.ConfiguredHelper("github.com")
	require.NoError(t, err)
	require.True(t, repoHelper.IsConfigured(), "expected our helper to be configured")
	require.True(t, repoHelper.IsOurs(), "expected the helper to be ours but was %q", repoHelper.Cmd)

	gistHelper, err := hc.ConfiguredHelper("gist.github.com")
	require.NoError(t, err)
	require.True(t, gistHelper.IsConfigured(), "expected our helper to be configured")
	require.True(t, gistHelper.IsOurs(), "expected the helper to be ours but was %q", gistHelper.Cmd)
}

func TestConfigureOursWithPreexistingHelpersConfigured(t *testing.T) {
	// Given there are other credential helpers configured
	withIsolatedGitConfig(t)
	configureTestCredentialHelper(t, "credential.helper")
	configureTestCredentialHelper(t, "credential.https://github.com.helper")

	// When we configure ourselves as the git credential helper
	hc := gitcredentials.HelperConfig{
		SelfExecutablePath: "/path/to/gh",
		GitClient:          &git.Client{},
	}
	require.NoError(t, hc.ConfigureOurs("github.com"))

	// Then the git config should be updated to use our credential repoHelper for both repos and gists
	repoHelper, err := hc.ConfiguredHelper("github.com")
	require.NoError(t, err)
	require.True(t, repoHelper.IsConfigured(), "expected our helper to be configured")
	require.True(t, repoHelper.IsOurs(), "expected the helper to be ours but was %q", repoHelper.Cmd)

	gistHelper, err := hc.ConfiguredHelper("gist.github.com")
	require.NoError(t, err)
	require.True(t, gistHelper.IsConfigured(), "expected our helper to be configured")
	require.True(t, gistHelper.IsOurs(), "expected the helper to be ours but was %q", gistHelper.Cmd)
}

func TestConfiguredHelperNoPreexistingHelpersConfigured(t *testing.T) {
	// Given there are no credential helpers configured
	withIsolatedGitConfig(t)

	// When we check the configured helper for a hostname
	hc := gitcredentials.HelperConfig{
		SelfExecutablePath: "/path/to/gh",
		GitClient:          &git.Client{},
	}
	helper, err := hc.ConfiguredHelper("github.com")

	// Then the helper should not be configured and not ours
	require.NoError(t, err)
	require.False(t, helper.IsConfigured(), "expected no helper to be configured")
	require.False(t, helper.IsOurs(), "expected the helper not to be ours but was %q", helper.Cmd)
}

func TestConfiguredHelperWithPreexistingGlobalHelperConfigured(t *testing.T) {
	// Given there is a global credential helper configured
	withIsolatedGitConfig(t)
	configureTestCredentialHelper(t, "credential.helper")

	// When we check the configured helper for a hostname
	hc := gitcredentials.HelperConfig{
		SelfExecutablePath: "/path/to/gh",
		GitClient:          &git.Client{},
	}
	helper, err := hc.ConfiguredHelper("github.com")

	// Then the helper should be configured, but not ours
	require.NoError(t, err)
	require.True(t, helper.IsConfigured(), "expected no helper to be configured")
	require.False(t, helper.IsOurs(), "expected the helper not to be ours but was %q", helper.Cmd)
}

func TestConfiguredHelperWithPreexistingHostHelperConfigured(t *testing.T) {
	// Given there is a credential helper configured for a hostname
	withIsolatedGitConfig(t)
	configureTestCredentialHelper(t, "credential.https://github.com.helper")

	// When we check the configured helper for a hostname
	hc := gitcredentials.HelperConfig{
		SelfExecutablePath: "/path/to/gh",
		GitClient:          &git.Client{},
	}
	helper, err := hc.ConfiguredHelper("github.com")

	// Then the helper should be configured, but not ours
	require.NoError(t, err)
	require.True(t, helper.IsConfigured(), "expected no helper to be configured")
	require.False(t, helper.IsOurs(), "expected the helper not to be ours but was %q", helper.Cmd)
}

func TestHelperIsOurs(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want bool
	}{
		{
			name: "blank",
			cmd:  "",
			want: false,
		},
		{
			name: "invalid",
			cmd:  "!",
			want: false,
		},
		{
			name: "osxkeychain",
			cmd:  "osxkeychain",
			want: false,
		},
		{
			name: "looks like gh but isn't",
			cmd:  "gh auth",
			want: false,
		},
		{
			name: "ours",
			cmd:  "!/path/to/gh auth",
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := gitcredentials.Helper{Cmd: tt.cmd}
			if got := h.IsOurs(); got != tt.want {
				t.Errorf("IsOurs() = %v, want %v", got, tt.want)
			}
		})
	}
}
