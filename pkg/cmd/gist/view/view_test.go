package view

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/cli/cli/pkg/cmd/gist/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdView(t *testing.T) {
	tests := []struct {
		name  string
		cli   string
		wants ViewOptions
		tty   bool
	}{
		{
			name: "tty no arguments",
			tty:  true,
			cli:  "123",
			wants: ViewOptions{
				Raw:      false,
				Selector: "123",
			},
		},
		{
			name: "nontty no arguments",
			cli:  "123",
			wants: ViewOptions{
				Raw:      true,
				Selector: "123",
			},
		},
		{
			name: "filename passed",
			cli:  "-fcool.txt 123",
			tty:  true,
			wants: ViewOptions{
				Raw:      false,
				Selector: "123",
				Filename: "cool.txt",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()
			io.SetStdoutTTY(tt.tty)

			f := &cmdutil.Factory{
				IOStreams: io,
			}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts *ViewOptions
			cmd := NewCmdView(f, func(opts *ViewOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.Raw, gotOpts.Raw)
			assert.Equal(t, tt.wants.Selector, gotOpts.Selector)
			assert.Equal(t, tt.wants.Filename, gotOpts.Filename)
		})
	}
}

func Test_viewRun(t *testing.T) {
	tests := []struct {
		name    string
		opts    *ViewOptions
		wantOut string
		gist    *shared.Gist
		wantErr bool
	}{
		{
			name: "no such gist",
			opts: &ViewOptions{
				Selector: "1234",
			},
			wantErr: true,
		},
		{
			name: "one file",
			opts: &ViewOptions{
				Selector: "1234",
			},
			gist: &shared.Gist{
				Files: map[string]*shared.GistFile{
					"cicada.txt": {
						Content: "bwhiizzzbwhuiiizzzz",
						Type:    "text/plain",
					},
				},
			},
			wantOut: "bwhiizzzbwhuiiizzzz\n\n",
		},
		{
			name: "filename selected",
			opts: &ViewOptions{
				Selector: "1234",
				Filename: "cicada.txt",
			},
			gist: &shared.Gist{
				Files: map[string]*shared.GistFile{
					"cicada.txt": {
						Content: "bwhiizzzbwhuiiizzzz",
						Type:    "text/plain",
					},
					"foo.md": {
						Content: "# foo",
						Type:    "application/markdown",
					},
				},
			},
			wantOut: "bwhiizzzbwhuiiizzzz\n\n",
		},
		{
			name: "multiple files, no description",
			opts: &ViewOptions{
				Selector: "1234",
			},
			gist: &shared.Gist{
				Files: map[string]*shared.GistFile{
					"cicada.txt": {
						Content: "bwhiizzzbwhuiiizzzz",
						Type:    "text/plain",
					},
					"foo.md": {
						Content: "# foo",
						Type:    "application/markdown",
					},
				},
			},
			wantOut: "cicada.txt\n\nbwhiizzzbwhuiiizzzz\n\nfoo.md\n\n\n  # foo                                                                       \n\n\n\n",
		},
		{
			name: "multiple files, description",
			opts: &ViewOptions{
				Selector: "1234",
			},
			gist: &shared.Gist{
				Description: "some files",
				Files: map[string]*shared.GistFile{
					"cicada.txt": {
						Content: "bwhiizzzbwhuiiizzzz",
						Type:    "text/plain",
					},
					"foo.md": {
						Content: "- foo",
						Type:    "application/markdown",
					},
				},
			},
			wantOut: "some files\ncicada.txt\n\nbwhiizzzbwhuiiizzzz\n\nfoo.md\n\n\n                                                                              \n  • foo                                                                       \n\n\n\n",
		},
		{
			name: "raw",
			opts: &ViewOptions{
				Selector: "1234",
				Raw:      true,
			},
			gist: &shared.Gist{
				Description: "some files",
				Files: map[string]*shared.GistFile{
					"cicada.txt": {
						Content: "bwhiizzzbwhuiiizzzz",
						Type:    "text/plain",
					},
					"foo.md": {
						Content: "- foo",
						Type:    "application/markdown",
					},
				},
			},
			wantOut: "some files\ncicada.txt\n\nbwhiizzzbwhuiiizzzz\n\nfoo.md\n\n- foo\n\n",
		},
	}

	for _, tt := range tests {
		reg := &httpmock.Registry{}
		if tt.gist == nil {
			reg.Register(httpmock.REST("GET", "gists/1234"),
				httpmock.StatusStringResponse(404, "Not Found"))
		} else {
			reg.Register(httpmock.REST("GET", "gists/1234"),
				httpmock.JSONResponse(tt.gist))
		}

		if tt.opts == nil {
			tt.opts = &ViewOptions{}
		}

		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}
		io, _, stdout, _ := iostreams.Test()
		io.SetStdoutTTY(true)
		tt.opts.IO = io

		t.Run(tt.name, func(t *testing.T) {
			err := viewRun(tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			assert.Equal(t, tt.wantOut, stdout.String())
			reg.Verify(t)
		})
	}
}
