package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSetArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    setOptions
		wantErr string
	}{
		{
			name: "name first, then flags",
			args: []string{"GET /repos/octo/hello", "--status", "404", "--header", "X-A: 1", "--header", "X-B:"},
			want: setOptions{name: "GET /repos/octo/hello", status: 404, headers: []string{"X-A: 1", "X-B:"}},
		},
		{
			name: "flags first, then name",
			args: []string{"--body", `{"a":1}`, "graphql IssueByNumber"},
			want: setOptions{name: "graphql IssueByNumber", body: `{"a":1}`, bodySet: true},
		},
		{
			name: "empty body clears it",
			args: []string{"GET /", "--body", ""},
			want: setOptions{name: "GET /", bodySet: true},
		},
		{
			name: "path edit on a chosen response",
			args: []string{"graphql IssueByNumber", "--index", "2", "--at", "data.repository.issue.author", "--json", "null"},
			want: setOptions{name: "graphql IssueByNumber", index: 2, at: "data.repository.issue.author", value: "null", valueSet: true},
		},
		{
			name: "append with no other changes",
			args: []string{"GET /flaky", "--append"},
			want: setOptions{name: "GET /flaky", appendNew: true},
		},
		{
			name: "host-qualified REST name",
			args: []string{"POST uploads.github.localhost/upload", "--status", "500"},
			want: setOptions{name: "POST uploads.github.localhost/upload", status: 500},
		},
		{
			name: "--off-schema alone is a change",
			args: []string{"GET /", "--off-schema"},
			want: setOptions{name: "GET /", offSchema: true},
		},
		{
			name:    "unquoted REST name",
			args:    []string{"GET", "/repos/octo/hello", "--status", "404"},
			wantErr: "unexpected arguments: /repos/octo/hello --status 404; quote the request name, such as 'GET /repos/octo/hello'",
		},
		{
			name:    "missing name",
			args:    []string{"--status", "404"},
			wantErr: `missing request name, such as "GET /repos/octo/hello" or "graphql IssueByNumber"`,
		},
		{
			name:    "lowercase method",
			args:    []string{"get /repos", "--status", "404"},
			wantErr: `request name "get /repos" must start with an uppercase HTTP method or "graphql"`,
		},
		{
			name:    "REST name without a path",
			args:    []string{"GET repos", "--status", "404"},
			wantErr: `request name "GET repos" must include a path, such as /repos/octo/hello`,
		},
		{
			name:    "name without a target",
			args:    []string{"graphql", "--status", "404"},
			wantErr: `request name "graphql" must look like "GET /repos/octo/hello" or "graphql IssueByNumber"`,
		},
		{
			name:    "body and body file",
			args:    []string{"GET /", "--body", "{}", "--body-file", "x.json"},
			wantErr: "use either --body or --body-file, not both",
		},
		{
			name:    "--at without --json",
			args:    []string{"GET /", "--at", "a.b"},
			wantErr: "--at needs --json",
		},
		{
			name:    "--json without --at",
			args:    []string{"GET /", "--json", "1"},
			wantErr: "--json needs --at",
		},
		{
			name:    "append and index",
			args:    []string{"GET /", "--append", "--index", "1"},
			wantErr: "use either --append or --index, not both",
		},
		{
			name:    "negative index",
			args:    []string{"GET /", "--index", "-1", "--status", "200"},
			wantErr: "--index starts at 1",
		},
		{
			name:    "status out of range",
			args:    []string{"GET /", "--status", "99"},
			wantErr: "--status must be between 100 and 599",
		},
		{
			name:    "header without a colon",
			args:    []string{"GET /", "--header", "X-A"},
			wantErr: `--header "X-A" must look like 'Name: value'`,
		},
		{
			name:    "nothing to change",
			args:    []string{"GET /"},
			wantErr: "nothing to change; pass --status, --header, --body, --body-file, --at or --off-schema",
		},
		{
			name:    "extra arguments",
			args:    []string{"GET /", "--status", "200", "extra"},
			wantErr: "unexpected arguments: extra",
		},
		{
			name:    "unknown flag",
			args:    []string{"GET /", "--nope"},
			wantErr: "flag provided but not defined: -nope",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSetArgs(tt.args)
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

func TestApplySet(t *testing.T) {
	tests := []struct {
		name      string
		responses string
		opts      setOptions
		wantPos   int
		want      string
		wantErr   string
	}{
		{
			name:      "--json rejects a trailing array delimiter",
			responses: `{}`,
			opts:      setOptions{name: "graphql A", at: "a", value: `1]`},
			wantErr:   "--json is not valid JSON: unexpected data after the JSON value",
		},
		{
			name:      "--json rejects a trailing object delimiter",
			responses: `{}`,
			opts:      setOptions{name: "graphql A", at: "a", value: `{"x":1}}`},
			wantErr:   "--json is not valid JSON: unexpected data after the JSON value",
		},
		{
			name:      "--json rejects multiple values",
			responses: `{}`,
			opts:      setOptions{name: "graphql A", at: "a", value: `1 2`},
			wantErr:   "--json is not valid JSON: unexpected data after the JSON value",
		},
		{
			name:      "--json accepts trailing whitespace",
			responses: `{}`,
			opts:      setOptions{name: "graphql A", at: "a", value: "1 \n\t"},
			want:      `[{"body":{"a":1}}]`,
		},
		{
			name:      "creates the first response",
			responses: `{}`,
			opts:      setOptions{name: "GET /x", status: 404, body: `{"message":"Not Found"}`, bodySet: true},
			want:      `[{"status":404,"body":{"message":"Not Found"}}]`,
		},
		{
			name:      "changes only the parts passed",
			responses: `{"GET /x":[{"status":201,"headers":{"X-A":"1"},"body":{"a":1}}]}`,
			opts:      setOptions{name: "GET /x", headers: []string{"x-b: 2"}},
			want:      `[{"status":201,"headers":{"X-A":"1","X-B":"2"},"body":{"a":1}}]`,
		},
		{
			name:      "an empty header value removes the header",
			responses: `{"GET /x":[{"headers":{"X-A":"1"}}]}`,
			opts:      setOptions{name: "GET /x", headers: []string{"x-a:"}},
			want:      `[{}]`,
		},
		{
			name:      "an empty body clears it",
			responses: `{"GET /x":[{"body":{"a":1}}]}`,
			opts:      setOptions{name: "GET /x", bodySet: true},
			want:      `[{}]`,
		},
		{
			name:      "edits the response chosen by index",
			responses: `{"GET /x":[{"status":500},{"status":200}]}`,
			opts:      setOptions{name: "GET /x", index: 2, status: 204},
			wantPos:   1,
			want:      `[{"status":500},{"status":204}]`,
		},
		{
			name:      "append adds a response after the others",
			responses: `{"GET /x":[{"status":500}]}`,
			opts:      setOptions{name: "GET /x", appendNew: true, body: `{"ok":true}`, bodySet: true},
			wantPos:   1,
			want:      `[{"status":500},{"body":{"ok":true}}]`,
		},
		{
			name:      "--at creates missing objects",
			responses: `{"graphql A":[{"body":{"data":{}}}]}`,
			opts:      setOptions{name: "graphql A", at: "data.repository.issue.title", value: `"Broken"`},
			want:      `[{"body":{"data":{"repository":{"issue":{"title":"Broken"}}}}}]`,
		},
		{
			name:      "--at indexes arrays",
			responses: `{"graphql A":[{"body":{"items":[{"n":1.0},{"n":2}]}}]}`,
			opts:      setOptions{name: "graphql A", at: "items.1.n", value: `5`},
			want:      `[{"body":{"items":[{"n":1.0},{"n":5}]}}]`,
		},
		{
			name:      "--at can set null",
			responses: `{"graphql A":[{"body":{"author":{"login":"monalisa"}}}]}`,
			opts:      setOptions{name: "graphql A", at: "author", value: `null`},
			want:      `[{"body":{"author":null}}]`,
		},
		{
			name:      "--at replaces null with an object",
			responses: `{"graphql A":[{"body":{"author":null}}]}`,
			opts:      setOptions{name: "graphql A", at: "author.login", value: `"hubot"`},
			want:      `[{"body":{"author":{"login":"hubot"}}}]`,
		},
		{
			name:      "--at with an index past the end of an array",
			responses: `{"graphql A":[{"body":{"items":[1,2]}}]}`,
			opts:      setOptions{name: "graphql A", at: "items.5", value: `3`},
			wantErr:   `items: "5" is not an index of this 2-item array`,
		},
		{
			name:      "--at inside a string",
			responses: `{"graphql A":[{"body":{"title":"x"}}]}`,
			opts:      setOptions{name: "graphql A", at: "title.length", value: `1`},
			wantErr:   `title: cannot set "length" inside a string`,
		},
		{
			name:      "--at with an empty segment",
			responses: `{}`,
			opts:      setOptions{name: "graphql A", at: "a..b", value: `1`},
			wantErr:   `--at "a..b" has an empty segment`,
		},
		{
			name:      "--json that isn't JSON",
			responses: `{}`,
			opts:      setOptions{name: "graphql A", at: "a", value: `nope`},
			wantErr:   "--json is not valid JSON: invalid character 'o' in literal null (expecting 'u')",
		},
		{
			name:      "body that isn't JSON",
			responses: `{}`,
			opts:      setOptions{name: "GET /x", body: `{"a":`, bodySet: true},
			wantErr:   "body is not valid JSON",
		},
		{
			name:      "index past the end",
			responses: `{"GET /x":[{"status":200}]}`,
			opts:      setOptions{name: "GET /x", index: 3, status: 500},
			wantErr:   "GET /x has 1 response(s); use --append to add another",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			responses := responseSet{}
			require.NoError(t, json.Unmarshal([]byte(tt.responses), &responses))

			pos, err := applySet(responses, tt.opts)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)

			assert.Equal(t, tt.wantPos, pos)
			got, err := json.Marshal(responses[tt.opts.name])
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(got))
		})
	}
}

