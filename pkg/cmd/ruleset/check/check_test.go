package check

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewCmdCheck(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		isTTY   bool
		want    CheckOptions
		wantErr string
	}{
		{
			name:  "no arguments",
			args:  "",
			isTTY: true,
			want: CheckOptions{
				Branch:  "",
				Default: false,
				WebMode: false,
			},
		},
		{
			name:  "branch name",
			args:  "my-branch",
			isTTY: true,
			want: CheckOptions{
				Branch:  "my-branch",
				Default: false,
				WebMode: false,
			},
		},
		{
			name:  "default",
			args:  "--default=true",
			isTTY: true,
			want: CheckOptions{
				Branch:  "",
				Default: true,
				WebMode: false,
			},
		},
		{
			name:  "web mode",
			args:  "--web",
			isTTY: true,
			want: CheckOptions{
				Branch:  "",
				Default: false,
				WebMode: true,
			},
		},
		{
			name:    "both --default and branch name specified",
			args:    "--default asdf",
			isTTY:   true,
			wantErr: "specify only one of `--default` or a branch name",
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

			var opts *CheckOptions
			cmd := NewCmdCheck(f, func(o *CheckOptions) error {
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

			assert.Equal(t, tt.want.Branch, opts.Branch)
			assert.Equal(t, tt.want.Default, opts.Default)
			assert.Equal(t, tt.want.WebMode, opts.WebMode)
		})
	}
}

func Test_checkRun(t *testing.T) {
	tests := []struct {
		name       string
		isTTY      bool
		opts       CheckOptions
		wantErr    string
		wantStdout string
		wantStderr string
		wantBrowse string
	}{
		{
			name:  "view rules for branch",
			isTTY: true,
			opts: CheckOptions{
				Branch: "my-branch",
			},
			wantStdout: heredoc.Doc(`
			6 rules apply to branch my-branch in repo my-org/repo-name

			- commit_author_email_pattern: [name: ] [negate: false] [operator: ends_with] [pattern: @example.com] 
			  (configured in ruleset 1234 from organization my-org)

			- commit_author_email_pattern: [name: ] [negate: false] [operator: ends_with] [pattern: @example.com] 
			  (configured in ruleset 5678 from repository my-org/repo-name)

			- commit_message_pattern: [name: ] [negate: false] [operator: starts_with] [pattern: fff] 
			  (configured in ruleset 1234 from organization my-org)

			- commit_message_pattern: [name: ] [negate: false] [operator: contains] [pattern: asdf] 
			  (configured in ruleset 5678 from repository my-org/repo-name)

			- creation
			  (configured in ruleset 5678 from repository my-org/repo-name)

			- required_signatures
			  (configured in ruleset 1234 from organization my-org)

			`),
			wantStderr: "",
			wantBrowse: "",
		},
		{
			name:  "web mode, TTY",
			isTTY: true,
			opts: CheckOptions{
				Branch:  "my-branch",
				WebMode: true,
			},
			wantStdout: "Opening https://github.com/my-org/repo-name/rules in your browser.\n",
			wantStderr: "",
			wantBrowse: "https://github.com/my-org/repo-name/rules?ref=refs%2Fheads%2Fmy-branch",
		},
		{
			name:  "web mode, TTY, special character in branch name",
			isTTY: true,
			opts: CheckOptions{
				Branch:  "my-feature/my-branch",
				WebMode: true,
			},
			wantStdout: "Opening https://github.com/my-org/repo-name/rules in your browser.\n",
			wantStderr: "",
			wantBrowse: "https://github.com/my-org/repo-name/rules?ref=refs%2Fheads%2Fmy-feature%2Fmy-branch",
		},
		{
			name:  "web mode, non-TTY",
			isTTY: false,
			opts: CheckOptions{
				Branch:  "my-branch",
				WebMode: true,
			},
			wantStdout: "",
			wantStderr: "",
			wantBrowse: "https://github.com/my-org/repo-name/rules?ref=refs%2Fheads%2Fmy-branch",
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
				httpmock.REST("GET", "repos/my-org/repo-name/rules/branches/my-branch"),
				httpmock.FileResponse("./fixtures/rulesetCheck.json"),
			)

			tt.opts.IO = ios
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: fakeHTTP}, nil
			}
			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.FromFullName("my-org/repo-name")
			}
			browser := &browser.Stub{}
			tt.opts.Browser = browser

			err := checkRun(&tt.opts)

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
