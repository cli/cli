package git

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
			in, out, errOut := &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}
			client := Client{
				Stdin:   in,
				Stdout:  out,
				Stderr:  errOut,
				RepoDir: tt.repoDir,
				GitPath: tt.gitPath,
			}
			cmd, err := client.Command(context.Background(), "ref-log")
			assert.NoError(t, err)
			assert.Equal(t, tt.wantExe, cmd.Path)
			assert.Equal(t, tt.wantArgs, cmd.Args)
			assert.Equal(t, in, cmd.Stdin)
			assert.Equal(t, out, cmd.Stdout)
			assert.Equal(t, errOut, cmd.Stderr)
		})
	}
}

func TestClientAuthenticatedCommand(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		pattern  CredentialPattern
		wantArgs []string
		wantErr  error
	}{
		{
			name:     "when credential pattern allows for anything, credential helper matches everything",
			path:     "path/to/gh",
			pattern:  AllMatchingCredentialsPattern,
			wantArgs: []string{"path/to/git", "-c", "credential.helper=", "-c", `credential.helper=!"path/to/gh" auth git-credential`, "fetch"},
		},
		{
			name:     "when credential pattern is set, credential helper only matches that pattern",
			path:     "path/to/gh",
			pattern:  CredentialPattern{pattern: "https://github.com"},
			wantArgs: []string{"path/to/git", "-c", "credential.https://github.com.helper=", "-c", `credential.https://github.com.helper=!"path/to/gh" auth git-credential`, "fetch"},
		},
		{
			name:     "fallback when GhPath is not set",
			pattern:  AllMatchingCredentialsPattern,
			wantArgs: []string{"path/to/git", "-c", "credential.helper=", "-c", `credential.helper=!"gh" auth git-credential`, "fetch"},
		},
		{
			name:    "errors when attempting to use an empty pattern that isn't marked all matching",
			pattern: CredentialPattern{allMatching: false, pattern: ""},
			wantErr: fmt.Errorf("empty credential pattern is not allowed unless provided explicitly"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{
				GhPath:  tt.path,
				GitPath: "path/to/git",
			}
			cmd, err := client.AuthenticatedCommand(context.Background(), tt.pattern, "fetch")
			if tt.wantErr != nil {
				require.Equal(t, tt.wantErr, err)
				return
			}
			require.Equal(t, tt.wantArgs, cmd.Args)
		})
	}
}

func TestClientRemotes(t *testing.T) {
	IsolateConfig(t)
	tempDir := t.TempDir()
	initRepo(t, tempDir)
	gitDir := filepath.Join(tempDir, ".git")
	remoteFile := filepath.Join(gitDir, "config")
	remotes := `
[remote "origin"]
	url = git@example.com:monalisa/origin.git
[remote "test"]
	url = git://github.com/hubot/test.git
	gh-resolved = other
[remote "upstream"]
	url = https://github.com/monalisa/upstream.git
	gh-resolved = base
[remote "github"]
	url = git@github.com:hubot/github.git
`
	f, err := os.OpenFile(remoteFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0755)
	assert.NoError(t, err)
	_, err = f.Write([]byte(remotes))
	assert.NoError(t, err)
	err = f.Close()
	assert.NoError(t, err)
	client := Client{
		RepoDir: tempDir,
	}
	rs, err := client.Remotes(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 4, len(rs))
	assert.Equal(t, "upstream", rs[0].Name)
	assert.Equal(t, "base", rs[0].Resolved)
	assert.Equal(t, "github", rs[1].Name)
	assert.Equal(t, "", rs[1].Resolved)
	assert.Equal(t, "origin", rs[2].Name)
	assert.Equal(t, "", rs[2].Resolved)
	assert.Equal(t, "test", rs[3].Name)
	assert.Equal(t, "other", rs[3].Resolved)
}

