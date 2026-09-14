package git

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientCommand(t *testing.T) {
	tests := []struct {
		name     string
		repoDir  string
		gitPath  string
		wantExe  string
		wantArgs []string
	}{
		{
			name:     "creates command",
			gitPath:  "path/to/git",
			wantExe:  "path/to/git",
			wantArgs: []string{"path/to/git", "ref-log"},
		},
		{
			name:     "adds repo directory configuration",
			repoDir:  "path/to/repo",
			gitPath:  "path/to/git",
			wantExe:  "path/to/git",
			wantArgs: []string{"path/to/git", "-C", "path/to/repo", "ref-log"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given a client with explicit executable, streams, and repository context
			in, out, errOut := &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}
			client := Client{
				Stdin:   in,
				Stdout:  out,
				Stderr:  errOut,
				RepoDir: tt.repoDir,
				GitPath: tt.gitPath,
			}

			// When a Git command is created
			cmd, err := client.Command(t.Context(), "ref-log")

			// Then the executable, repository arguments, and streams are wired
			require.NoError(t, err)
			assert.Equal(t, tt.wantExe, cmd.Path)
			assert.Equal(t, tt.wantArgs, cmd.Args)
			assert.Equal(t, in, cmd.Stdin)
			assert.Equal(t, out, cmd.Stdout)
			assert.Equal(t, errOut, cmd.Stderr)
		})
	}
}

func TestClientAuthenticatedCommandScopesCredentialHelperToAllHosts(t *testing.T) {
	// Given a client with an explicit gh executable
	client := Client{
		GhPath:  "path/to/gh",
		GitPath: "path/to/git",
	}

	// When an authenticated command is created for all hosts
	cmd, err := client.AuthenticatedCommand(t.Context(), AllMatchingCredentialsPattern, "fetch")

	// Then existing helpers are cleared and gh receives credentials for every host
	require.NoError(t, err)
	assert.Equal(t, []string{"path/to/git", "-c", "credential.helper=", "-c", `credential.helper=!"path/to/gh" auth git-credential`, "fetch"}, cmd.Args)
}

func TestClientAuthenticatedCommandScopesCredentialHelperToPattern(t *testing.T) {
	// Given a client with an explicit gh executable
	client := Client{
		GhPath:  "path/to/gh",
		GitPath: "path/to/git",
	}

	// When an authenticated command is created for one credential pattern
	cmd, err := client.AuthenticatedCommand(t.Context(), CredentialPattern{pattern: "https://github.com"}, "fetch")

	// Then existing helpers are cleared and gh receives credentials only for that pattern
	require.NoError(t, err)
	assert.Equal(t, []string{"path/to/git", "-c", "credential.https://github.com.helper=", "-c", `credential.https://github.com.helper=!"path/to/gh" auth git-credential`, "fetch"}, cmd.Args)
}

func TestClientAuthenticatedCommandDefaultsToGhOnPath(t *testing.T) {
	// Given a client without an explicit gh executable
	client := Client{GitPath: "path/to/git"}

	// When an authenticated command is created
	cmd, err := client.AuthenticatedCommand(t.Context(), AllMatchingCredentialsPattern, "fetch")

	// Then gh is resolved from PATH by the credential helper
	require.NoError(t, err)
	assert.Equal(t, []string{"path/to/git", "-c", "credential.helper=", "-c", `credential.helper=!"gh" auth git-credential`, "fetch"}, cmd.Args)
}

func TestClientAuthenticatedCommandRejectsEmptyCredentialPattern(t *testing.T) {
	// Given a client and an empty host-scoped credential pattern
	client := Client{GitPath: "path/to/git"}

	// When an authenticated command is created
	cmd, err := client.AuthenticatedCommand(t.Context(), CredentialPattern{}, "fetch")

	// Then the insecure ambiguous scope is rejected
	require.EqualError(t, err, "empty credential pattern is not allowed unless provided explicitly")
	assert.Nil(t, cmd)
}

func TestClientRemotesReturnsSortedRemotesWithResolutions(t *testing.T) {
	// Given remotes with resolution metadata
	repo := newTestRepo(t)
	repo.run(t, "remote", "add", "origin", "git@example.com:monalisa/origin.git")
	repo.run(t, "remote", "add", "test", "git://github.com/hubot/test.git")
	repo.run(t, "remote", "add", "upstream", "https://github.com/monalisa/upstream.git")
	repo.run(t, "remote", "add", "github", "git@github.com:hubot/github.git")
	repo.run(t, "config", "remote.test.gh-resolved", "other")
	repo.run(t, "config", "remote.upstream.gh-resolved", "base")

	// When the remotes are listed
	remotes, err := repo.client.Remotes(t.Context())

	// Then preferred names are sorted first and resolutions are associated by name
	require.NoError(t, err)
	require.Len(t, remotes, 4)
	assert.Equal(t, "upstream", remotes[0].Name)
	assert.Equal(t, "base", remotes[0].Resolved)
	assert.Equal(t, "github", remotes[1].Name)
	assert.Empty(t, remotes[1].Resolved)
	assert.Equal(t, "origin", remotes[2].Name)
	assert.Empty(t, remotes[2].Resolved)
	assert.Equal(t, "test", remotes[3].Name)
	assert.Equal(t, "other", remotes[3].Resolved)
}

func TestClientRemotesToleratesMissingResolutionConfig(t *testing.T) {
	// Given a remote without resolution metadata
	repo := newTestRepo(t)
	repo.run(t, "remote", "add", "origin", "git@example.com:monalisa/origin.git")

	// When the remotes are listed
	remotes, err := repo.client.Remotes(t.Context())

	// Then Git's no-matching-config status is treated as an empty resolution
	require.NoError(t, err)
	require.Len(t, remotes, 1)
	assert.Equal(t, "origin", remotes[0].Name)
	assert.Empty(t, remotes[0].Resolved)
}

func TestParseRemotes(t *testing.T) {
	// Given remote output with fetch-only, push-only, paired, and mixed URL forms
	remoteList := []string{
		"mona\tgit@github.com:monalisa/myfork.git (fetch)",
		"origin\thttps://github.com/monalisa/octo-cat.git (fetch)",
		"origin\thttps://github.com/monalisa/octo-cat-push.git (push)",
		"upstream\thttps://example.com/nowhere.git (fetch)",
		"upstream\thttps://github.com/hubot/tools (push)",
		"zardoz\thttps://example.com/zed.git (push)",
		"koke\tgit://github.com/koke/grit.git (fetch)",
		"koke\tgit://github.com/koke/grit.git (push)",
	}

	// When the output is parsed
	remotes := parseRemotes(remoteList)

	// Then each URL is associated with its named remote and direction
	require.Len(t, remotes, 5)
	assert.Equal(t, "mona", remotes[0].Name)
	assert.Equal(t, "ssh://git@github.com/monalisa/myfork.git", remotes[0].FetchURL.String())
	assert.Nil(t, remotes[0].PushURL)

	assert.Equal(t, "origin", remotes[1].Name)
	assert.Equal(t, "/monalisa/octo-cat.git", remotes[1].FetchURL.Path)
	assert.Equal(t, "/monalisa/octo-cat-push.git", remotes[1].PushURL.Path)

	assert.Equal(t, "upstream", remotes[2].Name)
	assert.Equal(t, "example.com", remotes[2].FetchURL.Host)
	assert.Equal(t, "github.com", remotes[2].PushURL.Host)

	assert.Equal(t, "zardoz", remotes[3].Name)
	assert.Nil(t, remotes[3].FetchURL)
	assert.Equal(t, "https://example.com/zed.git", remotes[3].PushURL.String())

	assert.Equal(t, "koke", remotes[4].Name)
	assert.Equal(t, "/koke/grit.git", remotes[4].FetchURL.Path)
	assert.Equal(t, "/koke/grit.git", remotes[4].PushURL.Path)
}

func TestClientUpdateRemoteURLUpdatesConfiguredRemote(t *testing.T) {
	// Given a repository with a local remote
	repo := newTestRepo(t)
	original := newBareTestRepo(t)
	replacement := newBareTestRepo(t)
	repo.run(t, "remote", "add", "test", original.fileURL())

	// When the remote URL is updated
	err := repo.client.UpdateRemoteURL(t.Context(), "test", replacement.fileURL())

	// Then Git stores the replacement URL
	require.NoError(t, err)
	assert.Equal(t, replacement.fileURL(), repo.run(t, "remote", "get-url", "test"))
}

func TestClientUpdateRemoteURLReportsMissingRemote(t *testing.T) {
	// Given a repository without the named remote
	repo := newTestRepo(t)

	// When the remote URL is updated
	err := repo.client.UpdateRemoteURL(t.Context(), "missing", newBareTestRepo(t).fileURL())

	// Then Git reports a semantic command failure
	require.Error(t, err)
	var gitErr *GitError
	require.ErrorAs(t, err, &gitErr)
	assert.NotZero(t, gitErr.ExitCode)
}

