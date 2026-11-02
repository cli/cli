package use

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/identity"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUseStoresScopesAndGitRoutingInSharedAuthConfig(t *testing.T) {
	// Given an owner identity and a directory identity outside any repository
	store, f := useFixture(t)
	for _, id := range []identity.Identity{
		{Name: "owner", Host: "github.com", Account: "bob"},
		{Name: "work", Host: "github.com", Account: "alice"},
	} {
		require.NoError(t, store.Save(id, false))
	}
	work, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Chdir(work)

	// When saving both choices through auth use
	runUse(t, f, "owner", "--owner", "company")
	runUse(t, f, "work")

	// Then one Git config beside identities contains the scope records and native routing
	path := filepath.Join(os.Getenv("GH_CONFIG_DIR"), "auth.gitconfig")
	out, err := exec.Command("git", "config", "--no-includes", "--file", path, "--get-regexp", `^gh-auth-binding\.`).CombinedOutput()
	require.NoError(t, err, "%s", out)
	assert.Contains(t, string(out), "identity owner\n")
	assert.Contains(t, string(out), "owner company\n")
	assert.Contains(t, string(out), "identity work\n")
	assert.Contains(t, string(out), "directory "+work+"\n")
	out, err = exec.Command("git", "config", "--global", "--get-all", "include.path").CombinedOutput()
	require.NoError(t, err, "%s", out)
	assert.Equal(t, path+"\n", string(out))
	assert.NoFileExists(t, filepath.Join(store.Dir, "bindings.json"))
	assert.NoFileExists(t, filepath.Join(store.Dir, "bindings.gitconfig"))
	var scopes []map[string]any
	require.NoError(t, json.Unmarshal([]byte(runUse(t, f, "--list")), &scopes))
	assert.Len(t, scopes, 2)
	for _, scope := range scopes {
		assert.NotContains(t, scope, "recursive")
		assert.NotContains(t, scope, "gitDir")
	}
	selected, err := store.Resolve("github.com", "company", work)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, "work", selected.Name)
}

func TestUseRejectsInvalidScopeStorage(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		contents string
		want     string
	}{
		{name: "old JSON", path: "identities/bindings.json", contents: "[]", want: "old prototype scope file"},
		{name: "old Git projection", path: "identities/bindings.gitconfig", contents: "# earlier prototype\n", want: "old prototype scope file"},
		{name: "malformed Git config", path: "auth.gitconfig", contents: "[unfinished\n", want: "read identity scopes"},
		{name: "duplicate field", path: "auth.gitconfig", contents: "[gh-auth-binding \"0\"]\nidentity = work\nidentity = other\nhost = github.com\nowner = company\n", want: "invalid identity scope field"},
		{name: "contradictory scopes", path: "auth.gitconfig", contents: "[gh-auth-binding \"0\"]\nidentity = work\nhost = github.com\nowner = company\ndirectory = /work\n", want: "invalid identity scope"},
		{name: "removed recursion switch", path: "auth.gitconfig", contents: "[gh-auth-binding \"0\"]\nidentity = work\nhost = github.com\ndirectory = /work\nrecursive = false\n", want: "unsupported identity scope field"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given scope storage that cannot represent a valid current choice
			_, f := useFixture(t)
			path := filepath.Join(os.Getenv("GH_CONFIG_DIR"), tt.path)
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
			require.NoError(t, os.WriteFile(path, []byte(tt.contents), 0o600))
			ios, _, stdout, stderr := iostreams.Test()
			f.IOStreams = ios
			cmd := NewCmdUse(f, nil)
			cmd.SetOut(stderr)
			cmd.SetErr(stderr)
			cmd.SetArgs([]string{"--list"})

			// When inspecting the saved choices
			err := cmd.Execute()

			// Then the invalid data is reported instead of looking like an empty set of choices
			require.ErrorContains(t, err, tt.want)
			assert.Empty(t, stdout.String())
		})
	}
}

