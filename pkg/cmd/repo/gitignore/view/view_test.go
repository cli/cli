package view

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestViewRun(t *testing.T) {
	tests := []struct {
		name       string
		opts       *ViewOptions
		isTTY      bool
		httpStubs  func(t *testing.T, reg *httpmock.Registry)
		wantStdout string
		wantStderr string
		wantErr    bool
		errMsg     string
	}{
		{
			name:    "No template",
			opts:    &ViewOptions{},
			wantErr: true,
			errMsg:  "no template provided",
		},
		{
			name:    "happy path with template",
			opts:    &ViewOptions{Template: "Go"},
			wantErr: false,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "gitignore/templates/Go"),
					httpmock.StringResponse(`{
						"name": "Go",
						"source": "# If you prefer the allow list template instead of the deny list, see community template:\n# https://github.com/github/gitignore/blob/main/community/Golang/Go.AllowList.gitignore\n#\n# Binaries for programs and plugins\n*.exe\n*.exe~\n*.dll\n*.so\n*.dylib\n\n# Test binary, built with go test -c\n*.test\n\n# Output of the go coverage tool, specifically when used with LiteIDE\n*.out\n\n# Dependency directories (remove the comment below to include it)\n# vendor/\n\n# Go workspace file\ngo.work\ngo.work.sum\n\n# env file\n.env\n"
						}`,
					))
			},
			wantStdout: heredoc.Doc(`# If you prefer the allow list template instead of the deny list, see community template:
				# https://github.com/github/gitignore/blob/main/community/Golang/Go.AllowList.gitignore
				#
				# Binaries for programs and plugins
				*.exe
				*.exe~
				*.dll
				*.so
				*.dylib

				# Test binary, built with go test -c
				*.test

				# Output of the go coverage tool, specifically when used with LiteIDE
				*.out

				# Dependency directories (remove the comment below to include it)
				# vendor/

				# Go workspace file
				go.work
				go.work.sum

				# env file
				.env
				`),
		},
		{
			name:    "non-existent template",
			opts:    &ViewOptions{Template: "non-existent"},
			wantErr: true,
			errMsg:  "'non-existent' is not a valid gitignore template. Run `gh repo gitignore list` for options",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "gitignore/templates/non-existent"),
					httpmock.StatusStringResponse(404, `{
						"message": "Not Found",
						"documentation_url": "https://docs.github.com/v3/gitignore",
						"status": "404"
						}`,
					))
			},
		},
	}
	for _, tt := range tests {
		reg := &httpmock.Registry{}
		if tt.httpStubs != nil {
			tt.httpStubs(t, reg)
		}
		tt.opts.Config = func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		}
		tt.opts.HTTPClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}
		ios, _, stdout, stderr := iostreams.Test()
		ios.SetStdoutTTY(tt.isTTY)
		ios.SetStdinTTY(tt.isTTY)
		ios.SetStderrTTY(tt.isTTY)
		tt.opts.IO = ios

		t.Run(tt.name, func(t *testing.T) {
			defer reg.Verify(t)
			err := viewRun(tt.opts)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tt.errMsg, err.Error())
				return
			}

			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}

func TestNewCmdView(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantErr  bool
		wantOpts *ViewOptions
		tty      bool
	}{
		{
			name:    "No template",
			args:    []string{},
			wantErr: true,
			wantOpts: &ViewOptions{
				Template: "",
			},
		},
		{
			name:    "Happy path single template",
			args:    []string{"Go"},
			wantErr: false,
			wantOpts: &ViewOptions{
				Template: "Go",
			},
		},
		{
			name:    "Happy path too many templates",
			args:    []string{"Go", "Ruby"},
			wantErr: true,
			wantOpts: &ViewOptions{
				Template: "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdinTTY(tt.tty)
			ios.SetStdoutTTY(tt.tty)
			ios.SetStderrTTY(tt.tty)

			f := &cmdutil.Factory{
				IOStreams: ios,
			}
			cmd := NewCmdView(f, func(*ViewOptions) error {
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

			assert.Equal(t, tt.wantOpts.Template, tt.wantOpts.Template)
		})
	}
}