func TestClientRemotes_no_resolved_remote(t *testing.T) {
	IsolateConfig(t)
	tempDir := t.TempDir()
	initRepo(t, tempDir)
	gitDir := filepath.Join(tempDir, ".git")
	remoteFile := filepath.Join(gitDir, "config")
	remotes := `
[remote "origin"]
	url = git@example.com:monalisa/origin.git
[remote "test"]
	url = git://github.com/hubot/test.git
[remote "upstream"]
	url = https://github.com/monalisa/upstream.git
[remote "github"]
	url = git@github.com:hubot/github.git
`
	f, err := os.OpenFile(remoteFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0755)
	assert.NoError(t, err)
	_, err = f.Write([]byte(remotes))
	assert.NoError(t, err)
	err = f.Close()
	assert.NoError(t, err)
	client := Client{
		RepoDir: tempDir,
	}
	rs, err := client.Remotes(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 4, len(rs))
	assert.Equal(t, "upstream", rs[0].Name)
	assert.Equal(t, "github", rs[1].Name)
	assert.Equal(t, "origin", rs[2].Name)
	assert.Equal(t, "", rs[2].Resolved)
	assert.Equal(t, "test", rs[3].Name)
}

func TestParseRemotes(t *testing.T) {
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

	r := parseRemotes(remoteList)
	assert.Equal(t, 5, len(r))

	assert.Equal(t, "mona", r[0].Name)
	assert.Equal(t, "ssh://git@github.com/monalisa/myfork.git", r[0].FetchURL.String())
	assert.Nil(t, r[0].PushURL)

	assert.Equal(t, "origin", r[1].Name)
	assert.Equal(t, "/monalisa/octo-cat.git", r[1].FetchURL.Path)
	assert.Equal(t, "/monalisa/octo-cat-push.git", r[1].PushURL.Path)

	assert.Equal(t, "upstream", r[2].Name)
	assert.Equal(t, "example.com", r[2].FetchURL.Host)
	assert.Equal(t, "github.com", r[2].PushURL.Host)

	assert.Equal(t, "zardoz", r[3].Name)
	assert.Nil(t, r[3].FetchURL)
	assert.Equal(t, "https://example.com/zed.git", r[3].PushURL.String())

	assert.Equal(t, "koke", r[4].Name)
	assert.Equal(t, "/koke/grit.git", r[4].FetchURL.Path)
	assert.Equal(t, "/koke/grit.git", r[4].PushURL.Path)
}

func TestClientUpdateRemoteURL(t *testing.T) {
	tests := []struct {
		name          string
		cmdExitStatus int
		cmdStdout     string
		cmdStderr     string
		wantCmdArgs   string
		wantErrorMsg  string
	}{
		{
			name:        "update remote url",
			wantCmdArgs: `path/to/git remote set-url test https://test.com`,
		},
		{
			name:          "git error",
			cmdExitStatus: 1,
			cmdStderr:     "git error message",
			wantCmdArgs:   `path/to/git remote set-url test https://test.com`,
			wantErrorMsg:  "failed to run git: git error message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, cmdCtx := createCommandContext(t, tt.cmdExitStatus, tt.cmdStdout, tt.cmdStderr)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			err := client.UpdateRemoteURL(context.Background(), "test", "https://test.com")
			assert.Equal(t, tt.wantCmdArgs, strings.Join(cmd.Args[3:], " "))
			if tt.wantErrorMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErrorMsg)
			}
		})
	}
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
	// Given Git will fail while setting a remote resolution
	_, cmdCtx := createCommandContext(t, 2, "", "git error message")
	client := Client{
		GitPath:        "path/to/git",
		commandContext: cmdCtx,
	}

	// When a remote resolution is set
	err := client.SetRemoteResolution(t.Context(), "origin", "base")

	// Then the Git error is returned
	require.Error(t, err)
	assert.EqualError(t, err, "failed to run git: git error message")
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
	// Given Git will fail with an unexpected exit status
	_, cmdCtx := createCommandContext(t, 2, "", "git error message")
	client := Client{
		GitPath:        "path/to/git",
		commandContext: cmdCtx,
	}

	// When configuration is read
	value, err := client.Config(t.Context(), "credential.helper")

	// Then the Git error is returned unchanged
	require.Error(t, err)
	assert.EqualError(t, err, "failed to run git: git error message")
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
	IsolateConfig(t)
	client := Client{
		RepoDir: "./fixtures/simple.git",
	}
	c, err := client.LastCommit(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "6f1a2405cace1633d89a79c74c65f22fe78f9659", c.Sha)
	assert.Equal(t, "Second commit", c.Title)
}

