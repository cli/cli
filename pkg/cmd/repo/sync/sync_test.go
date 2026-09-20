package sync

import (
	"bytes"
	stdctx "context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdSync(t *testing.T) {
	tests := []struct {
		name    string
		tty     bool
		input   string
		output  SyncOptions
		wantErr bool
		errMsg  string
	}{
		{
			name:   "no argument",
			tty:    true,
			input:  "",
			output: SyncOptions{},
		},
		{
			name:  "destination repo",
			tty:   true,
			input: "cli/cli",
			output: SyncOptions{
				DestArg: "cli/cli",
			},
		},
		{
			name:  "source repo",
			tty:   true,
			input: "--source cli/cli",
			output: SyncOptions{
				SrcArg: "cli/cli",
			},
		},
		{
			name:  "branch",
			tty:   true,
			input: "--branch trunk",
			output: SyncOptions{
				Branch: "trunk",
			},
		},
		{
			name:  "force",
			tty:   true,
			input: "--force",
			output: SyncOptions{
				Force: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdinTTY(tt.tty)
			ios.SetStdoutTTY(tt.tty)
			f := &cmdutil.Factory{
				IOStreams: ios,
			}
			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)
			var gotOpts *SyncOptions
			cmd := NewCmdSync(f, func(opts *SyncOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tt.errMsg, err.Error())
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.output.DestArg, gotOpts.DestArg)
			assert.Equal(t, tt.output.SrcArg, gotOpts.SrcArg)
			assert.Equal(t, tt.output.Branch, gotOpts.Branch)
			assert.Equal(t, tt.output.Force, gotOpts.Force)
		})
	}
}

// initSyncTestRepo creates a fresh git repository on branch "trunk" with an
// isolated config and a configured identity, returning its directory.
func initSyncTestRepo(t *testing.T) string {
	t.Helper()
	git.IsolateConfig(t)
	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "--quiet", "--initial-branch=trunk")
	runGit(t, repoDir, "config", "user.name", "Test User")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	return repoDir
}

