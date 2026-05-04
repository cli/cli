package viewfile

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

func TestViewFileRun(t *testing.T) {
	tests := []struct {
		name       string
		opts       *ViewFileOptions
		httpStubs  func(t *testing.T, reg *httpmock.Registry)
		wantStdout string
		wantErr    bool
		errMsg     string
	}{
		{
			name: "view text file",
			opts: &ViewFileOptions{
				Path: "README.md",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryFileView\b`),
					httpmock.StringResponse(`{"data":{"repository":{"object":{
						"__typename":"Blob",
						"text":"# Hello World\n",
						"byteSize":14,
						"isBinary":false
					}}}}`))
			},
			wantStdout: "# Hello World\n",
		},
		{
			name: "view file with ref",
			opts: &ViewFileOptions{
				Path: "src/main.go",
				Ref:  "feature-branch",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryFileView\b`),
					httpmock.StringResponse(`{"data":{"repository":{"object":{
						"__typename":"Blob",
						"text":"package main\n",
						"byteSize":13,
						"isBinary":false
					}}}}`))
			},
			wantStdout: "package main\n",
		},
		{
			name: "file not found",
			opts: &ViewFileOptions{
				Path: "nonexistent.txt",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryFileView\b`),
					httpmock.StringResponse(`{"data":{"repository":{"object":null}}}`))
			},
			wantErr: true,
			errMsg:  "file not found: nonexistent.txt (ref: HEAD)",
		},
		{
			name: "path is a directory",
			opts: &ViewFileOptions{
				Path: "src/",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryFileView\b`),
					httpmock.StringResponse(`{"data":{"repository":{"object":{
						"__typename":"Tree"
					}}}}`))
			},
			wantErr: true,
			errMsg:  "path is a directory, not a file: src/ (use `gh repo ls-files` to list directory contents)",
		},
		{
			name: "binary file",
			opts: &ViewFileOptions{
				Path: "image.png",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryFileView\b`),
					httpmock.StringResponse(`{"data":{"repository":{"object":{
						"__typename":"Blob",
						"text":"",
						"byteSize":4096,
						"isBinary":true
					}}}}`))
			},
			wantErr: true,
			errMsg:  "file is binary (4096 bytes): image.png",
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

			err := viewFileRun(tt.opts)

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

func TestNewCmdViewFile(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name:    "no arguments",
			args:    []string{},
			wantErr: true,
		},
		{
			name:    "path argument",
			args:    []string{"README.md"},
			wantErr: false,
		},
		{
			name:    "path with ref",
			args:    []string{"README.md", "--ref", "main"},
			wantErr: false,
		},
		{
			name:    "too many arguments",
			args:    []string{"file1", "file2"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}
			cmd := NewCmdViewFile(f, func(*ViewFileOptions) error {
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