func TestClientSetRemoteResolutionAddsResolution(t *testing.T) {
	// Given a repository with an existing remote resolution
	repo := newTestRepo(t)
	repo.run(t, "config", "--add", "remote.origin.gh-resolved", "base")

	// When another resolution is set
	err := repo.client.SetRemoteResolution(t.Context(), "origin", "other")

	// Then both resolutions are stored
	require.NoError(t, err)
	assert.Equal(t, "base\nother", repo.run(t, "config", "--get-all", "remote.origin.gh-resolved"))
}

func TestClientSetRemoteResolutionPropagatesGitError(t *testing.T) {
	// Given a client whose repository directory does not exist
	client := Client{RepoDir: filepath.Join(t.TempDir(), "missing")}

	// When a remote resolution is set
	err := client.SetRemoteResolution(t.Context(), "origin", "base")

	// Then the Git error is returned
	require.Error(t, err)
	var gitErr *GitError
	require.ErrorAs(t, err, &gitErr)
	assert.NotZero(t, gitErr.ExitCode)
}

func TestClientCurrentBranchReturnsCurrentBranch(t *testing.T) {
	// Given a repository checked out on a branch containing non-breaking spaces
	repo := newTestRepo(t)
	branchName := "branch\u00A0with\u00A0non\u00A0breaking\u00A0space"
	repo.run(t, "checkout", "--quiet", "-b", branchName)

	// When the current branch is requested
	branch, err := repo.client.CurrentBranch(t.Context())

	// Then the complete branch name is returned
	require.NoError(t, err)
	assert.Equal(t, branchName, branch)
}

func TestClientCurrentBranchReportsDetachedHead(t *testing.T) {
	// Given a repository with a detached HEAD
	repo := newTestRepo(t)
	repo.run(t, "commit", "--quiet", "--allow-empty", "-m", "initial commit")
	repo.run(t, "checkout", "--quiet", "--detach")

	// When the current branch is requested
	branch, err := repo.client.CurrentBranch(t.Context())

	// Then the detached HEAD error is returned
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotOnAnyBranch)
	assert.Empty(t, branch)
}

func TestClientShowRefsReturnsExistingRefsAlongsideMissingRefError(t *testing.T) {
	// Given a repository with one requested branch and one missing branch
	repo := newTestRepo(t)
	repo.run(t, "commit", "--quiet", "--allow-empty", "-m", "initial commit")
	wantHash := repo.run(t, "rev-parse", "HEAD")

	// When both refs are resolved
	refs, err := repo.client.ShowRefs(t.Context(), []string{"refs/heads/trunk", "refs/heads/missing"})

	// Then the existing ref and the semantic Git error are both returned
	require.Error(t, err)
	var gitError *GitError
	require.ErrorAs(t, err, &gitError)
	assert.NotZero(t, gitError.ExitCode)
	assert.Equal(t, []Ref{{Hash: wantHash, Name: "refs/heads/trunk"}}, refs)
}

func TestClientConfigReturnsConfiguredValue(t *testing.T) {
	// Given a repository with a configured credential helper
	repo := newTestRepo(t)
	repo.run(t, "config", "credential.helper", "test")

	// When the configuration is read
	value, err := repo.client.Config(t.Context(), "credential.helper")

	// Then the configured value is returned
	require.NoError(t, err)
	assert.Equal(t, "test", value)
}

func TestClientConfigReportsUnknownKey(t *testing.T) {
	// Given a repository without a credential helper
	repo := newTestRepo(t)

	// When the missing configuration is read
	value, err := repo.client.Config(t.Context(), "credential.helper")

	// Then the missing key is identified
	require.Error(t, err)
	assert.EqualError(t, err, "failed to run git: unknown config key credential.helper")
	assert.Empty(t, value)
}

func TestClientConfigPropagatesGitError(t *testing.T) {
	// Given a client whose repository directory does not exist
	client := Client{RepoDir: filepath.Join(t.TempDir(), "missing")}

	// When configuration is read
	value, err := client.Config(t.Context(), "credential.helper")

	// Then the Git error is returned unchanged
	require.Error(t, err)
	var gitErr *GitError
	require.ErrorAs(t, err, &gitErr)
	assert.NotZero(t, gitErr.ExitCode)
	assert.Empty(t, value)
}

