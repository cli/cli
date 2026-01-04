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
		{
			name:  "truncated file with raw and filename",
			isTTY: true,
			opts: &ViewOptions{
				Selector: "1234",
				Raw:      true,
				Filename: "large.txt",
			},
			mockGist: &shared.Gist{
				Files: map[string]*shared.GistFile{
					"large.txt": {
						Content:   "This is truncated content...",
						Type:      "text/plain",
						Truncated: true,
						RawURL:    "https://gist.githubusercontent.com/user/1234/raw/large.txt",
					},
				},
			},
			wantOut: "This is the full content of the large file retrieved from raw URL\n",
		},
		{
			name:  "truncated file without raw flag",
			isTTY: true,
			opts: &ViewOptions{
				Selector: "1234",
				Raw:      false,
				Filename: "large.txt",
			},
			mockGist: &shared.Gist{
				Files: map[string]*shared.GistFile{
					"large.txt": {
						Content:   "This is truncated content...",
						Type:      "text/plain",
						Truncated: true,
						RawURL:    "https://gist.githubusercontent.com/user/1234/raw/large.txt",
					},
				},
			},
			wantOut: "This is the full content of the large file retrieved from raw URL\n",
		},
		{
			name:  "multiple files with one truncated",
			isTTY: true,
			opts: &ViewOptions{
				Selector: "1234",
				Raw:      true,
			},
			mockGist: &shared.Gist{
				Description: "Mixed files",
				Files: map[string]*shared.GistFile{
					"normal.txt": {
						Content: "normal content",
						Type:    "text/plain",
					},
					"large.txt": {
						Content:   "This is truncated content...",
						Type:      "text/plain",
						Truncated: true,
						RawURL:    "https://gist.githubusercontent.com/user/1234/raw/large.txt",
					},
				},
			},
			wantOut: "Mixed files\n\nlarge.txt\n\nThis is the full content of the large file retrieved from raw URL\n\nnormal.txt\n\nnormal content\n",
		},
		{
			name:  "multiple files with subsequent files truncated as empty",
			isTTY: true,
			opts: &ViewOptions{
				Selector: "1234",
				Raw:      true,
			},
			mockGist: &shared.Gist{
				Description: "Large gist with multiple files",
				Files: map[string]*shared.GistFile{
					"large.txt": {
						Content:   "This is truncated content...",
						Type:      "text/plain",
						Truncated: true,
						RawURL:    "https://gist.githubusercontent.com/user/1234/raw/large.txt",
					},
					"also-truncated.txt": {
						Type:      "text/plain",
						Content:   "",   // Empty because GitHub truncates subsequent files
						Truncated: true, // Subsequent files are also marked as truncated
						RawURL:    "https://gist.githubusercontent.com/user/1234/raw/also-truncated.txt",
					},
				},
			},
			wantOut: "Large gist with multiple files\n\nalso-truncated.txt\n\nThis is the full content of the also-truncated file retrieved from raw URL\n\nlarge.txt\n\nThis is the full content of the large file retrieved from raw URL\n",
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

			for filename, file := range tt.mockGist.Files {
				if file.Truncated && file.RawURL != "" {
					if filename == "large.txt" {
						reg.Register(httpmock.REST("GET", "user/1234/raw/large.txt"),
							httpmock.StringResponse("This is the full content of the large file retrieved from raw URL"))
					} else if filename == "also-truncated.txt" {
						reg.Register(httpmock.REST("GET", "user/1234/raw/also-truncated.txt"),
							httpmock.StringResponse("This is the full content of the also-truncated file retrieved from raw URL"))
					}
				}
			}
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