func TestClientCommitBody(t *testing.T) {
	IsolateConfig(t)
	client := Client{
		RepoDir: "./fixtures/simple.git",
	}
	body, err := client.CommitBody(context.Background(), "6f1a2405cace1633d89a79c74c65f22fe78f9659")
	assert.NoError(t, err)
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

func Test_parseBranchConfig(t *testing.T) {
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
			branchConfig := parseBranchConfig(tt.configLines)
			assert.Equalf(t, tt.wantBranchConfig.RemoteName, branchConfig.RemoteName, "unexpected RemoteName")
			assert.Equalf(t, tt.wantBranchConfig.MergeRef, branchConfig.MergeRef, "unexpected MergeRef")
			assert.Equalf(t, tt.wantBranchConfig.MergeBase, branchConfig.MergeBase, "unexpected MergeBase")
			assert.Equalf(t, tt.wantBranchConfig.PushRemoteName, branchConfig.PushRemoteName, "unexpected PushRemoteName")
			if tt.wantBranchConfig.RemoteURL != nil {
				assert.Equalf(t, tt.wantBranchConfig.RemoteURL.String(), branchConfig.RemoteURL.String(), "unexpected RemoteURL")
			}
			if tt.wantBranchConfig.PushRemoteURL != nil {
				assert.Equalf(t, tt.wantBranchConfig.PushRemoteURL.String(), branchConfig.PushRemoteURL.String(), "unexpected PushRemoteURL")
			}
		})
	}
}

func Test_parseRemoteURLOrName(t *testing.T) {
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
			remoteURL, remoteName := parseRemoteURLOrName(tt.value)
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

func TestClientPushRevisionReportsMalformedSymbolicRef(t *testing.T) {
	// Given Git returns a symbolic push ref outside refs/remotes
	cmdCtx := createMockedCommandContext(t, mockedCommands{
		`path/to/git rev-parse --symbolic-full-name trunk@{push}`: {Stdout: "not/a/valid/remote/ref"},
	})
	client := Client{GitPath: "path/to/git", commandContext: cmdCtx}

	// When the push revision is parsed
	trackingRef, err := client.PushRevision(t.Context(), "trunk")

	// Then the malformed ref is reported with context
	require.EqualError(t, err, "could not parse push revision: remote tracking branch must have format refs/remotes/<remote>/<branch> but was: not/a/valid/remote/ref")
	assert.Empty(t, trackingRef)
}

func TestRemoteTrackingRef(t *testing.T) {
	t.Run("parsing", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name                  string
			remoteTrackingRef     string
			wantRemoteTrackingRef RemoteTrackingRef
			wantError             error
		}{
			{
				name:              "valid remote tracking ref without slash in branch name",
				remoteTrackingRef: "refs/remotes/origin/branchName",
				wantRemoteTrackingRef: RemoteTrackingRef{
					Remote: "origin",
					Branch: "branchName",
				},
			},
			{
				name:              "valid remote tracking ref with slash in branch name",
				remoteTrackingRef: "refs/remotes/origin/branch/name",
				wantRemoteTrackingRef: RemoteTrackingRef{
					Remote: "origin",
					Branch: "branch/name",
				},
			},
			// TODO: Uncomment when we support slashes in remote names
			// {
			// 	name: "valid remote tracking ref with slash in remote name",
			// 	remoteTrackingRef: "refs/remotes/my/origin/branchName",
			// 	wantRemoteTrackingRef: RemoteTrackingRef{
			// 		Remote: "my/origin",
			// 		Branch: "branchName",
			// 	},
			// },
			// {
			// 	name: 			"valid remote tracking ref with slash in remote name and branch name",
			// 	remoteTrackingRef: "refs/remotes/my/origin/branch/name",
			// 	wantRemoteTrackingRef: RemoteTrackingRef{
			// 		Remote: "my/origin",
			// 		Branch: "branch/name",
			// 	},
			// },
			{
				name:                  "incorrect parts",
				remoteTrackingRef:     "refs/remotes/origin",
				wantRemoteTrackingRef: RemoteTrackingRef{},
				wantError:             fmt.Errorf("remote tracking branch must have format refs/remotes/<remote>/<branch> but was: refs/remotes/origin"),
			},
			{
				name:                  "incorrect prefix type",
				remoteTrackingRef:     "invalid/remotes/origin/branchName",
				wantRemoteTrackingRef: RemoteTrackingRef{},
				wantError:             fmt.Errorf("remote tracking branch must have format refs/remotes/<remote>/<branch> but was: invalid/remotes/origin/branchName"),
			},
			{
				name:                  "incorrect ref type",
				remoteTrackingRef:     "refs/invalid/origin/branchName",
				wantRemoteTrackingRef: RemoteTrackingRef{},
				wantError:             fmt.Errorf("remote tracking branch must have format refs/remotes/<remote>/<branch> but was: refs/invalid/origin/branchName"),
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				trackingRef, err := ParseRemoteTrackingRef(tt.remoteTrackingRef)
				if tt.wantError != nil {
					require.Equal(t, tt.wantError, err)
					return
				}

				require.NoError(t, err)
				assert.Equal(t, tt.wantRemoteTrackingRef, trackingRef)
			})
		}
	})

	t.Run("stringifying", func(t *testing.T) {
		t.Parallel()

		remoteTrackingRef := RemoteTrackingRef{
			Remote: "origin",
			Branch: "branchName",
		}

		require.Equal(t, "refs/remotes/origin/branchName", remoteTrackingRef.String())
	})
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
	// Given Git will fail while unsetting a remote resolution
	_, cmdCtx := createCommandContext(t, 2, "", "git error message")
	client := Client{
		GitPath:        "path/to/git",
		commandContext: cmdCtx,
	}

	// When a remote resolution is unset
	err := client.UnsetRemoteResolution(t.Context(), "origin")

	// Then the Git error is returned
	require.Error(t, err)
	assert.EqualError(t, err, "failed to run git: git error message")
}

