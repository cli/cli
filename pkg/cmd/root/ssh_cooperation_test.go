package root

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/identity"
	"github.com/cli/cli/v2/internal/telemetry"
	authuse "github.com/cli/cli/v2/pkg/cmd/auth/use"
	"github.com/cli/cli/v2/pkg/cmd/factory"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/kballard/go-shellquote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSHCooperationPreservesEnterpriseCommandAcrossAccountChoices(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix executable fixtures; does not qualify Windows interoperability")
	}
	// Given an enterprise SSH wrapper accepting OpenSSH arguments and two saved identities
	isolateAccountCommand(t)
	cfg, _ := config.NewIsolatedTestConfig(t, accountGitConfig)
	t.Setenv("GH_ACCOUNT_TEST_HELPER", "1")
	t.Setenv("GIT_SSH_VARIANT", "")
	t.Setenv("GIT_TEMPLATE_DIR", t.TempDir())
	tools := t.TempDir()
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
	require.NoError(t, os.WriteFile(filepath.Join(tools, "ssh"), []byte("#!/bin/sh\ntest \"$1\" = -G || exit 1\nprintf 'hostname github.com\\n'\n"), 0o700))
	testBinary, err := os.Executable()
	require.NoError(t, err)
	helper := filepath.Join(tools, "gh")
	require.NoError(t, os.WriteFile(helper, []byte("#!/bin/sh\nexec "+shellquote.Join(testBinary, "-test.run=^TestAccountGitHelperProcess$", "--")+" \"$@\"\n"), 0o700))
	delegate := filepath.Join(tools, "enterprise-ssh")
	log := filepath.Join(tools, "connections")
	t.Setenv("GH_ACCOUNT_TEST_LOG", log)
	t.Setenv("GH_ACCOUNT_TEST_CONFIG", "company config")
	bare := t.TempDir()
	accountGit(t, bare, "init", "--bare", "-q")
	t.Setenv("GH_ACCOUNT_TEST_BARE", bare)
	require.NoError(t, os.WriteFile(delegate, []byte(`#!/bin/sh
set -eu
test "$1" = -F
test "$2" = "$GH_ACCOUNT_TEST_CONFIG"
shift 2
for arg do
  case "$arg" in git@*) printf '%s\n' "$arg" >>"$GH_ACCOUNT_TEST_LOG" ;; esac
done
exec git upload-pack "$GH_ACCOUNT_TEST_BARE"
`), 0o700))
	original := shellquote.Join(delegate, "-F", "company config")
	accountGit(t, tools, "config", "--global", "core.sshCommand", original)
	accountGit(t, tools, "config", "--global", "ssh.variant", "ssh")
	repo := t.TempDir()
	t.Chdir(repo)
	accountGit(t, repo, "init", "-q")
	accountGit(t, repo, "remote", "add", "origin", "git@github.com:company/project.git")
	s, err := identity.Open()
	require.NoError(t, err)
	require.NoError(t, s.Save(identity.Identity{Name: "alice", Host: "github.com", Account: "alice", SSHHost: "alice-ssh"}, false))
	require.NoError(t, s.Save(identity.Identity{Name: "bob", Host: "github.com", Account: "bob", SSHHost: "bob-ssh"}, false))
	ios, _, _, stderr := iostreams.Test()
	f := factory.New("test", "", func() (gh.Config, error) { return cfg, nil }, ios, helper, &telemetry.NoOpService{})
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	f.HttpClient = func() (*http.Client, error) {
		return &http.Client{Transport: api.AddAuthTokenHeaderWithRefresh(reg, cfg.Authentication())}, nil
	}
	f.ExtensionManager = &extensions.ExtensionManagerMock{ListFunc: func() []extensions.Extension { return nil }}
	use := authuse.NewCmdUse(f, nil)
	use.SetArgs([]string{"alice"})

	// When ordinary Git uses a persistent identity and gh temporarily overrides its account
	require.NoError(t, use.Execute(), "%s", stderr.String())
	accountGit(t, repo, "ls-remote", "origin")
	original = shellquote.Join(delegate, "-F", "updated company config")
	accountGit(t, repo, "config", "--global", "core.sshCommand", original)
	t.Setenv("GH_ACCOUNT_TEST_CONFIG", "updated company config")
	cmd, err := NewCmdRoot(f, &telemetry.NoOpService{}, "", "")
	require.NoError(t, err)
	cmd.SetArgs([]string{"repo", "sync", "--source", "company/project", "--branch", "main", "--account", "bob"})
	require.Error(t, cmd.Execute(), "the empty fixture has no main branch")
	assert.Contains(t, stderr.String(), "couldn't find remote ref")
	accountGit(t, repo, "ls-remote", "origin")
	unset := authuse.NewCmdUse(&cmdutil.Factory{IOStreams: ios, ExecutablePath: helper}, nil)
	unset.SetArgs([]string{"--unset"})
	require.NoError(t, unset.Execute())
	accountGit(t, repo, "ls-remote", "origin")

	// Then every connection preserves the enterprise command/options, and unsetting exposes the original setup
	got, err := os.ReadFile(log)
	require.NoError(t, err)
	assert.Equal(t, "git@alice-ssh\ngit@bob-ssh\ngit@alice-ssh\ngit@github.com\n", string(got))
	assert.Equal(t, original, accountGit(t, repo, "config", "--global", "--no-includes", "core.sshCommand"))
	assert.Equal(t, original, accountGit(t, repo, "config", "core.sshCommand"))
}
