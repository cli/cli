package readdir

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/jsonfieldstest"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONFields(t *testing.T) {
	jsonfieldstest.ExpectCommandToSupportJSONFields(t, NewCmdReadDir, []string{
		"name",
		"path",
		"nameRaw",
		"pathRaw",
		"type",
		"gitType",
		"mode",
		"modeOctal",
		"gitSHA",
		"size",
		"submodule",
	})
}

func Test_entryTypeFromMode(t *testing.T) {
	tests := []struct {
		mode int
		want string
	}{
		{0o040000, "dir"},
		{0o120000, "symlink"},
		{0o160000, "submodule"},
		{0o100644, "file"},
		{0o100755, "file"},
		{0o100600, "file"},
		{0, "unknown"},
		{0o020000, "unknown"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, entryTypeFromMode(tt.mode), "mode %o", tt.mode)
	}
}

func Test_dirEntry_isExecutable(t *testing.T) {
	tests := []struct {
		name  string
		entry dirEntry
		want  bool
	}{
		{name: "executable file", entry: dirEntry{Type: "file", Mode: 0o100755}, want: true},
		{name: "regular file", entry: dirEntry{Type: "file", Mode: 0o100644}, want: false},
		{name: "directory", entry: dirEntry{Type: "dir", Mode: 0o040000}, want: false},
		{name: "symlink", entry: dirEntry{Type: "symlink", Mode: 0o120000}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.entry.isExecutable())
		})
	}
}

func Test_dirEntry_modeOctal(t *testing.T) {
	tests := []struct {
		name  string
		entry dirEntry
		want  string
	}{
		{name: "executable file", entry: dirEntry{Mode: 0o100755}, want: "100755"},
		{name: "regular file", entry: dirEntry{Mode: 0o100644}, want: "100644"},
		{name: "directory", entry: dirEntry{Mode: 0o040000}, want: "040000"},
		{name: "submodule", entry: dirEntry{Mode: 0o160000}, want: "160000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.entry.modeOctal())
		})
	}
}

func Test_NewCmdReadDir(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		wantOpts ReadDirOptions
		wantErr  string
	}{
		{
			name: "no args lists root",
			args: "",
			wantOpts: ReadDirOptions{
				Path: "",
			},
		},
		{
			name: "path argument",
			args: "pkg/cmd",
			wantOpts: ReadDirOptions{
				Path: "pkg/cmd",
			},
		},
		{
			name: "ref flag",
			args: "docs --ref v1.2.3",
			wantOpts: ReadDirOptions{
				Path: "docs",
				Ref:  "v1.2.3",
			},
		},
		{
			name:    "too many arguments",
			args:    "a b",
			wantErr: "accepts at most 1 arg(s), received 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			var gotOpts *ReadDirOptions
			cmd := NewCmdReadDir(f, func(opts *ReadDirOptions) error {
				gotOpts = opts
				return nil
			})

			argv, err := shlex.Split(tt.args)
			require.NoError(t, err)
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			_, err = cmd.ExecuteC()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOpts.Path, gotOpts.Path)
			assert.Equal(t, tt.wantOpts.Ref, gotOpts.Ref)
		})
	}
}

