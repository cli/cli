package main

import (
	"bytes"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serveSchemas points schema downloads at a local server. A file missing from
// files gets a 404.
func serveSchemas(t *testing.T, files map[string]string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)

	oldClient, oldGraphQL, oldREST := downloadClient, graphQLSource.url, restSource.url
	t.Cleanup(func() { downloadClient, graphQLSource.url, restSource.url = oldClient, oldGraphQL, oldREST })
	downloadClient = srv.Client()
	graphQLSource.url = srv.URL + "/schema.graphql"
	restSource.url = srv.URL + "/rest.json"
}

// useCacheDir gives a test its own cache folder and returns ghmock's folder
// inside it.
func useCacheDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := userCacheDir
	t.Cleanup(func() { userCacheDir = old })
	userCacheDir = func() (string, error) { return dir, nil }
	return filepath.Join(dir, "ghmock")
}

func readTestdata(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return string(data)
}

func TestSchemaSourceLocate(t *testing.T) {
	schema := readTestdata(t, "schema.graphql")
	const oldCopy = "type Query { old: String }"

	tests := []struct {
		name         string
		env          string
		cached       string
		served       map[string]string
		wantPath     string
		wantContent  string
		wantProgress string
		wantErr      string
	}{
		{
			name:        "the environment variable wins",
			env:         "<testdata>/schema.graphql",
			cached:      oldCopy,
			wantPath:    "<testdata>/schema.graphql",
			wantContent: schema,
		},
		{
			name:    "a missing file in the environment variable is an error",
			env:     "<testdata>/nope.graphql",
			wantErr: "GHMOCK_GRAPHQL_SCHEMA: stat <testdata>/nope.graphql: no such file or directory",
		},
		{
			name:    "a folder in the environment variable is an error",
			env:     "<testdata>",
			wantErr: "GHMOCK_GRAPHQL_SCHEMA: <testdata> is a directory",
		},
		{
			name:        "a cached copy is used",
			cached:      oldCopy,
			wantPath:    "<cache>/schema.docs.graphql",
			wantContent: oldCopy,
		},
		{
			name:         "a missing copy is downloaded",
			served:       map[string]string{"/schema.graphql": schema},
			wantPath:     "<cache>/schema.docs.graphql",
			wantContent:  schema,
			wantProgress: "ghmock: downloading GitHub's GraphQL schema to <cache>/schema.docs.graphql\n",
		},
		{
			name:         "a failed download",
			served:       map[string]string{},
			wantProgress: "ghmock: downloading GitHub's GraphQL schema to <cache>/schema.docs.graphql\n",
			wantErr:      "downloading GitHub's GraphQL schema from <server>/schema.graphql: HTTP 404 (without network access, set GHMOCK_GRAPHQL_SCHEMA to a local copy)",
		},
		{
			name:         "a download that isn't a schema",
			served:       map[string]string{"/schema.graphql": "<html>"},
			wantProgress: "ghmock: downloading GitHub's GraphQL schema to <cache>/schema.docs.graphql\n",
			wantErr:      "downloading GitHub's GraphQL schema from <server>/schema.graphql: not a usable GraphQL schema: line 1, column 1: Unexpected <Invalid> (without network access, set GHMOCK_GRAPHQL_SCHEMA to a local copy)",
		},
		{
			name:         "an empty download",
			served:       map[string]string{"/schema.graphql": ""},
			wantProgress: "ghmock: downloading GitHub's GraphQL schema to <cache>/schema.docs.graphql\n",
			wantErr:      "downloading GitHub's GraphQL schema from <server>/schema.graphql: file is empty (without network access, set GHMOCK_GRAPHQL_SCHEMA to a local copy)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testdata, err := filepath.Abs("testdata")
			require.NoError(t, err)
			cache := useCacheDir(t)
			if tt.served != nil {
				serveSchemas(t, tt.served)
			}
			replacer := strings.NewReplacer(testdata, "<testdata>", cache, "<cache>", strings.TrimSuffix(graphQLSource.url, "/schema.graphql"), "<server>")
			if tt.env != "" {
				t.Setenv(graphQLSource.envVar, strings.ReplaceAll(tt.env, "<testdata>", testdata))
			}
			if tt.cached != "" {
				require.NoError(t, os.MkdirAll(cache, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(cache, graphQLSource.fileName), []byte(tt.cached), 0o644))
			}

			var progress bytes.Buffer
			got, err := graphQLSource.locate(&progress)
			assert.Equal(t, tt.wantProgress, replacer.Replace(progress.String()))
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Equal(t, tt.wantErr, replacer.Replace(err.Error()))
				entries, _ := os.ReadDir(cache)
				assert.Empty(t, entries, "a failed download leaves nothing behind")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantPath, replacer.Replace(got))
			data, err := os.ReadFile(got)
			require.NoError(t, err)
			assert.Equal(t, tt.wantContent, string(data))
			if tt.served != nil && runtime.GOOS != "windows" {
				info, err := os.Stat(got)
				require.NoError(t, err)
				assert.Equal(t, os.FileMode(0o644), info.Mode().Perm(), "other users can read a download")
			}
		})
	}
}

