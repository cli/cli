package root

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/identity"
	"github.com/cli/cli/v2/internal/keyring"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/telemetry"
	authcmd "github.com/cli/cli/v2/pkg/cmd/auth"
	"github.com/cli/cli/v2/pkg/cmd/factory"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/kballard/go-shellquote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const accountGitConfig = `
hosts:
  github.com:
    user: alice
    oauth_token: fixture-alice-token
    users:
      alice:
        oauth_token: fixture-alice-token
      bob:
        oauth_token: fixture-bob-token
`

// TestAccountGitHelperProcess runs the actual hidden auth commands when invoked by Git.
func TestAccountGitHelperProcess(t *testing.T) {
	if os.Getenv("GH_ACCOUNT_TEST_HELPER") != "1" {
		return
	}
	keyring.MockInit()
	cfg := config.NewMockConfigFromString(accountGitConfig)
	cmd := authcmd.NewCmdAuth(&cmdutil.Factory{
		IOStreams: iostreams.System(),
		Config:    func() (gh.Config, error) { return cfg, nil },
	}, &telemetry.NoOpService{})
	for i, arg := range os.Args {
		if arg == "--" && i+1 < len(os.Args) && os.Args[i+1] == "auth" {
			cmd.SetArgs(os.Args[i+2:])
			cmd.SilenceErrors, cmd.SilenceUsage = true, true
			if err := cmd.Execute(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			os.Exit(0)
		}
	}
	os.Exit(2)
}

func accountGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	return strings.TrimSpace(string(out))
}