func TestClientSetRemoteBranches(t *testing.T) {
	tests := []struct {
		name          string
		cmdExitStatus int
		cmdStdout     string
		cmdStderr     string
		wantCmdArgs   string
		wantErrorMsg  string
	}{
		{
			name:        "set remote branches",
			wantCmdArgs: `path/to/git remote set-branches origin trunk`,
		},
		{
			name:          "git error",
			cmdExitStatus: 1,
			cmdStderr:     "git error message",
			wantCmdArgs:   `path/to/git remote set-branches origin trunk`,
			wantErrorMsg:  "failed to run git: git error message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, cmdCtx := createCommandContext(t, tt.cmdExitStatus, tt.cmdStdout, tt.cmdStderr)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			err := client.SetRemoteBranches(context.Background(), "origin", "trunk")
			assert.Equal(t, tt.wantCmdArgs, strings.Join(cmd.Args[3:], " "))
			if tt.wantErrorMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErrorMsg)
			}
		})
	}
}

func TestClientFetch(t *testing.T) {
	tests := []struct {
		name         string
		mods         []CommandModifier
		commands     mockedCommands
		wantErrorMsg string
	}{
		{
			name: "fetch",
			commands: map[args]commandResult{
				`path/to/git -c credential.helper= -c credential.helper=!"gh" auth git-credential fetch origin trunk`: {
					ExitStatus: 0,
				},
			},
		},
		{
			name: "accepts command modifiers",
			mods: []CommandModifier{WithRepoDir("/path/to/repo")},
			commands: map[args]commandResult{
				`path/to/git -C /path/to/repo -c credential.helper= -c credential.helper=!"gh" auth git-credential fetch origin trunk`: {
					ExitStatus: 0,
				},
			},
		},
		{
			name: "git error on fetch",
			commands: map[args]commandResult{
				`path/to/git -c credential.helper= -c credential.helper=!"gh" auth git-credential fetch origin trunk`: {
					ExitStatus: 1,
					Stderr:     "fetch error message",
				},
			},
			wantErrorMsg: "failed to run git: fetch error message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmdCtx := createMockedCommandContext(t, tt.commands)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			err := client.Fetch(context.Background(), "origin", "trunk", tt.mods...)
			if tt.wantErrorMsg == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tt.wantErrorMsg)
			}
		})
	}
}

