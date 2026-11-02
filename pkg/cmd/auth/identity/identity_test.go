package identity

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/require"
)

func identityFactory(t *testing.T) *cmdutil.Factory {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("persistent identities are not supported on Windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GH_CONFIG_DIR", filepath.Join(home, "gh"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN", "GIT_CONFIG", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS", "GIT_DIR", "GIT_COMMON_DIR", "GIT_WORK_TREE", "GIT_SSH", "GIT_SSH_COMMAND"} {
		t.Setenv(name, "")
		require.NoError(t, os.Unsetenv(name))
	}
	cfg := config.NewMockConfigFromString("hosts:\n  github.com:\n    users:\n      alice:\n")
	ios, _, _, _ := iostreams.Test()
	return &cmdutil.Factory{
		IOStreams: ios, ExecutablePath: "/path/to/gh",
		Config: func() (gh.Config, error) { return cfg, nil },
	}
}

func runIdentity(t *testing.T, f *cmdutil.Factory, args ...string) string {
	t.Helper()
	ios, _, stdout, stderr := iostreams.Test()
	factory := *f
	factory.IOStreams = ios
	cmd := NewCmdIdentity(&factory)
	cmd.SetOut(stderr)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)
	require.NoError(t, cmd.Execute(), "%s", stderr.String())
	return stdout.String()
}