func TestExecuteLocalRepoSyncNonCurrentBranch(t *testing.T) {
	t.Run("branch checked out in another worktree", func(t *testing.T) {
		repoDir := initSyncTestRepo(t)
		worktreeDir := filepath.Join(t.TempDir(), "trunk-worktree")

		require.NoError(t, os.WriteFile(filepath.Join(repoDir, "file.txt"), []byte("old\n"), 0o600))
		runGit(t, repoDir, "add", "file.txt")
		runGit(t, repoDir, "commit", "--quiet", "--message=initial")
		runGit(t, repoDir, "switch", "--quiet", "--create", "test")
		runGit(t, repoDir, "worktree", "add", "--quiet", worktreeDir, "trunk")
		originalBranch := runGit(t, repoDir, "rev-parse", "trunk")

		require.NoError(t, os.WriteFile(filepath.Join(repoDir, "file.txt"), []byte("new\n"), 0o600))
		runGit(t, repoDir, "commit", "--quiet", "--all", "--message=upstream")
		runGit(t, repoDir, "fetch", "--quiet", ".", "test")

		opts := &SyncOptions{
			Branch: "trunk",
			Git:    &gitExecuter{client: &git.Client{RepoDir: repoDir}},
		}

		err := executeLocalRepoSync(ghrepo.New("OWNER", "REPO"), "origin", opts)
		require.ErrorContains(t, err, `can't sync "trunk" because it's checked out in another worktree at `)
		// Assert on the directory name rather than the full path because git reports the
		// symlink-resolved path, which differs from t.TempDir() on macOS.
		require.ErrorContains(t, err, filepath.Base(worktreeDir))
		require.ErrorContains(t, err, "tip: run `gh repo sync` from that worktree instead")
		require.Equal(t, originalBranch, runGit(t, repoDir, "rev-parse", "trunk"))
		require.Empty(t, runGit(t, worktreeDir, "status", "--porcelain"))
	})

	t.Run("branch associated with prunable worktree", func(t *testing.T) {
		repoDir := initSyncTestRepo(t)
		worktreeDir := filepath.Join(t.TempDir(), "missing-trunk-worktree")

		runGit(t, repoDir, "commit", "--quiet", "--allow-empty", "--message=initial")
		runGit(t, repoDir, "switch", "--quiet", "--create", "test")
		runGit(t, repoDir, "worktree", "add", "--quiet", worktreeDir, "trunk")
		require.NoError(t, os.RemoveAll(worktreeDir))
		runGit(t, repoDir, "commit", "--quiet", "--allow-empty", "--message=upstream")
		runGit(t, repoDir, "fetch", "--quiet", ".", "test")

		opts := &SyncOptions{
			Branch: "trunk",
			Git:    &gitExecuter{client: &git.Client{RepoDir: repoDir}},
		}

		err := executeLocalRepoSync(ghrepo.New("OWNER", "REPO"), "origin", opts)
		require.ErrorContains(t, err, `can't sync "trunk" because stale worktree metadata still associates it with `)
		require.ErrorContains(t, err, filepath.Base(worktreeDir))
		require.ErrorContains(t, err, "tip: run `git worktree prune` and retry")
	})

	t.Run("fast-forwards non-current branch", func(t *testing.T) {
		repoDir := initSyncTestRepo(t)

		require.NoError(t, os.WriteFile(filepath.Join(repoDir, "file.txt"), []byte("old\n"), 0o600))
		runGit(t, repoDir, "add", "file.txt")
		runGit(t, repoDir, "commit", "--quiet", "--message=initial")
		runGit(t, repoDir, "switch", "--quiet", "--create", "test")
		require.NoError(t, os.WriteFile(filepath.Join(repoDir, "file.txt"), []byte("new\n"), 0o600))
		runGit(t, repoDir, "commit", "--quiet", "--all", "--message=upstream")
		runGit(t, repoDir, "fetch", "--quiet", ".", "test")

		opts := &SyncOptions{
			Branch: "trunk",
			Git:    &gitExecuter{client: &git.Client{RepoDir: repoDir}},
		}

		err := executeLocalRepoSync(ghrepo.New("OWNER", "REPO"), "origin", opts)
		require.NoError(t, err)
		require.Equal(t, runGit(t, repoDir, "rev-parse", "FETCH_HEAD"), runGit(t, repoDir, "rev-parse", "trunk"))
		require.Equal(t, "test", runGit(t, repoDir, "branch", "--show-current"))
	})

	t.Run("force-updates diverged non-current branch", func(t *testing.T) {
		repoDir := initSyncTestRepo(t)

		require.NoError(t, os.WriteFile(filepath.Join(repoDir, "file.txt"), []byte("initial\n"), 0o600))
		runGit(t, repoDir, "add", "file.txt")
		runGit(t, repoDir, "commit", "--quiet", "--message=initial")
		runGit(t, repoDir, "branch", "test")
		require.NoError(t, os.WriteFile(filepath.Join(repoDir, "file.txt"), []byte("local\n"), 0o600))
		runGit(t, repoDir, "commit", "--quiet", "--all", "--message=local")
		runGit(t, repoDir, "switch", "--quiet", "test")
		require.NoError(t, os.WriteFile(filepath.Join(repoDir, "file.txt"), []byte("upstream\n"), 0o600))
		runGit(t, repoDir, "commit", "--quiet", "--all", "--message=upstream")
		runGit(t, repoDir, "fetch", "--quiet", ".", "test")

		opts := &SyncOptions{
			Branch: "trunk",
			Force:  true,
			Git:    &gitExecuter{client: &git.Client{RepoDir: repoDir}},
		}

		err := executeLocalRepoSync(ghrepo.New("OWNER", "REPO"), "origin", opts)
		require.NoError(t, err)
		require.Equal(t, runGit(t, repoDir, "rev-parse", "FETCH_HEAD"), runGit(t, repoDir, "rev-parse", "trunk"))
		require.Equal(t, "test", runGit(t, repoDir, "branch", "--show-current"))
	})
}

