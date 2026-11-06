package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMain lets the test binary stand in for gh. runBinary starts gh with an
// environment built from scratch, so a small wrapper script sets
// GHMOCK_FAKE_GH before running this binary.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "_git-ssh" {
		os.Exit(runMain(os.Args[1:], os.Stdout, os.Stderr))
	}
	if os.Getenv("GHMOCK_FAKE_GH") == "1" {
		os.Exit(fakeGH(os.Args[1:]))
	}
	os.Exit(runTests(m))
}

// runTests keeps tests away from the network and the real cache folder. Tests
// that need schemas call useTestSchemas.
func runTests(m *testing.M) int {
	cache, err := os.MkdirTemp("", "ghmock-test-cache-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(cache)
	userCacheDir = func() (string, error) { return cache, nil }
	downloadClient = &http.Client{Transport: failingTransport{}}
	os.Unsetenv(graphQLSource.envVar)
	os.Unsetenv(restSource.envVar)
	return m.Run()
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("downloads are off in tests")
}

// useTestSchemas points ghmock at the small schemas in testdata.
func useTestSchemas(t *testing.T) {
	t.Helper()
	for env, file := range map[string]string{
		graphQLSource.envVar: "schema.graphql",
		restSource.envVar:    "rest.json",
	} {
		p, err := filepath.Abs(filepath.Join("testdata", file))
		require.NoError(t, err)
		t.Setenv(env, p)
	}
}

// testChecker returns a checker that uses the schemas in testdata.
func testChecker(t *testing.T) *checker {
	t.Helper()
	useTestSchemas(t)
	chk := newChecker(io.Discard)
	chk.preload()
	require.Empty(t, chk.unavailable())
	return chk
}

// fakeGH implements the few commands the run tests need.
func fakeGH(args []string) int {
	if len(args) == 0 {
		return 2
	}
	switch args[0] {
	case "--version":
		if os.Getenv("GHMOCK_FAKE_VARIANT") == "config-probe" {
			if err := os.WriteFile(filepath.Join(os.Getenv("GH_CONFIG_DIR"), "probe.txt"), []byte("version probe"), 0o600); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
		}
		fmt.Printf("gh version 0.0.0-%s\n", os.Getenv("GHMOCK_FAKE_VARIANT"))
	case "config":
		fmt.Printf("GH_CONFIG_DIR=%s\n", os.Getenv("GH_CONFIG_DIR"))
		for _, name := range []string{"config.yml", "hosts.yml", "probe.txt"} {
			data, err := os.ReadFile(filepath.Join(os.Getenv("GH_CONFIG_DIR"), name))
			if errors.Is(err, os.ErrNotExist) {
				fmt.Printf("%s=(none)\n", name)
				continue
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			fmt.Printf("%s=%q\n", name, data)
		}
		if len(args) > 1 {
			data := []byte("changed-by-" + os.Getenv("GHMOCK_FAKE_VARIANT") + "\n")
			if err := os.WriteFile(filepath.Join(os.Getenv("GH_CONFIG_DIR"), "hosts.yml"), data, 0o600); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			if args[1] == "mutate-fail" {
				return 7
			}
		}
	case "env":
		wd, _ := os.Getwd()
		for _, kv := range [][2]string{
			{"GH_HOST", os.Getenv("GH_HOST")},
			{"GH_TOKEN", os.Getenv("GH_TOKEN")},
			{"GITHUB_TOKEN", os.Getenv("GITHUB_TOKEN")},
			{"GH_FORCE_TTY", os.Getenv("GH_FORCE_TTY")},
			{"NO_COLOR", os.Getenv("NO_COLOR")},
			{"TERM", os.Getenv("TERM")},
			{"COLORTERM", os.Getenv("COLORTERM")},
			{"SSL_CERT_FILE", os.Getenv("SSL_CERT_FILE")},
			{"SSL_CERT_DIR", os.Getenv("SSL_CERT_DIR")},
			{"GIT_SSL_CAINFO", os.Getenv("GIT_SSL_CAINFO")},
			{"HOME", os.Getenv("HOME")},
			{"PWD", wd},
		} {
			fmt.Printf("%s=%s\n", kv[0], kv[1])
		}
		for _, name := range []string{"config.yml", "hosts.yml"} {
			path := filepath.Join(os.Getenv("GH_CONFIG_DIR"), name)
			fmt.Printf("--- %s\n", path)
			data, err := os.ReadFile(path)
			if err != nil {
				fmt.Println("(none)")
				continue
			}
			fmt.Print(string(data))
		}
	case "stdin":
		_, _ = io.Copy(os.Stdout, os.Stdin)
	case "terminal-utf8":
		data := []byte("\u754c")
		_, _ = os.Stdout.Write(data[:1])
		time.Sleep(100 * time.Millisecond)
		_, _ = os.Stdout.Write(data[1:])
		fmt.Println()
	case "terminal-color":
		code := "31"
		if os.Getenv("GHMOCK_FAKE_VARIANT") == "green" {
			code = "32"
		}
		fmt.Printf("\x1b[%smsame\x1b[0m\n", code)
	case "terminal-quiet":
		time.Sleep(2200 * time.Millisecond)
		fmt.Println("finished")
	case "terminal-prompt":
		if os.Getenv("GHMOCK_FAKE_VARIANT") == "early" {
			fmt.Println("finished")
			return 0
		}
		fmt.Print("? " + os.Getenv("GHMOCK_FAKE_VARIANT") + " ")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		fmt.Print("\x1b[2J\x1b[Hfinished\n")
	case "prompt":
		if fi, err := os.Stdin.Stat(); err != nil || fi.Mode()&os.ModeCharDevice == 0 {
			fmt.Fprintln(os.Stderr, "fake gh: not a terminal")
			return 1
		}
		in := bufio.NewReader(os.Stdin)
		var answers []string
		for _, question := range args[1:] {
			fmt.Printf("? %s ", question)
			line, err := in.ReadString('\n')
			if err != nil {
				return 1
			}
			answers = append(answers, strings.TrimSpace(line))
		}
		fmt.Printf("Answers: %s\n", strings.Join(answers, ", "))
	case "exit":
		code, _ := strconv.Atoi(args[1])
		fmt.Fprintln(os.Stderr, "failing on purpose")
		return code
	case "sleep":
		time.Sleep(30 * time.Second)
	case "git":
		cmd := exec.Command("git", args[1:]...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		return processExitCode(cmd.Run())
	case "push-change":
		if err := os.WriteFile("example.txt", []byte(os.Getenv("GHMOCK_FAKE_VARIANT")+"\n"), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		for _, args := range [][]string{{"commit", "--quiet", "-am", "Change"}, {"push", "--quiet", "origin", "main"}} {
			cmd := exec.Command("git", args...)
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return processExitCode(err)
			}
		}
	case "sync-worktree":
		if err := exec.Command("git", "fetch", "--quiet", "origin", "main").Run(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		gitArgs := []string{"update-ref", "refs/heads/main", "FETCH_HEAD"}
		if os.Getenv("GHMOCK_FAKE_VARIANT") == "fixed" {
			gitArgs = []string{"branch", "-f", "main", "FETCH_HEAD"}
		}
		if err := exec.Command("git", gitArgs...).Run(); err != nil {
			fmt.Fprintln(os.Stderr, "main belongs to another worktree")
			return 1
		}
	case "api":
		base := "https://api.github.com/"
		if os.Getenv("GH_HOST") == "github.localhost" {
			base = "http://api.github.localhost/"
		}
		return fakeGet(http.DefaultClient, base+strings.TrimPrefix(args[1], "/"))
	case "get":
		return fakeGet(http.DefaultClient, args[1])
	case "untrusted":
		client := &http.Client{Transport: &http.Transport{
			Proxy:           http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{RootCAs: x509.NewCertPool()},
		}}
		if _, err := client.Get(args[1]); err != nil {
			fmt.Fprintln(os.Stderr, "fake gh: TLS handshake failed")
			return 1
		}
	default:
		fmt.Fprintf(os.Stderr, "fake gh: unknown command %q\n", args[0])
		return 2
	}
	return 0
}

// fakeGet prints the status and body of a GET. It fails for an error or a
// status of 300 or more.
func fakeGet(client *http.Client, url string) int {
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("%d %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
	if resp.StatusCode >= 300 {
		return 1
	}
	return 0
}

// writeFakeGH writes a wrapper that runs the test binary as gh.
func writeFakeGH(t *testing.T, variant string) string {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "gh-"+variant)
	// Race-enabled binaries sleep a second before exiting by default, which
	// would look like gh waiting for input to a terminal run.
	script := fmt.Sprintf("#!/bin/sh\nGHMOCK_FAKE_GH=1 GHMOCK_FAKE_VARIANT=%q GORACE=atexit_sleep_ms=0 exec %q \"$@\"\n", variant, exe)
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}

func TestParseRunArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    runOptions
		wantErr string
	}{
		{
			name: "one binary and gh arguments after --",
			args: []string{"--gh", "./bin/gh", "--", "issue", "view", "1"},
			want: runOptions{binaries: []string{"./bin/gh"}, timeout: time.Minute, args: []string{"issue", "view", "1"}},
		},
		{
			name: "prepared worktree",
			args: []string{"--gh", "a", "--in", "feature", "--", "repo", "sync"},
			want: runOptions{binaries: []string{"a"}, in: "feature", timeout: time.Minute, args: []string{"repo", "sync"}},
		},
		{
			name: "configuration fixture",
			args: []string{"--gh", "a", "--config-dir", "fixtures/accounts", "--", "auth", "status"},
			want: runOptions{binaries: []string{"a"}, configDir: "fixtures/accounts", timeout: time.Minute, args: []string{"auth", "status"}},
		},
		{
			name:    "empty configuration fixture path",
			args:    []string{"--gh", "a", "--config-dir", "", "--", "auth", "status"},
			wantErr: "--config-dir needs a fixture directory",
		},
		{
			name:    "missing configuration fixture flag value",
			args:    []string{"--gh", "a", "--config-dir"},
			wantErr: "flag needs an argument: -config-dir",
		},
		{
			name:    "configuration fixture conflicts with default logged-out state",
			args:    []string{"--gh", "a", "--config-dir", "fixtures/accounts", "--logged-out", "--", "auth", "status"},
			wantErr: "--config-dir and --logged-out cannot be combined; use a fixture without hosts.yml for a logged-out run",
		},
		{
			name:    "outside fixture",
			args:    []string{"--gh", "a", "--in", "../other", "--", "repo", "sync"},
			wantErr: "--in must be a relative path inside the Git workspace",
		},
		{
			name: "every flag",
			args: []string{"--gh", "a", "--gh", "b", "--tty", "--logged-out", "--localhost", "--stdin", "token", "--timeout", "5s", "--", "auth", "login"},
			want: runOptions{
				binaries:  []string{"a", "b"},
				tty:       true,
				stdin:     "token",
				loggedOut: true,
				localhost: true,
				timeout:   5 * time.Second,
				args:      []string{"auth", "login"},
			},
		},
		{
			name: "--keys runs gh in a terminal",
			args: []string{"--gh", "a", "--keys", "my-repo{enter}", "--keys", "{down}{enter}", "--", "repo", "create"},
			want: runOptions{
				binaries: []string{"a"},
				terminal: true,
				keys:     []string{"my-repo{enter}", "{down}{enter}"},
				timeout:  time.Minute,
				args:     []string{"repo", "create"},
			},
		},
		{
			name:    "--terminal with --tty",
			args:    []string{"--gh", "a", "--terminal", "--tty", "--", "repo", "create"},
			wantErr: "--terminal and --keys already give gh a terminal; leave out --tty",
		},
		{
			name:    "--keys with --stdin",
			args:    []string{"--gh", "a", "--keys", "x", "--stdin", "y", "--", "repo", "create"},
			wantErr: "in a terminal, gh reads what you type; use --keys instead of --stdin",
		},
		{
			name:    "an unknown key",
			args:    []string{"--gh", "a", "--keys", "{nope}", "--", "repo", "create"},
			wantErr: `--keys "{nope}" names an unknown key {nope}; keys are {backspace} {ctrl-c} {ctrl-d} {down} {enter} {esc} {left} {right} {space} {tab} {up}`,
		},
		{
			name: "gh arguments without --",
			args: []string{"--gh", "a", "issue", "view", "-R", "octo/hello"},
			want: runOptions{binaries: []string{"a"}, timeout: time.Minute, args: []string{"issue", "view", "-R", "octo/hello"}},
		},
		{
			name: "gh flags after --",
			args: []string{"--gh", "a", "--", "--version"},
			want: runOptions{binaries: []string{"a"}, timeout: time.Minute, args: []string{"--version"}},
		},
		{
			name:    "no binary",
			args:    []string{"--", "issue", "list"},
			wantErr: "at least one --gh is required",
		},
		{
			name:    "no gh arguments",
			args:    []string{"--gh", "a"},
			wantErr: "missing gh arguments; put them after --",
		},
		{
			name:    "zero timeout",
			args:    []string{"--gh", "a", "--timeout", "0s", "--", "issue", "list"},
			wantErr: "--timeout must be positive",
		},
		{
			name:    "unknown flag",
			args:    []string{"--gh", "a", "--nope", "--", "issue", "list"},
			wantErr: "flag provided but not defined: -nope",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRunArgs(tt.args)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				var ue usageError
				assert.True(t, errors.As(err, &ue), "want a usage error")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRunAll(t *testing.T) {
	const fixtureConfig = "version: \"1\"\ngit_protocol: ssh\n"
	const fixtureHosts = "github.com:\n    user: alice\n    oauth_token: gho_ghmock-alice\n    users:\n        alice:\n            oauth_token: gho_ghmock-alice\n        bob:\n            oauth_token: gho_ghmock-bob\n"
	fixtureOutput := fmt.Sprintf("GH_CONFIG_DIR=<config>\nconfig.yml=%q\nhosts.yml=%q\nprobe.txt=(none)\n", fixtureConfig, fixtureHosts)

	// resultSummary is the part of a run result that tests compare.
	type resultSummary struct {
		Version    string
		ExitCode   int
		TimedOut   bool
		Stdout     string
		Stderr     string
		Requests   []logSummary
		Steps      []terminalStep
		UnusedKeys int
	}

	tests := []struct {
		name         string
		variants     []string
		responses    responseSet
		checkSchemas bool
		// builds sets buildsDir: "own" is the folder holding the fake gh,
		// and "other" is a different folder.
		builds          string
		opts            runOptions
		want            []resultSummary
		wantCommand     string
		wantErr         string
		wantErrContains string
		afterAPI        bool
		resolveOnly     bool
		setup           func(*testing.T, *state)
		check           func(*testing.T, *state, runRecord)
	}{
		{
			name:      "a request-free command replaces the last run selection",
			variants:  []string{"a"},
			responses: responseSet{"GET /user": {{Body: json.RawMessage(`{}`)}}},
			opts:      runOptions{args: []string{"--version"}},
			want:      []resultSummary{{Version: "gh version 0.0.0-a", Stdout: "gh version 0.0.0-a\n"}},
			afterAPI:  true,
		},
		{
			name:        "PATH executable lookup uses platform permission rules",
			resolveOnly: true,
		},
		{
			name:     "direct command display preserves shell quoting",
			variants: []string{"a"},
			opts:     runOptions{args: []string{"stdin", "two words", "it's", "", "$HOME"}, stdin: "observed\n"},
			wantCommand: `ghmock run --gh <gh-a> --stdin 'observed
' --timeout 30s -- stdin 'two words' 'it'\''s' '' '$HOME'`,
			want: []resultSummary{{Version: "gh version 0.0.0-a", Stdout: "observed\n"}},
			check: func(t *testing.T, st *state, rec runRecord) {
				assert.Contains(t, rec.Report, "$ gh stdin 'two words' 'it'\\''s' '' '$HOME'\n")
			},
		},
		{
			name:     "configuration is independent per build and preserved in exports",
			variants: []string{"a", "b"},
			opts:     runOptions{configDir: "<state>/fixture config", args: []string{"config", "mutate"}},
			setup: func(t *testing.T, st *state) {
				writeConfigFixture(t, filepath.Join(st.dir, "fixture config"), map[string][]byte{
					"config.yml":    []byte(fixtureConfig),
					"hosts.yml":     []byte(fixtureHosts),
					"nested/.extra": []byte("retained"),
					".git":          []byte("gitdir: /not/a/real/path\n"),
				})
			},
			wantCommand: "ghmock run --gh <gh-a> --gh <gh-b> --config-dir '<state>/fixture config' --timeout 30s -- config mutate",
			want: []resultSummary{
				{Version: "gh version 0.0.0-a", Stdout: fixtureOutput},
				{Version: "gh version 0.0.0-b", Stdout: fixtureOutput},
			},
			check: func(t *testing.T, st *state, rec runRecord) {
				read := func(root, path string) string {
					t.Helper()
					data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
					require.NoError(t, err)
					return string(data)
				}
				assert.Equal(t, fixtureHosts, read(st.dir, "fixture config/hosts.yml"))
				assert.Equal(t, fixtureHosts, read(st.dir, rec.ConfigBaseline+"/hosts.yml"))
				for i, result := range rec.Results {
					require.NotNil(t, result.Config)
					assert.Equal(t, fmt.Sprintf("changed-by-%c\n", 'a'+i), read(st.dir, result.Config.Directory+"/hosts.yml"))
					info, err := os.Stat(filepath.Join(st.dir, result.Config.Directory, "hosts.yml"))
					require.NoError(t, err)
					assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
				}
				t.Setenv("GHMOCK_DIR", st.dir)
				directory := filepath.Join(t.TempDir(), "export")
				require.NoError(t, cmdExport([]string{directory}, io.Discard))
				assert.Equal(t, fixtureHosts, read(directory, rec.ConfigBaseline+"/hosts.yml"))
				archive := filepath.Join(t.TempDir(), "export.tar.gz")
				require.NoError(t, cmdExport([]string{"--archive", archive}, io.Discard))
				restored := filepath.Join(t.TempDir(), "restored")
				extractTestArchive(t, archive, restored)
				assert.Equal(t, fixtureHosts, read(restored, rec.ConfigBaseline+"/hosts.yml"))
				assert.Equal(t, "changed-by-a\n", read(restored, rec.Results[0].Config.Directory+"/hosts.yml"))
				assert.Equal(t, "retained", read(restored, rec.Results[1].Config.Directory+"/nested/.extra"))
				assert.Equal(t, "gitdir: /not/a/real/path\n", read(restored, rec.Results[1].Config.Directory+"/.git"))
				require.NoError(t, cmdReset(nil, io.Discard))
				assert.NoDirExists(t, filepath.Join(st.dir, "config"))
				assert.Equal(t, fixtureHosts, read(st.dir, "fixture config/hosts.yml"))
			},
		},
		{
			name:     "empty fixture gets no defaults or version-probe mutations",
			variants: []string{"config-probe"},
			opts:     runOptions{configDir: "<state>/fixture", args: []string{"config"}},
			setup: func(t *testing.T, st *state) {
				writeConfigFixture(t, filepath.Join(st.dir, "fixture"), nil)
			},
			wantCommand: "ghmock run --gh <gh-config-probe> --config-dir <state>/fixture --timeout 30s -- config",
			want: []resultSummary{{
				Version: "gh version 0.0.0-config-probe",
				Stdout:  "GH_CONFIG_DIR=<config>\nconfig.yml=(none)\nhosts.yml=(none)\nprobe.txt=(none)\n",
			}},
			check: func(t *testing.T, st *state, rec runRecord) {
				require.NotNil(t, rec.Results[0].Config)
				entries, err := os.ReadDir(filepath.Join(st.dir, rec.Results[0].Config.Directory))
				require.NoError(t, err)
				assert.Empty(t, entries)
			},
		},
		{
			name:     "malformed fixture is passed to the command unchanged",
			variants: []string{"a"},
			opts:     runOptions{configDir: "<state>/fixture", args: []string{"config"}},
			setup: func(t *testing.T, st *state) {
				writeConfigFixture(t, filepath.Join(st.dir, "fixture"), map[string][]byte{"hosts.yml": []byte("not: [valid")})
			},
			wantCommand: "ghmock run --gh <gh-a> --config-dir <state>/fixture --timeout 30s -- config",
			want:        []resultSummary{{Version: "gh version 0.0.0-a", Stdout: "GH_CONFIG_DIR=<config>\nconfig.yml=(none)\nhosts.yml=\"not: [valid\"\nprobe.txt=(none)\n"}},
		},
		{
			name:     "failed command retains its configuration changes",
			variants: []string{"a"},
			opts:     runOptions{configDir: "<state>/fixture", args: []string{"config", "mutate-fail"}},
			setup: func(t *testing.T, st *state) {
				writeConfigFixture(t, filepath.Join(st.dir, "fixture"), nil)
			},
			wantCommand: "ghmock run --gh <gh-a> --config-dir <state>/fixture --timeout 30s -- config mutate-fail",
			want:        []resultSummary{{Version: "gh version 0.0.0-a", ExitCode: 7, Stdout: "GH_CONFIG_DIR=<config>\nconfig.yml=(none)\nhosts.yml=(none)\nprobe.txt=(none)\n"}},
			check: func(t *testing.T, st *state, rec runRecord) {
				require.NotNil(t, rec.Results[0].Config)
				data, err := os.ReadFile(filepath.Join(st.dir, rec.Results[0].Config.Directory, "hosts.yml"))
				require.NoError(t, err)
				assert.Equal(t, "changed-by-a\n", string(data))
			},
		},
		{
			name:            "missing configuration fixture fails before execution",
			variants:        []string{"a"},
			opts:            runOptions{configDir: "<state>/missing", args: []string{"config"}},
			wantErrContains: "no such file or directory",
		},
		{
			name:     "configuration fixture must be a directory",
			variants: []string{"a"},
			opts:     runOptions{configDir: "<state>/file", args: []string{"config"}},
			setup: func(t *testing.T, st *state) {
				require.NoError(t, os.WriteFile(filepath.Join(st.dir, "file"), []byte("not a directory"), 0o600))
			},
			wantErrContains: "not a directory",
		},
		{
			name:     "configuration fixture symlinks are rejected",
			variants: []string{"a"},
			opts:     runOptions{configDir: "<state>/fixture", args: []string{"config"}},
			setup: func(t *testing.T, st *state) {
				writeConfigFixture(t, filepath.Join(st.dir, "fixture"), nil)
				outside := filepath.Join(t.TempDir(), "hosts.yml")
				require.NoError(t, os.WriteFile(outside, []byte("outside fixture"), 0o600))
				require.NoError(t, os.Symlink(outside, filepath.Join(st.dir, "fixture", "hosts.yml")))
			},
			wantErrContains: "not a regular file or directory",
		},
		{
			name:            "configuration source cannot contain the state destination",
			variants:        []string{"a"},
			opts:            runOptions{configDir: "<state>", args: []string{"config"}},
			wantErrContains: "cannot copy configuration into its source",
		},
		{
			name:     "configuration special files are rejected",
			variants: []string{"a"},
			opts:     runOptions{configDir: "<state>/fixture", args: []string{"config"}},
			setup: func(t *testing.T, st *state) {
				// Unix socket names have a smaller limit than t.TempDir paths.
				fixture, err := os.MkdirTemp("", "ghmock-config-")
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, os.RemoveAll(fixture)) })
				listener, err := net.Listen("unix", filepath.Join(fixture, "socket"))
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, listener.Close()) })
				require.NoError(t, os.Symlink(fixture, filepath.Join(st.dir, "fixture")))
			},
			wantErrContains: "not a regular file or directory",
		},
		{
			name:     "aliased configuration destination does not modify the source",
			variants: []string{"a"},
			opts:     runOptions{configDir: "<state>/fixture", args: []string{"config"}},
			setup: func(t *testing.T, st *state) {
				fixture := filepath.Join(st.dir, "fixture")
				writeConfigFixture(t, fixture, nil)
				require.NoError(t, os.Symlink(fixture, filepath.Join(st.dir, "config")))
				t.Cleanup(func() {
					entries, err := os.ReadDir(fixture)
					require.NoError(t, err)
					assert.Empty(t, entries)
				})
			},
			wantErrContains: "cannot copy configuration into its source",
		},
		{
			name:     "worktree runs preserve the baseline and separate each build",
			variants: []string{"old", "fixed"},
			opts:     runOptions{in: "feature", args: []string{"sync-worktree"}},
			setup: func(t *testing.T, st *state) {
				t.Setenv("GHMOCK_DIR", st.dir)
				prepareGitRepo(t, "git@github.com:octo/hello.git")
				gitForTest(t, "--in", "main", "--", "worktree", "add", "--quiet", "../feature", "-b", "feature")
				gitForTest(t, "--", "clone", "--quiet", "git@github.com:octo/hello.git", "producer")
				require.NoError(t, cmdWrite([]string{"producer/example.txt", "--text", "two\n"}, io.Discard))
				gitForTest(t, "--in", "producer", "--", "commit", "--quiet", "-am", "Second")
				gitForTest(t, "--in", "producer", "--", "push", "--quiet", "origin", "main")
			},
			wantCommand: "ghmock run --gh <gh-old> --gh <gh-fixed> --in feature --timeout 30s -- sync-worktree",
			want: []resultSummary{
				{Version: "gh version 0.0.0-old", Requests: []logSummary{{Seq: 1, Name: "SSH github.com/octo/hello.git", Matched: true}}},
				{Version: "gh version 0.0.0-fixed", ExitCode: 1, Stderr: "main belongs to another worktree\n", Requests: []logSummary{{Seq: 1, Name: "SSH github.com/octo/hello.git", Matched: true}}},
			},
			check: func(t *testing.T, st *state, rec runRecord) {
				t.Setenv("GHMOCK_DIR", st.dir)
				before := gitForTest(t, "--run", rec.ID, "--before", "--in", "main", "--", "rev-parse", "HEAD")
				old := gitForTest(t, "--run", rec.ID, "--build", "1", "--in", "main", "--", "rev-parse", "HEAD")
				fixed := gitForTest(t, "--run", rec.ID, "--build", "2", "--in", "main", "--", "rev-parse", "HEAD")
				assert.NotEqual(t, before, old)
				assert.Equal(t, before, fixed)
				assert.Equal(t, before, gitForTest(t, "--in", "main", "--", "rev-parse", "HEAD"))
				assert.Equal(t, "M  example.txt\n", gitForTest(t, "--run", rec.ID, "--build", "1", "--in", "main", "--", "status", "--porcelain=v1"))
				assert.Empty(t, gitForTest(t, "--run", rec.ID, "--build", "2", "--in", "main", "--", "status", "--porcelain=v1"))
				assert.Empty(t, gitForTest(t, "--in", "main", "--", "status", "--porcelain=v1"))
				assert.Equal(t, "two\n", gitForTest(t, "--run", rec.ID, "--build", "1", "--in", "main", "--", "show", "HEAD:example.txt"))
				assert.Equal(t, "one\n", gitForTest(t, "--run", rec.ID, "--build", "1", "--in", "main", "--", "show", ":example.txt"))
				for _, result := range rec.Results {
					require.NotNil(t, result.Git)
					require.Contains(t, result.Git.Version, "git version")
					data, err := os.ReadFile(filepath.Join(st.dir, result.Git.Directory, "work", "main", "example.txt"))
					require.NoError(t, err)
					assert.Equal(t, "one\n", string(data))
				}
				exported := filepath.Join(t.TempDir(), "export")
				require.NoError(t, cmdExport([]string{exported}, io.Discard))
				t.Setenv("GHMOCK_DIR", exported)
				assert.Equal(t, "M  example.txt\n", gitForTest(t, "--run", rec.ID, "--build", "1", "--in", "main", "--", "status", "--porcelain=v1"))
				assert.Empty(t, gitForTest(t, "--run", rec.ID, "--build", "2", "--in", "main", "--", "status", "--porcelain=v1"))
				gitForTest(t, "--run", rec.ID, "--build", "1", "--in", "main", "--", "reset", "--hard", "--quiet")
				t.Setenv("GHMOCK_DIR", st.dir)
				assert.Equal(t, "M  example.txt\n", gitForTest(t, "--run", rec.ID, "--build", "1", "--in", "main", "--", "status", "--porcelain=v1"), "changing the export must not change the original run")
				archive := filepath.Join(t.TempDir(), "evidence.tar.gz")
				require.NoError(t, cmdExport([]string{"--archive", archive}, io.Discard))
				restored := filepath.Join(t.TempDir(), "restored")
				extractTestArchive(t, archive, restored)
				require.NoError(t, cmdReset(nil, io.Discard))
				assert.NoDirExists(t, filepath.Join(st.dir, "git"))
				t.Setenv("GHMOCK_DIR", restored)
				assert.Equal(t, "M  example.txt\n", gitForTest(t, "--run", rec.ID, "--build", "1", "--in", "main", "--", "status", "--porcelain=v1"))
				assert.Empty(t, gitForTest(t, "--run", rec.ID, "--build", "2", "--in", "main", "--", "status", "--porcelain=v1"))
				assert.Equal(t, before, gitForTest(t, "--run", rec.ID, "--before", "--in", "main", "--", "rev-parse", "HEAD"))
				assert.Equal(t, before, gitForTest(t, "--run", rec.ID, "--build", "1", "--in", "feature", "--", "rev-parse", "HEAD"))
				assert.Equal(t, "one\n", gitForTest(t, "--run", rec.ID, "--build", "1", "--in", "main", "--", "show", ":example.txt"))
			},
		},
		{
			name:     "a push changes only that build's remote",
			variants: []string{"a", "b"},
			opts:     runOptions{in: "main", args: []string{"push-change"}},
			setup: func(t *testing.T, st *state) {
				t.Setenv("GHMOCK_DIR", st.dir)
				prepareGitRepo(t, "git@github.com:octo/hello.git")
			},
			wantCommand: "ghmock run --gh <gh-a> --gh <gh-b> --in main --timeout 30s -- push-change",
			want: []resultSummary{
				{Version: "gh version 0.0.0-a", Requests: []logSummary{{Seq: 1, Name: "SSH github.com/octo/hello.git", Matched: true}}},
				{Version: "gh version 0.0.0-b", Requests: []logSummary{{Seq: 1, Name: "SSH github.com/octo/hello.git", Matched: true}}},
			},
			check: func(t *testing.T, st *state, rec runRecord) {
				t.Setenv("GHMOCK_DIR", st.dir)
				before := gitForTest(t, "--in", "main", "--", "rev-parse", "HEAD")
				var tips []string
				for _, build := range []string{"1", "2"} {
					prefix := []string{"--run", rec.ID, "--build", build, "--in", "main", "--"}
					tip := gitForTest(t, append(prefix, "rev-parse", "HEAD")...)
					tips = append(tips, tip)
					assert.Equal(t, before, gitForTest(t, append(prefix, "rev-parse", "HEAD^")...))
					assert.Equal(t, strings.TrimSpace(tip)+"\trefs/heads/main\n", gitForTest(t, append(prefix, "ls-remote", "origin", "refs/heads/main")...))
				}
				assert.NotEqual(t, tips[0], tips[1])
				assert.Equal(t, strings.TrimSpace(before)+"\trefs/heads/main\n", gitForTest(t, "--in", "main", "--", "ls-remote", "origin", "refs/heads/main"))
			},
		},
		{
			name:     "staged unstaged and untracked files survive each copy",
			variants: []string{"a", "b"},
			opts:     runOptions{in: "main", args: []string{"git", "status", "--porcelain=v1"}},
			setup: func(t *testing.T, st *state) {
				t.Setenv("GHMOCK_DIR", st.dir)
				prepareGitRepo(t, "https://github.com/octo/hello.git")
				require.NoError(t, cmdWrite([]string{"main/example.txt", "--text", "staged\n"}, io.Discard))
				gitForTest(t, "--in", "main", "--", "add", "example.txt")
				require.NoError(t, cmdWrite([]string{"main/example.txt", "--text", "unstaged\n"}, io.Discard))
				require.NoError(t, cmdWrite([]string{"main/untracked", "--text", "not committed\n"}, io.Discard))
			},
			wantCommand: "ghmock run --gh <gh-a> --gh <gh-b> --in main --timeout 30s -- git status --porcelain=v1",
			want: []resultSummary{
				{Version: "gh version 0.0.0-a", Stdout: "MM example.txt\n?? untracked\n"},
				{Version: "gh version 0.0.0-b", Stdout: "MM example.txt\n?? untracked\n"},
			},
		},
		{
			name:     "missing Git fixture is an error",
			variants: []string{"a"},
			opts:     runOptions{in: "feature", args: []string{"git", "status"}},
			wantErr:  "no Git fixture yet; prepare it with ghmock repo, git or write",
		},
		{
			name:        "gh gets an isolated environment, logged in with a fake token",
			variants:    []string{"a"},
			opts:        runOptions{args: []string{"env"}},
			wantCommand: "ghmock run --gh <gh-a> --timeout 30s -- env",
			want: []resultSummary{{
				Version: "gh version 0.0.0-a",
				Stdout: `GH_HOST=
GH_TOKEN=
GITHUB_TOKEN=
GH_FORCE_TTY=
NO_COLOR=
TERM=
COLORTERM=
SSL_CERT_FILE=<scratch>/ghmock-ca.pem
SSL_CERT_DIR=<scratch>/certs
GIT_SSL_CAINFO=<scratch>/ghmock-ca.pem
HOME=<scratch>/home
PWD=<scratch>/work
--- <scratch>/config/config.yml
version: "1"
--- <scratch>/config/hosts.yml
github.com:
    users:
        monalisa:
            oauth_token: gho_ghmock-fake-token
    git_protocol: https
    oauth_token: gho_ghmock-fake-token
    user: monalisa
`,
			}},
		},
		{
			name:        "--localhost logs gh in to github.localhost and trusts no extra authority",
			variants:    []string{"a"},
			opts:        runOptions{args: []string{"env"}, localhost: true},
			wantCommand: "ghmock run --gh <gh-a> --localhost --timeout 30s -- env",
			want: []resultSummary{{
				Version: "gh version 0.0.0-a",
				Stdout: `GH_HOST=github.localhost
GH_TOKEN=
GITHUB_TOKEN=
GH_FORCE_TTY=
NO_COLOR=
TERM=
COLORTERM=
SSL_CERT_FILE=
SSL_CERT_DIR=
GIT_SSL_CAINFO=
HOME=<scratch>/home
PWD=<scratch>/work
--- <scratch>/config/config.yml
version: "1"
--- <scratch>/config/hosts.yml
github.localhost:
    users:
        monalisa:
            oauth_token: gho_ghmock-fake-token
    git_protocol: https
    oauth_token: gho_ghmock-fake-token
    user: monalisa
`,
			}},
		},
		{
			name:        "--logged-out skips the login and --tty forces a color terminal",
			variants:    []string{"a"},
			opts:        runOptions{args: []string{"env"}, loggedOut: true, tty: true},
			wantCommand: "ghmock run --gh <gh-a> --tty --logged-out --timeout 30s -- env",
			want: []resultSummary{{
				Version: "gh version 0.0.0-a",
				Stdout: `GH_HOST=
GH_TOKEN=
GITHUB_TOKEN=
GH_FORCE_TTY=120
NO_COLOR=
TERM=xterm-256color
COLORTERM=truecolor
SSL_CERT_FILE=<scratch>/ghmock-ca.pem
SSL_CERT_DIR=<scratch>/certs
GIT_SSL_CAINFO=<scratch>/ghmock-ca.pem
HOME=<scratch>/home
PWD=<scratch>/work
--- <scratch>/config/config.yml
version: "1"
--- <scratch>/config/hosts.yml
(none)
`,
			}},
		},
		{
			name:        "a ghmock built with a builds folder runs gh builds in it",
			variants:    []string{"a"},
			builds:      "own",
			opts:        runOptions{args: []string{"exit", "0"}},
			want:        []resultSummary{{Version: "gh version 0.0.0-a", Stderr: "failing on purpose\n"}},
			wantCommand: "ghmock run --gh <gh-a> --timeout 30s -- exit 0",
		},
		{
			name:     "a ghmock built with a builds folder refuses anything else",
			variants: []string{"a"},
			builds:   "other",
			opts:     runOptions{args: []string{"exit", "0"}},
			wantErr:  "<gh-a>: <gh-a> isn't a gh build in <builds>, and this ghmock only runs those",
		},
		{
			name:        "--stdin is passed to gh",
			variants:    []string{"a"},
			opts:        runOptions{args: []string{"stdin"}, stdin: "it's secret"},
			want:        []resultSummary{{Version: "gh version 0.0.0-a", Stdout: "it's secret"}},
			wantCommand: `ghmock run --gh <gh-a> --stdin 'it'\''s secret' --timeout 30s -- stdin`,
		},
		{
			name:        "--keys types at each prompt in a terminal",
			variants:    []string{"a"},
			opts:        runOptions{args: []string{"prompt", "Name", "Color"}, terminal: true, keys: []string{"Mona{enter}", "teal{enter}"}},
			wantCommand: "ghmock run --gh <gh-a> --keys 'Mona{enter}' --keys 'teal{enter}' --timeout 30s -- prompt Name Color",
			want: []resultSummary{{
				Version: "gh version 0.0.0-a",
				Stdout:  "? Name Mona\n? Color teal\nAnswers: Mona, teal\n",
				Steps: []terminalStep{
					{Keys: "Mona{enter}", Screen: "? Name\n"},
					{Keys: "teal{enter}", Screen: "? Name Mona\n? Color\n"},
				},
			}},
		},
		{
			name:        "an unanswered terminal prompt obeys the configured timeout",
			variants:    []string{"a"},
			opts:        runOptions{args: []string{"prompt", "Name"}, terminal: true, timeout: 500 * time.Millisecond},
			wantCommand: "ghmock run --gh <gh-a> --terminal --timeout 500ms -- prompt Name",
			want:        []resultSummary{{Version: "gh version 0.0.0-a", ExitCode: -1, TimedOut: true, Stdout: "? Name\n"}},
		},
		{
			name:        "quiet terminal work is not mistaken for an unanswered prompt",
			variants:    []string{"a"},
			opts:        runOptions{args: []string{"terminal-quiet"}, terminal: true, timeout: 4 * time.Second},
			wantCommand: "ghmock run --gh <gh-a> --terminal --timeout 4s -- terminal-quiet",
			want:        []resultSummary{{Version: "gh version 0.0.0-a", Stdout: "finished\n"}},
		},
		{
			name:        "terminal UTF8 survives read boundaries",
			variants:    []string{"a"},
			opts:        runOptions{args: []string{"terminal-utf8"}, terminal: true},
			wantCommand: "ghmock run --gh <gh-a> --terminal --timeout 30s -- terminal-utf8",
			want:        []resultSummary{{Version: "gh version 0.0.0-a", Stdout: "\u754c\n"}},
		},
		{
			name:        "terminal color remains in captured output and comparisons",
			variants:    []string{"red", "green"},
			opts:        runOptions{args: []string{"terminal-color"}, terminal: true},
			wantCommand: "ghmock run --gh <gh-red> --gh <gh-green> --terminal --timeout 30s -- terminal-color",
			want:        []resultSummary{{Version: "gh version 0.0.0-red", Stdout: "same\n"}, {Version: "gh version 0.0.0-green", Stdout: "same\n"}},
			check: func(t *testing.T, _ *state, rec runRecord) {
				assert.Contains(t, rec.Results[0].TerminalOutput, "\x1b[31m")
				assert.Contains(t, rec.Results[1].TerminalOutput, "\x1b[32m")
				assert.Contains(t, rec.Report, "terminal output:")
			},
		},
		{
			name:        "different prompts remain visible when final screens match",
			variants:    []string{"Name", "Title"},
			opts:        runOptions{args: []string{"terminal-prompt"}, terminal: true, keys: []string{"Mona{enter}"}},
			wantCommand: "ghmock run --gh <gh-Name> --gh <gh-Title> --keys 'Mona{enter}' --timeout 30s -- terminal-prompt",
			want: []resultSummary{
				{Version: "gh version 0.0.0-Name", Stdout: "finished\n", Steps: []terminalStep{{Keys: "Mona{enter}", Screen: "? Name\n"}}},
				{Version: "gh version 0.0.0-Title", Stdout: "finished\n", Steps: []terminalStep{{Keys: "Mona{enter}", Screen: "? Title\n"}}},
			},
			check: func(t *testing.T, _ *state, rec runRecord) {
				assert.Contains(t, rec.Report, "prompt snapshots differ")
				assert.NotContains(t, rec.Report, "same exit code, output and requests")
			},
		},
		{
			name:        "unused terminal keys are included in comparisons",
			variants:    []string{"early", "Name"},
			opts:        runOptions{args: []string{"terminal-prompt"}, terminal: true, keys: []string{"Mona{enter}"}},
			wantCommand: "ghmock run --gh <gh-early> --gh <gh-Name> --keys 'Mona{enter}' --timeout 30s -- terminal-prompt",
			want: []resultSummary{
				{Version: "gh version 0.0.0-early", Stdout: "finished\n", UnusedKeys: 1},
				{Version: "gh version 0.0.0-Name", Stdout: "finished\n", Steps: []terminalStep{{Keys: "Mona{enter}", Screen: "? Name\n"}}},
			},
			check: func(t *testing.T, _ *state, rec runRecord) {
				assert.Contains(t, rec.Report, "unused keys: 1 -> 0")
			},
		},
		{
			name:        "keys gh exits before getting are counted",
			variants:    []string{"a"},
			opts:        runOptions{args: []string{"exit", "0"}, terminal: true, keys: []string{"x"}},
			wantCommand: "ghmock run --gh <gh-a> --keys x --timeout 30s -- exit 0",
			want:        []resultSummary{{Version: "gh version 0.0.0-a", Stdout: "failing on purpose\n", UnusedKeys: 1}},
		},
		{
			name:        "without a terminal, gh can't prompt",
			variants:    []string{"a"},
			opts:        runOptions{args: []string{"prompt", "Name"}},
			wantCommand: "ghmock run --gh <gh-a> --timeout 30s -- prompt Name",
			want:        []resultSummary{{Version: "gh version 0.0.0-a", ExitCode: 1, Stderr: "fake gh: not a terminal\n"}},
		},
		{
			name:        "gh's exit code and stderr are reported",
			variants:    []string{"a"},
			opts:        runOptions{args: []string{"exit", "3"}},
			wantCommand: "ghmock run --gh <gh-a> --timeout 30s -- exit 3",
			want:        []resultSummary{{Version: "gh version 0.0.0-a", ExitCode: 3, Stderr: "failing on purpose\n"}},
		},
		{
			name:        "a run that takes too long is stopped",
			variants:    []string{"a"},
			opts:        runOptions{args: []string{"sleep"}, timeout: 300 * time.Millisecond},
			wantCommand: "ghmock run --gh <gh-a> --timeout 300ms -- sleep",
			want:        []resultSummary{{Version: "gh version 0.0.0-a", ExitCode: -1, TimedOut: true}},
		},
		{
			name:        "requests get stored responses",
			variants:    []string{"a"},
			responses:   responseSet{"GET /repos/octo/hello": {{Body: json.RawMessage(`{"name":"hello"}`)}}},
			opts:        runOptions{args: []string{"api", "repos/octo/hello"}},
			wantCommand: "ghmock run --gh <gh-a> --timeout 30s -- api repos/octo/hello",
			want: []resultSummary{{
				Version:  "gh version 0.0.0-a",
				Stdout:   "200 {\"name\":\"hello\"}\n",
				Requests: []logSummary{{Seq: 1, Name: "GET /repos/octo/hello", Status: 200, Matched: true, Response: 1, Responses: 1}},
			}},
		},
		{
			name:        "--localhost sends requests over plain HTTP",
			variants:    []string{"a"},
			responses:   responseSet{"GET /repos/octo/hello": {{Body: json.RawMessage(`{"name":"hello"}`)}}},
			opts:        runOptions{args: []string{"api", "repos/octo/hello"}, localhost: true},
			wantCommand: "ghmock run --gh <gh-a> --localhost --timeout 30s -- api repos/octo/hello",
			want: []resultSummary{{
				Version:  "gh version 0.0.0-a",
				Stdout:   "200 {\"name\":\"hello\"}\n",
				Requests: []logSummary{{Seq: 1, Name: "GET /repos/octo/hello", Status: 200, Matched: true, Response: 1, Responses: 1}},
			}},
		},
		{
			name:        "requests to other hosts keep the host in their name",
			variants:    []string{"a"},
			opts:        runOptions{args: []string{"get", "https://uploads.github.com/upload"}},
			wantCommand: "ghmock run --gh <gh-a> --timeout 30s -- get https://uploads.github.com/upload",
			want: []resultSummary{{
				Version:  "gh version 0.0.0-a",
				ExitCode: 1,
				Stdout:   "404 {\"message\":\"ghmock: no response for GET uploads.github.com/upload\"}\n",
				Requests: []logSummary{{Seq: 1, Name: "GET uploads.github.com/upload", Status: 404}},
			}},
		},
		{
			name:        "a gh that rejects ghmock's certificate is logged",
			variants:    []string{"a"},
			opts:        runOptions{args: []string{"untrusted", "https://api.github.com/zen"}},
			wantCommand: "ghmock run --gh <gh-a> --timeout 30s -- untrusted https://api.github.com/zen",
			want: []resultSummary{{
				Version:  "gh version 0.0.0-a",
				ExitCode: 1,
				Stderr:   "fake gh: TLS handshake failed\n",
				Requests: []logSummary{{Seq: 1, TLSFailed: true}},
			}},
		},
		{
			name:        "--localhost blocks HTTPS",
			variants:    []string{"a"},
			opts:        runOptions{args: []string{"get", "https://api.github.com/zen"}, localhost: true},
			wantCommand: "ghmock run --gh <gh-a> --localhost --timeout 30s -- get https://api.github.com/zen",
			want: []resultSummary{{
				Version:  "gh version 0.0.0-a",
				ExitCode: 1,
				Stderr:   "Get \"https://api.github.com/zen\": Forbidden\n",
				Requests: []logSummary{{Seq: 1, Status: 403, Blocked: true}},
			}},
		},
		{
			name:         "responses are checked against GitHub's schema",
			variants:     []string{"a"},
			responses:    responseSet{"GET /repos/octo/hello": {{Body: json.RawMessage(`{"id":1,"name":"hello","owner":{"login":"octo","id":2}}`)}}},
			checkSchemas: true,
			opts:         runOptions{args: []string{"api", "repos/octo/hello"}},
			wantCommand:  "ghmock run --gh <gh-a> --timeout 30s -- api repos/octo/hello",
			want: []resultSummary{{
				Version: "gh version 0.0.0-a",
				Stdout:  "200 {\"id\":1,\"name\":\"hello\",\"owner\":{\"login\":\"octo\",\"id\":2}}\n",
				Requests: []logSummary{{
					Seq: 1, Name: "GET /repos/octo/hello", Status: 200, Matched: true, Response: 1, Responses: 1,
					SchemaProblems: []string{"subscribers_count: missing, but GitHub's description requires it"},
				}},
			}},
		},
		{
			name:     "each binary starts from the first response",
			variants: []string{"a", "b"},
			responses: responseSet{"GET /flaky": {
				{Status: 502, Body: json.RawMessage(`{"message":"bad gateway"}`)},
				{Body: json.RawMessage(`{"ok":true}`)},
			}},
			opts:        runOptions{args: []string{"api", "flaky"}},
			wantCommand: "ghmock run --gh <gh-a> --gh <gh-b> --timeout 30s -- api flaky",
			want: []resultSummary{
				{
					Version:  "gh version 0.0.0-a",
					ExitCode: 1,
					Stdout:   "502 {\"message\":\"bad gateway\"}\n",
					Requests: []logSummary{{Seq: 1, Name: "GET /flaky", Status: 502, Matched: true, Response: 1, Responses: 2}},
				},
				{
					Version:  "gh version 0.0.0-b",
					ExitCode: 1,
					Stdout:   "502 {\"message\":\"bad gateway\"}\n",
					Requests: []logSummary{{Seq: 1, Name: "GET /flaky", Status: 502, Matched: true, Response: 1, Responses: 2}},
				},
			},
		},
	}

	oldSettle := settleTime
	settleTime = 150 * time.Millisecond
	t.Cleanup(func() { settleTime = oldSettle })

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.resolveOnly {
				executable, err := os.Executable()
				require.NoError(t, err)
				t.Setenv("PATH", filepath.Dir(executable)+string(os.PathListSeparator)+os.Getenv("PATH"))
				if runtime.GOOS == "windows" {
					t.Setenv("PATHEXT", ".EXE")
				}
				resolved, err := resolveBinary(filepath.Base(executable))
				require.NoError(t, err)
				assert.Equal(t, executable, resolved)
				return
			}
			if runtime.GOOS == "windows" {
				t.Skip("the fake gh wrapper needs /bin/sh")
			}
			// Neither should reach gh.
			t.Setenv("GH_TOKEN", "caller-token")
			t.Setenv("GITHUB_TOKEN", "caller-token")
			callerConfig := t.TempDir()
			writeConfigFixture(t, callerConfig, map[string][]byte{"hosts.yml": []byte("caller configuration")})
			t.Setenv("GH_CONFIG_DIR", callerConfig)

			st := &state{dir: t.TempDir()}
			if tt.setup != nil {
				tt.setup(t, st)
			}
			if tt.responses != nil {
				require.NoError(t, st.saveResponses(tt.responses))
			}
			opts := tt.opts
			opts.configDir = strings.ReplaceAll(opts.configDir, "<state>", st.dir)
			if opts.timeout == 0 {
				opts.timeout = 30 * time.Second
			}
			for _, v := range tt.variants {
				opts.binaries = append(opts.binaries, writeFakeGH(t, v))
			}
			var chk *checker
			if tt.checkSchemas {
				chk = testChecker(t)
			}
			replacer := strings.NewReplacer()
			if tt.builds != "" {
				dir := filepath.Dir(opts.binaries[0])
				if tt.builds == "other" {
					dir = t.TempDir()
				}
				old := buildsDir
				t.Cleanup(func() { buildsDir = old })
				buildsDir = dir
				replacer = strings.NewReplacer(opts.binaries[0], "<gh-a>", dir, "<builds>")
			}
			if tt.afterAPI {
				previous := opts
				previous.args = []string{"api", "user"}
				_, _, err := runAll(st, chk, previous)
				require.NoError(t, err)
			}

			rec, results, err := runAll(st, chk, opts)
			if tt.wantErr != "" || tt.wantErrContains != "" {
				require.Error(t, err)
				if tt.wantErrContains != "" {
					assert.Contains(t, err.Error(), tt.wantErrContains)
				} else {
					assert.Equal(t, tt.wantErr, replacer.Replace(err.Error()))
				}
				return
			}
			require.NoError(t, err)

			var got []resultSummary
			for _, r := range results {
				got = append(got, resultSummary{
					Version:    r.version,
					ExitCode:   r.exitCode,
					TimedOut:   r.timedOut,
					Stdout:     r.stdout,
					Stderr:     r.stderr,
					Requests:   summarize(r.requests),
					Steps:      r.steps,
					UnusedKeys: r.unusedKeys,
				})
			}
			assert.Equal(t, tt.want, got)

			logged, err := st.readLog()
			require.NoError(t, err)
			runs, err := st.readRuns()
			require.NoError(t, err)
			if tt.afterAPI {
				require.Len(t, runs, 2)
				require.Len(t, logged, 1)
				assert.Equal(t, runs[0].ID, logged[0].Run)
				assert.NotEqual(t, runs[0].ID, runs[1].ID)
				t.Setenv("GHMOCK_DIR", st.dir)
				var latest, all bytes.Buffer
				require.NoError(t, cmdLog(nil, &latest))
				assert.Equal(t, "No requests logged.\n", latest.String())
				require.NoError(t, cmdLog([]string{"--all"}, &all))
				assert.Contains(t, all.String(), "GET /user")
				return
			}
			require.Len(t, runs, 1)
			assert.NotEmpty(t, runs[0].ID)
			assert.False(t, runs[0].Time.IsZero())
			var wantLogged []logSummary
			for _, w := range tt.want {
				wantLogged = append(wantLogged, w.Requests...)
			}
			assert.Equal(t, wantLogged, summarize(logged))
			for _, e := range logged {
				assert.Equal(t, rec.ID, e.Run, "one run ID per ghmock run")
			}

			// The run is saved for ghmock export, with what ghmock run printed.
			saved, err := st.readRuns()
			require.NoError(t, err)
			require.Equal(t, []runRecord{rec}, saved)
			command := rec.Command
			command = strings.ReplaceAll(command, st.dir, "<state>")
			for i, b := range opts.binaries {
				command = strings.ReplaceAll(command, b, "<gh-"+tt.variants[i]+">")
			}
			assert.Equal(t, tt.wantCommand, command)
			var outcomes []runOutcome
			for i, r := range results {
				outcomes = append(outcomes, runOutcome{
					Binary: opts.binaries[i], Path: r.path, Version: r.version, ExitCode: r.exitCode,
					TimedOut: r.timedOut, Stdout: r.stdout, Stderr: r.stderr,
					Terminal: r.terminal, TerminalOutput: r.terminalOutput, Steps: r.steps, UnusedKeys: r.unusedKeys,
					Git: r.git, Config: r.config,
				})
			}
			assert.Equal(t, outcomes, rec.Results)
			var report strings.Builder
			printReport(&report, opts, results, nil)
			assert.Equal(t, report.String(), rec.Report)
			var schemas []string
			for _, f := range rec.Schemas {
				assert.Len(t, f.SHA256, 64)
				schemas = append(schemas, f.Name+" "+filepath.Base(f.Path))
			}
			if tt.checkSchemas {
				assert.Equal(t, []string{"GraphQL schema schema.graphql", "REST description rest.json"}, schemas)
			} else {
				assert.Empty(t, schemas)
			}
			if tt.check != nil {
				tt.check(t, st, rec)
			}
		})
	}
}