func TestClientPull(t *testing.T) {
	tests := []struct {
		name         string
		mods         []CommandModifier
		commands     mockedCommands
		wantErrorMsg string
	}{
		{
			name: "pull",
			commands: map[args]commandResult{
				`path/to/git -c credential.helper= -c credential.helper=!"gh" auth git-credential pull --ff-only origin trunk`: {
					ExitStatus: 0,
				},
			},
		},
		{
			name: "accepts command modifiers",
			mods: []CommandModifier{WithRepoDir("/path/to/repo")},
			commands: map[args]commandResult{
				`path/to/git -C /path/to/repo -c credential.helper= -c credential.helper=!"gh" auth git-credential pull --ff-only origin trunk`: {
					ExitStatus: 0,
				},
			},
		},
		{
			name: "git error on pull",
			commands: map[args]commandResult{
				`path/to/git -c credential.helper= -c credential.helper=!"gh" auth git-credential pull --ff-only origin trunk`: {
					ExitStatus: 1,
					Stderr:     "pull error message",
				},
			},
			wantErrorMsg: "failed to run git: pull error message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmdCtx := createMockedCommandContext(t, tt.commands)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			err := client.Pull(context.Background(), "origin", "trunk", tt.mods...)
			if tt.wantErrorMsg == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tt.wantErrorMsg)
			}
		})
	}
}

func TestClientPush(t *testing.T) {
	tests := []struct {
		name         string
		mods         []CommandModifier
		commands     mockedCommands
		wantErrorMsg string
	}{
		{
			name: "push",
			commands: map[args]commandResult{
				`path/to/git -c credential.helper= -c credential.helper=!"gh" auth git-credential push --set-upstream origin trunk`: {
					ExitStatus: 0,
				},
			},
		},
		{
			name: "accepts command modifiers",
			mods: []CommandModifier{WithRepoDir("/path/to/repo")},
			commands: map[args]commandResult{
				`path/to/git -C /path/to/repo -c credential.helper= -c credential.helper=!"gh" auth git-credential push --set-upstream origin trunk`: {
					ExitStatus: 0,
				},
			},
		},
		{
			name: "git error on push",
			commands: map[args]commandResult{
				`path/to/git -c credential.helper= -c credential.helper=!"gh" auth git-credential push --set-upstream origin trunk`: {
					ExitStatus: 1,
					Stderr:     "push error message",
				},
			},
			wantErrorMsg: "failed to run git: push error message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmdCtx := createMockedCommandContext(t, tt.commands)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			err := client.Push(context.Background(), "origin", "trunk", tt.mods...)
			if tt.wantErrorMsg == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tt.wantErrorMsg)
			}
		})
	}
}

func TestClientClone(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		mods          []CommandModifier
		cmdExitStatus int
		cmdStdout     string
		cmdStderr     string
		wantCmdArgs   string
		wantTarget    string
		wantErrorMsg  string
	}{
		{
			name:        "clone",
			args:        []string{},
			wantCmdArgs: `path/to/git -c credential.https://github.com.helper= -c credential.https://github.com.helper=!"gh" auth git-credential clone https://github.com/cli/cli`,
			wantTarget:  "cli",
		},
		{
			name:        "accepts command modifiers",
			args:        []string{},
			mods:        []CommandModifier{WithRepoDir("/path/to/repo")},
			wantCmdArgs: `path/to/git -C /path/to/repo -c credential.https://github.com.helper= -c credential.https://github.com.helper=!"gh" auth git-credential clone https://github.com/cli/cli`,
			wantTarget:  "cli",
		},
		{
			name:          "git error",
			args:          []string{},
			cmdExitStatus: 1,
			cmdStderr:     "git error message",
			wantCmdArgs:   `path/to/git -c credential.https://github.com.helper= -c credential.https://github.com.helper=!"gh" auth git-credential clone https://github.com/cli/cli`,
			wantErrorMsg:  "failed to run git: git error message",
		},
		{
			name:        "bare clone",
			args:        []string{"--bare"},
			wantCmdArgs: `path/to/git -c credential.https://github.com.helper= -c credential.https://github.com.helper=!"gh" auth git-credential clone --bare https://github.com/cli/cli`,
			wantTarget:  "cli.git",
		},
		{
			name:        "bare clone with explicit target",
			args:        []string{"cli-bare", "--bare"},
			wantCmdArgs: `path/to/git -c credential.https://github.com.helper= -c credential.https://github.com.helper=!"gh" auth git-credential clone --bare https://github.com/cli/cli cli-bare`,
			wantTarget:  "cli-bare",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, cmdCtx := createCommandContext(t, tt.cmdExitStatus, tt.cmdStdout, tt.cmdStderr)
			client := Client{
				GitPath:        "path/to/git",
				commandContext: cmdCtx,
			}
			target, err := client.Clone(context.Background(), "https://github.com/cli/cli", tt.args, tt.mods...)
			assert.Equal(t, tt.wantCmdArgs, strings.Join(cmd.Args[3:], " "))
			if tt.wantErrorMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErrorMsg)
			}
			assert.Equal(t, tt.wantTarget, target)
		})
	}
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
			args, dir := parseCloneArgs(tt.args)
			got := wanted{args: args, dir: dir}
			assert.Equal(t, got, tt.want)
		})
	}
}

