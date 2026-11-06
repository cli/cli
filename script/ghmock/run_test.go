package main

import (
	"bytes"
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
		fmt.Printf("gh version 0.0.0-%s\n", os.Getenv("GHMOCK_FAKE_VARIANT"))
	case "env":
		wd, _ := os.Getwd()
		for _, kv := range [][2]string{
			{"GH_HOST", os.Getenv("GH_HOST")},
			{"GH_TOKEN", os.Getenv("GH_TOKEN")},
			{"GITHUB_TOKEN", os.Getenv("GITHUB_TOKEN")},
			{"GH_FORCE_TTY", os.Getenv("GH_FORCE_TTY")},
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
		name         string
		variants     []string
		responses    responseSet
		checkSchemas bool
		// builds sets buildsDir: "own" is the folder holding the fake gh,
		// and "other" is a different folder.
		builds      string
		opts        runOptions
		want        []resultSummary
		wantCommand string
		wantErr     string
		afterAPI    bool
		resolveOnly bool
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
			name:        "gh gets an isolated environment, logged in with a fake token",
			variants:    []string{"a"},
			opts:        runOptions{args: []string{"env"}},
			wantCommand: "ghmock run --gh <gh-a> --timeout 30s -- env",
			want: []resultSummary{{
				Version: "gh version 0.0.0-a",
				Stdout: `GH_HOST=github.localhost
GH_TOKEN=
GITHUB_TOKEN=
GH_FORCE_TTY=
HOME=<scratch>/home
PWD=<scratch>/work
--- <scratch>/config/config.yml
version: "1"
--- <scratch>/config/hosts.yml
github.localhost:
    users:
        monalisa:
            oauth_token: ghmock-fake-token
    git_protocol: https
    oauth_token: ghmock-fake-token
    user: monalisa
`,
			}},
		},
		{
			name:        "--logged-out skips the login and --tty forces a terminal",
			variants:    []string{"a"},
			opts:        runOptions{args: []string{"env"}, loggedOut: true, tty: true},
			wantCommand: "ghmock run --gh <gh-a> --tty --logged-out --timeout 30s -- env",
			want: []resultSummary{{
				Version: "gh version 0.0.0-a",
				Stdout: `GH_HOST=github.localhost
GH_TOKEN=
GITHUB_TOKEN=
GH_FORCE_TTY=120
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
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Equal(t, tt.wantErr, replacer.Replace(err.Error()))
				return
			}
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
			for i, b := range opts.binaries {
				command = strings.ReplaceAll(command, b, "<gh-"+tt.variants[i]+">")
			}
			assert.Equal(t, tt.wantCommand, command)
			var outcomes []runOutcome
			for i, r := range results {
				outcomes = append(outcomes, runOutcome{
					Binary: opts.binaries[i], Path: r.path, Version: r.version, ExitCode: r.exitCode,
					TimedOut: r.timedOut, Stdout: r.stdout, Stderr: r.stderr,
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
1. graphql IssueByNumber: gh's query doesn't match GitHub's schema
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
