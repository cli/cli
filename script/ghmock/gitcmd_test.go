package main

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRepoArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want repoOptions
		err  string
	}{
		{name: "defaults", args: []string{"octo/hello"}, want: repoOptions{name: "octo/hello", branch: "main"}},
		{name: "synthetic namespace", args: []string{"example.invalid/hello"}, want: repoOptions{name: "example.invalid/hello", branch: "main"}},
		{name: "host and branch", args: []string{"example.com/octo/hello.git", "--branch", "topic/one"}, want: repoOptions{name: "example.com/octo/hello.git", branch: "topic/one"}},
		{name: "missing name", err: `invalid repository ""; use [HOST/]OWNER/REPO`},
		{name: "path traversal", args: []string{"octo/.."}, err: `invalid repository "octo/.."; use [HOST/]OWNER/REPO`},
		{name: "absolute path", args: []string{"/octo/hello"}, err: `invalid repository "/octo/hello"; use [HOST/]OWNER/REPO`},
		{name: "URL is not a name", args: []string{"https://github.com/octo/hello"}, err: `invalid repository "https://github.com/octo/hello"; use [HOST/]OWNER/REPO`},
		{name: "empty branch", args: []string{"octo/hello", "--branch", ""}, err: "--branch cannot be empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRepoArgs(tt.args)
			if tt.err != "" {
				require.EqualError(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCmdRepo(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		duplicate bool
		host      string
		branch    string
		err       string
		relative  bool
		repo      string
	}{
		{name: "empty remote", args: []string{"octo/hello"}, host: dotcomHost, branch: "main"},
		{name: "synthetic namespace", args: []string{"example.invalid/hello"}, host: dotcomHost, branch: "main", repo: "example.invalid/hello"},
		{name: "relative state directory", args: []string{"octo/hello"}, host: dotcomHost, branch: "main", relative: true},
		{name: "custom host and branch", args: []string{"example.com/octo/hello", "--branch", "topic/one"}, host: "example.com", branch: "topic/one"},
		{name: "never overwrites", args: []string{"octo/hello"}, duplicate: true, err: "already exists"},
		{name: "invalid Git branch", args: []string{"octo/hello", "--branch", "bad..ref"}, err: `invalid branch "bad..ref"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.relative {
				t.Chdir(dir)
				dir = "state"
			}
			t.Setenv("GHMOCK_DIR", dir)
			if tt.duplicate {
				require.NoError(t, cmdRepo(tt.args, io.Discard))
			}
			var out bytes.Buffer
			err := cmdRepo(tt.args, &out)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			repo := tt.repo
			if repo == "" {
				repo = "octo/hello"
			}
			assert.Equal(t, fmt.Sprintf("Created %s/%s (branch %s)\nHTTPS: https://%s/%s.git\nSSH: git@%s:%s.git\n", tt.host, repo, tt.branch, tt.host, repo, tt.host, repo), out.String())
			st, err := openState()
			require.NoError(t, err)
			head, err := os.ReadFile(filepath.Join(gitRepoPath(st.gitFixtureDir(), tt.host, repo), "HEAD"))
			require.NoError(t, err)
			assert.Equal(t, "ref: refs/heads/"+tt.branch+"\n", string(head))
		})
	}
}

func TestParseWriteArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want writeOptions
		err  string
	}{
		{name: "contents", args: []string{"main/a.txt", "--text", "one\n"}, want: writeOptions{path: "main/a.txt", text: "one\n"}},
		{name: "empty file", args: []string{"empty", "--text", ""}, want: writeOptions{path: "empty"}},
		{name: "missing text", args: []string{"empty"}, err: "pass --text with the file contents"},
		{name: "outside workspace", args: []string{"../escape", "--text", "x"}, err: "pass a file path relative to the Git workspace"},
		{name: "missing path", args: []string{"--text", "x"}, err: "pass a file path relative to the Git workspace"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseWriteArgs(tt.args)
			if tt.err != "" {
				require.EqualError(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCmdWrite(t *testing.T) {
	tests := []struct {
		name string
		old  string
		text string
		link bool
	}{
		{name: "new directories and file", text: "one\n"},
		{name: "replace contents", old: "one\n", text: "two\n"},
		{name: "empty file", old: "one\n"},
		{name: "refuse an outside symlink", text: "changed", link: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GHMOCK_DIR", t.TempDir())
			st, err := openState()
			require.NoError(t, err)
			world, err := st.ensureGitFixture()
			require.NoError(t, err)
			path := filepath.Join(world, "work", "main", "a.txt")
			if tt.old != "" {
				require.NoError(t, cmdWrite([]string{"main/a.txt", "--text", tt.old}, io.Discard))
			}
			if tt.link {
				require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(world, "work", "main")))
			}
			var out bytes.Buffer
			err = cmdWrite([]string{"main/a.txt", "--text", tt.text}, &out)
			if tt.link {
				require.Error(t, err)
				assert.Empty(t, out.String())
				return
			}
			require.NoError(t, err)
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, tt.text, string(data))
			assert.Equal(t, fmt.Sprintf("Wrote main/a.txt (%d bytes)\n", len(tt.text)), out.String())
		})
	}
}

func TestParseGitArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want gitOptions
		err  string
	}{
		{name: "setup", args: []string{"--", "init", "main"}, want: gitOptions{build: 1, timeout: time.Minute, args: []string{"init", "main"}}},
		{name: "inspect result", args: []string{"--run", "latest", "--build", "2", "--in", "feature", "--", "status"}, want: gitOptions{run: "latest", build: 2, in: "feature", timeout: time.Minute, args: []string{"status"}}},
		{name: "inspect baseline", args: []string{"--run", "id", "--before", "--", "status"}, want: gitOptions{run: "id", before: true, build: 1, timeout: time.Minute, args: []string{"status"}}},
		{name: "stdin", args: []string{"--stdin", "x", "--", "hash-object", "--stdin"}, want: gitOptions{build: 1, stdin: "x", timeout: time.Minute, args: []string{"hash-object", "--stdin"}}},
		{name: "missing command", err: "missing git arguments; put them after --"},
		{name: "nonpositive build", args: []string{"--build", "0", "--", "status"}, err: "--build must be positive"},
		{name: "build without run", args: []string{"--build", "2", "--", "status"}, err: "--build requires --run"},
		{name: "before without run", args: []string{"--before", "--", "status"}, err: "--before requires --run"},
		{name: "explicit default build without run", args: []string{"--build", "1", "--", "status"}, err: "--build requires --run"},
		{name: "empty run", args: []string{"--run", "", "--", "status"}, err: "--run cannot be empty"},
		{name: "before and build", args: []string{"--run", "latest", "--before", "--build", "1", "--", "status"}, err: "--before and --build cannot be used together"},
		{name: "outside workspace", args: []string{"--in", "../other", "--", "status"}, err: "--in must be a relative path inside the Git workspace"},
		{name: "nonpositive timeout", args: []string{"--timeout", "0s", "--", "status"}, err: "--timeout must be positive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseGitArgs(tt.args)
			if tt.err != "" {
				require.EqualError(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCmdGit(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T, *state)
	}{
		{name: "shared clones remain usable after the fixture is copied", run: func(t *testing.T, st *state) {
			prepareGitRepo(t, "https://github.com/octo/hello.git")
			gitForTest(t, "--", "clone", "--quiet", "--shared", "main", "shared")
			copied := filepath.Join(t.TempDir(), "state")
			destination := (&state{dir: copied}).gitFixtureDir()
			require.NoError(t, copyGitTree(st.gitFixtureDir(), destination))
			require.NoError(t, os.Rename(st.gitFixtureDir(), st.gitFixtureDir()+"-moved"))
			t.Setenv("GHMOCK_DIR", copied)
			assert.Equal(t, "one\n", gitForTest(t, "--in", "shared", "--", "show", "HEAD:example.txt"))
			data, err := os.ReadFile(filepath.Join(destination, "work", "shared", ".git", "objects", "info", "alternates"))
			require.NoError(t, err)
			assert.False(t, filepath.IsAbs(strings.TrimSpace(string(data))))
		}},
		{name: "SSH inherits an extended command deadline", run: func(t *testing.T, _ *state) {
			if runtime.GOOS == "windows" {
				t.Skip("uses a Unix test wrapper")
			}
			prepareGitRepo(t, "git@github.com:octo/hello.git")
			gitPath, err := exec.LookPath("git")
			require.NoError(t, err)
			bin := t.TempDir()
			captured := filepath.Join(bin, "deadline")
			wrapper := "#!/bin/sh\nif [ \"$1\" = upload-pack ]; then printf '%s' \"$GHMOCK_GIT_DEADLINE\" > " + shellQuote(captured) + "; fi\nexec " + shellQuote(gitPath) + " \"$@\"\n"
			require.NoError(t, os.WriteFile(filepath.Join(bin, "git"), []byte(wrapper), 0o755))
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("GIT_EXEC_PATH", bin)
			start := time.Now()
			gitForTest(t, "--timeout", "2m", "--", "ls-remote", "git@github.com:octo/hello.git")
			data, err := os.ReadFile(captured)
			require.NoError(t, err)
			deadline, err := time.Parse(time.RFC3339Nano, string(data))
			require.NoError(t, err)
			assert.WithinDuration(t, start.Add(2*time.Minute), deadline, 5*time.Second)
		}},
		{name: "SSH requests remain recorded when their helper is cancelled", run: func(t *testing.T, st *state) {
			if runtime.GOOS == "windows" {
				t.Skip("uses a Unix test wrapper")
			}
			prepareGitRepo(t, "git@github.com:octo/hello.git")
			gitPath, err := exec.LookPath("git")
			require.NoError(t, err)
			bin := t.TempDir()
			wrapper := "#!/bin/sh\nif [ \"$1\" = upload-pack ]; then sleep 15; fi\nexec " + shellQuote(gitPath) + " \"$@\"\n"
			require.NoError(t, os.WriteFile(filepath.Join(bin, "git"), []byte(wrapper), 0o755))
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("GIT_EXEC_PATH", bin)
			err = cmdGit([]string{"--timeout", "500ms", "--", "ls-remote", "git@github.com:octo/hello.git"}, io.Discard, io.Discard)
			require.ErrorContains(t, err, "git stopped after")
			records, err := readJSONL[gitCommandRecord](filepath.Join(st.gitFixtureDir(), "commands.jsonl"))
			require.NoError(t, err)
			requests := records[len(records)-1].Requests
			require.Len(t, requests, 1)
			assert.Equal(t, "SSH", requests[0].Method)
			assert.Equal(t, "git-upload-pack", requests[0].Git.Service)
			assert.NotZero(t, requests[0].Git.ExitCode)
		}},
		{name: "synthetic namespace uses real Git transport", run: func(t *testing.T, st *state) {
			require.NoError(t, cmdRepo([]string{"example.invalid/hello"}, io.Discard))
			gitForTest(t, "--", "clone", "--quiet", "https://github.com/example.invalid/hello.git", "main")
			assert.Equal(t, "https://github.com/example.invalid/hello.git\n", gitForTest(t, "--in", "main", "--", "remote", "get-url", "origin"))
		}},
		{name: "HTTPS clone fetch and push transfer real commits", run: func(t *testing.T, st *state) {
			prepareGitRepo(t, "https://github.com/octo/hello.git")
			gitForTest(t, "--", "clone", "--quiet", "https://github.com/octo/hello.git", "other")
			require.NoError(t, cmdWrite([]string{"main/example.txt", "--text", "two\n"}, io.Discard))
			gitForTest(t, "--in", "main", "--", "commit", "--quiet", "-am", "Second")
			gitForTest(t, "--in", "main", "--", "push", "--quiet", "origin", "main")
			gitForTest(t, "--in", "other", "--", "pull", "--quiet", "--ff-only")
			assert.Equal(t, "two\n", gitForTest(t, "--in", "other", "--", "show", "HEAD:example.txt"))
			assert.Equal(t, gitForTest(t, "--in", "main", "--", "rev-parse", "HEAD"), gitForTest(t, "--in", "other", "--", "rev-parse", "HEAD"))
			records, err := readJSONL[gitCommandRecord](filepath.Join(st.gitFixtureDir(), "commands.jsonl"))
			require.NoError(t, err)
			var services []string
			for _, rec := range records {
				for _, req := range rec.Requests {
					require.NotNil(t, req.Git)
					assert.Empty(t, req.Git.Error)
					assert.Empty(t, req.Body, "binary packs must not be saved as JSON bodies")
					assert.Empty(t, req.SchemaProblems)
					services = append(services, req.Git.Service)
				}
			}
			assert.Contains(t, services, "git-receive-pack")
			assert.Contains(t, services, "git-upload-pack")
		}},
		{name: "SSH uses local packs and preserves the remote URL", run: func(t *testing.T, st *state) {
			prepareGitRepo(t, "git@github.com:octo/hello.git")
			gitForTest(t, "--", "clone", "--quiet", "ssh://git@github.com/octo/hello.git", "other")
			assert.Equal(t, "one\n", gitForTest(t, "--in", "other", "--", "show", "HEAD:example.txt"))
			assert.Equal(t, "git@github.com:octo/hello.git\n", gitForTest(t, "--in", "main", "--", "remote", "get-url", "origin"))
			records, err := readJSONL[gitCommandRecord](filepath.Join(st.gitFixtureDir(), "commands.jsonl"))
			require.NoError(t, err)
			var methods []string
			for _, rec := range records {
				for _, req := range rec.Requests {
					methods = append(methods, req.Method)
					require.NotNil(t, req.Git)
					assert.Zero(t, req.Git.ExitCode)
				}
			}
			assert.Equal(t, []string{"SSH", "SSH", "SSH"}, methods)
		}},
		{name: "plain HTTP localhost aliases the dotcom fixture", run: func(t *testing.T, _ *state) {
			prepareGitRepo(t, "http://github.localhost/octo/hello.git")
			assert.Equal(t, "one\n", gitForTest(t, "--in", "main", "--", "show", "HEAD:example.txt"))
		}},
		{name: "Git identity and configuration are isolated", run: func(t *testing.T, _ *state) {
			t.Setenv("GIT_AUTHOR_NAME", "caller")
			t.Setenv("GIT_CONFIG_COUNT", "1")
			t.Setenv("GIT_CONFIG_KEY_0", "user.name")
			t.Setenv("GIT_CONFIG_VALUE_0", "caller")
			prepareGitRepo(t, "https://github.com/octo/hello.git")
			assert.Equal(t, "monalisa:monalisa@example.invalid:946684800\n", gitForTest(t, "--in", "main", "--", "log", "-1", "--format=%an:%ae:%at"))
		}},
		{name: "stdin is passed unchanged", run: func(t *testing.T, _ *state) {
			hash := sha1.Sum([]byte("blob 7\x00sample\n"))
			assert.Equal(t, fmt.Sprintf("%x\n", hash), gitForTest(t, "--stdin", "sample\n", "--", "hash-object", "--stdin"))
		}},
		{name: "unknown repository fails without contacting GitHub", run: func(t *testing.T, _ *state) {
			var out, diagnostic bytes.Buffer
			err := cmdGit([]string{"--", "ls-remote", "https://github.com/octo/missing.git"}, &out, &diagnostic)
			require.Error(t, err)
			assert.Contains(t, diagnostic.String(), "no Git repository github.com/octo/missing")
			assert.Empty(t, out.String())
		}},
		{name: "unknown run cannot inspect the fixture instead", run: func(t *testing.T, _ *state) {
			err := cmdGit([]string{"--run", "latest", "--", "status"}, io.Discard, io.Discard)
			require.EqualError(t, err, `no saved run "latest"`)
		}},
		{name: "timeout stops Git and its child process", run: func(t *testing.T, _ *state) {
			start := time.Now()
			err := cmdGit([]string{"--timeout", "200ms", "--", "-c", "alias.slow=!sleep 15", "slow"}, io.Discard, io.Discard)
			require.ErrorContains(t, err, "git stopped after 200ms")
			assert.Less(t, time.Since(start), 5*time.Second)
		}},
		{name: "pack larger than the JSON request limit is not truncated", run: func(t *testing.T, _ *state) {
			prepareGitRepo(t, "https://github.com/octo/hello.git")
			data := make([]byte, maxRequestBody+1<<20)
			rng := rand.New(rand.NewPCG(1, 2))
			for i := range data {
				data[i] = byte(rng.Uint32())
			}
			require.NoError(t, cmdWrite([]string{"main/large.bin", "--text", string(data)}, io.Discard))
			gitForTest(t, "--in", "main", "--", "add", "large.bin")
			gitForTest(t, "--in", "main", "--", "commit", "--quiet", "-m", "Large object")
			gitForTest(t, "--in", "main", "--", "push", "--quiet", "origin", "main")
			gitForTest(t, "--", "clone", "--quiet", "https://github.com/octo/hello.git", "other")
			assert.Equal(t, string(data), gitForTest(t, "--in", "other", "--", "show", "HEAD:large.bin"))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GHMOCK_DIR", t.TempDir())
			st, err := openState()
			require.NoError(t, err)
			tt.run(t, st)
		})
	}
}

func gitForTest(t *testing.T, args ...string) string {
	t.Helper()
	var out, diagnostic bytes.Buffer
	err := cmdGit(args, &out, &diagnostic)
	require.NoError(t, err, "ghmock git %s\n%s", strings.Join(args, " "), diagnostic.String())
	return out.String()
}

func prepareGitRepo(t *testing.T, remote string) {
	t.Helper()
	require.NoError(t, cmdRepo([]string{"octo/hello"}, io.Discard))
	gitForTest(t, "--", "clone", "--quiet", remote, "main")
	require.NoError(t, cmdWrite([]string{"main/example.txt", "--text", "one\n"}, io.Discard))
	gitForTest(t, "--in", "main", "--", "add", "example.txt")
	gitForTest(t, "--in", "main", "--", "commit", "--quiet", "-m", "First")
	gitForTest(t, "--in", "main", "--", "push", "--quiet", "origin", "main")
}