func TestCmdSet(t *testing.T) {
	const issueQuery = `query IssueTitle { repository(owner: "o", name: "r") { issue(number: 1) { title state } } }`
	logged := []loggedRequest{{
		Run: "r", Seq: 1, Method: "POST", Host: apiHost, Path: "/graphql", Name: "graphql IssueTitle",
		GraphQL: &graphQLRequest{Kind: "query", Operation: "IssueTitle", Query: issueQuery},
	}}
	const validIssue = `{"id":1,"number":1,"title":"Bug","state":"open","user":null,"labels":[],"created_at":"2026-01-01T00:00:00Z"}`
	const validTitle = `{"data":{"repository":{"issue":{"title":"Bug","state":"OPEN"}}}}`
	const badTitle = `{"data":{"repository":{"issue":{"title":"Bug","state":"open"}}}}`

	tests := []struct {
		name       string
		noSchemas  bool
		responses  string
		logged     []loggedRequest
		args       []string
		wantStdout string
		wantErr    string
		want       string
	}{
		{
			name: "response-only saves retain unknown conditional selections",
			logged: []loggedRequest{{
				Run: "r", Seq: 1, Method: "POST", Host: apiHost, Path: "/graphql", Name: "graphql Conditional",
				GraphQL: &graphQLRequest{Kind: "query", Operation: "Conditional", Query: `query Conditional($x:Boolean!) { viewer { login } viewer @include(if:$x) { name } }`},
			}},
			args:       []string{"graphql Conditional", "--body", `{"data":{"viewer":{"login":"monalisa"}}}`},
			wantStdout: "graphql Conditional: response 1 of 1: status 200, no headers, body 40 bytes\nSchema check: passed\n",
			want:       `{"graphql Conditional":[{"body":{"data":{"viewer":{"login":"monalisa"}}}}]}`,
		},
		{
			name: "response-only saves do not assume an operation default was used",
			logged: []loggedRequest{{
				Run: "r", Seq: 1, Method: "POST", Host: apiHost, Path: "/graphql", Name: "graphql Conditional",
				GraphQL: &graphQLRequest{Kind: "query", Operation: "Conditional", Query: `query Conditional($x:Boolean = true) { viewer { login } viewer @include(if:$x) { name } }`},
			}},
			args:       []string{"graphql Conditional", "--body", `{"data":{"viewer":{"login":"monalisa"}}}`},
			wantStdout: "graphql Conditional: response 1 of 1: status 200, no headers, body 40 bytes\nSchema check: passed\n",
			want:       `{"graphql Conditional":[{"body":{"data":{"viewer":{"login":"monalisa"}}}}]}`,
		},
		{
			name: "response-only saves select the named operation",
			logged: []loggedRequest{{
				Run: "r", Seq: 1, Method: "POST", Host: apiHost, Path: "/graphql", Name: "graphql IssueTitle",
				GraphQL: &graphQLRequest{Kind: "query", Operation: "IssueTitle", Query: `query Other { viewer { login } } ` + issueQuery},
			}},
			args:       []string{"graphql IssueTitle", "--body", validTitle},
			wantStdout: fmt.Sprintf("graphql IssueTitle: response 1 of 1: status 200, no headers, body %d bytes\nSchema check: passed\n", len(validTitle)),
			want:       `{"graphql IssueTitle":[{"body":` + validTitle + `}]}`,
		},
		{
			name:       "a REST response that matches GitHub's description",
			args:       []string{"GET /repos/o/r/issues/1", "--body", validIssue},
			wantStdout: fmt.Sprintf("GET /repos/o/r/issues/1: response 1 of 1: status 200, no headers, body %d bytes\nSchema check: passed\n", len(validIssue)),
			want:       `{"GET /repos/o/r/issues/1":[{"body":` + validIssue + `}]}`,
		},
		{
			name: "a REST response that doesn't match isn't saved",
			args: []string{"GET /repos/o/r/issues/1", "--body", `{"id":"1","number":1,"title":"Bug","state":"opened","user":null,"labels":[]}`},
			wantErr: "the response doesn't match GitHub's schema, so it wasn't saved:\n" +
				"  id: value must be an integer\n" +
				"  state: value is not one of the allowed values [\"open\",\"closed\"]\n" +
				"  created_at: missing, but GitHub's description requires it\n" +
				"Fix it, or pass --off-schema to save it anyway",
			want: `{}`,
		},
		{
			name: "--off-schema saves it anyway, labels it and lists the problems",
			args: []string{"GET /repos/o/r/issues/1", "--body", `{"id":"1"}`, "--off-schema"},
			wantStdout: "GET /repos/o/r/issues/1: response 1 of 1: status 200, no headers, body 10 bytes, off-schema\n" +
				"Schema check: failed, saved as off-schema:\n" +
				"  id: value must be an integer\n" +
				"  number: missing, but GitHub's description requires it\n" +
				"  title: missing, but GitHub's description requires it\n" +
				"  state: missing, but GitHub's description requires it\n" +
				"  user: missing, but GitHub's description requires it\n" +
				"  labels: missing, but GitHub's description requires it\n" +
				"  created_at: missing, but GitHub's description requires it\n",
			want: `{"GET /repos/o/r/issues/1":[{"body":{"id":"1"},"off_schema":true}]}`,
		},
		{
			name: "--off-schema on a response that passes says the flag isn't needed",
			args: []string{"GET /repos/o/r/issues/1", "--body", validIssue, "--off-schema"},
			wantStdout: fmt.Sprintf("GET /repos/o/r/issues/1: response 1 of 1: status 200, no headers, body %d bytes, off-schema\n", len(validIssue)) +
				"Schema check: passed, but saved as off-schema as asked. It matches GitHub's schema, so it doesn't need --off-schema\n",
			want: `{"GET /repos/o/r/issues/1":[{"body":` + validIssue + `,"off_schema":true}]}`,
		},
		{
			name:       "--off-schema on a response that can't be checked",
			args:       []string{"GET /repos/o/r/issues/1", "--status", "502", "--off-schema"},
			wantStdout: "GET /repos/o/r/issues/1: response 1 of 1: status 502, no headers, body 0 bytes, off-schema\nSchema check: skipped, saved as off-schema\n",
			want:       `{"GET /repos/o/r/issues/1":[{"status":502,"off_schema":true}]}`,
		},
		{
			name:       "a status GitHub doesn't document isn't checked",
			args:       []string{"GET /repos/o/r/issues/1", "--status", "502", "--body", `{"message":"bad gateway"}`},
			wantStdout: "GET /repos/o/r/issues/1: response 1 of 1: status 502, no headers, body 25 bytes\nSchema check: skipped, GitHub doesn't document status 502 for GET /repos/{owner}/{repo}/issues/{issue_number}\n",
			want:       `{"GET /repos/o/r/issues/1":[{"status":502,"body":{"message":"bad gateway"}}]}`,
		},
		{
			name:       "a request to another host isn't checked",
			args:       []string{"POST uploads.github.localhost/upload", "--status", "201"},
			wantStdout: "POST uploads.github.localhost/upload: response 1 of 1: status 201, no headers, body 0 bytes\nSchema check: skipped, not a REST API host\n",
			want:       `{"POST uploads.github.localhost/upload":[{"status":201}]}`,
		},
		{
			name:       "a GraphQL response waits for gh's query",
			args:       []string{"graphql IssueTitle", "--body", badTitle},
			wantStdout: fmt.Sprintf("graphql IssueTitle: response 1 of 1: status 200, no headers, body %d bytes\nSchema check: not yet, because gh hasn't sent graphql IssueTitle; the next run checks it\n", len(badTitle)),
			want:       `{"graphql IssueTitle":[{"body":` + badTitle + `}]}`,
		},
		{
			name:       "a GraphQL response is checked against gh's last query",
			logged:     logged,
			args:       []string{"graphql IssueTitle", "--body", validTitle},
			wantStdout: fmt.Sprintf("graphql IssueTitle: response 1 of 1: status 200, no headers, body %d bytes\nSchema check: passed\n", len(validTitle)),
			want:       `{"graphql IssueTitle":[{"body":` + validTitle + `}]}`,
		},
		{
			name:   "a GraphQL response that doesn't match gh's last query isn't saved",
			logged: logged,
			args:   []string{"graphql IssueTitle", "--body", badTitle},
			wantErr: "the response doesn't match GitHub's schema, so it wasn't saved:\n" +
				"  data.repository.issue.state: \"open\" isn't a value of IssueState; it can be CLOSED, OPEN\n" +
				"Fix it, or pass --off-schema to save it anyway",
			want: `{}`,
		},
		{
			name: "every problem is listed",
			logged: []loggedRequest{{
				Run: "r", Seq: 1, Method: "POST", Host: apiHost, Path: "/graphql", Name: "graphql Assignees",
				GraphQL: &graphQLRequest{Kind: "query", Operation: "Assignees", Query: `query Assignees { repository(owner: "o", name: "r") { issue(number: 1) { assignees { login } } } }`},
			}},
			args: []string{"graphql Assignees", "--body", `{"data":{"repository":{"issue":{"assignees":[` + strings.TrimSuffix(strings.Repeat("null,", 11), ",") + `]}}}}`},
			wantErr: "the response doesn't match GitHub's schema, so it wasn't saved:\n" + func() string {
				var b strings.Builder
				for i := range 11 {
					fmt.Fprintf(&b, "  data.repository.issue.assignees.%d: is null, but GitHub's schema says it can't be (User!)\n", i)
				}
				return b.String()
			}() + "Fix it, or pass --off-schema to save it anyway",
			want: `{}`,
		},
		{
			name:       "a GraphQL error status isn't checked",
			logged:     logged,
			args:       []string{"graphql IssueTitle", "--status", "502"},
			wantStdout: "graphql IssueTitle: response 1 of 1: status 502, no headers, body 0 bytes\nSchema check: skipped, status 502 has no GraphQL result to check\n",
			want:       `{"graphql IssueTitle":[{"status":502}]}`,
		},
		{
			name:      "changing an off-schema response checks it again",
			responses: `{"graphql IssueTitle":[{"body":` + badTitle + `,"off_schema":true}]}`,
			logged:    logged,
			args:      []string{"graphql IssueTitle", "--header", "X-A: 1"},
			wantErr: "the response doesn't match GitHub's schema, so it wasn't saved:\n" +
				"  data.repository.issue.state: \"open\" isn't a value of IssueState; it can be CLOSED, OPEN\n" +
				"Fix it, or pass --off-schema to save it anyway",
			want: `{"graphql IssueTitle":[{"body":` + badTitle + `,"off_schema":true}]}`,
		},
		{
			name:      "without schemas, the response is saved unchecked",
			noSchemas: true,
			args:      []string{"GET /repos/o/r/issues/1", "--status", "404"},
			wantStdout: "GET /repos/o/r/issues/1: response 1 of 1: status 404, no headers, body 0 bytes\n" +
				"Schema check: skipped, REST checks are off: downloading GitHub's REST description from " + restSource.url +
				`: Get "` + restSource.url + `": downloads are off in tests (without network access, set GHMOCK_REST_SCHEMA to a local copy)` + "\n",
			want: `{"GET /repos/o/r/issues/1":[{"status":404}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("GHMOCK_DIR", dir)
			if !tt.noSchemas {
				useTestSchemas(t)
			}
			st := &state{dir: dir}
			if tt.responses != "" {
				responses := responseSet{}
				require.NoError(t, json.Unmarshal([]byte(tt.responses), &responses))
				require.NoError(t, st.saveResponses(responses))
			}
			require.NoError(t, st.appendLog(tt.logged))

			var stdout bytes.Buffer
			err := cmdSet(tt.args, &stdout, io.Discard)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantStdout, stdout.String())

			stored, err := st.loadResponses()
			require.NoError(t, err)
			got, err := json.Marshal(stored)
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(got))
		})
	}
}
