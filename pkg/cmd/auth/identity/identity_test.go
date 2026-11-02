package identity

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	identities "github.com/cli/cli/v2/internal/identity"
	authuse "github.com/cli/cli/v2/pkg/cmd/auth/use"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
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

func TestIdentityAttributionEmailRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		email string
	}{
		{name: "public email", email: "alice@example.com"},
		{name: "GitHub noreply email", email: "123456+alice@users.noreply.github.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given a saved account and an isolated repository
			f := identityFactory(t)
			repo := t.TempDir()
			t.Chdir(repo)
			out, err := exec.Command("git", "init", "-q").CombinedOutput()
			require.NoError(t, err, "%s", out)

			// When creating and applying an identity with the chosen attribution email
			runIdentity(t, f, "create", "work", "--account", "alice",
				"--git-user-name", "Alice Example", "--git-user-email", tt.email)
			cmd := authuse.NewCmdUse(f, nil)
			cmd.SetArgs([]string{"work"})
			require.NoError(t, cmd.Execute())
			var viewed map[string]any
			require.NoError(t, json.Unmarshal([]byte(runIdentity(t, f, "view", "work")), &viewed))

			// Then the public fields and native Git settings preserve the email independently of authentication
			assert.Equal(t, "Alice Example", viewed["gitUserName"])
			assert.Equal(t, tt.email, viewed["gitUserEmail"])
			assert.Equal(t, "alice", viewed["account"])
			assert.NotContains(t, viewed, "gitName")
			assert.NotContains(t, viewed, "gitEmail")
			out, err = exec.Command("git", "config", "user.email").CombinedOutput()
			require.NoError(t, err, "%s", out)
			assert.Equal(t, tt.email+"\n", string(out))
		})
	}
}

func TestIdentityEditPreservesLegacyAttribution(t *testing.T) {
	// Given an identity saved by the earlier prototype with a public email
	f := identityFactory(t)
	store, err := identities.Open()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(store.Dir, 0o700))
	path := store.IdentityPath("work")
	require.NoError(t, os.WriteFile(path, []byte(`[gh-identity "github.com"]
	name = work
	account = alice
	gitName = Alice Example
	gitEmail = alice@example.com
`), 0o600))
	var existing map[string]any
	require.NoError(t, json.Unmarshal([]byte(runIdentity(t, f, "view", "work")), &existing))
	assert.Equal(t, "alice@example.com", existing["gitUserEmail"])

	// When changing only the attribution email to GitHub noreply
	runIdentity(t, f, "edit", "work", "--git-user-email", "123456+alice@users.noreply.github.com")

	// Then the name and account survive and both JSON and saved metadata use the new field names
	var viewed map[string]any
	require.NoError(t, json.Unmarshal([]byte(runIdentity(t, f, "view", "work")), &viewed))
	assert.Equal(t, "Alice Example", viewed["gitUserName"])
	assert.Equal(t, "123456+alice@users.noreply.github.com", viewed["gitUserEmail"])
	assert.Equal(t, "alice", viewed["account"])
	out, err := exec.Command("git", "config", "--file", path, "--get", "gh-identity.github.com.gitUserName").CombinedOutput()
	require.NoError(t, err, "%s", out)
	assert.Equal(t, "Alice Example\n", string(out))
	out, err = exec.Command("git", "config", "--file", path, "--get", "gh-identity.github.com.gitUserEmail").CombinedOutput()
	require.NoError(t, err, "%s", out)
	assert.Equal(t, "123456+alice@users.noreply.github.com\n", string(out))
}
