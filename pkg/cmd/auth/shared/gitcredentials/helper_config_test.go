package gitcredentials_test

import (
	"context"
	"runtime"
	"testing"

	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared/contract"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared/gitcredentials"
	"github.com/stretchr/testify/require"
)

func configureTestCredentialHelper(t *testing.T, key string) {
	t.Helper()

	gc := &git.Client{}
	cmd, err := gc.Command(context.Background(), "config", "--global", "--add", key, "test-helper")
	require.NoError(t, err)
	require.NoError(t, cmd.Run())
}

func TestHelperConfigContract(t *testing.T) {
	contract.HelperConfig{
		NewHelperConfig: func(t *testing.T) shared.HelperConfig {
			git.IsolateConfig(t)

			return &gitcredentials.HelperConfig{
				SelfExecutablePath: "/path/to/gh",
				GitClient:          &git.Client{},
			}
		},
		ConfigureHelper: func(t *testing.T, hostname string) {
			configureTestCredentialHelper(t, hostname)
		},
	}.Test(t)
}

// This is a whitebox test unlike the contract because although we don't use the exact configured command, it's
// important that it is exactly right since git uses it.
func TestSetsCorrectCommandInGitConfig(t *testing.T) {
	git.IsolateConfig(t)

	gc := &git.Client{}
	hc := &gitcredentials.HelperConfig{
		SelfExecutablePath: "/path/to/gh",
		GitClient:          gc,
	}
	require.NoError(t, hc.ConfigureOurs("github.com"))

	// Check that the correct command was set in the git config
	cmd, err := gc.Command(context.Background(), "config", "--get", "credential.https://github.com.helper")
	require.NoError(t, err)
	output, err := cmd.Output()
	require.NoError(t, err)
	require.Equal(t, "!/path/to/gh auth git-credential\n", string(output))
}

func TestHelperIsOurs(t *testing.T) {
	tests := []struct {
		name        string
		cmd         string
		want        bool
		windowsOnly bool
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
		{
			name:        "ours - Windows edition",
			cmd:         `!'C:\Program Files\GitHub CLI\gh.exe' auth git-credential`,
			want:        true,
			windowsOnly: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.windowsOnly && runtime.GOOS != "windows" {
				t.Skip("skipping test on non-Windows platform")
			}

			h := gitcredentials.Helper{Cmd: tt.cmd}
			if got := h.IsOurs(); got != tt.want {
				t.Errorf("IsOurs() = %v, want %v", got, tt.want)
			}
		})
	}
}