func writeConfigFixture(t *testing.T, directory string, files map[string][]byte) {
	t.Helper()
	require.NoError(t, os.MkdirAll(directory, 0o700))
	for name, data := range files {
		path := filepath.Join(directory, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, data, 0o600))
	}
}

func TestPrintReport(t *testing.T) {
	opts := runOptions{args: []string{"issue", "view", "1"}}
	viewRequest := loggedRequest{
		Seq:     1,
		Method:  "POST",
		Name:    "graphql IssueByNumber",
		GraphQL: &graphQLRequest{Kind: "query", Operation: "IssueByNumber", Variables: json.RawMessage(`{"number": 1}`)},
		Status:  200, Matched: true, Response: 1, Responses: 1,
	}
	missingRequest := loggedRequest{
		Seq:     2,
		Method:  "POST",
		Name:    "graphql IssueProjectItems",
		GraphQL: &graphQLRequest{Kind: "query", Operation: "IssueProjectItems"},
		Status:  200,
	}
	commentRequest := func(body string) loggedRequest {
		return loggedRequest{
			Seq:     2,
			Method:  "POST",
			Name:    "graphql CommentCreate",
			GraphQL: &graphQLRequest{Kind: "mutation", Operation: "CommentCreate", Variables: json.RawMessage(`{"body":"` + body + `"}`)},
			Status:  200, Matched: true, Response: 1, Responses: 1,
		}
	}
	readRequest := func(query, variables string) loggedRequest {
		return loggedRequest{Name: "graphql Issue", Method: "POST", GraphQL: &graphQLRequest{
			Kind: "query", Operation: "Issue", Query: query, Variables: json.RawMessage(variables),
		}}
	}
	const variableQuery = `query Issue($number:Int!) { repository(owner:"o",name:"r") { issue(number:$number) { title } } }`
	const firstQuery = `query Issue { repository(owner:"o",name:"r") { issue(number:1) { title } } }`
	const secondQuery = `query Issue { repository(owner:"o",name:"r") { issue(number:2) { title } } }`
	const firstUnusedQuery = `query First { viewer { login } } query Second { viewer { name } }`
	const secondUnusedQuery = `query First { viewer { id } } query Second { viewer { name } }`

	tests := []struct {
		name            string
		results         []runResult
		notes           []string
		want            string
		wantDifferences string
	}{
		{
			name: "field summaries use only the selected operation",
			results: []runResult{
				{version: "baseline", requests: []loggedRequest{{Name: "graphql Second", GraphQL: &graphQLRequest{Kind: "query", Operation: "Second", Query: firstUnusedQuery}}}},
				{version: "candidate", requests: []loggedRequest{{Name: "graphql Second", GraphQL: &graphQLRequest{Kind: "query", Operation: "Second", Query: secondUnusedQuery}}}},
			},
			wantDifferences: fmt.Sprintf("[2] candidate:\n  graphql Second sent different data:\n    [1] query %q\n    [2] query %q\n", firstUnusedQuery, secondUnusedQuery),
		},
		{
			name: "REST query and opaque body boundaries cannot collide",
			results: []runResult{
				{version: "baseline", requests: []loggedRequest{{Name: "POST /echo", RawQuery: "a=1", Body: "hello"}}},
				{version: "candidate", requests: []loggedRequest{{Name: "POST /echo", Body: "?a=1 hello"}}},
			},
			wantDifferences: "[2] candidate:\n  POST /echo sent different data:\n    [1] ?a=1 body \"hello\"\n    [2] body \"?a=1 hello\"\n",
		},
		{
			name: "REST read query parameters are compared",
			results: []runResult{
				{version: "baseline", requests: []loggedRequest{{Name: "GET /issues", RawQuery: "state=open"}}},
				{version: "candidate", requests: []loggedRequest{{Name: "GET /issues", RawQuery: "state=closed"}}},
			},
			wantDifferences: "[2] candidate:\n  GET /issues sent different data:\n    [1] ?state=open\n    [2] ?state=closed\n",
		},
		{
			name: "GraphQL read variable values are compared",
			results: []runResult{
				{version: "baseline", requests: []loggedRequest{readRequest(variableQuery, `{"number":1}`)}},
				{version: "candidate", requests: []loggedRequest{readRequest(variableQuery, `{"number":2}`)}},
			},
			wantDifferences: fmt.Sprintf("[2] candidate:\n  graphql Issue sent different data:\n    [1] query %q {\"number\":1}\n    [2] query %q {\"number\":2}\n", variableQuery, variableQuery),
		},
		{
			name: "GraphQL query text is compared even with the same fields",
			results: []runResult{
				{version: "baseline", requests: []loggedRequest{readRequest(firstQuery, "")}},
				{version: "candidate", requests: []loggedRequest{readRequest(secondQuery, "")}},
			},
			wantDifferences: fmt.Sprintf("[2] candidate:\n  graphql Issue sent different data:\n    [1] query %q\n    [2] query %q\n", firstQuery, secondQuery),
		},
		{
			name: "run bookkeeping is not request data",
			results: []runResult{
				{version: "baseline", requests: []loggedRequest{{Run: "old", Binary: "old", Seq: 1, Name: "GET /issues", RawQuery: "state=open"}}},
				{version: "candidate", requests: []loggedRequest{{Run: "new", Binary: "new", Seq: 99, Name: "GET /issues", RawQuery: "state=open"}}},
			},
			wantDifferences: "[2] candidate:\n  same exit code, output and requests\n",
		},
		{
			name: "raw write bodies are compared without printing binary data",
			results: []runResult{
				{path: "/old/gh", version: "old", requests: []loggedRequest{{Seq: 1, Method: "POST", Name: "POST /binary", RawBody: &[]byte{0, 255}, Status: 200, Matched: true, Response: 1, Responses: 1}}},
				{path: "/new/gh", version: "new", requests: []loggedRequest{{Seq: 1, Method: "POST", Name: "POST /binary", RawBody: &[]byte{0, 254}, Status: 200, Matched: true, Response: 1, Responses: 1}}},
			},
			want: fmt.Sprintf(`=== [1] old - /old/gh (baseline)
$ gh issue view 1
exit code: 0
--- stdout
(empty)
--- stderr
(empty)
--- requests (1)
1. POST /binary raw body %[1]s -> 200, response 1 of 1

=== [2] new - /new/gh
$ gh issue view 1
exit code: 0
--- stdout
(empty)
--- stderr
(empty)
--- requests (1)
1. POST /binary raw body %[2]s -> 200, response 1 of 1

=== differences from [1]
[2] new:
  POST /binary sent different data:
    [1] raw body %[1]s
    [2] raw body %[2]s
`, rawBodySummary([]byte{0, 255}), rawBodySummary([]byte{0, 254})),
		},
		{
			name: "one binary",
			results: []runResult{{
				path: "/bin/gh", version: "gh version 1.0.0", exitCode: 1,
				stderr:   "GraphQL: ghmock: no response for graphql IssueProjectItems\n",
				requests: []loggedRequest{viewRequest, missingRequest},
			}},
			want: `gh version 1.0.0 - /bin/gh
$ gh issue view 1
exit code: 1
--- stdout
(empty)
--- stderr
GraphQL: ghmock: no response for graphql IssueProjectItems
--- requests (2)
1. graphql IssueByNumber {"number":1} -> 200, response 1 of 1
2. graphql IssueProjectItems -> 200, no response stored
--- no response for
graphql IssueProjectItems
`,
		},
		{
			name: "two binaries that differ",
			results: []runResult{
				{
					path: "/old/gh", version: "gh version 1.0.0",
					stdout:   "title: Bug\nstate: OPEN\n",
					requests: []loggedRequest{viewRequest, commentRequest("hi")},
				},
				{
					path: "/new/gh", version: "gh version 2.0.0", exitCode: 1,
					stdout:   "title: Bug\nstate: CLOSED\n",
					requests: []loggedRequest{viewRequest, commentRequest("hello"), withSeq(missingRequest, 3)},
				},
			},
			want: `=== [1] gh version 1.0.0 - /old/gh (baseline)
$ gh issue view 1
exit code: 0
--- stdout
title: Bug
state: OPEN
--- stderr
(empty)
--- requests (2)
1. graphql IssueByNumber {"number":1} -> 200, response 1 of 1
2. graphql CommentCreate {"body":"hi"} -> 200, response 1 of 1

=== [2] gh version 2.0.0 - /new/gh
$ gh issue view 1
exit code: 1
--- stdout
title: Bug
state: CLOSED
--- stderr
(empty)
--- requests (3)
1. graphql IssueByNumber {"number":1} -> 200, response 1 of 1
2. graphql CommentCreate {"body":"hello"} -> 200, response 1 of 1
3. graphql IssueProjectItems -> 200, no response stored
--- no response for
graphql IssueProjectItems

=== differences from [1]
[2] gh version 2.0.0:
  exit code: 0 -> 1
  stdout:
    - state: OPEN
    + state: CLOSED
  graphql IssueProjectItems: 0 in [1], 1 in [2]
  graphql CommentCreate sent different data:
    [1] {"body":"hi"}
    [2] {"body":"hello"}
`,
		},
		{
			name: "a terminal run shows the screen before each --keys and at the end",
			results: []runResult{{
				path: "/bin/gh", version: "gh version 1.0.0", exitCode: -1, terminal: true, timedOut: true,
				steps:  []terminalStep{{Keys: "{enter}", Screen: "? What would you like to do?\n> Create\n"}},
				stdout: "? What would you like to do? Create\n? Repository name\n",
			}},
			want: `gh version 1.0.0 - /bin/gh
$ gh issue view 1
exit code: none, timed out
--- screen before --keys 1, {enter}
? What would you like to do?
> Create
--- final screen
? What would you like to do? Create
? Repository name
--- requests (0)
`,
		},
		{
			name: "escape codes show as \\e",
			results: []runResult{{
				path: "/bin/gh", version: "gh version 1.0.0",
				stdout: "\x1b[33mwarning\x1b[0m\n",
			}},
			want: `gh version 1.0.0 - /bin/gh
$ gh issue view 1
exit code: 0
--- stdout
\e[33mwarning\e[0m
--- stderr
(empty)
--- requests (0)
`,
		},
		{
			name: "a missing color reset shows in the differences",
			results: []runResult{
				{path: "/old/gh", version: "gh version 1.0.0", stdout: "\x1b[43mmatch\n"},
				{path: "/new/gh", version: "gh version 2.0.0", stdout: "\x1b[43mmatch\x1b[0m\n"},
			},
			want: `=== [1] gh version 1.0.0 - /old/gh (baseline)
$ gh issue view 1
exit code: 0
--- stdout
\e[43mmatch
--- stderr
(empty)
--- requests (0)

=== [2] gh version 2.0.0 - /new/gh
$ gh issue view 1
exit code: 0
--- stdout
\e[43mmatch\e[0m
--- stderr
(empty)
--- requests (0)

=== differences from [1]
[2] gh version 2.0.0:
  stdout:
    - \e[43mmatch
    + \e[43mmatch\e[0m
`,
		},
		{
			name: "with --localhost, requests for another host",
			results: []runResult{{
				path: "/bin/gh", version: "gh version 1.0.0", exitCode: 1,
				stderr: "error validating token: Get \"https://api.github.com/\": Forbidden\n",
				requests: []loggedRequest{
					{Seq: 1, Method: "CONNECT", Host: "api.github.com:443", Status: 403, Blocked: true},
					{Seq: 2, Method: "CONNECT", Host: "api.github.com:443", Status: 403, Blocked: true},
				},
			}},
			want: `gh version 1.0.0 - /bin/gh
$ gh issue view 1
exit code: 1
--- stdout
(empty)
--- stderr
error validating token: Get "https://api.github.com/": Forbidden
--- requests (2)
1. CONNECT api.github.com:443 -> blocked
2. CONNECT api.github.com:443 -> blocked
--- blocked, because with --localhost ghmock only serves github.localhost
CONNECT api.github.com:443
If gh picked that host because of --hostname or a URL, leave out --hostname and use github.localhost in URLs.
`,
		},
		{
			name: "gh rejects ghmock's certificate",
			results: []runResult{{
				path: "/bin/gh", version: "gh version 1.0.0", exitCode: 1,
				stderr: "x509: certificate signed by unknown authority\n",
				requests: []loggedRequest{
					{Seq: 1, Method: "CONNECT", Host: "api.github.com:443", TLSError: "remote error: tls: bad certificate"},
					{Seq: 2, Method: "CONNECT", Host: "api.github.com:443", TLSError: "remote error: tls: bad certificate"},
				},
			}},
			want: `gh version 1.0.0 - /bin/gh
$ gh issue view 1
exit code: 1
--- stdout
(empty)
--- stderr
x509: certificate signed by unknown authority
--- requests (2)
1. CONNECT api.github.com:443 -> TLS handshake failed
2. CONNECT api.github.com:443 -> TLS handshake failed
--- gh rejected ghmock's certificate
api.github.com:443: remote error: tls: bad certificate
gh must trust ghmock's certificate authority through SSL_CERT_FILE. Some builds ignore it, such as macOS builds made with older Go. Run those with --localhost.
`,
		},
		{
			name: "schema problems, off-schema responses and notes",
			results: []runResult{{
				path: "/bin/gh", version: "gh version 1.0.0",
				requests: []loggedRequest{
					withProblems(viewRequest),
					offSchema(withSeq(viewRequest, 2)),
					{Seq: 3, Method: "GET", Name: "GET /zen", Status: 200, Matched: true, Response: 1, Responses: 1, SchemaNote: "GitHub documents no JSON body for status 200"},
					{Seq: 4, Method: "POST", Name: "graphql Big", GraphQL: &graphQLRequest{Kind: "query", Operation: "Big"}, Status: 200, Matched: true, Response: 1, Responses: 1, SchemaProblems: numbered(12)},
				},
			}},
			notes: []string{"REST checks are off: downloads are off"},
			want: `gh version 1.0.0 - /bin/gh
$ gh issue view 1
exit code: 0
--- stdout
(empty)
--- stderr
(empty)
--- requests (4)
1. graphql IssueByNumber {"number":1} -> 200, response 1 of 1 [1 schema problem] [query: 1 schema problem]
2. graphql IssueByNumber {"number":1} -> 200, response 1 of 1 [off-schema]
3. GET /zen -> 200, response 1 of 1 [not checked: GitHub documents no JSON body for status 200]
4. graphql Big -> 200, response 1 of 1 [12 schema problems]
--- schema problems
1. graphql IssueByNumber: the response doesn't match GitHub's schema
   data.viewer.login: missing, but gh asked for it
1. graphql IssueByNumber: gh's query doesn't match GitHub's schema, so GitHub would answer with an error
   line 2, column 12: Cannot query field "nope" on type "User".
4. graphql Big: the response doesn't match GitHub's schema
   problem 1
   problem 2
   problem 3
   problem 4
   problem 5
   problem 6
   problem 7
   problem 8
   problem 9
   problem 10
   ...and 2 more; ghmock log --json shows them all
--- schema checks
REST checks are off: downloads are off
`,
		},
		{
			name: "two binaries that ask for different fields",
			results: []runResult{
				{path: "/old/gh", version: "gh version 1.0.0", requests: []loggedRequest{withQuery(viewRequest,
					`query IssueByNumber { repository { issue: issueOrPullRequest { ... on Issue { title body } } } }`)}},
				{path: "/new/gh", version: "gh version 2.0.0", requests: []loggedRequest{withQuery(viewRequest,
					`query IssueByNumber { repository { issue: issueOrPullRequest { ... on Issue { title subIssuesSummary { total completed } } } } }`)}},
			},
			want: `=== [1] gh version 1.0.0 - /old/gh (baseline)
$ gh issue view 1
exit code: 0
--- stdout
(empty)
--- stderr
(empty)
--- requests (1)
1. graphql IssueByNumber {"number":1} -> 200, response 1 of 1

=== [2] gh version 2.0.0 - /new/gh
$ gh issue view 1
exit code: 0
--- stdout
(empty)
--- stderr
(empty)
--- requests (1)
1. graphql IssueByNumber {"number":1} -> 200, response 1 of 1

=== differences from [1]
[2] gh version 2.0.0:
  graphql IssueByNumber asked for different fields:
    only [1]: data.repository.issue.body
    only [2]: data.repository.issue.subIssuesSummary
  graphql IssueByNumber sent different data:
    [1] query "query IssueByNumber { repository { issue: issueOrPullRequest { ... on Issue { title body } } } }" {"number":1}
    [2] query "query IssueByNumber { repository { issue: issueOrPullRequest { ... on Issue { title subIssuesSummary { total completed } } } } }" {"number":1}
`,
		},
		{
			name: "two binaries that match",
			results: []runResult{
				{path: "/a/gh", version: "gh version 1.0.0", stdout: "ok\n", requests: []loggedRequest{viewRequest}},
				{path: "/b/gh", version: "gh version 1.0.1", stdout: "ok\n", requests: []loggedRequest{viewRequest}},
			},
			want: `=== [1] gh version 1.0.0 - /a/gh (baseline)
$ gh issue view 1
exit code: 0
--- stdout
ok
--- stderr
(empty)
--- requests (1)
1. graphql IssueByNumber {"number":1} -> 200, response 1 of 1

=== [2] gh version 1.0.1 - /b/gh
$ gh issue view 1
exit code: 0
--- stdout
ok
--- stderr
(empty)
--- requests (1)
1. graphql IssueByNumber {"number":1} -> 200, response 1 of 1

=== differences from [1]
[2] gh version 1.0.1:
  same exit code, output and requests
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			printReport(&buf, opts, tt.results, tt.notes)
			if tt.wantDifferences != "" {
				_, differences, ok := strings.Cut(buf.String(), "=== differences from [1]\n")
				require.True(t, ok)
				assert.Equal(t, tt.wantDifferences, differences)
			} else {
				assert.Equal(t, tt.want, buf.String())
			}
		})
	}
}

func withSeq(e loggedRequest, seq int) loggedRequest {
	e.Seq = seq
	return e
}

func withQuery(e loggedRequest, query string) loggedRequest {
	gql := *e.GraphQL
	gql.Query = query
	e.GraphQL = &gql
	return e
}

func offSchema(e loggedRequest) loggedRequest {
	e.OffSchema = true
	return e
}

func numbered(n int) []string {
	var out []string
	for i := 1; i <= n; i++ {
		out = append(out, fmt.Sprintf("problem %d", i))
	}
	return out
}
