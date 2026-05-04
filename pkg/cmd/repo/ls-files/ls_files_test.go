package lsfiles

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLsFilesRun(t *testing.T) {
	tests := []struct {
		name       string
		opts       *LsFilesOptions
		httpStubs  func(t *testing.T, reg *httpmock.Registry)
		wantStdout string
		wantErr    bool
		errMsg     string
	}{
		{
			name: "list root directory",
			opts: &LsFilesOptions{},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryLsFiles\b`),
					httpmock.StringResponse(`{"data":{"repository":{"object":{
						"__typename":"Tree",
						"entries":[
							{"name":"README.md","type":"blob","path":"README.md","object":{"byteSize":100}},
							{"name":"src","type":"tree","path":"src","object":null}
						]
					}}}}`))
			},
			wantStdout: "  README.md\nd src\n",
		},
		{
			name: "list subdirectory",
			opts: &LsFilesOptions{
				Path: "src",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryLsFiles\b`),
					httpmock.StringResponse(`{"data":{"repository":{"object":{
						"__typename":"Tree",
						"entries":[
							{"name":"main.go","type":"blob","path":"src/main.go","object":{"byteSize":250}},
							{"name":"util.go","type":"blob","path":"src/util.go","object":{"byteSize":180}}
						]
					}}}}`))
			},
			wantStdout: "  main.go\n  util.go\n",
		},
		{
			name: "list with ref",
			opts: &LsFilesOptions{
				Ref: "v1.0.0",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryLsFiles\b`),
					httpmock.StringResponse(`{"data":{"repository":{"object":{
						"__typename":"Tree",
						"entries":[
							{"name":"go.mod","type":"blob","path":"go.mod","object":{"byteSize":50}}
						]
					}}}}`))
			},
			wantStdout: "  go.mod\n",
		},
		{
			name: "path not found",
			opts: &LsFilesOptions{
				Path: "nonexistent",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryLsFiles\b`),
					httpmock.StringResponse(`{"data":{"repository":{"object":null}}}`))
			},
			wantErr: true,
			errMsg:  "path not found: nonexistent (ref: HEAD)",
		},
		{
			name: "root not found",
			opts: &LsFilesOptions{},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryLsFiles\b`),
					httpmock.StringResponse(`{"data":{"repository":{"object":null}}}`))
			},
			wantErr: true,
			errMsg:  "path not found: / (ref: HEAD)",
		},
		{
			name: "path is a file",
			opts: &LsFilesOptions{
				Path: "README.md",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryLsFiles\b`),
					httpmock.StringResponse(`{"data":{"repository":{"object":{
						"__typename":"Blob"
					}}}}`))
			},
			wantErr: true,
			errMsg:  "path is a file, not a directory: README.md (use `gh repo view-file` to view file contents)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(t, reg)
			}
			defer reg.Verify(t)

			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.New("OWNER", "REPO"), nil
			}
			ios, _, stdout, _ := iostreams.Test()
			tt.opts.IO = ios

			err := lsFilesRun(tt.opts)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tt.errMsg, err.Error())
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantStdout, stdout.String())
		})
	}
}

func TestNewCmdLsFiles(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name:    "no arguments (root)",
			args:    []string{},
			wantErr: false,
		},
		{
			name:    "path argument",
			args:    []string{"src/"},
			wantErr: false,
		},
		{
			name:    "path with ref",
			args:    []string{"src/", "--ref", "main"},
			wantErr: false,
		},
		{
			name:    "too many arguments",
			args:    []string{"src/", "pkg/"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}
			cmd := NewCmdLsFiles(f, func(*LsFilesOptions) error {
				return nil
			})
			cmd.SetArgs(tt.args)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err := cmd.ExecuteC()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