func TestUseDoesNotSaveScopeWhenGlobalIncludeCannotBeInstalled(t *testing.T) {
	// Given a saved identity and another writer holding Git's global config lock
	store, f := useFixture(t)
	require.NoError(t, store.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
	t.Chdir(t.TempDir())
	require.NoError(t, os.WriteFile(os.Getenv("GIT_CONFIG_GLOBAL")+".lock", nil, 0o600))
	cmd := NewCmdUse(f, nil)
	cmd.SetOut(f.IOStreams.ErrOut)
	cmd.SetErr(f.IOStreams.ErrOut)
	cmd.SetArgs([]string{"work"})

	// When applying the identity without being able to configure independently invoked Git
	err := cmd.Execute()

	// Then setup fails and no gh-only scope is left behind
	require.ErrorContains(t, err, "could not lock config file")
	assert.NoFileExists(t, filepath.Join(os.Getenv("GH_CONFIG_DIR"), "auth.gitconfig"))
	choices, err := store.Bindings()
	require.NoError(t, err)
	assert.Empty(t, choices)
}

func useFixture(t *testing.T) (*identity.Store, *cmdutil.Factory) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("persistent identities are not supported on Windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GH_CONFIG_DIR", filepath.Join(home, "gh"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, key := range []string{"GH_TOKEN", "GITHUB_TOKEN", "GIT_DIR", "GIT_WORK_TREE", "GIT_SSH", "GIT_SSH_COMMAND", "GIT_SSH_VARIANT", "GH_IDENTITY_SSH_COMMAND", "GH_IDENTITY_SSH_VARIANT", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS", "GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(key, "")
		require.NoError(t, os.Unsetenv(key))
	}
	cfg := config.NewMockConfigFromString("hosts:\n  github.com:\n    users:\n      alice:\n      bob:\n")
	ios, _, _, _ := iostreams.Test()
	store, err := identity.Open()
	require.NoError(t, err)
	return store, &cmdutil.Factory{
		IOStreams: ios, ExecutablePath: "/path/to/gh",
		Config: func() (gh.Config, error) { return cfg, nil },
	}
}

func TestUseOutsideRepositoryAppliesToFutureDescendants(t *testing.T) {
	// Given an identity and a directory that is not a repository
	store, f := useFixture(t)
	require.NoError(t, store.Save(identity.Identity{
		Name: "work", Host: "github.com", Account: "alice",
		GitUserName: "Alice", GitUserEmail: "alice@work.test",
	}, false))
	work := t.TempDir()
	t.Chdir(work)

	// When auth use is invoked without a scope flag, before a child repository exists
	cmd := NewCmdUse(f, nil)
	cmd.SetArgs([]string{"work"})
	require.NoError(t, cmd.Execute())
	child := filepath.Join(work, "new-repository")
	require.NoError(t, os.Mkdir(child, 0o700))
	out, err := exec.Command("git", "-C", child, "init", "-q").CombinedOutput()
	require.NoError(t, err, "%s", out)

	// Then gh and independently invoked Git inherit the identity in the new repository
	selected, err := store.Resolve("github.com", "", child)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, "work", selected.Name)
	out, err = exec.Command("git", "-C", child, "config", "user.email").CombinedOutput()
	require.NoError(t, err, "%s", out)
	assert.Equal(t, "alice@work.test\n", string(out))
}

func runUse(t *testing.T, f *cmdutil.Factory, args ...string) string {
	t.Helper()
	ios, _, stdout, stderr := iostreams.Test()
	factory := *f
	factory.IOStreams = ios
	cmd := NewCmdUse(&factory, nil)
	cmd.SetOut(stderr)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)
	require.NoError(t, cmd.Execute(), "%s", stderr.String())
	return stdout.String()
}

func TestUseViewShowsSelectionWithoutChangingIt(t *testing.T) {
	// Given an identity selected for a directory
	store, f := useFixture(t)
	require.NoError(t, store.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
	t.Chdir(t.TempDir())
	runUse(t, f, "work")

	// When explicitly viewing the selection
	output := runUse(t, f, "--view")

	// Then it is the same JSON as bare auth use, and later inspection is unchanged
	assert.JSONEq(t, `{"name":"work","host":"github.com","account":"alice"}`, output)
	assert.Equal(t, output, runUse(t, f))
}

func TestUseViewRejectsOtherActions(t *testing.T) {
	for _, args := range [][]string{
		{"--view", "work"}, {"--view", "--unset"}, {"--view", "--list"},
		{"--view", "--explain"}, {"--view", "--explanation"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			// Given an explicit view combined with another action
			ios, _, _, _ := iostreams.Test()
			cmd := NewCmdUse(&cmdutil.Factory{IOStreams: ios}, nil)
			cmd.SetArgs(args)
			cmd.SilenceUsage, cmd.SilenceErrors = true, true

			// When parsing that command
			err := cmd.Execute()

			// Then it fails before reading or changing configuration
			require.ErrorContains(t, err, "--view cannot be combined")
		})
	}
}