func TestClientAddRemote(t *testing.T) {
	tests := []struct {
		title         string
		name          string
		url           string
		branches      []string
		dir           string
		cmdExitStatus int
		cmdStdout     string
		cmdStderr     string
		wantCmdArgs   string
		wantErrorMsg  string
	}{
		{
			title:       "fetch all",
			name:        "test",
			url:         "URL",
			dir:         "DIRECTORY",
			branches:    []string{},
			wantCmdArgs: `path/to/git -C DIRECTORY remote add test URL`,
		},
		{
			title:       "fetch specific branches only",
			name:        "test",
			url:         "URL",
			dir:         "DIRECTORY",
			branches:    []string{"trunk", "dev"},
			wantCmdArgs: `path/to/git -C DIRECTORY remote add -t trunk -t dev test URL`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			cmd, cmdCtx := createCommandContext(t, tt.cmdExitStatus, tt.cmdStdout, tt.cmdStderr)
			client := Client{
				GitPath:        "path/to/git",
				RepoDir:        tt.dir,
				commandContext: cmdCtx,
			}
			_, err := client.AddRemote(context.Background(), tt.name, tt.url, tt.branches)
			assert.Equal(t, tt.wantCmdArgs, strings.Join(cmd.Args[3:], " "))
			assert.NoError(t, err)
		})
	}
}

func initRepo(t *testing.T, dir string) {
	errBuf := &bytes.Buffer{}
	inBuf := &bytes.Buffer{}
	outBuf := &bytes.Buffer{}
	client := Client{
		RepoDir: dir,
		Stderr:  errBuf,
		Stdin:   inBuf,
		Stdout:  outBuf,
	}
	cmd, err := client.Command(context.Background(), []string{"init", "--quiet"}...)
	assert.NoError(t, err)
	_, err = cmd.Output()
	assert.NoError(t, err)
}

type testRepo struct {
	dir    string
	client *Client
}