func TestAccountOverridePRCreatePushesBeforeCreatingPR(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix executable fixtures for Git's SSH and HTTPS protocols")
	}
	for _, tt := range []struct {
		protocol string
		url      string
	}{
		{protocol: "https", url: "https://github.com/bob/project.git"},
		{protocol: "ssh", url: "git@github.com:bob/project.git"},
	} {
		t.Run(tt.protocol, func(t *testing.T) {
			// Given an unpublished branch with alice attribution, but a repository only bob can push to
			isolateAccountCommand(t)
			cfg, _ := config.NewIsolatedTestConfig(t, accountGitConfig)
			t.Setenv("GH_ACCOUNT_TEST_HELPER", "1")
			t.Setenv("GIT_TERMINAL_PROMPT", "0")
			t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
			t.Setenv("GIT_TEMPLATE_DIR", t.TempDir())
			for _, name := range []string{"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL", "GIT_SSH_VARIANT"} {
				t.Setenv(name, "")
				require.NoError(t, os.Unsetenv(name))
			}
			tools := t.TempDir()
			binary, err := os.Executable()
			require.NoError(t, err)
			helper := filepath.Join(tools, "gh")
			require.NoError(t, os.WriteFile(helper, []byte("#!/bin/sh\nexec "+shellquote.Join(binary, "-test.run=^TestAccountGitHelperProcess$", "--")+" \"$@\"\n"), 0o700))
			t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("GIT_EXEC_PATH", tools)
			bare := t.TempDir()
			accountGit(t, bare, "init", "--bare", "-q", "--initial-branch=main")
			t.Setenv("GH_ACCOUNT_TEST_BARE", bare)
			log := filepath.Join(tools, "authentication")
			t.Setenv("GH_ACCOUNT_TEST_LOG", log)
			require.NoError(t, os.WriteFile(filepath.Join(tools, "ssh"), []byte(`#!/bin/sh
set -eu
destination=""
command=""
for arg do
  case "$arg" in git@*) destination="$arg" ;; git-*) command="$arg" ;; esac
done
test "$destination" = git@bob-ssh || { echo "wrong SSH account" >&2; exit 1; }
printf '%s\n' "$destination" >>"$GH_ACCOUNT_TEST_LOG"
case "$command" in
  git-receive-pack*) exec git receive-pack "$GH_ACCOUNT_TEST_BARE" ;;
  git-upload-pack*) exec git upload-pack "$GH_ACCOUNT_TEST_BARE" ;;
  *) exit 2 ;;
esac
`), 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(tools, "git-remote-https"), []byte(`#!/bin/sh
set -eu
credential=$(printf 'url=%s\n\n' "$2" | git credential fill)
printf '%s\n' "$credential" | grep -qx 'username=bob'
printf '%s\n' "$credential" | grep -qx 'password=fixture-bob-token'
printf 'bob\n' >>"$GH_ACCOUNT_TEST_LOG"
while IFS= read -r line; do
  case "$line" in
    capabilities) printf 'connect\n\n' ;;
    option*) printf 'unsupported\n' ;;
    'connect git-receive-pack') printf '\n'; exec git receive-pack "$GH_ACCOUNT_TEST_BARE" ;;
    'connect git-upload-pack') printf '\n'; exec git upload-pack "$GH_ACCOUNT_TEST_BARE" ;;
    *) exit 2 ;;
  esac
done
`), 0o700))
			repo := t.TempDir()
			t.Chdir(repo)
			accountGit(t, repo, "init", "-q", "--initial-branch=main")
			accountGit(t, repo, "config", "credential.https://github.com.username", "alice")
			store, err := identity.Open()
			require.NoError(t, err)
			require.NoError(t, store.Save(identity.Identity{
				Name: "alice", Host: "github.com", Account: "alice", SSHHost: "alice-ssh",
				GitUserName: "Alice", GitUserEmail: "alice@example.invalid",
			}, false))
			require.NoError(t, store.Save(identity.Identity{Name: "bob", Host: "github.com", Account: "bob", SSHHost: "bob-ssh"}, false))
			require.NoError(t, store.Bind(identity.Binding{Identity: "alice", Directory: repo}, helper))
			accountGit(t, repo, "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "base")
			accountGit(t, repo, "push", "-q", bare, "main")
			accountGit(t, repo, "remote", "add", "origin", tt.url)
			accountGit(t, repo, "update-ref", "refs/remotes/origin/main", "HEAD")
			mainRepo := repo
			repo = filepath.Join(t.TempDir(), "agent-worktree")
			accountGit(t, mainRepo, "worktree", "add", "-b", "feature", repo)
			t.Chdir(repo)
			accountGit(t, repo, "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "feature")
			before := accountGit(t, repo, "log", "-1", "--format=%H|%an|%ae|%cn|%ce")
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			respond := func(body string) httpmock.Responder {
				return func(req *http.Request) (*http.Response, error) {
					assert.Equal(t, "api.github.com", req.URL.Host)
					assert.Equal(t, "token fixture-bob-token", req.Header.Get("Authorization"))
					return httpmock.StringResponse(body)(req)
				}
			}
			reg.Register(httpmock.GraphQL("RepositoryNetwork"), respond(httpmock.RepoNetworkStubResponse("bob", "project", "main", "WRITE")))
			reg.Register(httpmock.GraphQL("RepositoryNetwork"), respond(httpmock.RepoNetworkStubResponse("bob", "project", "main", "WRITE")))
			reg.Register(httpmock.GraphQL("UserCurrent"), respond(`{"data":{"viewer":{"login":"bob"}}}`))
			reg.Register(httpmock.GraphQL("PullRequestForBranch"), respond(`{"data":{"repository":{"pullRequests":{"nodes":[]}}}}`))
			reg.Register(httpmock.GraphQL("PullRequestCreate"), func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, accountGit(t, repo, "rev-parse", "HEAD"), accountGit(t, bare, "rev-parse", "refs/heads/feature"))
				return respond(`{"data":{"createPullRequest":{"pullRequest":{"url":"https://github.com/bob/project/pull/1"}}}}`)(req)
			})
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdinTTY(true)
			ios.SetStdoutTTY(true)
			f := factory.New("test", "", func() (gh.Config, error) { return cfg, nil }, ios, helper, &telemetry.NoOpService{})
			f.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: api.AddAuthTokenHeaderWithResolver(reg, identity.TokenResolver(cfg.Authentication()))}, nil
			}
			f.ExtensionManager = &extensions.ExtensionManagerMock{ListFunc: func() []extensions.Extension { return nil }}
			f.Prompter = &prompter.PrompterMock{SelectFunc: func(prompt, _ string, choices []string) (int, error) {
				assert.Equal(t, "Where should we push the 'feature' branch?", prompt)
				return prompter.IndexFor(choices, "bob/project")
			}}
			cmd, err := NewCmdRoot(f, &telemetry.NoOpService{}, "", "")
			require.NoError(t, err)
			cmd.SetArgs([]string{"pr", "create", "--account", "bob", "--title", "Feature", "--body", "Body", "--base", "main"})

			// When gh creates the PR from an external worktree, including its unpublished Git push
			require.NoError(t, cmd.Execute(), "%s", stderr.String())

			// Then Git and the API authenticate as bob, but the commit, remote URL, and persistent selection are unchanged
			assert.Contains(t, stdout.String(), "https://github.com/bob/project/pull/1")
			evidence, err := os.ReadFile(log)
			require.NoError(t, err)
			assert.Contains(t, string(evidence), "bob")
			assert.Equal(t, before, accountGit(t, repo, "log", "-1", "--format=%H|%an|%ae|%cn|%ce"))
			assert.Equal(t, tt.url, accountGit(t, repo, "config", "remote.origin.url"))
			selected, err := store.Resolve("github.com", "bob", repo)
			require.NoError(t, err)
			assert.Equal(t, "alice", selected.Account)
			mainSelected, err := store.Resolve("github.com", "bob", mainRepo)
			require.NoError(t, err)
			assert.Equal(t, "alice", mainSelected.Account)
		})
	}
}
