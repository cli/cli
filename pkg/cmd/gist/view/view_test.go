package view

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/gist/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
				Raw:       false,
				Selector:  "123",
				ListFiles: false,
			},
		},
		{
			name: "nontty no arguments",
			cli:  "123",
			wants: ViewOptions{
				Raw:       true,
				Selector:  "123",
				ListFiles: false,
			},
		},
		{
			name: "filename passed",
			cli:  "-fcool.txt 123",
			tty:  true,
			wants: ViewOptions{
				Raw:       false,
				Selector:  "123",
				Filename:  "cool.txt",
				ListFiles: false,
			},
		},
		{
			name: "files passed",
			cli:  "--files 123",
			tty:  true,
			wants: ViewOptions{
				Raw:       false,
				Selector:  "123",
				ListFiles: true,
			},
		},
		{
			name: "tty no ID supplied",
			cli:  "",
			tty:  true,
			wants: ViewOptions{
				Raw:       false,
				Selector:  "",
				ListFiles: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdoutTTY(tt.tty)

			f := &cmdutil.Factory{
				IOStreams: ios,
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
		name         string
		opts         *ViewOptions
		wantOut      string
		mockGist     *shared.Gist
		mockGistList bool
		isTTY        bool
		wantErr      string
	}{
		{
			name:  "no such gist",
			isTTY: false,
			opts: &ViewOptions{
				Selector:  "1234",
				ListFiles: false,
			},
			wantErr: "not found",
		},
		{
			name:  "one file",
			isTTY: true,
			opts: &ViewOptions{
				Selector:  "1234",
				ListFiles: false,
			},
			mockGist: &shared.Gist{
				Files: map[string]*shared.GistFile{
					"cicada.txt": {
						Content: "bwhiizzzbwhuiiizzzz",
						Type:    "text/plain",
					},
				},
			},
			wantOut: "bwhiizzzbwhuiiizzzz\n",
		},
		{
			name:  "one file, no ID supplied",
			isTTY: true,
			opts: &ViewOptions{
				Selector:  "",
				ListFiles: false,
			},
			mockGistList: true,
			mockGist: &shared.Gist{
				Files: map[string]*shared.GistFile{
					"cicada.txt": {
						Content: "test interactive mode",
						Type:    "text/plain",
					},
				},
			},
			wantOut: "test interactive mode\n",
		},
		{
			name:    "no arguments notty",
			isTTY:   false,
			wantErr: "gist ID or URL required when not running interactively",
		},
		{
			name:  "filename selected",
			isTTY: true,
			opts: &ViewOptions{
				Selector:  "1234",
				Filename:  "cicada.txt",
				ListFiles: false,
			},
			mockGist: &shared.Gist{
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
			wantOut: "bwhiizzzbwhuiiizzzz\n",
		},
		{
			name:  "filename selected, raw",
			isTTY: true,
			opts: &ViewOptions{
				Selector:  "1234",
				Filename:  "cicada.txt",
				Raw:       true,
				ListFiles: false,
			},
			mockGist: &shared.Gist{
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
			wantOut: "bwhiizzzbwhuiiizzzz\n",
		},
		{
			name:  "multiple files, no description",
			isTTY: true,
			opts: &ViewOptions{
				Selector:  "1234",
				ListFiles: false,
			},
			mockGist: &shared.Gist{
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
			wantOut: "cicada.txt\n\nbwhiizzzbwhuiiizzzz\n\nfoo.md\n\n\n  # foo                                                                       \n\n",
		},
		{
			name:  "multiple files, trailing newlines",
			isTTY: true,
			opts: &ViewOptions{
				Selector:  "1234",
				ListFiles: false,
			},
			mockGist: &shared.Gist{
				Files: map[string]*shared.GistFile{
					"cicada.txt": {
						Content: "bwhiizzzbwhuiiizzzz\n",
						Type:    "text/plain",
					},
					"foo.txt": {
						Content: "bar\n",
						Type:    "text/plain",
					},
				},
			},
			wantOut: "cicada.txt\n\nbwhiizzzbwhuiiizzzz\n\nfoo.txt\n\nbar\n",
		},
		{
			name:  "multiple files, description",
			isTTY: true,
			opts: &ViewOptions{
				Selector:  "1234",
				ListFiles: false,
			},
			mockGist: &shared.Gist{
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
			wantOut: "some files\n\ncicada.txt\n\nbwhiizzzbwhuiiizzzz\n\nfoo.md\n\n\n                                                                              \n  • foo                                                                       \n\n",
		},
		{
			name:  "multiple files, raw",
			isTTY: true,
			opts: &ViewOptions{
				Selector:  "1234",
				Raw:       true,
				ListFiles: false,
			},
			mockGist: &shared.Gist{
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
			wantOut: "some files\n\ncicada.txt\n\nbwhiizzzbwhuiiizzzz\n\nfoo.md\n\n- foo\n",
		},
		{
			name:  "one file, list files",
			isTTY: true,
			opts: &ViewOptions{
				Selector:  "1234",
				Raw:       false,
				ListFiles: true,
			},
			mockGist: &shared.Gist{
				Description: "some files",
				Files: map[string]*shared.GistFile{
					"cicada.txt": {
						Content: "bwhiizzzbwhuiiizzzz",
						Type:    "text/plain",
					},
				},
			},
			wantOut: "cicada.txt\n",
		},
		{
			name:  "multiple file, list files",
			isTTY: true,
			opts: &ViewOptions{
				Selector:  "1234",
				Raw:       false,
				ListFiles: true,
			},
			mockGist: &shared.Gist{
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
			wantOut: "cicada.txt\nfoo.md\n",
		},
	}

	for _, tt := range tests {
		reg := &httpmock.Registry{}
		if tt.mockGist == nil {
			reg.Register(httpmock.REST("GET", "gists/1234"),
				httpmock.StatusStringResponse(404, "Not Found"))
		} else {
			reg.Register(httpmock.REST("GET", "gists/1234"),
				httpmock.JSONResponse(tt.mockGist))
		}

		if tt.opts == nil {
			tt.opts = &ViewOptions{}
		}

		if tt.mockGistList {
			sixHours, _ := time.ParseDuration("6h")
			sixHoursAgo := time.Now().Add(-sixHours)
			reg.Register(
				httpmock.GraphQL(`query GistList\b`),
				httpmock.StringResponse(fmt.Sprintf(
					`{ "data": { "viewer": { "gists": { "nodes": [
							{
								"name": "1234",
								"files": [{ "name": "cool.txt" }],
								"description": "",
								"updatedAt": "%s",
								"isPublic": true
							}
						] } } } }`,
					sixHoursAgo.Format(time.RFC3339),
				)),
			)

			pm := prompter.NewMockPrompter(t)
			pm.RegisterSelect("Select a gist", []string{"cool.txt  about 6 hours ago"}, func(_, _ string, opts []string) (int, error) {
				return 0, nil
			})
			tt.opts.Prompter = pm
		}

		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}

		tt.opts.Config = func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		}

		ios, _, stdout, _ := iostreams.Test()
		ios.SetStdoutTTY(tt.isTTY)
		ios.SetStdinTTY(tt.isTTY)
		ios.SetStderrTTY(tt.isTTY)

		tt.opts.IO = ios

		t.Run(tt.name, func(t *testing.T) {
			err := viewRun(tt.opts)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.wantOut, stdout.String())
			reg.Verify(t)
		})
	}
}