func TestClientUncommittedChangeCountReturnsZeroForCleanRepository(t *testing.T) {
	// Given a clean repository
	repo := newTestRepo(t)

	// When uncommitted changes are counted
	count, err := repo.client.UncommittedChangeCount(t.Context())

	// Then no changes are reported
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestClientUncommittedChangeCountIncludesTrackedAndUntrackedFiles(t *testing.T) {
	// Given a repository with one modified file and one untracked file
	repo := newTestRepo(t)
	trackedFile := filepath.Join(repo.dir, "tracked.txt")
	require.NoError(t, os.WriteFile(trackedFile, []byte("tracked\n"), 0600))
	repo.run(t, "add", "tracked.txt")
	repo.run(t, "commit", "--quiet", "-m", "add tracked file")
	require.NoError(t, os.WriteFile(trackedFile, []byte("modified\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(repo.dir, "untracked.txt"), []byte("untracked\n"), 0600))

	// When uncommitted changes are counted
	count, err := repo.client.UncommittedChangeCount(t.Context())

	// Then both changes are reported
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestClientCommitsReturnsCommitMetadata(t *testing.T) {
	// Given a branch with commits containing empty and multiline bodies
	repo := newTestRepo(t)
	repo.run(t, "commit", "--quiet", "--allow-empty", "-m", "base")
	baseRef := repo.run(t, "rev-parse", "HEAD")
	repo.run(t, "checkout", "--quiet", "-b", "feature")
	repo.run(t, "commit", "--quiet", "--allow-empty", "-m", "first commit")
	firstSHA := repo.run(t, "rev-parse", "HEAD")
	repo.run(t, "commit", "--quiet", "--allow-empty", "-m", "second commit", "-m", "first line\nsecond line")
	secondSHA := repo.run(t, "rev-parse", "HEAD")

	// When commits unique to the feature branch are requested
	commits, err := repo.client.Commits(t.Context(), baseRef, "feature")

	// Then each commit's repository metadata is returned newest first
	require.NoError(t, err)
	assert.Equal(t, []*Commit{
		{Sha: secondSHA, Title: "second commit", Body: "first line\nsecond line\n"},
		{Sha: firstSHA, Title: "first commit"},
	}, commits)
}

func TestClientCommitsReportsNoCommitsBetweenRefs(t *testing.T) {
	// Given two refs that identify the same commit
	repo := newTestRepo(t)
	repo.run(t, "commit", "--quiet", "--allow-empty", "-m", "initial commit")

	// When commits between the refs are requested
	commits, err := repo.client.Commits(t.Context(), "trunk", "trunk")

	// Then the empty comparison is reported
	require.EqualError(t, err, "could not find any commits between trunk and trunk")
	assert.Nil(t, commits)
}

func TestClientCommitsExcludesBaseSideCommits(t *testing.T) {
	// Given divergent branches with patch-equivalent commits and one unique feature commit
	repo := newTestRepo(t)
	repo.run(t, "commit", "--quiet", "--allow-empty", "-m", "initial commit")
	repo.run(t, "checkout", "--quiet", "-b", "feature")
	require.NoError(t, os.WriteFile(filepath.Join(repo.dir, "shared.txt"), []byte("shared\n"), 0600))
	repo.run(t, "add", "shared.txt")
	t.Setenv("GIT_COMMITTER_DATE", "2000-01-01T00:00:00Z")
	repo.run(t, "commit", "--quiet", "-m", "feature shared change")
	sharedCommit := repo.run(t, "rev-parse", "HEAD")
	repo.run(t, "commit", "--quiet", "--allow-empty", "-m", "feature-only commit")
	uniqueCommit := repo.run(t, "rev-parse", "HEAD")
	repo.run(t, "checkout", "--quiet", "trunk")
	t.Setenv("GIT_COMMITTER_DATE", "2001-01-01T00:00:00Z")
	repo.run(t, "cherry-pick", "--quiet", sharedCommit)

	// When commits unique to the feature branch are requested
	commits, err := repo.client.Commits(t.Context(), "trunk", "feature")

	// Then only the feature side of the symmetric difference is returned
	require.NoError(t, err)
	assert.Equal(t, []*Commit{
		{Sha: uniqueCommit, Title: "feature-only commit"},
		{Sha: sharedCommit, Title: "feature shared change"},
	}, commits)
}

func TestClientCommitsReportsInvalidRef(t *testing.T) {
	// Given a repository without the requested head ref
	repo := newTestRepo(t)
	repo.run(t, "commit", "--quiet", "--allow-empty", "-m", "initial commit")

	// When commits for the missing ref are requested
	commits, err := repo.client.Commits(t.Context(), "trunk", "missing")

	// Then Git's invalid revision error is returned
	require.Error(t, err)
	var gitError *GitError
	require.ErrorAs(t, err, &gitError)
	assert.NotZero(t, gitError.ExitCode)
	assert.Contains(t, gitError.Stderr, "trunk...missing")
	assert.Nil(t, commits)
}

func TestClientLastCommit(t *testing.T) {
	// Given a repository fixture with two commits
	IsolateConfig(t)
	client := Client{RepoDir: "./fixtures/simple.git"}

	// When its last commit is requested
	commit, err := client.LastCommit(t.Context())

	// Then the commit identity and title are returned
	require.NoError(t, err)
	assert.Equal(t, "6f1a2405cace1633d89a79c74c65f22fe78f9659", commit.Sha)
	assert.Equal(t, "Second commit", commit.Title)
}

func TestClientCommitBody(t *testing.T) {
	// Given a repository fixture with a commit body
	IsolateConfig(t)
	client := Client{RepoDir: "./fixtures/simple.git"}

	// When the body is requested
	body, err := client.CommitBody(t.Context(), "6f1a2405cace1633d89a79c74c65f22fe78f9659")

	// Then the complete body is returned
	require.NoError(t, err)
	assert.Equal(t, "I'm starting to get the hang of things\n", body)
}

func TestClientReadBranchConfigReturnsEmptyConfigWhenUnset(t *testing.T) {
	// Given a repository without branch configuration
	repo := newTestRepo(t)

	// When the branch configuration is read
	branchConfig, err := repo.client.ReadBranchConfig(t.Context(), "trunk")

	// Then an empty configuration is returned without error
	require.NoError(t, err)
	assert.Equal(t, BranchConfig{}, branchConfig)
}

func TestClientReadBranchConfigReturnsConfiguredValues(t *testing.T) {
	// Given all supported branch configuration values set through the client
	repo := newTestRepo(t)
	require.NoError(t, repo.client.SetBranchConfig(t.Context(), "trunk", "remote", "upstream"))
	require.NoError(t, repo.client.SetBranchConfig(t.Context(), "trunk", "merge", "refs/heads/trunk"))
	require.NoError(t, repo.client.SetBranchConfig(t.Context(), "trunk", "pushremote", "origin"))
	require.NoError(t, repo.client.SetBranchConfig(t.Context(), "trunk", MergeBaseConfig, "release"))

	// When the branch configuration is read
	branchConfig, err := repo.client.ReadBranchConfig(t.Context(), "trunk")

	// Then the persisted configuration is returned
	require.NoError(t, err)
	assert.Equal(t, BranchConfig{
		RemoteName:     "upstream",
		PushRemoteName: "origin",
		MergeRef:       "refs/heads/trunk",
		MergeBase:      "release",
	}, branchConfig)
}

func TestClientReadBranchConfigPropagatesGitError(t *testing.T) {
	// Given a client whose repository directory no longer exists
	repo := newTestRepo(t)
	require.NoError(t, os.RemoveAll(repo.dir))

	// When branch configuration is read
	branchConfig, err := repo.client.ReadBranchConfig(t.Context(), "trunk")

	// Then the repository error is preserved
	require.Error(t, err)
	var gitError *GitError
	require.ErrorAs(t, err, &gitError)
	assert.Equal(t, 128, gitError.ExitCode)
	assert.Empty(t, branchConfig)
}

func TestParseBranchConfig(t *testing.T) {
	tests := []struct {
		name             string
		configLines      []string
		wantBranchConfig BranchConfig
	}{
		{
			name:        "remote branch",
			configLines: []string{"branch.trunk.remote origin"},
			wantBranchConfig: BranchConfig{
				RemoteName: "origin",
			},
		},
		{
			name:        "merge ref",
			configLines: []string{"branch.trunk.merge refs/heads/trunk"},
			wantBranchConfig: BranchConfig{
				MergeRef: "refs/heads/trunk",
			},
		},
		{
			name:        "merge base",
			configLines: []string{"branch.trunk.gh-merge-base gh-merge-base"},
			wantBranchConfig: BranchConfig{
				MergeBase: "gh-merge-base",
			},
		},
		{
			name:        "pushremote",
			configLines: []string{"branch.trunk.pushremote pushremote"},
			wantBranchConfig: BranchConfig{
				PushRemoteName: "pushremote",
			},
		},
		{
			name: "remote and pushremote are specified by name",
			configLines: []string{
				"branch.trunk.remote upstream",
				"branch.trunk.pushremote origin",
			},
			wantBranchConfig: BranchConfig{
				RemoteName:     "upstream",
				PushRemoteName: "origin",
			},
		},
		{
			name: "remote and pushremote are specified by url",
			configLines: []string{
				"branch.trunk.remote git@github.com:UPSTREAMOWNER/REPO.git",
				"branch.trunk.pushremote git@github.com:ORIGINOWNER/REPO.git",
			},
			wantBranchConfig: BranchConfig{
				RemoteURL: &url.URL{
					Scheme: "ssh",
					User:   url.User("git"),
					Host:   "github.com",
					Path:   "/UPSTREAMOWNER/REPO.git",
				},
				PushRemoteURL: &url.URL{
					Scheme: "ssh",
					User:   url.User("git"),
					Host:   "github.com",
					Path:   "/ORIGINOWNER/REPO.git",
				},
			},
		},
		{
			name: "remote, pushremote, gh-merge-base, and merge ref all specified",
			configLines: []string{
				"branch.trunk.remote remote",
				"branch.trunk.pushremote pushremote",
				"branch.trunk.gh-merge-base gh-merge-base",
				"branch.trunk.merge refs/heads/trunk",
			},
			wantBranchConfig: BranchConfig{
				RemoteName:     "remote",
				PushRemoteName: "pushremote",
				MergeBase:      "gh-merge-base",
				MergeRef:       "refs/heads/trunk",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given branch configuration output
			configLines := tt.configLines

			// When the output is parsed
			branchConfig := parseBranchConfig(configLines)

			// Then each supported key is mapped to its typed field
			assert.Equal(t, tt.wantBranchConfig, branchConfig)
		})
	}
}

func TestParseRemoteURLOrName(t *testing.T) {
	tests := []struct {
		name           string
		value          string
		wantRemoteURL  *url.URL
		wantRemoteName string
	}{
		{
			name:           "empty value",
			value:          "",
			wantRemoteURL:  nil,
			wantRemoteName: "",
		},
		{
			name:  "remote URL",
			value: "git@github.com:foo/bar.git",
			wantRemoteURL: &url.URL{
				Scheme: "ssh",
				User:   url.User("git"),
				Host:   "github.com",
				Path:   "/foo/bar.git",
			},
			wantRemoteName: "",
		},
		{
			name:           "remote name",
			value:          "origin",
			wantRemoteURL:  nil,
			wantRemoteName: "origin",
		},
		{
			name:           "remote name is from filesystem",
			value:          "./path/to/repo",
			wantRemoteURL:  nil,
			wantRemoteName: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given a configured branch remote value
			value := tt.value

			// When it is classified
			remoteURL, remoteName := parseRemoteURLOrName(value)

			// Then URLs and names are returned through their distinct fields
			assert.Equal(t, tt.wantRemoteURL, remoteURL)
			assert.Equal(t, tt.wantRemoteName, remoteName)
		})
	}
}

func TestClientPushDefaultReturnsGitDefaultWhenUnset(t *testing.T) {
	// Given a repository without push.default
	repo := newTestRepo(t)

	// When push.default is read
	pushDefault, err := repo.client.PushDefault(t.Context())

	// Then Git's default behavior is returned
	require.NoError(t, err)
	assert.Equal(t, PushDefaultSimple, pushDefault)
}

func TestClientPushDefaultReturnsConfiguredValue(t *testing.T) {
	// Given a repository configured to push the current branch
	repo := newTestRepo(t)
	repo.run(t, "config", "push.default", "current")

	// When push.default is read
	pushDefault, err := repo.client.PushDefault(t.Context())

	// Then the configured behavior is returned
	require.NoError(t, err)
	assert.Equal(t, PushDefaultCurrent, pushDefault)
}

func TestClientPushDefaultPropagatesGitError(t *testing.T) {
	// Given a client whose repository directory no longer exists
	repo := newTestRepo(t)
	require.NoError(t, os.RemoveAll(repo.dir))

	// When push.default is read
	pushDefault, err := repo.client.PushDefault(t.Context())

	// Then the repository error is preserved
	require.Error(t, err)
	var gitError *GitError
	require.ErrorAs(t, err, &gitError)
	assert.Equal(t, 128, gitError.ExitCode)
	assert.Empty(t, pushDefault)
}

func TestClientRemotePushDefaultReturnsEmptyWhenUnset(t *testing.T) {
	// Given a repository without remote.pushDefault
	repo := newTestRepo(t)

	// When remote.pushDefault is read
	remote, err := repo.client.RemotePushDefault(t.Context())

	// Then no preferred remote is returned
	require.NoError(t, err)
	assert.Empty(t, remote)
}

func TestClientRemotePushDefaultReturnsConfiguredRemote(t *testing.T) {
	// Given a repository with a preferred push remote
	repo := newTestRepo(t)
	repo.run(t, "config", "remote.pushDefault", "origin")

	// When remote.pushDefault is read
	remote, err := repo.client.RemotePushDefault(t.Context())

	// Then the configured remote is returned
	require.NoError(t, err)
	assert.Equal(t, "origin", remote)
}

func TestClientRemotePushDefaultPropagatesGitError(t *testing.T) {
	// Given a client whose repository directory no longer exists
	repo := newTestRepo(t)
	require.NoError(t, os.RemoveAll(repo.dir))

	// When remote.pushDefault is read
	remote, err := repo.client.RemotePushDefault(t.Context())

	// Then the repository error is preserved
	require.Error(t, err)
	var gitError *GitError
	require.ErrorAs(t, err, &gitError)
	assert.Equal(t, 128, gitError.ExitCode)
	assert.Empty(t, remote)
}

func TestClientPushRevisionReturnsConfiguredTrackingRef(t *testing.T) {
	// Given a branch configured to push to an origin tracking branch
	repo := newTestRepo(t)
	repo.run(t, "commit", "--quiet", "--allow-empty", "-m", "initial commit")
	repo.run(t, "remote", "add", "origin", filepath.Join(t.TempDir(), "remote.git"))
	repo.run(t, "config", "branch.trunk.remote", "origin")
	repo.run(t, "config", "branch.trunk.merge", "refs/heads/trunk")
	repo.run(t, "update-ref", "refs/remotes/origin/trunk", "HEAD")

	// When the branch's push revision is resolved
	trackingRef, err := repo.client.PushRevision(t.Context(), "trunk")

	// Then the configured remote tracking ref is returned
	require.NoError(t, err)
	assert.Equal(t, RemoteTrackingRef{Remote: "origin", Branch: "trunk"}, trackingRef)
}

func TestClientPushRevisionReportsUnresolvedBranch(t *testing.T) {
	// Given a repository without the requested branch
	repo := newTestRepo(t)

	// When its push revision is resolved
	trackingRef, err := repo.client.PushRevision(t.Context(), "missing")

	// Then Git's revision error is returned
	require.Error(t, err)
	var gitError *GitError
	require.ErrorAs(t, err, &gitError)
	assert.NotZero(t, gitError.ExitCode)
	assert.Empty(t, trackingRef)
}

func TestParsePushRevisionReportsMalformedSymbolicRef(t *testing.T) {
	// Given Git returns a symbolic push ref outside refs/remotes
	output := []byte("not/a/valid/remote/ref")

	// When the push revision is parsed
	trackingRef, err := parsePushRevision(output)

	// Then the malformed ref is reported with context
	require.EqualError(t, err, "could not parse push revision: remote tracking branch must have format refs/remotes/<remote>/<branch> but was: not/a/valid/remote/ref")
	assert.Empty(t, trackingRef)
}

func TestParseRemoteTrackingRef(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want RemoteTrackingRef
	}{
		{
			name: "branch without slash",
			ref:  "refs/remotes/origin/branchName",
			want: RemoteTrackingRef{Remote: "origin", Branch: "branchName"},
		},
		{
			name: "branch with slash",
			ref:  "refs/remotes/origin/branch/name",
			want: RemoteTrackingRef{Remote: "origin", Branch: "branch/name"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given a qualified remote-tracking ref
			ref := tt.ref

			// When it is parsed
			trackingRef, err := ParseRemoteTrackingRef(ref)

			// Then the remote and complete branch name are returned
			require.NoError(t, err)
			assert.Equal(t, tt.want, trackingRef)
		})
	}
}

func TestParseRemoteTrackingRefReportsMalformedInput(t *testing.T) {
	tests := []struct {
		name string
		ref  string
	}{
		{name: "missing branch", ref: "refs/remotes/origin"},
		{name: "incorrect refs prefix", ref: "invalid/remotes/origin/branchName"},
		{name: "incorrect ref type", ref: "refs/invalid/origin/branchName"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given a string outside the remote-tracking ref grammar
			ref := tt.ref

			// When it is parsed
			trackingRef, err := ParseRemoteTrackingRef(ref)

			// Then the invalid value is included in the format diagnostic
			require.EqualError(t, err, "remote tracking branch must have format refs/remotes/<remote>/<branch> but was: "+ref)
			assert.Empty(t, trackingRef)
		})
	}
}

func TestRemoteTrackingRefString(t *testing.T) {
	// Given a remote and branch name
	trackingRef := RemoteTrackingRef{Remote: "origin", Branch: "branchName"}

	// When the tracking ref is formatted
	ref := trackingRef.String()

	// Then a qualified remote-tracking ref is returned
	assert.Equal(t, "refs/remotes/origin/branchName", ref)
}

func TestClientDeleteLocalTagRemovesTag(t *testing.T) {
	// Given a repository with a local tag
	repo := newTestRepo(t)
	repo.run(t, "commit", "--quiet", "--allow-empty", "-m", "initial commit")
	repo.run(t, "tag", "v1.0")

	// When the tag is deleted
	err := repo.client.DeleteLocalTag(t.Context(), "v1.0")

	// Then the tag no longer resolves
	require.NoError(t, err)
	assert.Empty(t, repo.run(t, "tag", "--list", "v1.0"))
}

func TestClientDeleteLocalTagReportsMissingTag(t *testing.T) {
	// Given a repository without the requested tag
	repo := newTestRepo(t)

	// When the missing tag is deleted
	err := repo.client.DeleteLocalTag(t.Context(), "missing")

	// Then Git's missing tag error is returned
	require.Error(t, err)
	var gitError *GitError
	require.ErrorAs(t, err, &gitError)
	assert.NotZero(t, gitError.ExitCode)
}

func TestClientDeleteLocalBranchRemovesBranch(t *testing.T) {
	// Given a repository with a local feature branch
	repo := newTestRepo(t)
	repo.run(t, "commit", "--quiet", "--allow-empty", "-m", "initial commit")
	repo.run(t, "branch", "feature")

	// When the branch is deleted
	err := repo.client.DeleteLocalBranch(t.Context(), "feature")

	// Then the branch no longer exists
	require.NoError(t, err)
	assert.False(t, repo.client.HasLocalBranch(t.Context(), "feature"), "deleted branch should not resolve")
}

func TestClientDeleteLocalBranchReportsCheckedOutBranch(t *testing.T) {
	// Given a repository whose trunk branch is checked out
	repo := newTestRepo(t)
	repo.run(t, "commit", "--quiet", "--allow-empty", "-m", "initial commit")

	// When the checked out branch is deleted
	err := repo.client.DeleteLocalBranch(t.Context(), "trunk")

	// Then Git rejects the deletion and the branch remains
	require.Error(t, err)
	var gitError *GitError
	require.ErrorAs(t, err, &gitError)
	assert.NotZero(t, gitError.ExitCode)
	assert.True(t, repo.client.HasLocalBranch(t.Context(), "trunk"), "checked out branch should remain")
}

func TestClientHasLocalBranchFindsExistingBranch(t *testing.T) {
	// Given a repository with a trunk commit
	repo := newTestRepo(t)
	repo.run(t, "commit", "--quiet", "--allow-empty", "-m", "initial commit")

	// When the local branch is checked
	exists := repo.client.HasLocalBranch(t.Context(), "trunk")

	// Then the branch is found
	assert.True(t, exists, "trunk should resolve as a local branch")
}

func TestClientHasLocalBranchRejectsMissingBranch(t *testing.T) {
	// Given a repository without a feature branch
	repo := newTestRepo(t)

	// When the local branch is checked
	exists := repo.client.HasLocalBranch(t.Context(), "feature")

	// Then the branch is not found
	assert.False(t, exists, "missing branch should not resolve")
}

func TestClientCheckoutBranchSwitchesBranches(t *testing.T) {
	// Given a repository checked out on feature with a trunk branch
	repo := newTestRepo(t)
	repo.run(t, "commit", "--quiet", "--allow-empty", "-m", "initial commit")
	repo.run(t, "checkout", "--quiet", "-b", "feature")

	// When trunk is checked out
	err := repo.client.CheckoutBranch(t.Context(), "trunk")

	// Then trunk becomes the current branch
	require.NoError(t, err)
	assert.Equal(t, "trunk", repo.run(t, "branch", "--show-current"))
}

func TestClientCheckoutBranchReportsMissingBranch(t *testing.T) {
	// Given a repository without the requested branch
	repo := newTestRepo(t)

	// When the missing branch is checked out
	err := repo.client.CheckoutBranch(t.Context(), "missing")

	// Then Git's checkout error is returned
	require.Error(t, err)
	var gitError *GitError
	require.ErrorAs(t, err, &gitError)
	assert.NotZero(t, gitError.ExitCode)
}

func TestClientCheckoutNewBranchTracksRemoteBranch(t *testing.T) {
	// Given a repository with an origin feature tracking ref
	repo := newTestRepo(t)
	repo.run(t, "commit", "--quiet", "--allow-empty", "-m", "initial commit")
	repo.run(t, "remote", "add", "origin", filepath.Join(t.TempDir(), "remote.git"))
	repo.run(t, "update-ref", "refs/remotes/origin/feature", "HEAD")

	// When a local branch is created from that tracking ref
	err := repo.client.CheckoutNewBranch(t.Context(), "origin", "feature")

	// Then the new branch is current and tracks origin
	require.NoError(t, err)
	assert.Equal(t, "feature", repo.run(t, "branch", "--show-current"))
	assert.Equal(t, "origin", repo.run(t, "config", "branch.feature.remote"))
	assert.Equal(t, "refs/heads/feature", repo.run(t, "config", "branch.feature.merge"))
}

func TestClientCheckoutNewBranchReportsMissingTrackingBranch(t *testing.T) {
	// Given a repository without the requested remote tracking branch
	repo := newTestRepo(t)

	// When a local branch is created from it
	err := repo.client.CheckoutNewBranch(t.Context(), "origin", "missing")

	// Then Git's checkout error is returned and no branch is created
	require.Error(t, err)
	var gitError *GitError
	require.ErrorAs(t, err, &gitError)
	assert.NotZero(t, gitError.ExitCode)
	assert.False(t, repo.client.HasLocalBranch(t.Context(), "missing"), "failed checkout should not create a branch")
}

func TestClientToplevelDirReturnsRepositoryRootFromSubdirectory(t *testing.T) {
	// Given a client operating from a nested repository directory
	repo := newTestRepo(t)
	nestedDir := filepath.Join(repo.dir, "some", "path")
	require.NoError(t, os.MkdirAll(nestedDir, 0700))
	client := Client{RepoDir: nestedDir}

	// When the top-level directory is requested
	dir, err := client.ToplevelDir(t.Context())

	// Then the repository root is returned
	require.NoError(t, err)
	assert.Equal(t, filepath.ToSlash(repo.dir), filepath.ToSlash(dir))
}

func TestClientToplevelDirReportsNonRepository(t *testing.T) {
	// Given a client outside a Git repository
	IsolateConfig(t)
	client := Client{RepoDir: t.TempDir()}

	// When the top-level directory is requested
	dir, err := client.ToplevelDir(t.Context())

	// Then Git's repository error is returned
	require.Error(t, err)
	var gitError *GitError
	require.ErrorAs(t, err, &gitError)
	assert.Equal(t, 128, gitError.ExitCode)
	assert.Empty(t, dir)
}

func TestClientGitDirReturnsRepositoryMetadataDirectory(t *testing.T) {
	// Given a local Git repository
	repo := newTestRepo(t)

	// When the Git metadata directory is requested
	dir, err := repo.client.GitDir(t.Context())

	// Then Git identifies the repository's metadata directory
	require.NoError(t, err)
	assert.Equal(t, ".git", dir)
}

func TestClientGitDirReportsNonRepository(t *testing.T) {
	// Given a client outside a Git repository
	IsolateConfig(t)
	client := Client{RepoDir: t.TempDir()}

	// When the Git metadata directory is requested
	dir, err := client.GitDir(t.Context())

	// Then Git's repository error is returned
	require.Error(t, err)
	var gitError *GitError
	require.ErrorAs(t, err, &gitError)
	assert.Equal(t, 128, gitError.ExitCode)
	assert.Empty(t, dir)
}

func TestClientPathFromRootReturnsNestedPath(t *testing.T) {
	// Given a client operating from a nested repository directory
	repo := newTestRepo(t)
	nestedDir := filepath.Join(repo.dir, "some", "path")
	require.NoError(t, os.MkdirAll(nestedDir, 0700))
	client := Client{RepoDir: nestedDir}

	// When its path relative to the repository root is requested
	dir := client.PathFromRoot(t.Context())

	// Then the nested path is returned without a trailing separator
	assert.Equal(t, "some/path", filepath.ToSlash(dir))
}

func TestClientPathFromRootReturnsEmptyOutsideRepository(t *testing.T) {
	// Given a client outside a Git repository
	IsolateConfig(t)
	client := Client{RepoDir: t.TempDir()}

	// When its path relative to a repository root is requested
	dir := client.PathFromRoot(t.Context())

	// Then no path is returned
	assert.Empty(t, dir)
}

func TestClientUnsetRemoteResolutionRemovesResolution(t *testing.T) {
	// Given a repository with a resolved remote
	repo := newTestRepo(t)
	repo.run(t, "remote", "add", "origin", "https://github.com/cli/cli.git")
	repo.run(t, "config", "remote.origin.gh-resolved", "base")

	// When the remote resolution is unset
	err := repo.client.UnsetRemoteResolution(t.Context(), "origin")

	// Then the remote no longer has a resolution
	require.NoError(t, err)
	remotes, err := repo.client.Remotes(t.Context())
	require.NoError(t, err)
	require.Len(t, remotes, 1)
	assert.Empty(t, remotes[0].Resolved)
}

func TestClientUnsetRemoteResolutionPropagatesGitError(t *testing.T) {
	// Given a client whose repository directory does not exist
	client := Client{RepoDir: filepath.Join(t.TempDir(), "missing")}

	// When a remote resolution is unset
	err := client.UnsetRemoteResolution(t.Context(), "origin")

	// Then the Git error is returned
	require.Error(t, err)
	var gitErr *GitError
	require.ErrorAs(t, err, &gitErr)
	assert.NotZero(t, gitErr.ExitCode)
}

func TestClientSetRemoteBranchesLimitsTrackedBranches(t *testing.T) {
	// Given a repository that fetches every branch from a local remote
	repo := newTestRepo(t)
	repo.run(t, "remote", "add", "origin", newBareTestRepo(t).fileURL())

	// When the tracked remote branches are limited to trunk
	err := repo.client.SetRemoteBranches(t.Context(), "origin", "trunk")

	// Then only the trunk remote-tracking ref is configured
	require.NoError(t, err)
	assert.Equal(t, "+refs/heads/trunk:refs/remotes/origin/trunk", repo.run(t, "config", "--get-all", "remote.origin.fetch"))
}

func TestClientSetRemoteBranchesReportsMissingRemote(t *testing.T) {
	// Given a repository without the named remote
	repo := newTestRepo(t)

	// When its tracked branches are changed
	err := repo.client.SetRemoteBranches(t.Context(), "missing", "trunk")

	// Then Git reports a semantic command failure
	require.Error(t, err)
	var gitErr *GitError
	require.ErrorAs(t, err, &gitErr)
	assert.NotZero(t, gitErr.ExitCode)
}

func TestClientFetchUpdatesRemoteTrackingBranchInModifiedRepoDir(t *testing.T) {
	// Given a local remote with a trunk commit and a separate destination repository
	source := newTestRepo(t)
	remote := newBareTestRepo(t)
	wantHead := source.commit(t, "remote commit")
	source.run(t, "remote", "add", "origin", remote.fileURL())
	source.run(t, "push", "--quiet", "origin", "trunk")
	destination := newTestRepo(t)
	destination.run(t, "remote", "add", "origin", remote.fileURL())
	client := &Client{}

	// When trunk is fetched using a repository-directory modifier
	err := client.Fetch(t.Context(), "origin", "trunk", WithRepoDir(destination.dir))

	// Then the destination remote-tracking branch points at the remote commit
	require.NoError(t, err)
	assert.Equal(t, wantHead, destination.run(t, "rev-parse", "refs/remotes/origin/trunk"))
}

func TestClientFetchReportsMissingRef(t *testing.T) {
	// Given a repository connected to an empty local remote
	repo := newTestRepo(t)
	repo.run(t, "remote", "add", "origin", newBareTestRepo(t).fileURL())

	// When a missing ref is fetched
	err := repo.client.Fetch(t.Context(), "origin", "missing")

	// Then Git reports a semantic command failure
	require.Error(t, err)
	var gitErr *GitError
	require.ErrorAs(t, err, &gitErr)
	assert.NotZero(t, gitErr.ExitCode)
}

func TestClientFetchUsesAuthenticatedCommand(t *testing.T) {
	// Given a client that records the Git command
	var gotArgs []string
	client := Client{GitPath: "path/to/git"}

	// When a remote branch is fetched
	err := client.Fetch(t.Context(), "origin", "trunk", recordCommandArgs(t, &gotArgs))

	// Then Git clears existing helpers and scopes gh credentials to all hosts
	require.NoError(t, err)
	assert.Equal(t, []string{"path/to/git", "-c", "credential.helper=", "-c", `credential.helper=!"gh" auth git-credential`, "fetch", "origin", "trunk"}, gotArgs)
}

func TestClientPullFastForwardsCurrentBranch(t *testing.T) {
	// Given a local branch behind its local remote
	source := newTestRepo(t)
	remote := newBareTestRepo(t)
	source.commit(t, "initial commit")
	source.run(t, "remote", "add", "origin", remote.fileURL())
	source.run(t, "push", "--quiet", "origin", "trunk")
	destination := newTestRepo(t)
	destination.run(t, "remote", "add", "origin", remote.fileURL())
	destination.run(t, "fetch", "--quiet", "origin", "trunk")
	destination.run(t, "reset", "--quiet", "--hard", "origin/trunk")
	wantHead := source.commit(t, "remote update")
	source.run(t, "push", "--quiet", "origin", "trunk")

	// When the remote branch is pulled
	err := destination.client.Pull(t.Context(), "origin", "trunk")

	// Then the local branch fast-forwards to the remote commit
	require.NoError(t, err)
	assert.Equal(t, wantHead, destination.run(t, "rev-parse", "HEAD"))
	assert.Equal(t, wantHead, destination.run(t, "rev-parse", "refs/remotes/origin/trunk"))
}

func TestClientPullRejectsNonFastForwardUpdate(t *testing.T) {
	// Given local and remote trunk branches that have diverged
	source := newTestRepo(t)
	remote := newBareTestRepo(t)
	source.commit(t, "initial commit")
	source.run(t, "remote", "add", "origin", remote.fileURL())
	source.run(t, "push", "--quiet", "origin", "trunk")
	destination := newTestRepo(t)
	destination.run(t, "remote", "add", "origin", remote.fileURL())
	destination.run(t, "fetch", "--quiet", "origin", "trunk")
	destination.run(t, "reset", "--quiet", "--hard", "origin/trunk")
	localHead := destination.commit(t, "local update")
	source.commit(t, "remote update")
	source.run(t, "push", "--quiet", "origin", "trunk")
	destination.run(t, "config", "pull.rebase", "false")

	// When the remote branch is pulled
	err := destination.client.Pull(t.Context(), "origin", "trunk")

	// Then the fast-forward-only pull fails without merging either branch
	require.Error(t, err)
	var gitErr *GitError
	require.ErrorAs(t, err, &gitErr)
	assert.NotZero(t, gitErr.ExitCode)
	assert.Equal(t, localHead, destination.run(t, "rev-parse", "HEAD"))
	assert.Equal(t, "2", destination.run(t, "rev-list", "--count", "HEAD"))
}

func TestClientPullUsesAuthenticatedFastForwardOnlyCommand(t *testing.T) {
	// Given a client that records the Git command
	var gotArgs []string
	client := Client{GitPath: "path/to/git"}

	// When a remote branch is pulled
	err := client.Pull(t.Context(), "origin", "trunk", recordCommandArgs(t, &gotArgs))

	// Then Git clears existing helpers, scopes gh credentials, and requires a fast-forward
	require.NoError(t, err)
	assert.Equal(t, []string{"path/to/git", "-c", "credential.helper=", "-c", `credential.helper=!"gh" auth git-credential`, "pull", "--ff-only", "origin", "trunk"}, gotArgs)
}

func TestClientPushCreatesRemoteBranchAndUpstream(t *testing.T) {
	// Given a local trunk branch and an empty local remote
	repo := newTestRepo(t)
	remote := newBareTestRepo(t)
	wantHead := repo.commit(t, "local commit")
	repo.run(t, "remote", "add", "origin", remote.fileURL())

	// When trunk is pushed
	err := repo.client.Push(t.Context(), "origin", "trunk")

	// Then the remote branch is created and configured as the local upstream
	require.NoError(t, err)
	assert.Equal(t, wantHead, remote.run(t, "rev-parse", "refs/heads/trunk"))
	assert.Equal(t, "origin/trunk", repo.run(t, "rev-parse", "--abbrev-ref", "trunk@{upstream}"))
}

func TestClientPushReportsMissingRemote(t *testing.T) {
	// Given a repository with a local trunk commit but no remote
	repo := newTestRepo(t)
	repo.commit(t, "local commit")

	// When trunk is pushed to a missing remote
	err := repo.client.Push(t.Context(), "missing", "trunk")

	// Then Git reports a semantic command failure
	require.Error(t, err)
	var gitErr *GitError
	require.ErrorAs(t, err, &gitErr)
	assert.NotZero(t, gitErr.ExitCode)
}

func TestClientPushUsesAuthenticatedUpstreamCommand(t *testing.T) {
	// Given a client that records the Git command
	var gotArgs []string
	client := Client{GitPath: "path/to/git"}

	// When a branch is pushed
	err := client.Push(t.Context(), "origin", "trunk", recordCommandArgs(t, &gotArgs))

	// Then Git clears existing helpers, scopes gh credentials, and sets the upstream
	require.NoError(t, err)
	assert.Equal(t, []string{"path/to/git", "-c", "credential.helper=", "-c", `credential.helper=!"gh" auth git-credential`, "push", "--set-upstream", "origin", "trunk"}, gotArgs)
}

func TestClientCloneCreatesWorkingRepositoryInModifiedRepoDir(t *testing.T) {
	// Given a local remote with a trunk commit and an empty destination parent
	source := newTestRepo(t)
	remote := newBareTestRepo(t)
	wantHead := source.commit(t, "remote commit")
	source.run(t, "remote", "add", "origin", remote.fileURL())
	source.run(t, "push", "--quiet", "origin", "trunk")
	parentDir := t.TempDir()
	client := &Client{}

	// When the remote is cloned using a repository-directory modifier
	target, err := client.Clone(t.Context(), remote.fileURL(), nil, WithRepoDir(parentDir))

	// Then the reported target contains a working repository on trunk
	require.NoError(t, err)
	assert.Equal(t, "remote", target)
	clonedDir := filepath.Join(parentDir, target)
	assert.Equal(t, "false", runGitAt(t, clonedDir, "rev-parse", "--is-bare-repository"))
	assert.Equal(t, "trunk", runGitAt(t, clonedDir, "branch", "--show-current"))
	assert.Equal(t, wantHead, runGitAt(t, clonedDir, "rev-parse", "HEAD"))
}

func TestClientCloneCreatesBareRepository(t *testing.T) {
	// Given a populated local remote and an empty destination parent
	source := newTestRepo(t)
	remote := newBareTestRepo(t)
	wantHead := source.commit(t, "remote commit")
	source.run(t, "remote", "add", "origin", remote.fileURL())
	source.run(t, "push", "--quiet", "origin", "trunk")
	parentDir := t.TempDir()
	client := &Client{}

	// When the remote is cloned as bare
	target, err := client.Clone(t.Context(), remote.fileURL(), []string{"--bare"}, WithRepoDir(parentDir))

	// Then the target keeps its .git suffix and contains the trunk ref
	require.NoError(t, err)
	assert.Equal(t, "remote.git", target)
	clonedDir := filepath.Join(parentDir, target)
	assert.Equal(t, "true", runGitAt(t, clonedDir, "rev-parse", "--is-bare-repository"))
	assert.Equal(t, wantHead, runGitAt(t, clonedDir, "rev-parse", "refs/heads/trunk"))
}

func TestClientCloneUsesExplicitBareTarget(t *testing.T) {
	// Given a populated local remote and an empty destination parent
	source := newTestRepo(t)
	remote := newBareTestRepo(t)
	source.commit(t, "remote commit")
	source.run(t, "remote", "add", "origin", remote.fileURL())
	source.run(t, "push", "--quiet", "origin", "trunk")
	parentDir := t.TempDir()
	client := &Client{}

	// When the remote is cloned as bare with an explicit target
	target, err := client.Clone(t.Context(), remote.fileURL(), []string{"custom-target", "--bare"}, WithRepoDir(parentDir))

	// Then the reported and created directory use the explicit target
	require.NoError(t, err)
	assert.Equal(t, "custom-target", target)
	assert.Equal(t, "true", runGitAt(t, filepath.Join(parentDir, target), "rev-parse", "--is-bare-repository"))
}

func TestClientCloneReportsMissingRepository(t *testing.T) {
	// Given a local repository URL that does not exist
	IsolateConfig(t)
	missingURL := fileURL(filepath.Join(t.TempDir(), "missing.git"))
	client := &Client{}

	// When the repository is cloned
	target, err := client.Clone(t.Context(), missingURL, nil, WithRepoDir(t.TempDir()))

	// Then Git reports a semantic command failure and no target
	require.Error(t, err)
	var gitErr *GitError
	require.ErrorAs(t, err, &gitErr)
	assert.NotZero(t, gitErr.ExitCode)
	assert.Empty(t, target)
}

func TestClientCloneUsesHostScopedAuthenticatedCommand(t *testing.T) {
	// Given a client that records the Git command
	var gotArgs []string
	client := Client{GitPath: "path/to/git"}

	// When an HTTPS repository is cloned
	_, err := client.Clone(t.Context(), "https://github.com/cli/cli", nil, recordCommandArgs(t, &gotArgs))

	// Then Git clears existing helpers and scopes gh credentials to the repository host
	require.NoError(t, err)
	assert.Equal(t, []string{"path/to/git", "-c", "credential.https://github.com.helper=", "-c", `credential.https://github.com.helper=!"gh" auth git-credential`, "clone", "https://github.com/cli/cli"}, gotArgs)
}

func TestParseCloneArgs(t *testing.T) {
	type wanted struct {
		args []string
		dir  string
	}
	tests := []struct {
		name string
		args []string
		want wanted
	}{
		{
			name: "args and target",
			args: []string{"target_directory", "-o", "upstream", "--depth", "1"},
			want: wanted{
				args: []string{"-o", "upstream", "--depth", "1"},
				dir:  "target_directory",
			},
		},
		{
			name: "only args",
			args: []string{"-o", "upstream", "--depth", "1"},
			want: wanted{
				args: []string{"-o", "upstream", "--depth", "1"},
				dir:  "",
			},
		},
		{
			name: "only target",
			args: []string{"target_directory"},
			want: wanted{
				args: []string{},
				dir:  "target_directory",
			},
		},
		{
			name: "no args",
			args: []string{},
			want: wanted{
				args: []string{},
				dir:  "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given optional clone modifiers and a possible target directory
			extraArgs := tt.args

			// When clone arguments are separated from the target
			args, dir := parseCloneArgs(extraArgs)
			got := wanted{args: args, dir: dir}

			// Then modifiers stay ordered and a leading target is returned separately
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestClientAddRemoteTracksAllBranches(t *testing.T) {
	// Given a repository and a local bare remote
	repo := newTestRepo(t)
	remoteURL := newBareTestRepo(t).fileURL()

	// When the remote is added without branch restrictions
	remote, err := repo.client.AddRemote(t.Context(), "test", remoteURL, nil)

	// Then Git stores the remote and its all-branches fetch refspec
	require.NoError(t, err)
	assert.Equal(t, "test", remote.Name)
	assert.Equal(t, remoteURL, remote.FetchURL.String())
	assert.Equal(t, remoteURL, repo.run(t, "remote", "get-url", "test"))
	assert.Equal(t, "+refs/heads/*:refs/remotes/test/*", repo.run(t, "config", "--get-all", "remote.test.fetch"))
}

func TestClientAddRemoteTracksSpecificBranches(t *testing.T) {
	// Given a repository and a local bare remote
	repo := newTestRepo(t)
	remoteURL := newBareTestRepo(t).fileURL()

	// When the remote is added with specific tracking branches
	remote, err := repo.client.AddRemote(t.Context(), "test", remoteURL, []string{"trunk", "dev"})

	// Then Git stores one fetch refspec for each requested branch
	require.NoError(t, err)
	assert.Equal(t, "test", remote.Name)
	assert.Equal(t, remoteURL, repo.run(t, "remote", "get-url", "test"))
	assert.Equal(t, "+refs/heads/trunk:refs/remotes/test/trunk\n+refs/heads/dev:refs/remotes/test/dev", repo.run(t, "config", "--get-all", "remote.test.fetch"))
}

type testRepo struct {
	dir    string
	client *Client
}

func newTestRepo(t *testing.T) *testRepo {
	t.Helper()
	return newTestRepoWithMode(t, false)
}

func newBareTestRepo(t *testing.T) *testRepo {
	t.Helper()
	return newTestRepoWithMode(t, true)
}

func newTestRepoWithMode(t *testing.T, bare bool) *testRepo {
	t.Helper()
	IsolateConfig(t)
	t.Setenv("GIT_TERMINAL_PROMPT", "0")

	dir := t.TempDir()
	if bare {
		dir = filepath.Join(dir, "remote.git")
		require.NoError(t, os.Mkdir(dir, 0700))
	}
	dir, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	repo := &testRepo{dir: dir}
	repo.client = &Client{RepoDir: repo.dir}
	initArgs := []string{"init", "--quiet", "--initial-branch=trunk"}
	if bare {
		initArgs = append(initArgs, "--bare")
	}
	repo.run(t, initArgs...)
	if !bare {
		repo.run(t, "config", "user.name", "GitHub CLI Test")
		repo.run(t, "config", "user.email", "gh-test@example.com")
	}
	return repo
}

func (r *testRepo) run(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = r.dir
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %s failed:\n%s", strings.Join(args, " "), output)
	return strings.TrimSpace(string(output))
}

func (r *testRepo) commit(t *testing.T, message string) string {
	t.Helper()
	r.run(t, "commit", "--quiet", "--allow-empty", "-m", message)
	return r.run(t, "rev-parse", "HEAD")
}

func (r *testRepo) fileURL() string {
	return fileURL(r.dir)
}

func fileURL(filePath string) string {
	urlPath := filepath.ToSlash(filePath)
	if filepath.VolumeName(filePath) != "" {
		urlPath = "/" + urlPath
	}
	return (&url.URL{Scheme: "file", Path: urlPath}).String()
}

func runGitAt(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %s failed:\n%s", strings.Join(args, " "), output)
	return strings.TrimSpace(string(output))
}

func TestGitCommandHelperProcess(t *testing.T) {
	if len(os.Args) != 7 || os.Args[3] != "git-command-helper" {
		return
	}

	exitStatus, err := strconv.Atoi(os.Args[4])
	if err != nil {
		fmt.Fprint(os.Stderr, "invalid helper process exit status")
		os.Exit(1)
	}
	fmt.Fprint(os.Stdout, os.Args[5])
	if exitStatus != 0 {
		fmt.Fprint(os.Stderr, os.Args[6])
	}
	os.Exit(exitStatus)
}

func TestCredentialPatternFromGitURL(t *testing.T) {
	// Given a well-formed HTTPS Git URL
	gitURL := "https://github.com/OWNER/REPO.git"

	// When its credential scope is derived
	credentialPattern, err := CredentialPatternFromGitURL(gitURL)

	// Then credentials are scoped to the URL's HTTPS host
	require.NoError(t, err)
	assert.Equal(t, CredentialPattern{pattern: "https://github.com"}, credentialPattern)
}

func TestCredentialPatternFromGitURLReportsMalformedURL(t *testing.T) {
	// Given a Git URL with a malformed bracketed host
	gitURL := "ssh://git@[/tmp/git-repo"

	// When its credential scope is derived
	credentialPattern, err := CredentialPatternFromGitURL(gitURL)

	// Then the URL parsing failure is reported with context
	require.ErrorContains(t, err, "failed to parse remote URL")
	assert.Empty(t, credentialPattern)
}

func TestCredentialPatternFromHost(t *testing.T) {
	// Given a Git host
	host := "github.com"

	// When its credential scope is derived
	credentialPattern := CredentialPatternFromHost(host)

	// Then credentials are scoped to that host over HTTPS
	assert.Equal(t, CredentialPattern{pattern: "https://github.com"}, credentialPattern)
}

func TestParsePushDefault(t *testing.T) {
	tests := []struct {
		value string
		want  PushDefault
	}{
		{value: "nothing", want: PushDefaultNothing},
		{value: "current", want: PushDefaultCurrent},
		{value: "upstream", want: PushDefaultUpstream},
		{value: "tracking", want: PushDefaultTracking},
		{value: "simple", want: PushDefaultSimple},
		{value: "matching", want: PushDefaultMatching},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			// Given a supported push.default value
			value := tt.value

			// When it is parsed
			pushDefault, err := ParsePushDefault(value)

			// Then the matching typed value is returned
			require.NoError(t, err)
			assert.Equal(t, tt.want, pushDefault)
		})
	}
}

func TestParsePushDefaultReportsUnknownValue(t *testing.T) {
	// Given an unsupported push.default value
	value := "invalid"

	// When it is parsed
	pushDefault, err := ParsePushDefault(value)

	// Then the unknown value is identified
	require.EqualError(t, err, "unknown push.default value: invalid")
	assert.Empty(t, pushDefault)
}

func createCommand(t *testing.T, exitStatus int, stdout, stderr string) *exec.Cmd {
	t.Helper()
	return exec.CommandContext(
		context.Background(),
		os.Args[0],
		"-test.run=^TestGitCommandHelperProcess$",
		"--",
		"git-command-helper",
		strconv.Itoa(exitStatus),
		stdout,
		stderr,
	)
}

func recordCommandArgs(t *testing.T, gotArgs *[]string) CommandModifier {
	t.Helper()
	return func(cmd *Command) {
		*gotArgs = append([]string(nil), cmd.Args...)
		cmd.Cmd = createCommand(t, 0, "", "")
	}
}

func TestClientRemoteURLReturnsConfiguredURL(t *testing.T) {
	// Given a repository with a configured remote
	repo := newTestRepo(t)
	repo.run(t, "remote", "add", "origin", "https://github.com/monalisa/skills-repo.git")

	// When the remote URL is requested
	remoteURL, err := repo.client.RemoteURL(t.Context(), "origin")

	// Then the configured URL is returned
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/monalisa/skills-repo.git", remoteURL)
}

func TestClientRemoteURLReportsMissingRemote(t *testing.T) {
	// Given a repository without the requested remote
	repo := newTestRepo(t)

	// When the remote URL is requested
	remoteURL, err := repo.client.RemoteURL(t.Context(), "nonexistent")

	// Then Git's failure is returned
	require.Error(t, err)
	var gitErr *GitError
	assert.ErrorAs(t, err, &gitErr)
	assert.Empty(t, remoteURL)
}

func TestClientRemoteURLSeparatesRemoteNameFromOptions(t *testing.T) {
	// Given a configured remote whose name resembles a Git option
	repo := newTestRepo(t)
	repo.run(t, "config", "remote.--upload-pack=malicious.url", "https://example.com/repo.git")

	// When the remote URL is requested
	remoteURL, err := repo.client.RemoteURL(t.Context(), "--upload-pack=malicious")

	// Then an option separator protects the remote name
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/repo.git", remoteURL)
}

func TestClientRemoteURLReportsMissingGitExecutable(t *testing.T) {
	// Given Git cannot be resolved
	t.Setenv("PATH", "")
	client := Client{}

	// When a remote URL is requested
	remoteURL, err := client.RemoteURL(t.Context(), "origin")

	// Then the resolution failure is returned
	require.Error(t, err)
	assert.Empty(t, remoteURL)
}

func TestClientIsIgnoredReturnsTrueForIgnoredPath(t *testing.T) {
	// Given a repository that ignores the requested path
	repo := newTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repo.dir, ".gitignore"), []byte(".github/skills\n"), 0600))

	// When the path is checked
	ignored, err := repo.client.IsIgnored(t.Context(), ".github/skills")

	// Then it is reported as ignored
	require.NoError(t, err)
	assert.True(t, ignored, "expected configured ignore rule to match")
}

func TestClientIsIgnoredReturnsFalseForUnignoredPath(t *testing.T) {
	// Given a repository without an ignore rule for the requested path
	repo := newTestRepo(t)

	// When the path is checked
	ignored, err := repo.client.IsIgnored(t.Context(), ".github/skills")

	// Then it is reported as unignored
	require.NoError(t, err)
	assert.False(t, ignored, "expected path without an ignore rule not to match")
}

func TestClientIsIgnoredReportsFatalGitError(t *testing.T) {
	// Given a directory that is not a Git repository
	IsolateConfig(t)
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
	client := Client{RepoDir: t.TempDir()}

	// When a path is checked
	ignored, err := client.IsIgnored(t.Context(), ".github/skills")

	// Then Git's fatal error is returned
	require.Error(t, err)
	assert.False(t, ignored, "a fatal Git error must not report the path as ignored")
}

func TestClientIsIgnoredSeparatesPathFromOptions(t *testing.T) {
	// Given an ignored path that resembles a Git option
	repo := newTestRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repo.dir, ".gitignore"), []byte("--no-index\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(repo.dir, "--no-index"), nil, 0600))

	// When the path is checked
	ignored, err := repo.client.IsIgnored(t.Context(), "--no-index")

	// Then an option separator protects the path
	require.NoError(t, err)
	assert.True(t, ignored, "expected option-like path to match its ignore rule")
}

func TestClientIsIgnoredReportsMissingGitExecutable(t *testing.T) {
	// Given Git cannot be resolved
	t.Setenv("PATH", "")
	client := Client{}

	// When a path is checked
	ignored, err := client.IsIgnored(t.Context(), ".github/skills")

	// Then the resolution failure is returned
	require.Error(t, err)
	assert.False(t, ignored, "a missing Git executable must not report the path as ignored")
}

func TestShortSHA(t *testing.T) {
	tests := []struct {
		name string
		sha  string
		want string
	}{
		{name: "long SHA", sha: "abc123def456789", want: "abc123de"},
		{name: "short value", sha: "short", want: "short"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given a commit identifier
			sha := tt.sha

			// When it is shortened for display
			shortSHA := ShortSHA(sha)

			// Then at most the first eight characters are returned
			assert.Equal(t, tt.want, shortSHA)
		})
	}
}