func newTestRepo(t *testing.T) *testRepo {
	t.Helper()
	IsolateConfig(t)
	t.Setenv("GIT_TERMINAL_PROMPT", "0")

	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	repo := &testRepo{dir: dir}
	repo.client = &Client{RepoDir: repo.dir}
	repo.run(t, "init", "--quiet", "--initial-branch=trunk")
	repo.run(t, "config", "user.name", "GitHub CLI Test")
	repo.run(t, "config", "user.email", "gh-test@example.com")
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

type args string

type commandResult struct {
	ExitStatus int    `json:"exitStatus"`
	Stdout     string `json:"out"`
	Stderr     string `json:"err"`
}

type mockedCommands map[args]commandResult

// TestCommandMocking is an invoked test helper that emulates expected behavior for predefined shell commands, erroring when unexpected conditions are encountered.
func TestCommandMocking(t *testing.T) {
	if os.Getenv("GH_WANT_HELPER_PROCESS_RICH") != "1" {
		return
	}

	jsonVar, ok := os.LookupEnv("GH_HELPER_PROCESS_RICH_COMMANDS")
	if !ok {
		fmt.Fprint(os.Stderr, "missing GH_HELPER_PROCESS_RICH_COMMANDS")
		// Exit 1 is used for empty key values in the git config. This is non-breaking in those use cases,
		// so this is returning a non-zero exit code to avoid suppressing this error for those use cases.
		os.Exit(16)
	}

	var commands mockedCommands
	if err := json.Unmarshal([]byte(jsonVar), &commands); err != nil {
		fmt.Fprint(os.Stderr, "failed to unmarshal GH_HELPER_PROCESS_RICH_COMMANDS")
		// Exit 1 is used for empty key values in the git config. This is non-breaking in those use cases,
		// so this is returning a non-zero exit code to avoid suppressing this error for those use cases.
		os.Exit(16)
	}

	// The discarded args are those for the go test binary itself, e.g. `-test.run=TestHelperProcessRich`
	realArgs := os.Args[3:]

	commandResult, ok := commands[args(strings.Join(realArgs, " "))]
	if !ok {
		fmt.Fprintf(os.Stderr, "unexpected command: %s\n", strings.Join(realArgs, " "))
		// Exit 1 is used for empty key values in the git config. This is non-breaking in those use cases,
		// so this is returning a non-zero exit code to avoid suppressing this error for those use cases.
		os.Exit(16)
	}

	if commandResult.Stdout != "" {
		fmt.Fprint(os.Stdout, commandResult.Stdout)
	}

	if commandResult.Stderr != "" {
		fmt.Fprint(os.Stderr, commandResult.Stderr)
	}

	os.Exit(commandResult.ExitStatus)
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GH_WANT_HELPER_PROCESS") != "1" {
		return
	}
	if err := func(args []string) error {
		fmt.Fprint(os.Stdout, os.Getenv("GH_HELPER_PROCESS_STDOUT"))
		exitStatus := os.Getenv("GH_HELPER_PROCESS_EXIT_STATUS")
		if exitStatus != "0" {
			return errors.New("error")
		}
		return nil
	}(os.Args[3:]); err != nil {
		fmt.Fprint(os.Stderr, os.Getenv("GH_HELPER_PROCESS_STDERR"))
		exitStatus := os.Getenv("GH_HELPER_PROCESS_EXIT_STATUS")
		i, err := strconv.Atoi(exitStatus)
		if err != nil {
			os.Exit(1)
		}
		os.Exit(i)
	}
	os.Exit(0)
}

func TestCredentialPatternFromGitURL(t *testing.T) {
	tests := []struct {
		name                  string
		gitURL                string
		wantErr               bool
		wantCredentialPattern CredentialPattern
	}{
		{
			name:   "Given a well formed gitURL, it returns the corresponding CredentialPattern",
			gitURL: "https://github.com/OWNER/REPO.git",
			wantCredentialPattern: CredentialPattern{
				pattern:     "https://github.com",
				allMatching: false,
			},
		},
		{
			name: "Given a malformed gitURL, it returns an error",
			// This pattern is copied from the tests in ParseURL
			// Unexpectedly, a non URL-like string did not error in ParseURL
			gitURL:  "ssh://git@[/tmp/git-repo",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credentialPattern, err := CredentialPatternFromGitURL(tt.gitURL)
			if tt.wantErr {
				assert.ErrorContains(t, err, "failed to parse remote URL")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantCredentialPattern, credentialPattern)
			}
		})
	}
}

func TestCredentialPatternFromHost(t *testing.T) {
	tests := []struct {
		name                  string
		host                  string
		wantCredentialPattern CredentialPattern
	}{
		{
			name: "Given a well formed host, it returns the corresponding CredentialPattern",
			host: "github.com",
			wantCredentialPattern: CredentialPattern{
				pattern:     "https://github.com",
				allMatching: false,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credentialPattern := CredentialPatternFromHost(tt.host)
			require.Equal(t, tt.wantCredentialPattern, credentialPattern)
		})
	}
}

