package use

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/internal/identity"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsePreservesUnsupportedSSHSetups(t *testing.T) {
	for _, tt := range []struct {
		name   string
		config [][]string
		want   string
	}{
		{"unknown wrapper", [][]string{{"--global", "core.sshCommand", "enterprise-ssh"}}, "unknown argument conventions"},
		{"different variant", [][]string{{"--global", "core.sshCommand", "plink"}, {"--global", "ssh.variant", "plink"}}, "does not support SSH variant"},
		{"shell expressions", [][]string{{"--global", "core.sshCommand", "ssh -F $SSH_CONFIG"}}, "SSH shell expressions"},
		{"repository command", [][]string{{"--local", "core.sshCommand", "ssh -F company.conf"}}, "would bypass gh's global SSH dispatcher"},
		{"recursive dispatcher", [][]string{{"--global", "core.sshCommand", "gh auth git-ssh"}}, "recursively invoke"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Given an SSH setup that cannot be safely wrapped by the persistent global dispatcher
			s, f := useFixture(t)
			t.Chdir(t.TempDir())
			out, err := exec.Command("git", "init", "-q").CombinedOutput()
			require.NoError(t, err, "%s", out)
			for _, args := range tt.config {
				out, err := exec.Command("git", append([]string{"config"}, args...)...).CombinedOutput()
				require.NoError(t, err, "%s", out)
			}
			before, err := exec.Command("git", "config", "--list", "--show-origin").Output()
			require.NoError(t, err)
			require.NoError(t, s.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice", SSHHost: "work"}, false))
			ios, _, _, _ := iostreams.Test()
			f.IOStreams = ios
			cmd := NewCmdUse(f, nil)
			cmd.SilenceUsage, cmd.SilenceErrors = true, true
			cmd.SetArgs([]string{"work"})

			// When attempting to install a persistent identity
			err = cmd.Execute()

			// Then no binding or Git changes survive, and gh does not tell the user to delete their setup
			require.ErrorContains(t, err, tt.want)
			assert.NotContains(t, err.Error(), "remove it")
			bindings, err := s.Bindings()
			require.NoError(t, err)
			assert.Empty(t, bindings)
			after, err := exec.Command("git", "config", "--list", "--show-origin").Output()
			require.NoError(t, err)
			assert.Equal(t, string(before), string(after))
		})
	}
}

func TestUseRollsBackWhenExistingIncludeOrderPreventsSSHCooperation(t *testing.T) {
	// Given an include section preceding an existing SSH command in the global config
	s, f := useFixture(t)
	t.Chdir(t.TempDir())
	extra := filepath.Join(t.TempDir(), "existing.gitconfig")
	require.NoError(t, os.WriteFile(extra, []byte("[user]\nname = Existing\n"), 0o600))
	for _, args := range [][]string{{"include.path", extra}, {"core.sshCommand", "ssh -F company.conf"}} {
		out, err := exec.Command("git", append([]string{"config", "--global"}, args...)...).CombinedOutput()
		require.NoError(t, err, "%s", out)
	}
	before, err := os.ReadFile(os.Getenv("GIT_CONFIG_GLOBAL"))
	require.NoError(t, err)
	require.NoError(t, s.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice", SSHHost: "work"}, false))
	cmd := NewCmdUse(f, nil)
	cmd.SilenceErrors, cmd.SilenceUsage = true, true
	cmd.SetArgs([]string{"work"})

	// When Git inserts gh's include before the user's command rather than after it
	err = cmd.Execute()

	// Then gh refuses to claim installation succeeded and removes only its attempted changes
	require.ErrorContains(t, err, "takes precedence")
	after, err := os.ReadFile(os.Getenv("GIT_CONFIG_GLOBAL"))
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
	bindings, err := s.Bindings()
	require.NoError(t, err)
	assert.Empty(t, bindings)
}
