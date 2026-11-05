package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLogArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    logOptions
		wantErr string
	}{
		{name: "no flags", want: logOptions{}},
		{name: "every flag", args: []string{"--all", "--full", "--json"}, want: logOptions{all: true, full: true, asJSON: true}},
		{name: "extra arguments", args: []string{"extra"}, wantErr: "unexpected arguments: extra"},
		{name: "unknown flag", args: []string{"--nope"}, wantErr: "flag provided but not defined: -nope"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLogArgs(tt.args)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCmdLog(t *testing.T) {
	first := loggedRequest{Run: "run-1", Binary: "./old", Seq: 1, Method: "GET", Name: "GET /user", Status: 200, Matched: true, Response: 1, Responses: 1}
	query := loggedRequest{
		Run: "run-2", Binary: "./new", Seq: 1, Method: "POST", Name: "graphql Viewer",
		GraphQL: &graphQLRequest{Kind: "query", Operation: "Viewer", Query: "query Viewer {\n  viewer { login }\n}", Variables: json.RawMessage(`{"a": 1}`)},
		Status:  200, Matched: true, Response: 1, Responses: 1,
	}
	write := loggedRequest{Run: "run-2", Binary: "./new", Seq: 2, Method: "POST", Name: "POST /repos/o/r/issues", Body: `{"title": "Bug"}`, Status: 404}
	raw := []byte{0, 255}

	tests := []struct {
		name    string
		entries []loggedRequest
		runs    []runRecord
		args    []string
		want    string
		large   bool
	}{
		{
			name:    "the latest run can have no requests",
			entries: []loggedRequest{first},
			runs:    []runRecord{{ID: "run-1"}, {ID: "run-2"}},
			want:    "No requests logged.\n",
		},
		{
			name:    "history remains available after a request-free run",
			entries: []loggedRequest{first},
			runs:    []runRecord{{ID: "run-1"}, {ID: "run-2"}},
			args:    []string{"--all"},
			want:    "=== run run-1, gh ./old\n1. GET /user -> 200, response 1 of 1\n",
		},
		{
			name:  "raw response larger than the old JSONL scanner limit",
			args:  []string{"--full"},
			large: true,
		},
		{
			name:    "raw bytes are retained in JSON",
			entries: []loggedRequest{{Seq: 1, Method: "POST", Name: "POST /binary", RawBody: &raw, ResponseRawBody: &raw, Status: 200}},
			args:    []string{"--json"},
			want:    `{"run":"","binary":"","seq":1,"method":"POST","host":"","name":"POST /binary","raw_body":"AP8=","status":200,"matched":false,"response_raw_body":"AP8="}` + "\n",
		},
		{
			name:    "raw bytes are summarized safely in full logs",
			entries: []loggedRequest{{Seq: 1, Method: "POST", Name: "POST /binary", RawBody: &raw, ResponseRawBody: &raw, Status: 200}},
			args:    []string{"--full"},
			want:    fmt.Sprintf("1. POST /binary raw body 2 bytes, sha256 %[1]x -> 200, no response stored\n   raw request body: 2 bytes, sha256 %[1]x (bytes in --json)\n   raw response body: 2 bytes, sha256 %[1]x (bytes in --json)\n", sha256.Sum256(raw)),
		},
		{
			name: "nothing logged",
			want: "No requests logged.\n",
		},
		{
			name:    "only the last run by default",
			entries: []loggedRequest{first, query, write},
			want: `=== run run-2, gh ./new
1. graphql Viewer {"a":1} -> 200, response 1 of 1
2. POST /repos/o/r/issues body {"title":"Bug"} -> 404, no response stored
`,
		},
		{
			name:    "every run",
			entries: []loggedRequest{first, query},
			args:    []string{"--all"},
			want: `=== run run-1, gh ./old
1. GET /user -> 200, response 1 of 1
=== run run-2, gh ./new
1. graphql Viewer {"a":1} -> 200, response 1 of 1
`,
		},
		{
			name:    "full details",
			entries: []loggedRequest{query, write},
			args:    []string{"--full"},
			want: `=== run run-2, gh ./new
1. graphql Viewer {"a":1} -> 200, response 1 of 1
   query Viewer
   variables: {"a":1}
   query:
     query Viewer {
       viewer { login }
     }
2. POST /repos/o/r/issues body {"title":"Bug"} -> 404, no response stored
   body: {"title":"Bug"}
`,
		},
		{
			name: "full details show the response gh got",
			entries: []loggedRequest{{
				Run: "run-3", Binary: "./new", Seq: 1, Method: "GET", Name: "GET /", Status: 200, Matched: true, Response: 1, Responses: 1,
				ResponseHeaders: map[string]string{"X-Oauth-Scopes": "repo", "Link": "<next>"},
				ResponseBody:    json.RawMessage(`{"current_user_url": "https://api.github.com/user"}`),
			}},
			args: []string{"--full"},
			want: `=== run run-3, gh ./new
1. GET / -> 200, response 1 of 1
   response headers: Link: <next>, X-Oauth-Scopes: repo
   response: {"current_user_url":"https://api.github.com/user"}
`,
		},
		{
			name:    "full details list schema problems",
			entries: []loggedRequest{withProblems(query)},
			args:    []string{"--full"},
			want: `=== run run-2, gh ./new
1. graphql Viewer {"a":1} -> 200, response 1 of 1 [1 schema problem] [query: 1 schema problem]
   query Viewer
   variables: {"a":1}
   query:
     query Viewer {
       viewer { login }
     }
   the response doesn't match GitHub's schema:
     data.viewer.login: missing, but gh asked for it
   gh's query doesn't match GitHub's schema, so GitHub would answer with an error:
     line 2, column 12: Cannot query field "nope" on type "User".
`,
		},
		{
			name:    "JSON lines",
			entries: []loggedRequest{first},
			args:    []string{"--json"},
			want:    `{"run":"run-1","binary":"./old","seq":1,"method":"GET","host":"","name":"GET /user","status":200,"matched":true,"response":1,"responses":1}` + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("GHMOCK_DIR", dir)
			if tt.large {
				data := make([]byte, 48<<20)
				tt.entries = []loggedRequest{{Seq: 1, Method: "GET", Name: "GET /artifact", Status: 200, ResponseRawBody: &data}}
				tt.want = fmt.Sprintf("1. GET /artifact -> 200, no response stored\n   raw response body: %d bytes, sha256 %x (bytes in --json)\n", len(data), sha256.Sum256(data))
			}
			require.NoError(t, (&state{dir: dir}).appendLog(tt.entries))
			for _, run := range tt.runs {
				require.NoError(t, (&state{dir: dir}).appendRun(run))
			}

			var out bytes.Buffer
			require.NoError(t, cmdLog(tt.args, &out))
			assert.Equal(t, tt.want, out.String())
		})
	}
}

func TestRunMain(t *testing.T) {
	t.Setenv("GHMOCK_EXPORT_PATH", "")
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "help",
			args:       []string{"help"},
			wantStdout: usage,
		},
		{
			name:       "help flag on a command",
			args:       []string{"set", "--help"},
			wantStdout: usage,
		},
		{
			name:       "help flag after a request name",
			args:       []string{"scaffold", "graphql IssueByNumber", "-h"},
			wantStdout: usage,
		},
		{
			name:       "help flag on reset deletes nothing",
			args:       []string{"reset", "--help"},
			wantStdout: usage,
		},
		{
			name:       "help flag on schemas update",
			args:       []string{"schemas", "update", "--help"},
			wantStdout: usage,
		},
		{
			name:       "help flag after -- belongs to gh",
			args:       []string{"run", "--", "--help"},
			wantCode:   2,
			wantStderr: "ghmock run: at least one --gh is required\n",
		},
		{
			name:       "no command",
			wantCode:   2,
			wantStderr: usage,
		},
		{
			name:       "unknown command",
			args:       []string{"nope"},
			wantCode:   2,
			wantStderr: "ghmock: unknown command \"nope\"\n\n" + usage,
		},
		{
			name:       "usage error",
			args:       []string{"run", "--", "issue", "list"},
			wantCode:   2,
			wantStderr: "ghmock run: at least one --gh is required\n",
		},
		{
			name:       "failure that isn't a usage error",
			args:       []string{"set", "GET /x", "--body-file", "does-not-exist.json"},
			wantCode:   1,
			wantStderr: "ghmock set: open does-not-exist.json: no such file or directory\n",
		},
		{
			name:       "schema",
			args:       []string{"schema"},
			wantCode:   2,
			wantStderr: "ghmock schema: missing name, such as Issue, Issue.title or \"GET /repos/octo/hello\"\n",
		},
		{
			name:       "scaffold",
			args:       []string{"scaffold"},
			wantCode:   2,
			wantStderr: "ghmock scaffold: pass a request name, such as \"graphql IssueByNumber\", or --missing\n",
		},
		{
			name:       "export",
			args:       []string{"export"},
			wantCode:   2,
			wantStderr: "ghmock export: missing folder to export to\n",
		},
		{
			name:       "schemas",
			args:       []string{"schemas"},
			wantCode:   2,
			wantStderr: "ghmock schemas: missing subcommand; use ghmock schemas update\n",
		},
		{
			name:       "reset",
			args:       []string{"reset"},
			wantStdout: "Deleted responses, request log and saved runs in <dir>\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("GHMOCK_DIR", dir)

			var stdout, stderr bytes.Buffer
			code := runMain(tt.args, &stdout, &stderr)

			assert.Equal(t, tt.wantCode, code)
			assert.Equal(t, tt.wantStdout, string(bytes.ReplaceAll(stdout.Bytes(), []byte(dir), []byte("<dir>"))))
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}

func TestCmdSetAndReset(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GHMOCK_DIR", dir)
	useTestSchemas(t)
	st := &state{dir: dir}

	var out bytes.Buffer
	require.NoError(t, cmdSet([]string{"GET /user", "--status", "401", "--header", "X-GitHub-SSO: required"}, &out, io.Discard))
	assert.Equal(t, "GET /user: response 1 of 1: status 401, headers X-Github-Sso, body 0 bytes\n"+
		"Schema check: skipped, GitHub doesn't document status 401 for GET /user\n", out.String())

	responses, err := st.loadResponses()
	require.NoError(t, err)
	assert.Equal(t, responseSet{"GET /user": {{Status: 401, Headers: map[string]string{"X-Github-Sso": "required"}}}}, responses)

	require.NoError(t, st.appendLog([]loggedRequest{{Run: "r", Name: "GET /user"}}))
	require.NoError(t, st.appendRun(runRecord{ID: "r"}))
	out.Reset()
	require.NoError(t, cmdReset(nil, &out))
	responses, err = st.loadResponses()
	require.NoError(t, err)
	assert.Empty(t, responses)
	entries, err := st.readLog()
	require.NoError(t, err)
	assert.Empty(t, entries)
	runs, err := st.readRuns()
	require.NoError(t, err)
	assert.Empty(t, runs)
}

// withProblems adds schema problems to a GraphQL request for report tests.
func withProblems(e loggedRequest) loggedRequest {
	e.SchemaProblems = []string{"data.viewer.login: missing, but gh asked for it"}
	e.QueryProblems = []string{`line 2, column 12: Cannot query field "nope" on type "User".`}
	return e
}