func TestParseWorktrees(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want []Worktree
	}{
		{
			name: "empty output",
			out:  "",
			want: nil,
		},
		{
			name: "single worktree",
			out: heredoc.Doc(`
				worktree /path/to/main
				HEAD abc123
				branch refs/heads/main
			`),
			want: []Worktree{
				{Path: "/path/to/main", Ref: "refs/heads/main"},
			},
		},
		{
			name: "multiple worktrees",
			out: heredoc.Doc(`
				worktree /path/to/main
				HEAD abc123
				branch refs/heads/main

				worktree /path/to/feature-wt
				HEAD def456
				branch refs/heads/feature
			`),
			want: []Worktree{
				{Path: "/path/to/main", Ref: "refs/heads/main"},
				{Path: "/path/to/feature-wt", Ref: "refs/heads/feature"},
			},
		},
		{
			name: "detached HEAD has no branch",
			out: heredoc.Doc(`
				worktree /path/to/main
				HEAD abc123
				branch refs/heads/main

				worktree /path/to/detached
				HEAD def456
				detached
			`),
			want: []Worktree{
				{Path: "/path/to/main", Ref: "refs/heads/main"},
				{Path: "/path/to/detached", Ref: ""},
			},
		},
		{
			name: "no trailing blank line",
			out:  "worktree /path/to/main\nHEAD abc123\nbranch refs/heads/main",
			want: []Worktree{
				{Path: "/path/to/main", Ref: "refs/heads/main"},
			},
		},
		{
			name: "bare main worktree has no branch",
			out: heredoc.Doc(`
				worktree /path/to/bare
				bare

				worktree /path/to/feature-wt
				HEAD def456
				branch refs/heads/feature
			`),
			want: []Worktree{
				{Path: "/path/to/bare", Ref: ""},
				{Path: "/path/to/feature-wt", Ref: "refs/heads/feature"},
			},
		},
		{
			name: "prunable worktree with spaces and windows line endings",
			out:  "worktree /path/to/main\r\nHEAD abc123\r\nbranch refs/heads/main\r\n\r\nworktree /path/to/feature work\r\nHEAD def456\r\nbranch refs/heads/feature/one\r\nprunable gitdir file points to non-existent location\r\n",
			want: []Worktree{
				{Path: "/path/to/main", Ref: "refs/heads/main"},
				{Path: "/path/to/feature work", Ref: "refs/heads/feature/one", Prunable: true},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given porcelain output from git worktree list
			output := []byte(tt.out)

			// When the worktree records are parsed
			worktrees := parseWorktrees(output)

			// Then paths, refs, and state flags are preserved
			assert.Equal(t, tt.want, worktrees)
		})
	}
}

