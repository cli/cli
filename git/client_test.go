package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

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
		wantArgs []string
	}{
		{
			name:     "adds credential helper config options",
			path:     "path/to/gh",
			wantArgs: []string{"path/to/git", "-c", "credential.helper=", "-c", "credential.helper=!\"path/to/gh\" auth git-credential", "fetch"},
		},
		{
			name:     "fallback when GhPath is not set",
			wantArgs: []string{"path/to/git", "-c", "credential.helper=", "-c", "credential.helper=!\"gh\" auth git-credential", "fetch"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{
				GhPath:  tt.path,
				GitPath: "path/to/git",
			}
			cmd, err := client.AuthenticatedCommand(context.Background(), "fetch")
			assert.NoError(t, err)
			assert.Equal(t, tt.wantArgs, cmd.Args)
		})
	}
}

func TestClientRemotes(t *testing.T) {
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
		name         string
		stub         commandCtx
		wantErrorMsg string
	}{
		{
			name: "update remote url",
			stub: stubCommandContext(t, `git remote set-url test https://test.com`, 0, "", ""),
		},
		{
			name:         "git error",
			stub:         stubCommandContext(t, `git remote set-url test https://test.com`, 1, "", "git error message"),
			wantErrorMsg: "failed to run git: git error message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{
				GitPath:        "path/to/git",
				commandContext: tt.stub,
			}
			err := client.UpdateRemoteURL(context.Background(), "test", "https://test.com")
			if tt.wantErrorMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErrorMsg)
			}
		})
	}
}

func TestClientSetRemoteResolution(t *testing.T) {
	tests := []struct {
		name         string
		stub         commandCtx
		wantErrorMsg string
	}{
		{
			name: "set remote resolution",
			stub: stubCommandContext(t, `git config --add remote.origin.gh-resolved base`, 0, "", ""),
		},
		{
			name:         "git error",
			stub:         stubCommandContext(t, `git config --add remote.origin.gh-resolved base`, 1, "", "git error message"),
			wantErrorMsg: "failed to run git: git error message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{
				GitPath:        "path/to/git",
				commandContext: tt.stub,
			}
			err := client.SetRemoteResolution(context.Background(), "origin", "base")
			if tt.wantErrorMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErrorMsg)
			}
		})
	}
}

func TestClientCurrentBranch(t *testing.T) {
	tests := []struct {
		name     string
		stub     string
		expected string
	}{
		{
			name:     "branch name",
			stub:     "branch-name\n",
			expected: "branch-name",
		},
		{
			name:     "ref",
			stub:     "refs/heads/branch-name\n",
			expected: "branch-name",
		},
		{
			name:     "escaped ref",
			stub:     "refs/heads/branch\u00A0with\u00A0non\u00A0breaking\u00A0space\n",
			expected: "branch\u00A0with\u00A0non\u00A0breaking\u00A0space",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{
				GitPath:        "path/to/git",
				commandContext: stubCommandContext(t, `git symbolic-ref --quiet HEAD`, 0, tt.stub, ""),
			}
			branch, err := client.CurrentBranch(context.Background())
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, branch)
		})
	}
}

func TestClientCurrentBranch_detached_head_state(t *testing.T) {
	client := Client{
		GitPath:        "path/to/git",
		commandContext: stubCommandContext(t, `git symbolic-ref --quiet HEAD`, 1, "", ""),
	}
	_, err := client.CurrentBranch(context.Background())
	assert.EqualError(t, err, "failed to run git: not on any branch")
}

