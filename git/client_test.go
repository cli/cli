package git

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/internal/run"
	"github.com/stretchr/testify/assert"
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
			wantArgs: []string{"git", "-c", "credential.helper=", "-c", "credential.helper=!\"path/to/gh\" auth git-credential", "fetch"},
		},
		{
			name:     "fallback when GhPath is not set",
			wantArgs: []string{"git", "-c", "credential.helper=", "-c", "credential.helper=!\"gh\" auth git-credential", "fetch"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := Client{
				GhPath:  tt.path,
				GitPath: "git",
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

func TestClientRemotesNoResolvedRemote(t *testing.T) {
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
			cs, restore := run.Stub()
			defer restore(t)
			cs.Register(`git status --porcelain`, 0, tt.output)
			client := Client{}
			ucc, err := client.UncommittedChangeCount(context.Background())
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, ucc)
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
			cs, teardown := run.Stub()
			defer teardown(t)
			cs.Register(`git symbolic-ref --quiet HEAD`, 0, tt.stub)
			client := Client{}
			branch, err := client.CurrentBranch(context.Background())
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, branch)
		})
	}
}

func TestClientCurrentBranch_detached_head(t *testing.T) {
	cs, teardown := run.Stub()
	defer teardown(t)
	cs.Register(`git symbolic-ref --quiet HEAD`, 1, "")
	client := Client{}
	_, err := client.CurrentBranch(context.Background())
	assert.EqualError(t, err, "failed to run git: not on any branch")
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
			want:     "git -C DIRECTORY remote add -f test URL",
		},
		{
			title:    "fetch specific branches only",
			name:     "test",
			url:      "URL",
			dir:      "DIRECTORY",
			branches: []string{"trunk", "dev"},
			want:     "git -C DIRECTORY remote add -t trunk -t dev -f test URL",
		},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			cs, cmdTeardown := run.Stub()
			defer cmdTeardown(t)
			cs.Register(tt.want, 0, "")
			client := Client{
				RepoDir: tt.dir,
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
	err = cmd.Run()
	assert.NoError(t, err)
}