func TestWorktreeForBranchReturnsExactMatch(t *testing.T) {
	// Given worktrees for main and a nested feature branch
	worktrees := []Worktree{
		{Path: "/path/to/main", Ref: "refs/heads/main"},
		{Path: "/path/to/feature", Ref: "refs/heads/feature/one"},
	}

	// When the nested feature branch is located
	worktree := WorktreeForBranch(worktrees, "feature/one")

	// Then its exact worktree is returned
	assert.Equal(t, &worktrees[1], worktree)
}

func TestWorktreeForBranchRejectsNonMatchingBranch(t *testing.T) {
	worktrees := []Worktree{
		{Path: "/path/to/main", Ref: "refs/heads/main"},
		{Path: "/path/to/feature", Ref: "refs/heads/feature/one"},
	}
	tests := []struct {
		name   string
		branch string
	}{
		{name: "partial branch name", branch: "feature"},
		{name: "missing branch", branch: "missing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given worktrees without the exact requested branch
			branch := tt.branch

			// When the branch is located
			worktree := WorktreeForBranch(worktrees, branch)

			// Then no worktree is returned
			assert.Nil(t, worktree)
		})
	}
}

func TestClientWorktreesListsRepositoryWorktrees(t *testing.T) {
	// Given a repository with a linked feature worktree
	repo := newTestRepo(t)
	repo.run(t, "commit", "--quiet", "--allow-empty", "-m", "initial commit")
	repo.run(t, "branch", "feature")
	worktreeParent, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	worktreeDir := filepath.Join(worktreeParent, "feature")
	repo.run(t, "worktree", "add", "--quiet", worktreeDir, "feature")

	// When repository worktrees are listed
	worktrees, err := repo.client.Worktrees(t.Context())

	// Then the primary and linked worktrees are returned with their refs
	require.NoError(t, err)
	assert.Equal(t, []Worktree{
		{Path: filepath.ToSlash(repo.dir), Ref: "refs/heads/trunk"},
		{Path: filepath.ToSlash(worktreeDir), Ref: "refs/heads/feature"},
	}, worktrees)
}