func TestClientShowRefs(t *testing.T) {
	tests := []struct {
		name         string
		stub         commandCtx
		wantRefs     []Ref
		wantErrorMsg string
	}{
		{
			name: "show refs with one vaid ref and one invalid ref",
			stub: stubCommandContext(t,
				`git show-ref --verify -- refs/heads/valid refs/heads/invalid`,
				128,
				"9ea76237a557015e73446d33268569a114c0649c refs/heads/valid",
				"fatal: 'refs/heads/invalid' - not a valid ref"),
			wantRefs: []Ref{{
				Hash: "9ea76237a557015e73446d33268569a114c0649c",
				Name: "refs/heads/valid",
			}},
			wantErrorMsg: "failed to run git: fatal: 'refs/heads/invalid' - not a valid ref",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{
				GitPath:        "path/to/git",
				commandContext: tt.stub,
			}
			refs, err := client.ShowRefs(context.Background(), "refs/heads/valid", "refs/heads/invalid")
			assert.EqualError(t, err, tt.wantErrorMsg)
			assert.Equal(t, tt.wantRefs, refs)
		})
	}
}

func TestClientConfig(t *testing.T) {
	tests := []struct {
		name         string
		stub         commandCtx
		wantOut      string
		wantErrorMsg string
	}{
		{
			name:    "get config key",
			stub:    stubCommandContext(t, `git config credential.helper`, 0, "test", ""),
			wantOut: "test",
		},
		{
			name:         "get unknown config key",
			stub:         stubCommandContext(t, `git config credential.helper`, 1, "", "git error message"),
			wantErrorMsg: "failed to run git: unknown config key credential.helper",
		},
		{
			name:         "git error",
			stub:         stubCommandContext(t, `git config credential.helper`, 2, "", "git error message"),
			wantErrorMsg: "failed to run git: git error message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{
				GitPath:        "path/to/git",
				commandContext: tt.stub,
			}
			out, err := client.Config(context.Background(), "credential.helper")
			if tt.wantErrorMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErrorMsg)
			}
			assert.Equal(t, tt.wantOut, out)
		})
	}
}

func TestClientUncommittedChangeCount(t *testing.T) {
	tests := []struct {
		name     string
		expected int
		output   string
	}{
		{
			name:     "no changes",
			expected: 0,
			output:   "",
		},
		{
			name:     "one change",
			expected: 1,
			output:   " M poem.txt",
		},
		{
			name:     "untracked file",
			expected: 2,
			output:   " M poem.txt\n?? new.txt",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{
				GitPath:        "path/to/git",
				commandContext: stubCommandContext(t, `git status --porcelain`, 0, tt.output, ""),
			}
			ucc, err := client.UncommittedChangeCount(context.Background())
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, ucc)
		})
	}
}

func TestClientCommits(t *testing.T) {
	tests := []struct {
		name         string
		stub         commandCtx
		wantCommits  []*Commit
		wantErrorMsg string
	}{
		{
			name: "get commits",
			stub: stubCommandContext(t,
				`git -c log.ShowSignature=false log --pretty=format:%H,%s --cherry SHA1...SHA2`,
				0,
				"6a6872b918c601a0e730710ad8473938a7516d30,testing testability test",
				""),
			wantCommits: []*Commit{{
				Sha:   "6a6872b918c601a0e730710ad8473938a7516d30",
				Title: "testing testability test",
			}},
		},
		{
			name: "no commits between SHAs",
			stub: stubCommandContext(t,
				`git -c log.ShowSignature=false log --pretty=format:%H,%s --cherry SHA1...SHA2`,
				0,
				"",
				""),
			wantErrorMsg: "could not find any commits between SHA1 and SHA2",
		},
		{
			name: "git error",
			stub: stubCommandContext(t,
				`git -c log.ShowSignature=false log --pretty=format:%H,%s --cherry SHA1...SHA2`,
				1,
				"",
				"git error message"),
			wantErrorMsg: "failed to run git: git error message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{
				GitPath:        "path/to/git",
				commandContext: tt.stub,
			}
			commits, err := client.Commits(context.Background(), "SHA1", "SHA2")
			if tt.wantErrorMsg != "" {
				assert.EqualError(t, err, tt.wantErrorMsg)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantCommits, commits)
		})
	}
}

func TestClientLastCommit(t *testing.T) {
	client := Client{
		RepoDir: "./fixtures/simple.git",
	}
	c, err := client.LastCommit(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "6f1a2405cace1633d89a79c74c65f22fe78f9659", c.Sha)
	assert.Equal(t, "Second commit", c.Title)
}

