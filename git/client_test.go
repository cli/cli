package git

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
	rs, err := client.Remotes()
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

// func TestLastCommit(t *testing.T) {
// 	setGitDir(t, "./fixtures/simple.git")
// 	c, err := LastCommit()
// 	if err != nil {
// 		t.Fatalf("LastCommit error: %v", err)
// 	}
// 	if c.Sha != "6f1a2405cace1633d89a79c74c65f22fe78f9659" {
// 		t.Errorf("expected sha %q, got %q", "6f1a2405cace1633d89a79c74c65f22fe78f9659", c.Sha)
// 	}
// 	if c.Title != "Second commit" {
// 		t.Errorf("expected title %q, got %q", "Second commit", c.Title)
// 	}
// }
//
// func TestCommitBody(t *testing.T) {
// 	setGitDir(t, "./fixtures/simple.git")
// 	body, err := CommitBody("6f1a2405cace1633d89a79c74c65f22fe78f9659")
// 	if err != nil {
// 		t.Fatalf("CommitBody error: %v", err)
// 	}
// 	if body != "I'm starting to get the hang of things\n" {
// 		t.Errorf("expected %q, got %q", "I'm starting to get the hang of things\n", body)
// 	}
// }
//
// /*
// 	NOTE: below this are stubbed git tests, i.e. those that do not actually invoke `git`. If possible, utilize
// 	`setGitDir()` to allow new tests to interact with `git`. For write operations, you can use `t.TempDir()` to
// 	host a temporary git repository that is safe to be changed.
// */
//
// func Test_UncommittedChangeCount(t *testing.T) {
// 	type c struct {
// 		Label    string
// 		Expected int
// 		Output   string
// 	}
// 	cases := []c{
// 		{Label: "no changes", Expected: 0, Output: ""},
// 		{Label: "one change", Expected: 1, Output: " M poem.txt"},
// 		{Label: "untracked file", Expected: 2, Output: " M poem.txt\n?? new.txt"},
// 	}
//
// 	for _, v := range cases {
// 		t.Run(v.Label, func(t *testing.T) {
// 			cs, restore := run.Stub()
// 			defer restore(t)
// 			cs.Register(`git status --porcelain`, 0, v.Output)
//
// 			ucc, _ := UncommittedChangeCount()
// 			if ucc != v.Expected {
// 				t.Errorf("UncommittedChangeCount() = %d, expected %d", ucc, v.Expected)
// 			}
// 		})
// 	}
// }
//
// func Test_CurrentBranch(t *testing.T) {
// 	type c struct {
// 		Stub     string
// 		Expected string
// 	}
// 	cases := []c{
// 		{
// 			Stub:     "branch-name\n",
// 			Expected: "branch-name",
// 		},
// 		{
// 			Stub:     "refs/heads/branch-name\n",
// 			Expected: "branch-name",
// 		},
// 		{
// 			Stub:     "refs/heads/branch\u00A0with\u00A0non\u00A0breaking\u00A0space\n",
// 			Expected: "branch\u00A0with\u00A0non\u00A0breaking\u00A0space",
// 		},
// 	}
//
// 	for _, v := range cases {
// 		cs, teardown := run.Stub()
// 		cs.Register(`git symbolic-ref --quiet HEAD`, 0, v.Stub)
//
// 		result, err := CurrentBranch()
// 		if err != nil {
// 			t.Errorf("got unexpected error: %v", err)
// 		}
// 		if result != v.Expected {
// 			t.Errorf("unexpected branch name: %s instead of %s", result, v.Expected)
// 		}
// 		teardown(t)
// 	}
// }
//
// func Test_CurrentBranch_detached_head(t *testing.T) {
// 	cs, teardown := run.Stub()
// 	defer teardown(t)
// 	cs.Register(`git symbolic-ref --quiet HEAD`, 1, "")
//
// 	_, err := CurrentBranch()
// 	if err == nil {
// 		t.Fatal("expected an error, got nil")
// 	}
// 	if err != ErrNotOnAnyBranch {
// 		t.Errorf("got unexpected error: %s instead of %s", err, ErrNotOnAnyBranch)
// 	}
// }
//
// func TestParseExtraCloneArgs(t *testing.T) {
// 	type Wanted struct {
// 		args []string
// 		dir  string
// 	}
// 	tests := []struct {
// 		name string
// 		args []string
// 		want Wanted
// 	}{
// 		{
// 			name: "args and target",
// 			args: []string{"target_directory", "-o", "upstream", "--depth", "1"},
// 			want: Wanted{
// 				args: []string{"-o", "upstream", "--depth", "1"},
// 				dir:  "target_directory",
// 			},
// 		},
// 		{
// 			name: "only args",
// 			args: []string{"-o", "upstream", "--depth", "1"},
// 			want: Wanted{
// 				args: []string{"-o", "upstream", "--depth", "1"},
// 				dir:  "",
// 			},
// 		},
// 		{
// 			name: "only target",
// 			args: []string{"target_directory"},
// 			want: Wanted{
// 				args: []string{},
// 				dir:  "target_directory",
// 			},
// 		},
// 		{
// 			name: "no args",
// 			args: []string{},
// 			want: Wanted{
// 				args: []string{},
// 				dir:  "",
// 			},
// 		},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			args, dir := parseCloneArgs(tt.args)
// 			got := Wanted{
// 				args: args,
// 				dir:  dir,
// 			}
//
// 			if !reflect.DeepEqual(got, tt.want) {
// 				t.Errorf("got %#v want %#v", got, tt.want)
// 			}
// 		})
// 	}
// }
//
// func TestAddNamedRemote(t *testing.T) {
// 	tests := []struct {
// 		title    string
// 		name     string
// 		url      string
// 		dir      string
// 		branches []string
// 		want     string
// 	}{
// 		{
// 			title:    "fetch all",
// 			name:     "test",
// 			url:      "URL",
// 			dir:      "DIRECTORY",
// 			branches: []string{},
// 			want:     "git -C DIRECTORY remote add -f test URL",
// 		},
// 		{
// 			title:    "fetch specific branches only",
// 			name:     "test",
// 			url:      "URL",
// 			dir:      "DIRECTORY",
// 			branches: []string{"trunk", "dev"},
// 			want:     "git -C DIRECTORY remote add -t trunk -t dev -f test URL",
// 		},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.title, func(t *testing.T) {
// 			cs, cmdTeardown := run.Stub()
// 			defer cmdTeardown(t)
//
// 			cs.Register(tt.want, 0, "")
//
// 			err := AddNamedRemote(tt.url, tt.name, tt.dir, tt.branches)
// 			if err != nil {
// 				t.Fatalf("error running command `git remote add -f`: %v", err)
// 			}
// 		})
// 	}
// }

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
	cmd, err := client.Command([]string{"init", "--quiet"}...)
	assert.NoError(t, err)
	err = cmd.Run()
	assert.NoError(t, err)
}