func TestExplanationShowsNestedDirectoryPrecedenceAndEffectiveGitMetadata(t *testing.T) {
	// Given an owner choice, a broad directory choice, and a more specific directory choice
	store, f := useFixture(t)
	for _, id := range []identity.Identity{
		{Name: "owner", Host: "github.com", Account: "alice"},
		{Name: "work", Host: "github.com", Account: "alice", GitUserName: "Alice", GitUserEmail: "alice@work.test"},
		{Name: "client", Host: "github.com", Account: "bob", SSHHost: "github-client"},
	} {
		require.NoError(t, store.Save(id, false))
	}
	work, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	client := filepath.Join(work, "client")
	repo := filepath.Join(client, "project")
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "src"), 0o700))
	runUse(t, f, "owner", "--owner", "company")
	runUse(t, f, "work", "--directory", work)
	runUse(t, f, "client", "--directory", client)
	for _, args := range [][]string{
		{"init", "-q"},
		{"remote", "add", "origin", "git@github.com:company/project.git"},
		{"config", "user.email", "local@client.test"},
	} {
		out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
		require.NoError(t, err, "%s", out)
	}
	t.Chdir(filepath.Join(repo, "src"))

	// When explaining the identity from a repository subdirectory
	output := runUse(t, f, "--explanation")

	// Then the explanation traces the winning choice, broader matches, and actual inherited/overridden Git values
	assert.Contains(t, output, `Using identity "client" (bob on github.com)`)
	assert.Contains(t, output, "Selected: directory "+client+" (includes descendants)")
	assert.Contains(t, output, "Overrides directory "+work+" -> work: more specific directory")
	assert.Contains(t, output, "Overrides owner company -> owner: directory choices take precedence")
	assert.Contains(t, output, "gh API:    bob\n")
	assert.Contains(t, output, "SSH Git:   github-client (SSH host)")
	assert.Contains(t, output, "gh has not checked which account this SSH host signs in as.")
	assert.Contains(t, output, "Git user.name: Alice")
	assert.Contains(t, output, "work.gitconfig")
	assert.Contains(t, output, "Git user.email: local@client.test")
	assert.Contains(t, output, ".git/config")
}

func TestExplanationOutsideRepositoryShowsIdentityAttribution(t *testing.T) {
	// Given an identity selected in a directory with no repository
	store, f := useFixture(t)
	require.NoError(t, store.Save(identity.Identity{
		Name: "work", Host: "github.com", Account: "alice",
		GitUserName: "Alice", GitUserEmail: "alice@work.test",
	}, false))
	t.Chdir(t.TempDir())
	runUse(t, f, "work")

	// When explaining the selection outside a repository
	output := runUse(t, f, "--explanation")

	// Then attribution describes the selected identity rather than unrelated current Git defaults
	assert.Contains(t, output, "You're not in a Git repository. This identity sets:")
	assert.Contains(t, output, "user.name:  Alice")
	assert.Contains(t, output, "user.email: alice@work.test")
	assert.Contains(t, output, "SSH Git:   Not configured.")
	for _, phrase := range []string{"[declared]", "[blocked]", "Persistent selection", "No remote authentication checks", "target read", "target not observed", "Actual include directives", "Conditional identity includes may apply"} {
		assert.NotContains(t, output, phrase)
	}
	assert.NotContains(t, output, "Current Git defaults outside a repository:")
	assert.NotContains(t, output, "Git user.name:")
	assert.NotContains(t, output, "Git user.email:")
	assert.NotContains(t, output, "Git name/email are defaults for both author and committer")
}