func TestClientCommitBody(t *testing.T) {
	client := Client{
		RepoDir: "./fixtures/simple.git",
	}
	body, err := client.CommitBody(context.Background(), "6f1a2405cace1633d89a79c74c65f22fe78f9659")
	assert.NoError(t, err)
	assert.Equal(t, "I'm starting to get the hang of things\n", body)
}

func TestClientReadBranchConfig(t *testing.T) {
	tests := []struct {
		name             string
		stub             commandCtx
		wantBranchConfig BranchConfig
	}{
		{
			name: "read branch config",
			stub: stubCommandContext(t,
				`git config --get-regexp \^branch\\\.trunk\\\.\(remote\|merge\)\$`,
				0,
				"branch.trunk.remote origin\nbranch.trunk.merge refs/heads/trunk",
				""),
			wantBranchConfig: BranchConfig{RemoteName: "origin", MergeRef: "refs/heads/trunk"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{
				GitPath:        "path/to/git",
				commandContext: tt.stub,
			}
			branchConfig := client.ReadBranchConfig(context.Background(), "trunk")
			assert.Equal(t, tt.wantBranchConfig, branchConfig)
		})
	}
}

func TestClientDeleteLocalBranch(t *testing.T) {
	tests := []struct {
		name         string
		stub         commandCtx
		wantErrorMsg string
	}{
		{
			name: "delete local branch",
			stub: stubCommandContext(t, `git branch -D trunk`, 0, "", ""),
		},
		{
			name:         "git error",
			stub:         stubCommandContext(t, `git branch -D trunk`, 1, "", "git error message"),
			wantErrorMsg: "failed to run git: git error message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{
				GitPath:        "path/to/git",
				commandContext: tt.stub,
			}
			err := client.DeleteLocalBranch(context.Background(), "trunk")
			if tt.wantErrorMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErrorMsg)
			}
		})
	}
}

func TestClientHasLocalBranch(t *testing.T) {
	tests := []struct {
		name    string
		stub    commandCtx
		wantOut bool
	}{
		{
			name:    "has local branch",
			stub:    stubCommandContext(t, `git rev-parse --verify refs/heads/trunk`, 0, "", ""),
			wantOut: true,
		},
		{
			name:    "does not have local branch",
			stub:    stubCommandContext(t, `git rev-parse --verify refs/heads/trunk`, 1, "", ""),
			wantOut: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{
				GitPath:        "path/to/git",
				commandContext: tt.stub,
			}
			out := client.HasLocalBranch(context.Background(), "trunk")
			assert.Equal(t, out, tt.wantOut)
		})
	}
}

func TestClientCheckoutBranch(t *testing.T) {
	tests := []struct {
		name         string
		stub         commandCtx
		wantErrorMsg string
	}{
		{
			name: "checkout branch",
			stub: stubCommandContext(t, `git checkout trunk`, 0, "", ""),
		},
		{
			name:         "git error",
			stub:         stubCommandContext(t, `git checkout trunk`, 1, "", "git error message"),
			wantErrorMsg: "failed to run git: git error message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{
				GitPath:        "path/to/git",
				commandContext: tt.stub,
			}
			err := client.CheckoutBranch(context.Background(), "trunk")
			if tt.wantErrorMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErrorMsg)
			}
		})
	}
}

func TestClientCheckoutNewBranch(t *testing.T) {
	tests := []struct {
		name         string
		stub         commandCtx
		wantErrorMsg string
	}{
		{
			name: "checkout new branch",
			stub: stubCommandContext(t, `git checkout -b trunk --track origin`, 0, "", ""),
		},
		{
			name:         "git error",
			stub:         stubCommandContext(t, `git checkout -b trunk --track origin`, 1, "", "git error message"),
			wantErrorMsg: "failed to run git: git error message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{
				GitPath:        "path/to/git",
				commandContext: tt.stub,
			}
			err := client.CheckoutNewBranch(context.Background(), "origin", "trunk")
			if tt.wantErrorMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErrorMsg)
			}
		})
	}
}