func Test_readDirRun(t *testing.T) {
	tests := []struct {
		name       string
		tty        bool
		opts       ReadDirOptions
		httpStubs  func(*httpmock.Registry)
		jsonFields []string
		wantOut    string
		wantStderr string
		wantErrMsg string
	}{
		{
			name: "base repo resolution error",
			opts: ReadDirOptions{
				BaseRepo: func() (ghrepo.Interface, error) {
					return nil, errors.New("some error")
				},
			},
			wantErrMsg: "some error. Run this command from within a git repository, or use the `--repo` flag to specify one",
		},
		{
			name: "root listing (tty)",
			tty:  true,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepoReadDir\b`),
					treeResponse(
						entry(".github", 0o040000, 0, nil),
						entry("README.md", 0o100644, 2048, nil),
						entry("build.sh", 0o100755, 512, nil),
						entry("latest", 0o120000, 18, nil),
						entry("vendor", 0o160000, 0, nil),
					),
				)
			},
			wantOut: heredoc.Doc(`
				Showing 5 entries in OWNER/REPO

				TYPE       NAME       SIZE
				dir        .github    -
				file       README.md  2.0 KB
				file*      build.sh   512 B
				symlink    latest     18 B
				submodule  vendor     -
			`),
		},
		{
			name: "single entry uses singular noun (tty)",
			tty:  true,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepoReadDir\b`),
					treeResponse(entry("only.txt", 0o100644, 3, nil)),
				)
			},
			wantOut: heredoc.Doc(`
				Showing 1 entry in OWNER/REPO

				TYPE  NAME      SIZE
				file  only.txt  3 B
			`),
		},
		{
			name: "subdir listing header includes path (tty)",
			tty:  true,
			opts: ReadDirOptions{
				Path: "foo/bar",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepoReadDir\b`),
					httpmock.GraphQLQuery(
						buildTreeData(entry("baz.txt", 0o100644, 10, nil)),
						func(_ string, vars map[string]interface{}) {
							assert.Equal(t, "HEAD:foo/bar", vars["expression"])
						},
					),
				)
			},
			wantOut: heredoc.Doc(`
				Showing 1 entry in OWNER/REPO/foo/bar

				TYPE  NAME     SIZE
				file  baz.txt  10 B
			`),
		},
		{
			name: "ref is used to build the expression",
			opts: ReadDirOptions{
				Ref:  "v1.2.3",
				Path: "docs",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepoReadDir\b`),
					httpmock.GraphQLQuery(
						buildTreeData(entry("guide.md", 0o100644, 1, nil)),
						func(_ string, vars map[string]interface{}) {
							assert.Equal(t, "v1.2.3:docs", vars["expression"])
						},
					),
				)
			},
			wantOut: "file\tguide.md\t100644\t1\n",
		},
		{
			name: "non-tty output is tab separated with octal mode and raw size",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepoReadDir\b`),
					treeResponse(
						entry(".github", 0o040000, 0, nil),
						entry("build.sh", 0o100755, 512, nil),
						entry("latest", 0o120000, 18, nil),
					),
				)
			},
			wantOut: "dir\t.github\t040000\t0\n" +
				"file\tbuild.sh\t100755\t512\n" +
				"symlink\tlatest\t120000\t18\n",
		},
		{
			name: "json selects per-entry fields",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepoReadDir\b`),
					treeResponse(
						entry("a.txt", 0o100644, 5, nil),
						entry("docs", 0o040000, 0, nil),
					),
				)
			},
			jsonFields: []string{"name", "type", "size", "gitType", "mode", "modeOctal"},
			wantOut: `{"entries":[` +
				`{"gitType":"blob","mode":33188,"modeOctal":"100644","name":"a.txt","size":5,"type":"file"},` +
				`{"gitType":"tree","mode":16384,"modeOctal":"040000","name":"docs","size":0,"type":"dir"}` +
				`],"gitSHA":"tree-sha","id":"tree-id"}` + "\n",
		},
		{
			name: "json includes submodule details",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepoReadDir\b`),
					treeResponse(
						entry("vendor", 0o160000, 0, &submodulePayload{
							GitURL:              "https://github.com/OWNER/sub",
							Branch:              "main",
							SubprojectCommitOid: "abc123",
						}),
					),
				)
			},
			jsonFields: []string{"name", "type", "submodule"},
			wantOut: `{"entries":[` +
				`{"name":"vendor","submodule":{"branch":"main","gitUrl":"https://github.com/OWNER/sub","subprojectCommitOid":"abc123"},"type":"submodule"}` +
				`],"gitSHA":"tree-sha","id":"tree-id"}` + "\n",
		},
		{
			name: "empty directory warns and exits zero (tty)",
			tty:  true,
			opts: ReadDirOptions{
				Path: "empty",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepoReadDir\b`),
					treeResponse(),
				)
			},
			wantStderr: "No entries found in OWNER/REPO/empty\n",
		},
		{
			name: "empty root includes repo name in warning",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepoReadDir\b`),
					treeResponse(),
				)
			},
			wantStderr: "No entries found in OWNER/REPO\n",
		},
		{
			name: "path or ref not found",
			opts: ReadDirOptions{
				Path: "missing",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepoReadDir\b`),
					// The API returns a null object for a missing path, a missing ref, or both, so we
					// cannot tell which one is wrong and infer the message from whether a ref was given.
					httpmock.StringResponse(`{"data":{"repository":{"object":null}}}`),
				)
			},
			wantErrMsg: `could not find "missing" in OWNER/REPO (the path or ref may not exist)`,
		},
		{
			name: "repository not found surfaces the graphql error",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepoReadDir\b`),
					httpmock.StringResponse(`{"data":{"repository":null},"errors":[{"type":"NOT_FOUND","path":["repository"],"message":"Could not resolve to a Repository with the name 'OWNER/REPO'."}]}`),
				)
			},
			wantErrMsg: "Could not resolve to a Repository",
		},
		{
			name: "path points to a file",
			opts: ReadDirOptions{Path: "README.md"},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepoReadDir\b`),
					httpmock.StringResponse(`{"data":{"repository":{"object":{"__typename":"Blob"}}}}`),
				)
			},
			wantErrMsg: "\"README.md\" is a file, not a directory; use `gh repo read-file` instead",
		},
		{
			name: "path points to a submodule",
			opts: ReadDirOptions{Path: "vendor-lib"},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepoReadDir\b`),
					httpmock.StringResponse(`{"data":{"repository":{"object":{"__typename":"Commit"}}}}`),
				)
			},
			wantErrMsg: `"vendor-lib" is not a directory`,
		},
		{
			name: "path points to a tag object",
			opts: ReadDirOptions{Path: "some-tag"},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepoReadDir\b`),
					httpmock.StringResponse(`{"data":{"repository":{"object":{"__typename":"Tag"}}}}`),
				)
			},
			wantErrMsg: `"some-tag" is not a directory`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)

			opts := tt.opts
			opts.IO = ios
			opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			if opts.BaseRepo == nil {
				opts.BaseRepo = func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				}
			}

			if tt.jsonFields != nil {
				exporter := cmdutil.NewJSONExporter()
				exporter.SetFields(tt.jsonFields)
				opts.Exporter = exporter
			}

			err := readDirRun(&opts)
			if tt.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOut, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}

