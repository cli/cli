package view

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewCmdView(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		isTTY   bool
		want    ViewOptions
		wantErr string
	}{
		{
			name:  "no arguments",
			args:  "",
			isTTY: true,
			want: ViewOptions{
				ID:              "",
				WebMode:         false,
				IncludeParents:  true,
				InteractiveMode: true,
				Organization:    "",
			},
		},
		{
			name:  "only ID",
			args:  "3",
			isTTY: true,
			want: ViewOptions{
				ID:              "3",
				WebMode:         false,
				IncludeParents:  true,
				InteractiveMode: false,
				Organization:    "",
			},
		},
		{
			name:  "org",
			args:  "--org \"my-org\"",
			isTTY: true,
			want: ViewOptions{
				ID:              "",
				WebMode:         false,
				IncludeParents:  true,
				InteractiveMode: true,
				Organization:    "my-org",
			},
		},
		{
			name:  "web mode",
			args:  "--web",
			isTTY: true,
			want: ViewOptions{
				ID:              "",
				WebMode:         true,
				IncludeParents:  true,
				InteractiveMode: true,
				Organization:    "",
			},
		},
		{
			name:  "parents",
			args:  "--parents=false",
			isTTY: true,
			want: ViewOptions{
				ID:              "",
				WebMode:         false,
				IncludeParents:  false,
				InteractiveMode: true,
				Organization:    "",
			},
		},
		{
			name:    "repo and org specified",
			args:    "--org \"my-org\" -R \"owner/repo\"",
			isTTY:   true,
			wantErr: "only one of --repo and --org may be specified",
		},
		{
			name:    "invalid ID",
			args:    "1.5",
			isTTY:   true,
			wantErr: "invalid value for ruleset ID: 1.5 is not an integer",
		},
		{
			name:    "ID not provided and not TTY",
			args:    "",
			isTTY:   false,
			wantErr: "a ruleset ID must be provided when not running interactively",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)

			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			var opts *ViewOptions
			cmd := NewCmdView(f, func(o *ViewOptions) error {
				opts = o
				return nil
			})
			cmd.PersistentFlags().StringP("repo", "R", "", "")

			argv, err := shlex.Split(tt.args)
			require.NoError(t, err)
			cmd.SetArgs(argv)

			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			_, err = cmd.ExecuteC()
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.want.ID, opts.ID)
			assert.Equal(t, tt.want.WebMode, opts.WebMode)
			assert.Equal(t, tt.want.IncludeParents, opts.IncludeParents)
			assert.Equal(t, tt.want.InteractiveMode, opts.InteractiveMode)
			assert.Equal(t, tt.want.Organization, opts.Organization)
		})
	}
}

func Test_viewRun(t *testing.T) {
	tests := []struct {
		name       string
		isTTY      bool
		opts       ViewOptions
		wantErr    string
		wantStdout string
		wantStderr string
		wantBrowse string
	}{
		{
			name:  "view repo ruleset",
			isTTY: true,
			opts: ViewOptions{
				ID: "42",
			},
			wantStdout: heredoc.Doc(`

			Test Ruleset
			ID: 42
			Source: my-owner/repo-name (Repository)
			Enforceument: Active
			
			Bypass List
			- OrganizationAdmin (ID: 1), mode: always
			- RepositoryRole (ID: 5), mode: always
			
			Conditions
			- ref_name: [exclude: []] [include: [~ALL]] 
			
			Rules
			- commit_author_email_pattern: [name: ] [negate: false] [operator: ends_with] [pattern: @example.com] 
			- commit_message_pattern: [name: ] [negate: false] [operator: contains] [pattern: asdf] 
			- creation
			`),
			wantStderr: "",
			wantBrowse: "",
		},
		{
			name:  "web mode, TTY",
			isTTY: true,
			opts: ViewOptions{
				ID:      "42",
				WebMode: true,
			},
			wantStdout: "Opening github.com/my-owner/repo-name/rules/42 in your browser.\n",
			wantStderr: "",
			wantBrowse: "https://github.com/my-owner/repo-name/rules/42",
		},
		{
			name:  "web mode, non-TTY",
			isTTY: false,
			opts: ViewOptions{
				ID:      "42",
				WebMode: true,
			},
			wantStdout: "",
			wantStderr: "",
			wantBrowse: "https://github.com/my-owner/repo-name/rules/42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)

			fakeHTTP := &httpmock.Registry{}
			fakeHTTP.Register(
				httpmock.REST("GET", "repos/my-owner/repo-name/rulesets/42"),
				httpmock.FileResponse("./fixtures/rulesetView.json"),
			)

			tt.opts.IO = ios
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: fakeHTTP}, nil
			}
			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.FromFullName("my-owner/repo-name")
			}
			browser := &browser.Stub{}
			tt.opts.Browser = browser
			tt.opts.Config = func() (config.Config, error) {
				return config.NewBlankConfig(), nil
			}

			err := viewRun(&tt.opts)

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			} else {
				require.NoError(t, err)
			}

			if tt.wantBrowse != "" {
				browser.Verify(t, tt.wantBrowse)
			}

			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}