func TestUpdateBranchInSingleWorktreeRepository(t *testing.T) {
	git.IsolateConfig(t)

	repoDir := t.TempDir()

	runGit(t, repoDir, "init", "--quiet", "--initial-branch=trunk")
	runGit(t, repoDir, "config", "user.name", "Test User")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "file.txt"), []byte("old\n"), 0o600))
	runGit(t, repoDir, "add", "file.txt")
	runGit(t, repoDir, "commit", "--quiet", "--message=initial")
	runGit(t, repoDir, "switch", "--quiet", "--create", "test")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "file.txt"), []byte("new\n"), 0o600))
	runGit(t, repoDir, "commit", "--quiet", "--all", "--message=upstream")
	runGit(t, repoDir, "fetch", "--quiet", ".", "test")

	gitClient := &git.Client{RepoDir: repoDir}
	executer := &gitExecuter{client: gitClient}

	err := executer.UpdateBranch("trunk", "FETCH_HEAD")
	require.NoError(t, err)
	require.Equal(t, runGit(t, repoDir, "rev-parse", "FETCH_HEAD"), runGit(t, repoDir, "rev-parse", "trunk"))
	require.Equal(t, "test", runGit(t, repoDir, "branch", "--show-current"))
	require.Empty(t, runGit(t, repoDir, "status", "--porcelain"))
}

func runGit(t *testing.T, repoDir string, args ...string) string {
	t.Helper()
	client := &git.Client{RepoDir: repoDir}
	cmd, err := client.Command(stdctx.Background(), args...)
	require.NoError(t, err)
	output, err := cmd.Output()
	require.NoErrorf(t, err, "git %s failed", strings.Join(args, " "))
	return strings.TrimSpace(string(output))
}