type submodulePayload struct {
	GitURL              string
	Branch              string
	SubprojectCommitOid string
}

// entry builds a single GraphQL TreeEntry payload for use in test responses.
func entry(name string, mode, size int, sub *submodulePayload) map[string]interface{} {
	e := map[string]interface{}{
		"name":    name,
		"path":    name,
		"nameRaw": name,
		"pathRaw": name,
		"type":    gitTypeFromMode(mode),
		"mode":    mode,
		"oid":     "blob-" + name,
		"size":    size,
	}
	if sub != nil {
		e["submodule"] = map[string]interface{}{
			"gitUrl":              sub.GitURL,
			"branch":              sub.Branch,
			"subprojectCommitOid": sub.SubprojectCommitOid,
		}
	} else {
		e["submodule"] = nil
	}
	return e
}

func gitTypeFromMode(mode int) string {
	switch mode {
	case 0o040000:
		return "tree"
	case 0o160000:
		return "commit"
	default:
		return "blob"
	}
}

// buildTreeData marshals a successful GraphQL response body for the given entries.
func buildTreeData(entries ...map[string]interface{}) string {
	if entries == nil {
		entries = []map[string]interface{}{}
	}
	body := map[string]interface{}{
		"data": map[string]interface{}{
			"repository": map[string]interface{}{
				"object": map[string]interface{}{
					"__typename": "Tree",
					"oid":        "tree-sha",
					"id":         "tree-id",
					"entries":    entries,
				},
			},
		},
	}
	out, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return string(out)
}

// treeResponse is an httpmock responder that returns a Tree with the given entries.
func treeResponse(entries ...map[string]interface{}) httpmock.Responder {
	return httpmock.StringResponse(buildTreeData(entries...))
}