func TestClientWorktreeRemoveRemovesOptionLikePath(t *testing.T) {
	// Given a linked worktree whose relative path begins with a dash
	repo := newTestRepo(t)
	repo.run(t, "commit", "--quiet", "--allow-empty", "-m", "initial commit")
	repo.run(t, "branch", "feature")
	repo.run(t, "worktree", "add", "--quiet", "--", "-feature", "feature")

	// When the option-like path is removed
	err := repo.client.WorktreeRemove(t.Context(), "-feature")

	// Then the option separator protects the path and the worktree is removed
	require.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(repo.dir, "-feature"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
	worktrees, listErr := repo.client.Worktrees(t.Context())
	require.NoError(t, listErr)
	assert.Nil(t, WorktreeForBranch(worktrees, "feature"))
}

func TestClientWorktreePruneRemovesMissingWorktreeMetadata(t *testing.T) {
	// Given a linked worktree whose directory was removed outside Git
	repo := newTestRepo(t)
	repo.run(t, "commit", "--quiet", "--allow-empty", "-m", "initial commit")
	repo.run(t, "branch", "feature")
	worktreeDir := filepath.Join(t.TempDir(), "feature")
	repo.run(t, "worktree", "add", "--quiet", worktreeDir, "feature")
	require.NoError(t, os.RemoveAll(worktreeDir))

	// When stale worktree metadata is pruned
	err := repo.client.WorktreePrune(t.Context())

	// Then the missing worktree is no longer listed
	require.NoError(t, err)
	worktrees, listErr := repo.client.Worktrees(t.Context())
	require.NoError(t, listErr)
	assert.Nil(t, WorktreeForBranch(worktrees, "feature"))
}