func TestCmdSchemas(t *testing.T) {
	schema := readTestdata(t, "schema.graphql")
	rest := readTestdata(t, "rest.json")

	tests := []struct {
		name       string
		args       []string
		env        map[string]string
		served     map[string]string
		wantStdout string
		wantStderr string
		wantErr    string
		wantFiles  map[string]string
	}{
		{
			name:   "downloads both files",
			args:   []string{"update"},
			served: map[string]string{"/schema.graphql": schema, "/rest.json": rest},
			wantStdout: "GraphQL schema: <cache>/schema.docs.graphql (<graphql size>)\n" +
				"REST description: <cache>/api.github.com.json (<rest size>)\n",
			wantStderr: "ghmock: downloading GitHub's GraphQL schema to <cache>/schema.docs.graphql\n" +
				"ghmock: downloading GitHub's REST description to <cache>/api.github.com.json\n",
			wantFiles: map[string]string{"schema.docs.graphql": schema, "api.github.com.json": rest},
		},
		{
			name:   "downloads to the path in an environment variable, which ghmock uses",
			args:   []string{"update"},
			env:    map[string]string{"GHMOCK_REST_SCHEMA": "<cache>/pinned/rest.json"},
			served: map[string]string{"/schema.graphql": schema, "/rest.json": rest},
			wantStdout: "GraphQL schema: <cache>/schema.docs.graphql (<graphql size>)\n" +
				"REST description: <cache>/pinned/rest.json (<rest size>)\n",
			wantStderr: "ghmock: downloading GitHub's GraphQL schema to <cache>/schema.docs.graphql\n" +
				"ghmock: downloading GitHub's REST description to <cache>/pinned/rest.json\n",
			wantFiles: map[string]string{"schema.docs.graphql": schema, "api.github.com.json": "old", "pinned/rest.json": rest},
		},
		{
			name:       "reports a download that fails and keeps the other",
			args:       []string{"update"},
			served:     map[string]string{"/rest.json": rest},
			wantStdout: "REST description: <cache>/api.github.com.json (<rest size>)\n",
			wantStderr: "ghmock: downloading GitHub's GraphQL schema to <cache>/schema.docs.graphql\n" +
				"ghmock: downloading GitHub's REST description to <cache>/api.github.com.json\n",
			wantErr:   "downloading GitHub's GraphQL schema from <server>/schema.graphql: HTTP 404 (without network access, set GHMOCK_GRAPHQL_SCHEMA to a local copy)",
			wantFiles: map[string]string{"api.github.com.json": rest},
		},
		{
			name:    "missing subcommand",
			wantErr: "missing subcommand; use ghmock schemas update",
		},
		{
			name:    "unknown subcommand",
			args:    []string{"list"},
			wantErr: `unknown subcommand "list"; use ghmock schemas update`,
		},
		{
			name:    "extra arguments",
			args:    []string{"update", "now"},
			wantErr: "unexpected arguments: now",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := useCacheDir(t)
			serveSchemas(t, tt.served)
			for k, v := range tt.env {
				t.Setenv(k, strings.ReplaceAll(v, "<cache>", cache))
			}
			// An old copy that a successful download replaces.
			require.NoError(t, os.MkdirAll(cache, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(cache, restSource.fileName), []byte("old"), 0o644))
			replacer := strings.NewReplacer(cache, "<cache>", strings.TrimSuffix(graphQLSource.url, "/schema.graphql"), "<server>",
				formatSize(int64(len(schema))), "<graphql size>", formatSize(int64(len(rest))), "<rest size>")

			var stdout, stderr bytes.Buffer
			err := cmdSchemas(tt.args, &stdout, &stderr)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Equal(t, tt.wantErr, replacer.Replace(err.Error()))
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantStdout, replacer.Replace(stdout.String()))
			assert.Equal(t, tt.wantStderr, replacer.Replace(stderr.String()))

			if tt.wantFiles == nil {
				return
			}
			got := map[string]string{}
			require.NoError(t, filepath.WalkDir(cache, func(path string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return err
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				rel, err := filepath.Rel(cache, path)
				got[filepath.ToSlash(rel)] = string(data)
				return err
			}))
			assert.Equal(t, tt.wantFiles, got)
		})
	}
}
