package main

import (
	"encoding/json"
	"errors"
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
			wantErr: "nothing to change; pass --status, --header, --body, --body-file or --at",
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