func TestClientToplevelDir(t *testing.T) {
	tests := []struct {
		name         string
		stub         commandCtx
		wantDir      string
		wantErrorMsg string
	}{
		{
			name:    "top level dir",
			stub:    stubCommandContext(t, `git rev-parse --show-toplevel`, 0, "/path/to/repo", ""),
			wantDir: "/path/to/repo",
		},
		{
			name:         "git error",
			stub:         stubCommandContext(t, `git rev-parse --show-toplevel`, 1, "", "git error message"),
			wantErrorMsg: "failed to run git: git error message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{
				GitPath:        "path/to/git",
				commandContext: tt.stub,
			}
			dir, err := client.ToplevelDir(context.Background())
			if tt.wantErrorMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErrorMsg)
			}
			assert.Equal(t, tt.wantDir, dir)
		})
	}
}

func TestClientGitDir(t *testing.T) {
	tests := []struct {
		name         string
		stub         commandCtx
		wantDir      string
		wantErrorMsg string
	}{
		{
			name:    "git dir",
			stub:    stubCommandContext(t, `git rev-parse --git-dir`, 0, "/path/to/repo/.git", ""),
			wantDir: "/path/to/repo/.git",
		},
		{
			name:         "git error",
			stub:         stubCommandContext(t, `git rev-parse --git-dir`, 1, "", "git error message"),
			wantErrorMsg: "failed to run git: git error message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{
				GitPath:        "path/to/git",
				commandContext: tt.stub,
			}
			dir, err := client.GitDir(context.Background())
			if tt.wantErrorMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErrorMsg)
			}
			assert.Equal(t, tt.wantDir, dir)
		})
	}
}

func TestClientPathFromRoot(t *testing.T) {
	tests := []struct {
		name    string
		stub    commandCtx
		wantDir string
	}{
		{
			name:    "current path from root",
			stub:    stubCommandContext(t, `git rev-parse --show-prefix`, 0, "some/path/", ""),
			wantDir: "some/path",
		},
		{
			name:    "git error",
			stub:    stubCommandContext(t, `git rev-parse --show-prefix`, 1, "", "git error message"),
			wantDir: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{
				GitPath:        "path/to/git",
				commandContext: tt.stub,
			}
			dir := client.PathFromRoot(context.Background())
			assert.Equal(t, tt.wantDir, dir)
		})
	}
}

func TestClientFetch(t *testing.T) {
	tests := []struct {
		name         string
		mods         []CommandModifier
		stub         commandCtx
		wantErrorMsg string
	}{
		{
			name: "fetch",
			stub: stubCommandContext(t, `git fetch origin trunk`, 0, "", ""),
		},
		{
			name: "accepts command modifiers",
			mods: []CommandModifier{WithRepoDir("/path/to/repo")},
			stub: stubCommandContext(t, `git fetch origin trunk`, 0, "", ""),
		},
		{
			name:         "git error",
			stub:         stubCommandContext(t, `git fetch origin trunk`, 1, "", "git error message"),
			wantErrorMsg: "failed to run git: git error message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{
				GitPath:        "path/to/git",
				commandContext: tt.stub,
			}
			err := client.Fetch(context.Background(), "origin", "trunk", tt.mods...)
			if tt.wantErrorMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErrorMsg)
			}
		})
	}
}

func TestClientPull(t *testing.T) {
	tests := []struct {
		name         string
		mods         []CommandModifier
		stub         commandCtx
		wantErrorMsg string
	}{
		{
			name: "pull",
			stub: stubCommandContext(t, `git pull --ff-only origin trunk`, 0, "", ""),
		},
		{
			name: "accepts command modifiers",
			mods: []CommandModifier{WithRepoDir("/path/to/repo")},
			stub: stubCommandContext(t, `git pull --ff-only origin trunk`, 0, "", ""),
		},
		{
			name:         "git error",
			stub:         stubCommandContext(t, `git pull --ff-only origin trunk`, 1, "", "git error message"),
			wantErrorMsg: "failed to run git: git error message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{
				GitPath:        "path/to/git",
				commandContext: tt.stub,
			}
			err := client.Pull(context.Background(), "origin", "trunk", tt.mods...)
			if tt.wantErrorMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErrorMsg)
			}
		})
	}
}

