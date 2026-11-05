package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
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
	if os.Getenv("GHMOCK_FAKE_GH") == "1" {
		os.Exit(fakeGH(os.Args[1:]))
	}
	os.Exit(m.Run())
}

// fakeGH implements the few commands the run tests need.
func fakeGH(args []string) int {
	if len(args) == 0 {
		return 2
	}
	switch args[0] {
	case "--version":
		fmt.Printf("gh version 0.0.0-%s\n", os.Getenv("GHMOCK_FAKE_VARIANT"))
	case "env":
		home := os.Getenv("HOME")
		if strings.Contains(home, "ghmock-run-") {
			home = "<scratch>/" + filepath.Base(home)
		}
		wd, _ := os.Getwd()
		if strings.Contains(wd, "ghmock-run-") {
			wd = "<scratch>/" + filepath.Base(wd)
		}
		for _, kv := range [][2]string{
			{"GH_HOST", os.Getenv("GH_HOST")},
			{"GH_TOKEN", os.Getenv("GH_TOKEN")},
			{"GITHUB_TOKEN", os.Getenv("GITHUB_TOKEN")},
			{"GH_FORCE_TTY", os.Getenv("GH_FORCE_TTY")},
			{"HOME", home},
			{"PWD", wd},
		} {
			fmt.Printf("%s=%s\n", kv[0], kv[1])
		}
	case "stdin":
		_, _ = io.Copy(os.Stdout, os.Stdin)
	case "exit":
		code, _ := strconv.Atoi(args[1])
		fmt.Fprintln(os.Stderr, "failing on purpose")
		return code
	case "sleep":
		time.Sleep(30 * time.Second)
	case "api":
		resp, err := http.Get("http://api.github.localhost/" + strings.TrimPrefix(args[1], "/"))
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
	default:
		fmt.Fprintf(os.Stderr, "fake gh: unknown command %q\n", args[0])
		return 2
	}
	return 0
}

// writeFakeGH writes a wrapper that runs the test binary as gh.
func writeFakeGH(t *testing.T, variant string) string {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "gh-"+variant)
	script := fmt.Sprintf("#!/bin/sh\nGHMOCK_FAKE_GH=1 GHMOCK_FAKE_VARIANT=%q exec %q \"$@\"\n", variant, exe)
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
			name: "every flag",
			args: []string{"--gh", "a", "--gh", "b", "--tty", "--logged-out", "--stdin", "token", "--timeout", "5s", "--", "auth", "login"},
			want: runOptions{
				binaries:  []string{"a", "b"},
				tty:       true,
				stdin:     "token",
				loggedOut: true,
				timeout:   5 * time.Second,
				args:      []string{"auth", "login"},
			},
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
	if runtime.GOOS == "windows" {
		t.Skip("the fake gh wrapper needs /bin/sh")
	}

	// resultSummary is the part of a run result that tests compare.
	type resultSummary struct {
		Version  string
		ExitCode int
		TimedOut bool
		Stdout   string
		Stderr   string
		Requests []logSummary
	}

	tests := []struct {
		name      string
		variants  []string
		responses responseSet
		opts      runOptions
		want      []resultSummary
	}{
		{
			name:     "gh gets an isolated environment with a fake token",
			variants: []string{"a"},
			opts:     runOptions{args: []string{"env"}},
			want: []resultSummary{{
				Version: "gh version 0.0.0-a",
				Stdout: "GH_HOST=github.localhost\nGH_TOKEN=ghmock-fake-token\nGITHUB_TOKEN=\n" +
					"GH_FORCE_TTY=\nHOME=<scratch>/home\nPWD=<scratch>/work\n",
			}},
		},
		{
			name:     "--logged-out drops the token and --tty forces a terminal",
			variants: []string{"a"},
			opts:     runOptions{args: []string{"env"}, loggedOut: true, tty: true},
			want: []resultSummary{{
				Version: "gh version 0.0.0-a",
				Stdout: "GH_HOST=github.localhost\nGH_TOKEN=\nGITHUB_TOKEN=\n" +
					"GH_FORCE_TTY=120\nHOME=<scratch>/home\nPWD=<scratch>/work\n",
			}},
		},
		{
			name:     "--stdin is passed to gh",
			variants: []string{"a"},
			opts:     runOptions{args: []string{"stdin"}, stdin: "secret-token"},
			want:     []resultSummary{{Version: "gh version 0.0.0-a", Stdout: "secret-token"}},
		},
		{
			name:     "gh's exit code and stderr are reported",
			variants: []string{"a"},
			opts:     runOptions{args: []string{"exit", "3"}},
			want:     []resultSummary{{Version: "gh version 0.0.0-a", ExitCode: 3, Stderr: "failing on purpose\n"}},
		},
		{
			name:     "a run that takes too long is stopped",
			variants: []string{"a"},
			opts:     runOptions{args: []string{"sleep"}, timeout: 300 * time.Millisecond},
			want:     []resultSummary{{Version: "gh version 0.0.0-a", ExitCode: -1, TimedOut: true}},
		},
		{
			name:      "requests get stored responses",
			variants:  []string{"a"},
			responses: responseSet{"GET /repos/octo/hello": {{Body: json.RawMessage(`{"name":"hello"}`)}}},
			opts:      runOptions{args: []string{"api", "repos/octo/hello"}},
			want: []resultSummary{{
				Version:  "gh version 0.0.0-a",
				Stdout:   "200 {\"name\":\"hello\"}\n",
				Requests: []logSummary{{Seq: 1, Name: "GET /repos/octo/hello", Status: 200, Matched: true, Response: 1, Responses: 1}},
			}},
		},
		{
			name:     "each binary starts from the first response",
			variants: []string{"a", "b"},
			responses: responseSet{"GET /flaky": {
				{Status: 502, Body: json.RawMessage(`{"message":"bad gateway"}`)},
				{Body: json.RawMessage(`{"ok":true}`)},
			}},
			opts: runOptions{args: []string{"api", "flaky"}},
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Neither should reach gh.
			t.Setenv("GH_TOKEN", "caller-token")
			t.Setenv("GITHUB_TOKEN", "caller-token")

			st := &state{dir: t.TempDir()}
			if tt.responses != nil {
				require.NoError(t, st.saveResponses(tt.responses))
			}
			opts := tt.opts
			if opts.timeout == 0 {
				opts.timeout = 30 * time.Second
			}
			for _, v := range tt.variants {
				opts.binaries = append(opts.binaries, writeFakeGH(t, v))
			}

			results, err := runAll(st, opts)
			require.NoError(t, err)

			var got []resultSummary
			for _, r := range results {
				got = append(got, resultSummary{
					Version:  r.version,
					ExitCode: r.exitCode,
					TimedOut: r.timedOut,
					Stdout:   r.stdout,
					Stderr:   r.stderr,
					Requests: summarize(r.requests),
				})
			}
			assert.Equal(t, tt.want, got)

			logged, err := st.readLog()
			require.NoError(t, err)
			var wantLogged []logSummary
			for _, w := range tt.want {
				wantLogged = append(wantLogged, w.Requests...)
			}
			assert.Equal(t, wantLogged, summarize(logged))
			for _, e := range logged {
				assert.Equal(t, logged[0].Run, e.Run, "one run ID per ghmock run")
			}
		})
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

	tests := []struct {
		name    string
		results []runResult
		want    string
	}{
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
			printReport(&buf, opts, tt.results)
			assert.Equal(t, tt.want, buf.String())
		})
	}
}

func withSeq(e loggedRequest, seq int) loggedRequest {
	e.Seq = seq
	return e
}
