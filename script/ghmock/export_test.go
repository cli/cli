package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseExportArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		defaultPath string
		want        exportOptions
		wantErr     string
	}{
		{name: "folder", args: []string{"out"}, want: exportOptions{path: "out"}},
		{name: "configured destination", defaultPath: "configured", want: exportOptions{path: "configured"}},
		{name: "explicit path wins", args: []string{"out"}, defaultPath: "configured", want: exportOptions{path: "out"}},
		{name: "archive", args: []string{"--archive", "out.tar.gz"}, want: exportOptions{path: "out.tar.gz", archive: true}},
		{name: "configured archive", args: []string{"--archive"}, defaultPath: "out.tar.gz", want: exportOptions{path: "out.tar.gz", archive: true}},
		{name: "no folder", wantErr: "missing folder to export to"},
		{name: "two folders", args: []string{"a", "b"}, wantErr: "unexpected arguments: b"},
		{name: "unknown flag", args: []string{"--nope", "out"}, wantErr: "flag provided but not defined: -nope"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GHMOCK_EXPORT_PATH", tt.defaultPath)
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
	const legacyRunJSON = `{
		"id": "20261102T205540Z-bbbbbb",
		"time": "2026-11-02T20:55:40Z",
		"command": "ghmock run --gh ./bin/gh --terminal --keys {enter} -- repo create",
		"results": [{
			"binary": "./bin/gh",
			"path": "/bin/gh",
			"version": "gh version 1.0.0",
			"exit_code": -1,
			"terminal": true,
			"waiting_for_input": true
		}],
		"report": "gh version 1.0.0 - /bin/gh\n$ gh repo create\nexit code: none, gh was still waiting for input, so ghmock stopped it\n"
	}`
	var legacyRun runRecord
	require.NoError(t, json.Unmarshal([]byte(legacyRunJSON), &legacyRun))
	require.Len(t, legacyRun.Results, 1)
	assert.True(t, legacyRun.Results[0].Waiting)
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
		legacyRun,
	}
	entries := []loggedRequest{
		{Run: runs[0].ID, Binary: "./bin/gh", Seq: 1, Method: "POST", Name: "graphql IssueByNumber", Status: 200, Matched: true,
			Response: 1, Responses: 1, ResponseBody: json.RawMessage(`{"data":{"repository":null}}`)},
		{Run: runs[1].ID, Binary: "./bin/gh", Seq: 1, Method: "GET", Name: "GET /", Status: 404, ResponseRawBody: &[]byte{0, 255}},
	}
	responses := responseSet{
		"graphql IssueByNumber": {{Body: json.RawMessage(`{"data":{"repository":null}}`), OffSchema: true}},
		"GET /":                 {{Status: 401, RawBody: &[]byte{0, 255}}},
	}
	const wantReport = `### Run 20261102T205440Z-aaaaaa at 2026-11-02T20:54:40Z
$ ghmock run --gh ./bin/gh -- issue view 1
GraphQL schema: sha256 abc123, /cache/schema.docs.graphql
REST description: not loaded, so not checked: downloads are off

gh version 1.0.0 - /bin/gh
$ gh issue view 1
exit code: 0

### Run 20261102T205540Z-bbbbbb at 2026-11-02T20:55:40Z
$ ghmock run --gh ./bin/gh --terminal --keys {enter} -- repo create

gh version 1.0.0 - /bin/gh
$ gh repo create
exit code: none, gh was still waiting for input, so ghmock stopped it
`

	tests := []struct {
		name            string
		noRuns          bool
		existing        []string
		archive         bool
		configured      bool
		withGit         bool
		shared          bool
		fileExists      bool
		badWorktree     bool
		errContains     bool
		invalidResponse bool
		wantStdout      string
		wantErr         string
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
			name:       "configured archive needs no path argument",
			archive:    true,
			configured: true,
			wantStdout: "Exported 2 runs, 2 requests and 2 responses, 1 off-schema, to <out>\n",
		},
		{
			name:       "portable archive retains Git worktrees and hidden files",
			archive:    true,
			withGit:    true,
			wantStdout: "Exported 2 runs, 2 requests and 2 responses, 1 off-schema, to <out>\n",
		},
		{
			name:       "portable archive relocates every shared-clone alternate",
			archive:    true,
			withGit:    true,
			shared:     true,
			wantStdout: "Exported 2 runs, 2 requests and 2 responses, 1 off-schema, to <out>\n",
		},
		{
			name:       "archive does not overwrite an existing file",
			archive:    true,
			fileExists: true,
			wantErr:    "<out> already exists; export to a new file",
		},
		{
			name:        "failed Git export removes the incomplete archive",
			archive:     true,
			withGit:     true,
			badWorktree: true,
			errContains: true,
			wantErr:     "points outside the fixture",
		},
		{
			name:            "ambiguous raw and JSON response state is rejected",
			invalidResponse: true,
			errContains:     true,
			wantErr:         "has both JSON and raw bodies",
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
			t.Setenv("GHMOCK_EXPORT_PATH", "")
			st := &state{dir: dir}
			if !tt.noRuns {
				for _, r := range runs {
					require.NoError(t, st.appendRun(r))
				}
				require.NoError(t, st.appendLog(entries))
				require.NoError(t, st.saveResponses(responses))
				if tt.invalidResponse {
					require.NoError(t, st.saveResponses(responseSet{"GET /": {{Body: json.RawMessage(`{}`), RawBody: &[]byte{0, 255}}}}))
				}
			}
			out := filepath.Join(t.TempDir(), "export")
			args := []string{out}
			if tt.archive {
				out += ".tar.gz"
				args = []string{"--archive", out}
			}
			if tt.configured {
				t.Setenv("GHMOCK_EXPORT_PATH", out)
				args = args[:len(args)-1]
			}
			if tt.fileExists {
				require.NoError(t, os.WriteFile(out, []byte("keep"), 0o600))
			}
			if tt.existing != nil {
				require.NoError(t, os.Mkdir(out, 0o755))
				for _, name := range tt.existing {
					require.NoError(t, os.WriteFile(filepath.Join(out, name), nil, 0o644))
				}
			}
			if tt.withGit {
				prepareGitRepo(t, "git@github.com:octo/hello.git")
				if tt.shared {
					gitForTest(t, "--", "clone", "--quiet", "--shared", "main", "shared")
					objects := filepath.Join(st.gitFixtureDir(), "repos", dotcomHost, "octo", "hello.git", "objects")
					path := filepath.Join(st.gitFixtureDir(), "work", "shared", ".git", "objects", "info", "alternates")
					file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
					require.NoError(t, err)
					_, err = file.WriteString(objects + "\n")
					require.NoError(t, err)
					require.NoError(t, file.Close())
				}
				gitForTest(t, "--in", "main", "--", "worktree", "add", "--quiet", "../feature", "-b", "feature")
				require.NoError(t, cmdWrite([]string{"feature/example.txt", "--text", "changed\n"}, io.Discard))
				require.NoError(t, cmdWrite([]string{"feature/.notes", "--text", "hidden\n"}, io.Discard))
				if runtime.GOOS != "windows" {
					target, err := filepath.EvalSymlinks(filepath.Join(st.gitFixtureDir(), "work", "feature", "example.txt"))
					require.NoError(t, err)
					require.NoError(t, os.Symlink(target, filepath.Join(st.gitFixtureDir(), "work", "main", "link")))
				}
				if tt.badWorktree {
					outside, err := filepath.EvalSymlinks(t.TempDir())
					require.NoError(t, err)
					require.NoError(t, os.WriteFile(filepath.Join(st.gitFixtureDir(), "work", "feature", ".git"), []byte("gitdir: "+outside+"\n"), 0o644))
				}
			}

			var stdout bytes.Buffer
			err := cmdExport(args, &stdout)
			if tt.wantErr != "" {
				require.Error(t, err)
				if tt.errContains {
					assert.Contains(t, err.Error(), tt.wantErr)
				} else {
					assert.Equal(t, tt.wantErr, strings.ReplaceAll(err.Error(), out, "<out>"))
				}
				if tt.fileExists {
					data, err := os.ReadFile(out)
					require.NoError(t, err)
					assert.Equal(t, "keep", string(data))
				} else if tt.archive {
					assert.NoFileExists(t, out)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantStdout, strings.ReplaceAll(stdout.String(), out, "<out>"))
			if tt.archive {
				extracted := filepath.Join(t.TempDir(), "extracted")
				extractTestArchive(t, out, extracted)
				require.NoError(t, st.reset())
				out, err = filepath.EvalSymlinks(extracted)
				require.NoError(t, err)
			}

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
			if tt.withGit {
				t.Setenv("GHMOCK_DIR", out)
				if tt.shared {
					assert.Equal(t, "one\n", gitForTest(t, "--in", "shared", "--", "show", "HEAD:example.txt"))
					data, err := os.ReadFile(filepath.Join(out, gitFixture, "work", "shared", ".git", "objects", "info", "alternates"))
					require.NoError(t, err)
					lines := strings.Split(strings.TrimSpace(string(data)), "\n")
					require.Len(t, lines, 2)
					for _, line := range lines {
						assert.False(t, filepath.IsAbs(line))
					}
				}
				assert.FileExists(t, filepath.Join(out, gitFixture, "work", "main", ".git", "config"))
				assert.FileExists(t, filepath.Join(out, gitFixture, "work", "main", ".git", "index"))
				assert.Equal(t, "one\n", gitForTest(t, "--in", "feature", "--", "show", ":example.txt"))
				assert.Contains(t, gitForTest(t, "--in", "feature", "--", "status", "--porcelain=v1"), " M example.txt\n")
				assert.Contains(t, gitForTest(t, "--in", "main", "--", "worktree", "list", "--porcelain"), filepath.Join(out, gitFixture, "work", "feature"))
				data, err := os.ReadFile(filepath.Join(out, gitFixture, "work", "feature", ".notes"))
				require.NoError(t, err)
				assert.Equal(t, "hidden\n", string(data))
				if runtime.GOOS != "windows" {
					data, err := os.ReadFile(filepath.Join(out, gitFixture, "work", "main", "link"))
					require.NoError(t, err)
					assert.Equal(t, "changed\n", string(data))
				}
			}
		})
	}
}

func extractTestArchive(t *testing.T, path, destination string) {
	t.Helper()
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()
	gz, err := gzip.NewReader(file)
	require.NoError(t, err)
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		require.True(t, filepath.IsLocal(filepath.FromSlash(header.Name)))
		target := filepath.Join(destination, filepath.FromSlash(header.Name))
		require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
		switch header.Typeflag {
		case tar.TypeDir:
			require.NoError(t, os.MkdirAll(target, 0o755))
		case tar.TypeReg:
			data, err := io.ReadAll(tr)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(target, data, fs.FileMode(header.Mode).Perm()))
		case tar.TypeSymlink:
			require.True(t, pathWithin(destination, filepath.Join(filepath.Dir(target), filepath.FromSlash(header.Linkname))))
			require.NoError(t, os.Symlink(filepath.FromSlash(header.Linkname), target))
		default:
			t.Fatalf("unexpected archive entry type %d", header.Typeflag)
		}
	}
}
