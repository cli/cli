package root

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/identity"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResetExperimentDetachesIntegrationWithoutDeletingAccounts(t *testing.T) {
	// Given a configured experiment in an isolated home
	isolateAccountCommand(t)
	cfg, _ := config.NewIsolatedTestConfig(t, accountGitConfig)
	s, err := identity.Open()
	require.NoError(t, err)
	require.NoError(t, s.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
	repo := t.TempDir()
	accountGit(t, repo, "init", "-q")
	require.NoError(t, s.Bind(identity.Binding{Identity: "work", Directory: repo}, "/path/to/gh"))
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	cmd, stdout, stderr := accountCommand(t, cfg, reg)
	cmd.SetArgs([]string{"auth", "reset-multi-account-experiment", "--confirm"})

	// When resetting through the discoverable auth command
	err = cmd.Execute()

	// Then identities/bindings/integration are removed but ordinary saved accounts remain
	require.NoError(t, err, "%s", stderr.String())
	assert.Empty(t, stdout.String())
	assert.NoFileExists(t, filepath.Join(os.Getenv("GH_CONFIG_DIR"), "auth.gitconfig"))
	assert.NoFileExists(t, s.IdentityPath("work"))
	assert.Equal(t, []string{"alice", "bob"}, cfg.Authentication().UsersForHost("github.com"))
	bindings, err := s.Bindings()
	require.NoError(t, err)
	assert.Empty(t, bindings)
}

func TestResetExperimentRequiresNoninteractiveConfirmation(t *testing.T) {
	// Given a noninteractive invocation without confirmation
	isolateAccountCommand(t)
	cfg, _ := config.NewIsolatedTestConfig(t, accountGitConfig)
	cmd, _, _ := accountCommand(t, cfg, &httpmock.Registry{})
	cmd.SetArgs([]string{"auth", "reset-multi-account-experiment"})

	// When the reset command executes
	err := cmd.Execute()

	// Then it requires the explicit deletion-confirmation flag
	require.ErrorContains(t, err, "--confirm required when not running interactively")
}