func TestPushDefault(t *testing.T) {
	t.Run("it parses valid values correctly", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			value               string
			expectedPushDefault PushDefault
		}{
			{"nothing", PushDefaultNothing},
			{"current", PushDefaultCurrent},
			{"upstream", PushDefaultUpstream},
			{"tracking", PushDefaultTracking},
			{"simple", PushDefaultSimple},
			{"matching", PushDefaultMatching},
		}

		for _, test := range tests {
			t.Run(test.value, func(t *testing.T) {
				t.Parallel()

				pushDefault, err := ParsePushDefault(test.value)
				require.NoError(t, err)
				assert.Equal(t, test.expectedPushDefault, pushDefault)
			})
		}
	})

	t.Run("it returns an error for invalid values", func(t *testing.T) {
		t.Parallel()

		_, err := ParsePushDefault("invalid")
		require.Error(t, err)
	})
}

func createCommandContext(t *testing.T, exitStatus int, stdout, stderr string) (*exec.Cmd, commandCtx) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), os.Args[0], "-test.run=TestHelperProcess", "--")
	cmd.Env = append(os.Environ(),
		"GH_WANT_HELPER_PROCESS=1",
		fmt.Sprintf("GH_HELPER_PROCESS_STDOUT=%s", stdout),
		fmt.Sprintf("GH_HELPER_PROCESS_STDERR=%s", stderr),
		fmt.Sprintf("GH_HELPER_PROCESS_EXIT_STATUS=%v", exitStatus),
	)
	return cmd, func(ctx context.Context, exe string, args ...string) *exec.Cmd {
		cmd.Args = append(cmd.Args, exe)
		cmd.Args = append(cmd.Args, args...)
		return cmd
	}
}

func createMockedCommandContext(t *testing.T, commands mockedCommands) commandCtx {
	marshaledCommands, err := json.Marshal(commands)
	require.NoError(t, err)

	// invokes helper within current test binary, emulating desired behavior
	return func(ctx context.Context, exe string, args ...string) *exec.Cmd {
		cmd := exec.CommandContext(context.Background(), os.Args[0], "-test.run=TestCommandMocking", "--")
		cmd.Env = append(os.Environ(),
			"GH_WANT_HELPER_PROCESS_RICH=1",
			fmt.Sprintf("GH_HELPER_PROCESS_RICH_COMMANDS=%s", string(marshaledCommands)),
		)

		cmd.Args = append(cmd.Args, exe)
		cmd.Args = append(cmd.Args, args...)
		return cmd
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
	// Given a remote name that resembles a Git option
	cmd, cmdCtx := createCommandContext(t, 0, "https://example.com/repo.git\n", "")
	client := Client{
		GitPath:        "path/to/git",
		commandContext: cmdCtx,
	}

	// When the remote URL is requested
	_, err := client.RemoteURL(t.Context(), "--upload-pack=malicious")

	// Then an option separator protects the remote name
	require.NoError(t, err)
	assert.Equal(t, "path/to/git remote get-url -- --upload-pack=malicious", strings.Join(cmd.Args[3:], " "))
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
	// Given a path that resembles a Git option
	cmd, cmdCtx := createCommandContext(t, 0, "", "")
	client := Client{
		GitPath:        "path/to/git",
		commandContext: cmdCtx,
	}

	// When the path is checked
	ignored, err := client.IsIgnored(t.Context(), "--no-index")

	// Then an option separator protects the path
	require.NoError(t, err)
	assert.True(t, ignored, "the successful Git result should report the path as ignored")
	assert.Equal(t, "path/to/git check-ignore -q -- --no-index", strings.Join(cmd.Args[3:], " "))
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
	assert.Equal(t, "abc123de", ShortSHA("abc123def456789"))
	assert.Equal(t, "short", ShortSHA("short"))
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
			got := parseWorktrees([]byte(tt.out))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestWorktreeForBranch(t *testing.T) {
	worktrees := []Worktree{
		{Path: "/path/to/main", Ref: "refs/heads/main"},
		{Path: "/path/to/feature", Ref: "refs/heads/feature/one"},
	}

	assert.Equal(t, &worktrees[1], WorktreeForBranch(worktrees, "feature/one"))
	assert.Nil(t, WorktreeForBranch(worktrees, "feature"))
	assert.Nil(t, WorktreeForBranch(worktrees, "missing"))
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