func Test_SyncRun(t *testing.T) {
	tests := []struct {
		name       string
		tty        bool
		opts       *SyncOptions
		remotes    []*context.Remote
		httpStubs  func(*httpmock.Registry)
		gitStubs   func(*mockGitClient)
		wantStdout string
		wantErr    bool
		errMsg     string
	}{
		{
			name: "sync local repo with parent - tty",
			tty:  true,
			opts: &SyncOptions{},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`{"data":{"repository":{"defaultBranchRef":{"name": "trunk"}}}}`))
			},
			gitStubs: func(mgc *mockGitClient) {
				mgc.On("Fetch", "origin", "refs/heads/trunk").Return(nil).Once()
				mgc.On("HasLocalBranch", "trunk").Return(true).Once()
				mgc.On("IsAncestor", "trunk", "FETCH_HEAD").Return(true, nil).Once()
				mgc.On("CurrentBranch").Return("trunk", nil).Once()
				mgc.On("IsDirty").Return(false, nil).Once()
				mgc.On("MergeFastForward", "FETCH_HEAD").Return(nil).Once()
			},
			wantStdout: "✓ Synced the \"trunk\" branch from \"OWNER/REPO\" to local repository\n",
		},
		{
			name: "sync local repo with parent - notty",
			tty:  false,
			opts: &SyncOptions{
				Branch: "trunk",
			},
			gitStubs: func(mgc *mockGitClient) {
				mgc.On("Fetch", "origin", "refs/heads/trunk").Return(nil).Once()
				mgc.On("HasLocalBranch", "trunk").Return(true).Once()
				mgc.On("IsAncestor", "trunk", "FETCH_HEAD").Return(true, nil).Once()
				mgc.On("CurrentBranch").Return("trunk", nil).Once()
				mgc.On("IsDirty").Return(false, nil).Once()
				mgc.On("MergeFastForward", "FETCH_HEAD").Return(nil).Once()
			},
			wantStdout: "",
		},
		{
			name: "sync local repo with specified source repo",
			tty:  true,
			opts: &SyncOptions{
				Branch: "trunk",
				SrcArg: "OWNER2/REPO2",
			},
			gitStubs: func(mgc *mockGitClient) {
				mgc.On("Fetch", "upstream", "refs/heads/trunk").Return(nil).Once()
				mgc.On("HasLocalBranch", "trunk").Return(true).Once()
				mgc.On("IsAncestor", "trunk", "FETCH_HEAD").Return(true, nil).Once()
				mgc.On("CurrentBranch").Return("trunk", nil).Once()
				mgc.On("IsDirty").Return(false, nil).Once()
				mgc.On("MergeFastForward", "FETCH_HEAD").Return(nil).Once()
			},
			wantStdout: "✓ Synced the \"trunk\" branch from \"OWNER2/REPO2\" to local repository\n",
		},
		{
			name: "sync local repo with parent and force specified",
			tty:  true,
			opts: &SyncOptions{
				Branch: "trunk",
				Force:  true,
			},
			gitStubs: func(mgc *mockGitClient) {
				mgc.On("Fetch", "origin", "refs/heads/trunk").Return(nil).Once()
				mgc.On("HasLocalBranch", "trunk").Return(true).Once()
				mgc.On("IsAncestor", "trunk", "FETCH_HEAD").Return(false, nil).Once()
				mgc.On("CurrentBranch").Return("trunk", nil).Once()
				mgc.On("IsDirty").Return(false, nil).Once()
				mgc.On("ResetHard", "FETCH_HEAD").Return(nil).Once()
			},
			wantStdout: "✓ Synced the \"trunk\" branch from \"OWNER/REPO\" to local repository\n",
		},
		{
			name: "sync local repo with specified source repo and force specified",
			tty:  true,
			opts: &SyncOptions{
				Branch: "trunk",
				SrcArg: "OWNER2/REPO2",
				Force:  true,
			},
			gitStubs: func(mgc *mockGitClient) {
				mgc.On("Fetch", "upstream", "refs/heads/trunk").Return(nil).Once()
				mgc.On("HasLocalBranch", "trunk").Return(true).Once()
				mgc.On("IsAncestor", "trunk", "FETCH_HEAD").Return(false, nil).Once()
				mgc.On("CurrentBranch").Return("trunk", nil).Once()
				mgc.On("IsDirty").Return(false, nil).Once()
				mgc.On("ResetHard", "FETCH_HEAD").Return(nil).Once()
			},
			wantStdout: "✓ Synced the \"trunk\" branch from \"OWNER2/REPO2\" to local repository\n",
		},
		{
			name: "sync local repo with parent and not fast forward merge",
			tty:  true,
			opts: &SyncOptions{
				Branch: "trunk",
			},
			gitStubs: func(mgc *mockGitClient) {
				mgc.On("Fetch", "origin", "refs/heads/trunk").Return(nil).Once()
				mgc.On("HasLocalBranch", "trunk").Return(true).Once()
				mgc.On("IsAncestor", "trunk", "FETCH_HEAD").Return(false, nil).Once()
			},
			wantErr: true,
			errMsg:  "can't sync because there are diverging changes; use `--force` to overwrite the destination branch",
		},
		{
			name: "sync local repo with specified source repo and not fast forward merge",
			tty:  true,
			opts: &SyncOptions{
				Branch: "trunk",
				SrcArg: "OWNER2/REPO2",
			},
			gitStubs: func(mgc *mockGitClient) {
				mgc.On("Fetch", "upstream", "refs/heads/trunk").Return(nil).Once()
				mgc.On("HasLocalBranch", "trunk").Return(true).Once()
				mgc.On("IsAncestor", "trunk", "FETCH_HEAD").Return(false, nil).Once()
			},
			wantErr: true,
			errMsg:  "can't sync because there are diverging changes; use `--force` to overwrite the destination branch",
		},
		{
			name: "sync local repo with parent and local changes",
			tty:  true,
			opts: &SyncOptions{
				Branch: "trunk",
			},
			gitStubs: func(mgc *mockGitClient) {
				mgc.On("Fetch", "origin", "refs/heads/trunk").Return(nil).Once()
				mgc.On("HasLocalBranch", "trunk").Return(true).Once()
				mgc.On("IsAncestor", "trunk", "FETCH_HEAD").Return(true, nil).Once()
				mgc.On("CurrentBranch").Return("trunk", nil).Once()
				mgc.On("IsDirty").Return(true, nil).Once()
			},
			wantErr: true,
			errMsg:  "refusing to sync due to uncommitted/untracked local changes\ntip: use `git stash --all` before retrying the sync and run `git stash pop` afterwards",
		},
		{
			name: "sync local repo with parent - existing branch, non-current",
			tty:  true,
			opts: &SyncOptions{
				Branch: "trunk",
			},
			gitStubs: func(mgc *mockGitClient) {
				mgc.On("Fetch", "origin", "refs/heads/trunk").Return(nil).Once()
				mgc.On("HasLocalBranch", "trunk").Return(true).Once()
				mgc.On("IsAncestor", "trunk", "FETCH_HEAD").Return(true, nil).Once()
				mgc.On("CurrentBranch").Return("test", nil).Once()
				mgc.On("UpdateBranch", "trunk", "FETCH_HEAD").Return(nil).Once()
			},
			wantStdout: "✓ Synced the \"trunk\" branch from \"OWNER/REPO\" to local repository\n",
		},
		{
			name: "sync local repo with parent - existing branch checked out in another worktree",
			tty:  true,
			opts: &SyncOptions{
				Branch: "trunk",
			},
			gitStubs: func(mgc *mockGitClient) {
				mgc.On("Fetch", "origin", "refs/heads/trunk").Return(nil).Once()
				mgc.On("HasLocalBranch", "trunk").Return(true).Once()
				mgc.On("IsAncestor", "trunk", "FETCH_HEAD").Return(true, nil).Once()
				mgc.On("CurrentBranch").Return("test", nil).Once()
				mgc.On("UpdateBranch", "trunk", "FETCH_HEAD").Return(errors.New("branch is checked out in another worktree")).Once()
				mgc.On("Worktrees").Return([]git.Worktree{{Path: "/path/to/worktree", Ref: "refs/heads/trunk"}}, nil).Once()
			},
			wantErr: true,
			errMsg:  "can't sync \"trunk\" because it's checked out in another worktree at /path/to/worktree\ntip: run `gh repo sync` from that worktree instead",
		},
		{
			name: "sync local repo with parent - existing branch associated with prunable worktree",
			tty:  true,
			opts: &SyncOptions{
				Branch: "trunk",
			},
			gitStubs: func(mgc *mockGitClient) {
				mgc.On("Fetch", "origin", "refs/heads/trunk").Return(nil).Once()
				mgc.On("HasLocalBranch", "trunk").Return(true).Once()
				mgc.On("IsAncestor", "trunk", "FETCH_HEAD").Return(true, nil).Once()
				mgc.On("CurrentBranch").Return("test", nil).Once()
				mgc.On("UpdateBranch", "trunk", "FETCH_HEAD").Return(errors.New("branch is checked out in another worktree")).Once()
				mgc.On("Worktrees").Return([]git.Worktree{{Path: "/path/to/missing-worktree", Ref: "refs/heads/trunk", Prunable: true}}, nil).Once()
			},
			wantErr: true,
			errMsg:  "can't sync \"trunk\" because stale worktree metadata still associates it with /path/to/missing-worktree\ntip: run `git worktree prune` and retry",
		},
		{
			name: "sync local repo with parent - create new branch",
			tty:  true,
			opts: &SyncOptions{
				Branch: "trunk",
			},
			gitStubs: func(mgc *mockGitClient) {
				mgc.On("Fetch", "origin", "refs/heads/trunk").Return(nil).Once()
				mgc.On("HasLocalBranch", "trunk").Return(false).Once()
				mgc.On("CurrentBranch").Return("test", nil).Once()
				mgc.On("CreateBranch", "trunk", "FETCH_HEAD", "origin/trunk").Return(nil).Once()
			},
			wantStdout: "✓ Synced the \"trunk\" branch from \"OWNER/REPO\" to local repository\n",
		},
		{
			name: "sync remote fork with parent with new api - tty",
			tty:  true,
			opts: &SyncOptions{
				DestArg: "FORKOWNER/REPO-FORK",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`{"data":{"repository":{"defaultBranchRef":{"name": "trunk"}}}}`))
				reg.Register(
					httpmock.REST("POST", "repos/FORKOWNER/REPO-FORK/merge-upstream"),
					httpmock.StatusStringResponse(200, `{"base_branch": "OWNER:trunk"}`))
			},
			wantStdout: "✓ Synced the \"FORKOWNER:trunk\" branch from \"OWNER:trunk\"\n",
		},
		{
			name: "sync remote fork with parent using api fallback - tty",
			tty:  true,
			opts: &SyncOptions{
				DestArg: "FORKOWNER/REPO-FORK",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryFindParent\b`),
					httpmock.StringResponse(`{"data":{"repository":{"parent":{"name":"REPO","owner":{"login": "OWNER"}}}}}`))
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`{"data":{"repository":{"defaultBranchRef":{"name": "trunk"}}}}`))
				reg.Register(
					httpmock.REST("POST", "repos/FORKOWNER/REPO-FORK/merge-upstream"),
					httpmock.StatusStringResponse(422, `{}`))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/git/refs/heads%2Ftrunk"),
					httpmock.StringResponse(`{"object":{"sha":"0xDEADBEEF"}}`))
				reg.Register(
					httpmock.REST("PATCH", "repos/FORKOWNER/REPO-FORK/git/refs/heads%2Ftrunk"),
					httpmock.StringResponse(`{}`))
			},
			wantStdout: "✓ Synced the \"FORKOWNER:trunk\" branch from \"OWNER:trunk\"\n",
		},
		{
			name: "sync remote fork with parent - notty",
			tty:  false,
			opts: &SyncOptions{
				DestArg: "FORKOWNER/REPO-FORK",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`{"data":{"repository":{"defaultBranchRef":{"name": "trunk"}}}}`))
				reg.Register(
					httpmock.REST("POST", "repos/FORKOWNER/REPO-FORK/merge-upstream"),
					httpmock.StatusStringResponse(200, `{"base_branch": "OWNER:trunk"}`))
			},
			wantStdout: "",
		},
		{
			name: "sync remote repo with no parent",
			tty:  true,
			opts: &SyncOptions{
				DestArg: "OWNER/REPO",
				Branch:  "trunk",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/merge-upstream"),
					httpmock.StatusStringResponse(422, `{"message": "Validation Failed"}`))
				reg.Register(
					httpmock.GraphQL(`query RepositoryFindParent\b`),
					httpmock.StringResponse(`{"data":{"repository":{"parent":null}}}`))
			},
			wantErr: true,
			errMsg:  "can't determine source repository for OWNER/REPO because repository is not fork",
		},
		{
			name: "sync remote repo with specified source repo",
			tty:  true,
			opts: &SyncOptions{
				DestArg: "OWNER/REPO",
				SrcArg:  "OWNER2/REPO2",
				Branch:  "trunk",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO/merge-upstream"),
					httpmock.StatusStringResponse(200, `{"base_branch": "OWNER2:trunk"}`))
			},
			wantStdout: "✓ Synced the \"OWNER:trunk\" branch from \"OWNER2:trunk\"\n",
		},
		{
			name: "sync remote fork with parent and specified branch",
			tty:  true,
			opts: &SyncOptions{
				DestArg: "OWNER/REPO-FORK",
				Branch:  "test",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO-FORK/merge-upstream"),
					httpmock.StatusStringResponse(200, `{"base_branch": "OWNER:test"}`))
			},
			wantStdout: "✓ Synced the \"OWNER:test\" branch from \"OWNER:test\"\n",
		},
		{
			name: "sync remote fork with parent and force specified",
			tty:  true,
			opts: &SyncOptions{
				DestArg: "OWNER/REPO-FORK",
				Force:   true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryFindParent\b`),
					httpmock.StringResponse(`{"data":{"repository":{"parent":{"name":"REPO","owner":{"login": "OWNER"}}}}}`))
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`{"data":{"repository":{"defaultBranchRef":{"name": "trunk"}}}}`))
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO-FORK/merge-upstream"),
					httpmock.StatusStringResponse(409, `{"message": "Merge conflict"}`))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/git/refs/heads%2Ftrunk"),
					httpmock.StringResponse(`{"object":{"sha":"0xDEADBEEF"}}`))
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO-FORK/git/refs/heads%2Ftrunk"),
					httpmock.StringResponse(`{}`))
			},
			wantStdout: "✓ Synced the \"OWNER:trunk\" branch from \"OWNER:trunk\"\n",
		},
		{
			name: "sync remote fork with parent and not fast forward merge",
			tty:  true,
			opts: &SyncOptions{
				DestArg: "OWNER/REPO-FORK",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryFindParent\b`),
					httpmock.StringResponse(`{"data":{"repository":{"parent":{"name":"REPO","owner":{"login": "OWNER"}}}}}`))
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`{"data":{"repository":{"defaultBranchRef":{"name": "trunk"}}}}`))
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO-FORK/merge-upstream"),
					httpmock.StatusStringResponse(409, `{"message": "Merge conflict"}`))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/git/refs/heads%2Ftrunk"),
					httpmock.StringResponse(`{"object":{"sha":"0xDEADBEEF"}}`))
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO-FORK/git/refs/heads%2Ftrunk"),
					func(req *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: 422,
							Request:    req,
							Header:     map[string][]string{"Content-Type": {"application/json"}},
							Body:       io.NopCloser(bytes.NewBufferString(`{"message":"Update is not a fast forward"}`)),
						}, nil
					})
			},
			wantErr: true,
			errMsg:  "can't sync because there are diverging changes; use `--force` to overwrite the destination branch",
		},
		{
			name: "sync remote fork with parent and no existing branch on fork",
			tty:  true,
			opts: &SyncOptions{
				DestArg: "OWNER/REPO-FORK",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryFindParent\b`),
					httpmock.StringResponse(`{"data":{"repository":{"parent":{"name":"REPO","owner":{"login": "OWNER"}}}}}`))
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`{"data":{"repository":{"defaultBranchRef":{"name": "trunk"}}}}`))
				reg.Register(
					httpmock.REST("POST", "repos/OWNER/REPO-FORK/merge-upstream"),
					httpmock.StatusStringResponse(409, `{"message": "Merge conflict"}`))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/git/refs/heads%2Ftrunk"),
					httpmock.StringResponse(`{"object":{"sha":"0xDEADBEEF"}}`))
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO-FORK/git/refs/heads%2Ftrunk"),
					func(req *http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: 422,
							Request:    req,
							Header:     map[string][]string{"Content-Type": {"application/json"}},
							Body:       io.NopCloser(bytes.NewBufferString(`{"message":"Reference does not exist"}`)),
						}, nil
					})
			},
			wantErr: true,
			errMsg:  "trunk branch does not exist on OWNER/REPO-FORK repository",
		},
		{
			name: "sync remote fork with missing workflow scope on OAuth App token",
			opts: &SyncOptions{
				DestArg: "FORKOWNER/REPO-FORK",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`{"data":{"repository":{"defaultBranchRef":{"name": "trunk"}}}}`))
				reg.Register(
					httpmock.REST("POST", "repos/FORKOWNER/REPO-FORK/merge-upstream"),
					httpmock.StatusJSONResponse(422, struct {
						Message string `json:"message"`
					}{Message: "refusing to allow an OAuth App to create or update workflow `.github/workflows/unimportant.yml` without `workflow` scope"}))
			},
			wantErr: true,
			errMsg:  "Upstream commits contain workflow changes, which require the `workflow` scope or permission to merge. To request it, run: gh auth refresh -s workflow",
		},
		{
			name: "sync remote fork with missing workflow permission on GitHub App token",
			opts: &SyncOptions{
				DestArg: "FORKOWNER/REPO-FORK",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`{"data":{"repository":{"defaultBranchRef":{"name": "trunk"}}}}`))
				reg.Register(
					httpmock.REST("POST", "repos/FORKOWNER/REPO-FORK/merge-upstream"),
					httpmock.StatusJSONResponse(422, struct {
						Message string `json:"message"`
					}{Message: "refusing to allow a GitHub App to create or update workflow `.github/workflows/unimportant.yml` without `workflows` permission"}))
			},
			wantErr: true,
			errMsg:  "Upstream commits contain workflow changes, which require the `workflow` scope or permission to merge. To request it, run: gh auth refresh -s workflow",
		},
		{
			name: "sync remote fork with missing workflow scope on personal access token",
			opts: &SyncOptions{
				DestArg: "FORKOWNER/REPO-FORK",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`{"data":{"repository":{"defaultBranchRef":{"name": "trunk"}}}}`))
				reg.Register(
					httpmock.REST("POST", "repos/FORKOWNER/REPO-FORK/merge-upstream"),
					httpmock.StatusJSONResponse(422, struct {
						Message string `json:"message"`
					}{Message: "refusing to allow a Personal Access Token to create or update workflow `.github/workflows/unimportant.yml` without `workflow` scope"}))
			},
			wantErr: true,
			errMsg:  "Upstream commits contain workflow changes, which require the `workflow` scope or permission to merge. To request it, run: gh auth refresh -s workflow",
		},
	}
	for _, tt := range tests {
		reg := &httpmock.Registry{}
		if tt.httpStubs != nil {
			tt.httpStubs(reg)
		}
		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}

		ios, _, stdout, _ := iostreams.Test()
		ios.SetStdinTTY(tt.tty)
		ios.SetStdoutTTY(tt.tty)
		tt.opts.IO = ios

		repo1, _ := ghrepo.FromFullName("OWNER/REPO")
		repo2, _ := ghrepo.FromFullName("OWNER2/REPO2")
		tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
			return repo1, nil
		}

		tt.opts.Remotes = func() (context.Remotes, error) {
			if tt.remotes == nil {
				return []*context.Remote{
					{
						Remote: &git.Remote{Name: "origin"},
						Repo:   repo1,
					},
					{
						Remote: &git.Remote{Name: "upstream"},
						Repo:   repo2,
					},
				}, nil
			}
			return tt.remotes, nil
		}

		t.Run(tt.name, func(t *testing.T) {
			tt.opts.Git = newMockGitClient(t, tt.gitStubs)
			defer reg.Verify(t)
			err := syncRun(tt.opts)
			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
				return
			} else if err != nil {
				t.Fatalf("syncRun() unexpected error: %v", err)
			}
			assert.Equal(t, tt.wantStdout, stdout.String())
		})
	}
}

func newMockGitClient(t *testing.T, config func(*mockGitClient)) *mockGitClient {
	t.Helper()
	m := &mockGitClient{}
	m.Test(t)
	t.Cleanup(func() {
		t.Helper()
		m.AssertExpectations(t)
	})
	if config != nil {
		config(m)
	}
	return m
}
