package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseExportArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr string
	}{
		{name: "folder", args: []string{"out"}, want: "out"},
		{name: "no folder", wantErr: "missing folder to export to"},
		{name: "two folders", args: []string{"a", "b"}, wantErr: "unexpected arguments: b"},
		{name: "unknown flag", args: []string{"--nope", "out"}, wantErr: "flag provided but not defined: -nope"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseExportArgs(tt.args)
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

func TestCmdExport(t *testing.T) {
	start := time.Date(2026, 11, 2, 20, 54, 40, 0, time.UTC)
	runs := []runRecord{
		{
			ID: "20261102T205440Z-aaaaaa", Time: start, Command: "ghmock run --gh ./bin/gh -- issue view 1",
			Schemas: []schemaFile{
				{Name: "GraphQL schema", Path: "/cache/schema.docs.graphql", SHA256: "abc123"},
				{Name: "REST description", Error: "downloads are off"},
			},
			Results: []runOutcome{{Binary: "./bin/gh", Path: "/bin/gh", Version: "gh version 1.0.0", Stdout: "title:\tBug\n"}},
			Report:  "gh version 1.0.0 - /bin/gh\n$ gh issue view 1\nexit code: 0\n",
		},
		{
			ID: "20261102T205540Z-bbbbbb", Time: start.Add(time.Minute), Command: "ghmock run --gh ./bin/gh --logged-out -- auth status",
			Results: []runOutcome{{Binary: "./bin/gh", Path: "/bin/gh", Version: "gh version 1.0.0", ExitCode: 1}},
			Report:  "gh version 1.0.0 - /bin/gh\n$ gh auth status\nexit code: 1\n",
		},
	}
	entries := []loggedRequest{
		{Run: runs[0].ID, Binary: "./bin/gh", Seq: 1, Method: "POST", Name: "graphql IssueByNumber", Status: 200, Matched: true,
			Response: 1, Responses: 1, ResponseBody: json.RawMessage(`{"data":{"repository":null}}`)},
		{Run: runs[1].ID, Binary: "./bin/gh", Seq: 1, Method: "GET", Name: "GET /", Status: 404},
	}
	responses := responseSet{
		"graphql IssueByNumber": {{Body: json.RawMessage(`{"data":{"repository":null}}`), OffSchema: true}},
		"GET /":                 {{Status: 401}},
	}
	const wantReport = `### Run 20261102T205440Z-aaaaaa at 2026-11-02T20:54:40Z
$ ghmock run --gh ./bin/gh -- issue view 1
GraphQL schema: sha256 abc123, /cache/schema.docs.graphql
REST description: not loaded, so not checked: downloads are off

gh version 1.0.0 - /bin/gh
$ gh issue view 1
exit code: 0

### Run 20261102T205540Z-bbbbbb at 2026-11-02T20:55:40Z
$ ghmock run --gh ./bin/gh --logged-out -- auth status

gh version 1.0.0 - /bin/gh
$ gh auth status
exit code: 1
`

	tests := []struct {
		name       string
		noRuns     bool
		existing   []string
		wantStdout string
		wantErr    string
	}{
		{
			name:       "writes the runs, requests and responses",
			wantStdout: "Exported 2 runs, 2 requests and 2 responses, 1 off-schema, to <out>\n",
		},
		{
			name:       "into an empty folder that exists",
			existing:   []string{},
			wantStdout: "Exported 2 runs, 2 requests and 2 responses, 1 off-schema, to <out>\n",
		},
		{
			name:     "never into a folder with files in it",
			existing: []string{"notes.txt"},
			wantErr:  "<out> isn't empty; export to a new folder",
		},
		{
			name:    "nothing to export before a run",
			noRuns:  true,
			wantErr: "nothing to export yet; run gh with ghmock run first",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("GHMOCK_DIR", dir)
			st := &state{dir: dir}
			if !tt.noRuns {
				for _, r := range runs {
					require.NoError(t, st.appendRun(r))
				}
				require.NoError(t, st.appendLog(entries))
				require.NoError(t, st.saveResponses(responses))
			}
			out := filepath.Join(t.TempDir(), "export")
			if tt.existing != nil {
				require.NoError(t, os.Mkdir(out, 0o755))
				for _, name := range tt.existing {
					require.NoError(t, os.WriteFile(filepath.Join(out, name), nil, 0o644))
				}
			}

			var stdout bytes.Buffer
			err := cmdExport([]string{out}, &stdout)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Equal(t, tt.wantErr, strings.ReplaceAll(err.Error(), out, "<out>"))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantStdout, strings.ReplaceAll(stdout.String(), out, "<out>"))

			read := func(name string) string {
				data, err := os.ReadFile(filepath.Join(out, name))
				require.NoError(t, err)
				return string(data)
			}
			assert.Equal(t, wantReport, read("report.txt"))
			assert.Equal(t, exportReadme(), read("README.md"))
			for _, name := range []string{"report.txt", "requests.jsonl", "responses.json", "runs.jsonl"} {
				assert.Contains(t, exportReadme(), "`"+name+"`", "the README describes %s", name)
			}

			exported := &state{dir: out}
			gotRuns, err := exported.readRuns()
			require.NoError(t, err)
			assert.Equal(t, runs, gotRuns)
			gotEntries, err := exported.readLog()
			require.NoError(t, err)
			assert.Equal(t, entries, gotEntries)
			gotResponses, err := exported.loadResponses()
			require.NoError(t, err)
			want, err := json.Marshal(responses)
			require.NoError(t, err)
			got, err := json.Marshal(gotResponses)
			require.NoError(t, err)
			assert.JSONEq(t, string(want), string(got))
		})
	}
}