func TestClientPush(t *testing.T) {
	tests := []struct {
		name         string
		mods         []CommandModifier
		stub         commandCtx
		wantErrorMsg string
	}{
		{
			name: "push",
			stub: stubCommandContext(t, `git push --set-upstream origin trunk`, 0, "", ""),
		},
		{
			name: "accepts command modifiers",
			mods: []CommandModifier{WithRepoDir("/path/to/repo")},
			stub: stubCommandContext(t, `git push --set-upstream origin trunk`, 0, "", ""),
		},
		{
			name:         "git error",
			stub:         stubCommandContext(t, `git push --set-upstream origin trunk`, 1, "", "git error message"),
			wantErrorMsg: "failed to run git: git error message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{
				GitPath:        "path/to/git",
				commandContext: tt.stub,
			}
			err := client.Push(context.Background(), "origin", "trunk", tt.mods...)
			if tt.wantErrorMsg == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantErrorMsg)
			}
		})
	}
}

func TestClientClone(t *testing.T) {
	tests := []struct {
		name         string
		mods         []CommandModifier
		stub         commandCtx
		wantTarget   string
		wantErrorMsg string
	}{
		{
			name:       "clone",
			stub:       stubCommandContext(t, `git clone github.com/cli/cli`, 0, "", ""),
			wantTarget: "cli",
		},
		{
			name:       "accepts command modifiers",
			mods:       []CommandModifier{WithRepoDir("/path/to/repo")},
			stub:       stubCommandContext(t, `git clone github.com/cli/cli`, 0, "", ""),
			wantTarget: "cli",
		},
		{
			name:         "git error",
			stub:         stubCommandContext(t, `git clone github.com/cli/cli`, 1, "", "git error message"),
			wantErrorMsg: "failed to run git: git error message",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{
				GitPath:        "path/to/git",
				commandContext: tt.stub,
			}
			target, err := client.Clone(context.Background(), "github.com/cli/cli", []string{}, tt.mods...)
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
		title    string
		name     string
		url      string
		dir      string
		branches []string
		want     string
	}{
		{
			title:    "fetch all",
			name:     "test",
			url:      "URL",
			dir:      "DIRECTORY",
			branches: []string{},
			want:     `git -C DIRECTORY remote add -f test URL`,
		},
		{
			title:    "fetch specific branches only",
			name:     "test",
			url:      "URL",
			dir:      "DIRECTORY",
			branches: []string{"trunk", "dev"},
			want:     `git -C DIRECTORY remote add -t trunk -t dev -f test URL`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			client := Client{
				GitPath:        "path/to/git",
				RepoDir:        tt.dir,
				commandContext: stubCommandContext(t, tt.want, 0, "", ""),
			}
			_, err := client.AddRemote(context.Background(), tt.name, tt.url, tt.branches)
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

func stubCommandContext(t *testing.T, pattern string, exitStatus int, stdout, stderr string) commandCtx {
	return func(ctx context.Context, exe string, args ...string) *exec.Cmd {
		p := strings.Join(append([]string{exe}, args...), " ")
		require.Regexp(t, pattern, p)
		args = append([]string{os.Args[0], "-test.run=TestHelperProcess", "--", exe}, args...)
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		stdoutEnv := fmt.Sprintf("GH_HELPER_PROCESS_STDOUT=%s", stdout)
		stderrEnv := fmt.Sprintf("GH_HELPER_PROCESS_STDERR=%s", stderr)
		exitStatusEnv := fmt.Sprintf("GH_HELPER_PROCESS_EXIT_STATUS=%v", exitStatus)
		cmd.Env = []string{
			"GH_WANT_HELPER_PROCESS=1",
			stdoutEnv,
			stderrEnv,
			exitStatusEnv,
		}
		return cmd
	}
}