func TestUseCuratesScope(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want UseOptions
	}{
		{name: "directory tree", args: []string{"work"}, want: UseOptions{Identity: "work", Host: "github.com"}},
		{name: "owner", args: []string{"work", "--owner", "company"}, want: UseOptions{Identity: "work", Host: "github.com", Owner: "company"}},
		{name: "explicit directory", args: []string{"work", "--directory", "/work"}, want: UseOptions{Identity: "work", Host: "github.com", Directory: "/work"}},
		{name: "explanation context", args: []string{"--explanation", "--directory", "/work"}, want: UseOptions{Host: "github.com", Directory: "/work", Explanation: true}},
		{name: "unset owner", args: []string{"--unset", "--owner", "company", "--hostname", "example.com"}, want: UseOptions{Host: "example.com", Owner: "company", Unset: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Given an auth use invocation with an explicit scope
			ios, _, _, _ := iostreams.Test()
			var got UseOptions
			cmd := NewCmdUse(&cmdutil.Factory{IOStreams: ios}, func(opts *UseOptions) error {
				got = *opts
				return nil
			})
			cmd.SetArgs(tt.args)

			// When Cobra executes the command
			err := cmd.Execute()

			// Then the run function receives the scope without eager repository discovery
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExplanationFromParentShowsDescendantOverrides(t *testing.T) {
	// Given a broad choice and a different identity in one of its subdirectories
	store, f := useFixture(t)
	require.NoError(t, store.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
	require.NoError(t, store.Save(identity.Identity{Name: "client", Host: "github.com", Account: "bob"}, false))
	work, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	client := filepath.Join(work, "client")
	require.NoError(t, os.Mkdir(client, 0o700))
	runUse(t, f, "work", "--directory", work)
	runUse(t, f, "client", "--directory", client)
	t.Chdir(work)

	// When explaining the broader directory
	output := runUse(t, f, "--explanation")

	// Then the current identity and future descendant exceptions are clearly distinguished
	assert.Contains(t, output, `Using identity "work" (alice on github.com)`)
	assert.Contains(t, output, "Descendant overrides (do not affect this directory):")
	assert.Contains(t, output, "directory "+client+" -> client")
	assert.NotContains(t, output, "Overrides directory "+client)
}

func TestUnsetRemovesDirectoryChoice(t *testing.T) {
	// Given an identity applied at a repository root
	store, f := useFixture(t)
	require.NoError(t, store.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
	repo := t.TempDir()
	out, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput()
	require.NoError(t, err, "%s", out)
	t.Chdir(repo)
	runUse(t, f, "work")

	// When removing the choice through the current public command
	runUse(t, f, "--unset")

	// Then the directory choice no longer selects an identity
	selected, err := store.Resolve("github.com", "", repo)
	require.NoError(t, err)
	assert.Nil(t, selected)
}

func TestExplanationShowsGitSSHBehindManagedRouting(t *testing.T) {
	// Given managed SSH routing with GIT_SSH supplying the underlying connection program
	store, f := useFixture(t)
	require.NoError(t, store.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice", SSHHost: "github-work"}, false))
	t.Chdir(t.TempDir())
	runUse(t, f, "work")
	t.Setenv("GIT_SSH", "/some/other/ssh")

	// When explaining the effective SSH route
	output := runUse(t, f, "--explanation")

	// Then gh still selects the route while GIT_SSH supplies the connection program
	assert.Contains(t, output, "SSH Git:   github-work (SSH host)")
	assert.Contains(t, output, "Connects using /some/other/ssh.")
	assert.NotContains(t, output, "GIT_SSH overrides")
}

func TestExplanationReportsAccountBlockerAndCommitOverrideWithoutToken(t *testing.T) {
	// Given an identity whose account is blocked by a token override and whose author email is overridden
	store, f := useFixture(t)
	require.NoError(t, store.Save(identity.Identity{Name: "work", Host: "github.com", Account: "alice"}, false))
	t.Chdir(t.TempDir())
	runUse(t, f, "work")
	t.Setenv("GH_TOKEN", "fixture-secret-never-print")
	t.Setenv("GIT_AUTHOR_EMAIL", "overridden@example.invalid")

	// When requesting an explanation, rather than performing an authenticated operation
	output := runUse(t, f, "--explanation")

	// Then the blockers are explained without claiming agreement or disclosing the token
	assert.Contains(t, output, "gh API:    Cannot use alice:")
	assert.Contains(t, output, "HTTPS Git: Cannot use alice:")
	assert.Contains(t, output, "GH_TOKEN overrides saved accounts")
	assert.Contains(t, output, "Active commit override: GIT_AUTHOR_EMAIL")
	assert.NotContains(t, output, "fixture-secret-never-print")
}

func TestUseRejectsContradictoryScopes(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "owner and directory", args: []string{"work", "--owner", "company", "--directory", "/work"}, want: "--owner cannot be combined"},
		{name: "identity and unset", args: []string{"work", "--unset"}, want: "--unset does not take an identity"},
		{name: "identity and list", args: []string{"work", "--list"}, want: "--list cannot be combined"},
		{name: "identity and hostname", args: []string{"work", "--hostname", "example.com"}, want: "a named identity supplies its own hostname"},
		{name: "empty owner", args: []string{"work", "--owner", ""}, want: "--owner cannot be empty"},
		{name: "explanation and identity", args: []string{"work", "--explanation"}, want: "--explanation cannot be combined"},
		{name: "explanation and unset", args: []string{"--unset", "--explanation"}, want: "--explanation cannot be combined"},
		{name: "explanation and list", args: []string{"--list", "--explanation"}, want: "--explanation cannot be combined"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Given an invocation whose scope flags contradict each other
			ios, _, _, _ := iostreams.Test()
			cmd := NewCmdUse(&cmdutil.Factory{IOStreams: ios}, func(*UseOptions) error {
				t.Fatal("invalid scopes must not reach the run function")
				return nil
			})
			cmd.SetArgs(tt.args)
			cmd.SetOut(ios.ErrOut)
			cmd.SetErr(ios.ErrOut)

			// When Cobra executes it
			err := cmd.Execute()

			// Then it reports a flag error without changing configuration
			require.ErrorContains(t, err, tt.want)
		})
	}
}
