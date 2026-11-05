package main

import (
	"bytes"
	"encoding/json"
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

	tests := []struct {
		name    string
		entries []loggedRequest
		args    []string
		want    string
	}{
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
			require.NoError(t, (&state{dir: dir}).appendLog(tt.entries))

			var out bytes.Buffer
			require.NoError(t, cmdLog(tt.args, &out))
			assert.Equal(t, tt.want, out.String())
		})
	}
}

func TestRunMain(t *testing.T) {
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
			name:       "reset",
			args:       []string{"reset"},
			wantStdout: "Deleted responses and request log in <dir>\n",
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
	st := &state{dir: dir}

	var out bytes.Buffer
	require.NoError(t, cmdSet([]string{"GET /user", "--status", "401", "--header", "X-GitHub-SSO: required"}, &out))
	assert.Equal(t, "GET /user: response 1 of 1: status 401, headers X-Github-Sso, body 0 bytes\n", out.String())

	responses, err := st.loadResponses()
	require.NoError(t, err)
	assert.Equal(t, responseSet{"GET /user": {{Status: 401, Headers: map[string]string{"X-Github-Sso": "required"}}}}, responses)

	require.NoError(t, st.appendLog([]loggedRequest{{Run: "r", Name: "GET /user"}}))
	out.Reset()
	require.NoError(t, cmdReset(nil, &out))
	responses, err = st.loadResponses()
	require.NoError(t, err)
	assert.Empty(t, responses)
	entries, err := st.readLog()
	require.NoError(t, err)
	assert.Empty(t, entries)
}
