package delete

import (
	"bytes"
	"errors"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdDelete(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		tty        bool
		output     DeleteOptions
		wantErr    bool
		wantErrMsg string
		wantStderr string
	}{
		{
			name:   "confirm flag",
			tty:    true,
			input:  "OWNER/REPO --confirm",
			output: DeleteOptions{RepoArg: "OWNER/REPO", Confirmed: true},
		},
		{
			name:   "yes flag",
			tty:    true,
			input:  "OWNER/REPO --yes",
			output: DeleteOptions{RepoArg: "OWNER/REPO", Confirmed: true},
		},
		{
			name:       "no confirmation notty",
			input:      "OWNER/REPO",
			output:     DeleteOptions{RepoArg: "OWNER/REPO"},
			wantErr:    true,
			wantErrMsg: "--yes required when not running interactively",
		},
		{
			name:   "base repo resolution",
			input:  "",
			tty:    true,
			output: DeleteOptions{},
		},
		{
			name:       "yes flag ignored when no argument tty",
			tty:        true,
			input:      "--yes",
			output:     DeleteOptions{Confirmed: false}, // --yes should be ignored
			wantErr:    false,
			wantStderr: "Warning: `--yes` is ignored since no repository was specified\n",
		},
		{
			name:       "yes flag error when no argument notty",
			input:      "--yes",
			wantErr:    true,
			wantErrMsg: "cannot non-interactively delete current repository. Please specify a repository or run interactively",
		},
		{
			name:       "confirm flag error when no argument notty",
			input:      "--confirm",
			wantErr:    true,
			wantErrMsg: "cannot non-interactively delete current repository. Please specify a repository or run interactively",
		},
		{
			name:       "confirm flag also ignored when no argument tty",
			tty:        true,
			input:      "--confirm",
			output:     DeleteOptions{Confirmed: false}, // --confirm should also be ignored
			wantStderr: "Warning: `--yes` is ignored since no repository was specified\n",
			// Note: This does not confuse the user, as deprecation warnings are shown: "Flag --confirm has been deprecated, use `--yes` instead"
		},
		{
			name:   "accept cascading deletion of private forks flag",
			tty:    true,
			input:  "OWNER/REPO --accept-cascading-deletion-of-private-forks",
			output: DeleteOptions{RepoArg: "OWNER/REPO", AcceptCascadingDeletionOfPrivateForks: true},
		},
		{
			name:       "accept cascading deletion of private forks flag does not replace yes notty",
			input:      "OWNER/REPO --accept-cascading-deletion-of-private-forks",
			wantErr:    true,
			wantErrMsg: "--yes required when not running interactively",
		},
		{
			name:   "yes and accept cascading deletion of private forks flags notty",
			input:  "OWNER/REPO --yes --accept-cascading-deletion-of-private-forks",
			output: DeleteOptions{RepoArg: "OWNER/REPO", Confirmed: true, AcceptCascadingDeletionOfPrivateForks: true},
		},
		{
			name:       "accept cascading deletion of private forks flag ignored when no argument tty",
			tty:        true,
			input:      "--accept-cascading-deletion-of-private-forks",
			output:     DeleteOptions{AcceptCascadingDeletionOfPrivateForks: false},
			wantStderr: "Warning: `--accept-cascading-deletion-of-private-forks` is ignored since no repository was specified\n",
		},
		{
			name:       "yes and accept cascading deletion of private forks flags ignored when no argument tty",
			tty:        true,
			input:      "--yes --accept-cascading-deletion-of-private-forks",
			output:     DeleteOptions{Confirmed: false, AcceptCascadingDeletionOfPrivateForks: false},
			wantStderr: "Warning: `--yes` is ignored since no repository was specified\nWarning: `--accept-cascading-deletion-of-private-forks` is ignored since no repository was specified\n",
		},
		{
			name:       "accept cascading deletion of private forks flag error when no argument notty",
			input:      "--accept-cascading-deletion-of-private-forks",
			wantErr:    true,
			wantErrMsg: "cannot non-interactively delete current repository. Please specify a repository or run interactively",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, stdErr := iostreams.Test()
			ios.SetStdinTTY(tt.tty)
			ios.SetStdoutTTY(tt.tty)
			f := &cmdutil.Factory{
				IOStreams: ios,
			}
			argv, err := shlex.Split(tt.input)
			require.NoError(t, err)
			var gotOpts *DeleteOptions
			cmd := NewCmdDelete(f, func(opts *DeleteOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(stdErr)

			_, err = cmd.ExecuteC()

			if tt.wantErr {
				require.EqualError(t, err, tt.wantErrMsg)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantStderr, stdErr.String())
			assert.Equal(t, tt.output.RepoArg, gotOpts.RepoArg)
			assert.Equal(t, tt.output.Confirmed, gotOpts.Confirmed)
			assert.Equal(t, tt.output.AcceptCascadingDeletionOfPrivateForks, gotOpts.AcceptCascadingDeletionOfPrivateForks)
		})
	}
}

func Test_deleteRun(t *testing.T) {
	tests := []struct {
		name                 string
		tty                  bool
		opts                 *DeleteOptions
		httpStubs            func(*httpmock.Registry)
		prompterStubs        func(*prompter.PrompterMock)
		wantConfirmPrompt    string
		wantConfirmDeletion  string
		wantStdout           string
		wantStderr           string
		wantErr              bool
		errMsg               string
		wantScopesSuggestion string
	}{
		{
			name:                "prompting confirmation tty",
			tty:                 true,
			opts:                &DeleteOptions{RepoArg: "OWNER/REPO"},
			wantConfirmDeletion: "OWNER/REPO",
			wantStdout:          "✓ Deleted repository OWNER/REPO\n",
			prompterStubs: func(p *prompter.PrompterMock) {
				p.ConfirmDeletionFunc = func(_ string) error {
					return nil
				}
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO"),
					httpmock.StringResponse(`{"private":false,"fork":false,"forks_count":0}`))
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(204, "{}"))
			},
		},
		{
			name:                "infer base repo",
			tty:                 true,
			opts:                &DeleteOptions{},
			wantConfirmDeletion: "OWNER/REPO",
			wantStdout:          "✓ Deleted repository OWNER/REPO\n",
			prompterStubs: func(p *prompter.PrompterMock) {
				p.ConfirmDeletionFunc = func(_ string) error {
					return nil
				}
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO"),
					httpmock.StringResponse(`{"private":false,"fork":false,"forks_count":0}`))
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(204, "{}"))
			},
		},
		{
			name: "confirmation no tty",
			opts: &DeleteOptions{
				RepoArg:   "OWNER/REPO",
				Confirmed: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO"),
					httpmock.StringResponse(`{"private":false,"fork":false,"forks_count":0}`))
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(204, "{}"))
			},
		},
		{
			name:                "short repo name",
			opts:                &DeleteOptions{RepoArg: "REPO"},
			wantConfirmDeletion: "OWNER/REPO",
			wantStdout:          "✓ Deleted repository OWNER/REPO\n",
			tty:                 true,
			prompterStubs: func(p *prompter.PrompterMock) {
				p.ConfirmDeletionFunc = func(_ string) error {
					return nil
				}
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"OWNER"}}}`))
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO"),
					httpmock.StringResponse(`{"private":false,"fork":false,"forks_count":0}`))
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(204, "{}"))
			},
		},
		{
			name:       "repo transferred ownership",
			opts:       &DeleteOptions{RepoArg: "OWNER/REPO", Confirmed: true},
			wantErr:    true,
			errMsg:     "SilentError",
			wantStderr: "X Failed to delete repository: OWNER/REPO has changed name or transferred ownership\n",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO"),
					httpmock.StringResponse(`{"private":false,"fork":false,"forks_count":0}`))
				// The Location header is what makes this a real redirect. Without it Go declines
				// to follow the 307 regardless of the client's CheckRedirect policy, so the stub
				// would pass even if deleteRepo stopped suppressing redirects.
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO"),
					httpmock.WithHeader(
						httpmock.StatusStringResponse(307, "{}"),
						"Location", "https://api.github.com/repos/OWNER/RENAMED"))
			},
		},
		{
			name:                "private repo with forks asks to delete forks tty",
			tty:                 true,
			opts:                &DeleteOptions{RepoArg: "OWNER/REPO"},
			wantConfirmPrompt:   "Delete OWNER/REPO and its forks?",
			wantConfirmDeletion: "OWNER/REPO",
			wantStdout:          "✓ Deleted repository OWNER/REPO\n",
			wantStderr:          "! This will also delete all 3 forks since this is a private repository.\n",
			prompterStubs: func(p *prompter.PrompterMock) {
				p.ConfirmFunc = func(_ string, _ bool) (bool, error) {
					return true, nil
				}
				p.ConfirmDeletionFunc = func(_ string) error {
					return nil
				}
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO"),
					httpmock.StringResponse(`{"private":true,"fork":false,"forks_count":3}`))
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(204, "{}"))
			},
		},
		{
			// --yes skips typing the repository name, but not the fork prompt.
			name:              "private repo with forks asks to delete forks even with yes tty",
			tty:               true,
			opts:              &DeleteOptions{RepoArg: "OWNER/REPO", Confirmed: true},
			wantConfirmPrompt: "Delete OWNER/REPO and its forks?",
			wantStdout:        "✓ Deleted repository OWNER/REPO\n",
			wantStderr:        "! This will also delete 1 fork since this is a private repository.\n",
			prompterStubs: func(p *prompter.PrompterMock) {
				p.ConfirmFunc = func(_ string, _ bool) (bool, error) {
					return true, nil
				}
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO"),
					httpmock.StringResponse(`{"private":true,"fork":false,"forks_count":1}`))
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(204, "{}"))
			},
		},
		{
			name:              "declining to delete forks cancels tty",
			tty:               true,
			opts:              &DeleteOptions{RepoArg: "OWNER/REPO"},
			wantConfirmPrompt: "Delete OWNER/REPO and its forks?",
			wantStderr:        "! This will also delete all 3 forks since this is a private repository.\n",
			wantErr:           true,
			errMsg:            "CancelError",
			prompterStubs: func(p *prompter.PrompterMock) {
				p.ConfirmFunc = func(_ string, _ bool) (bool, error) {
					return false, nil
				}
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO"),
					httpmock.StringResponse(`{"private":true,"fork":false,"forks_count":3}`))
			},
		},
		{
			name:              "fork prompt failure stops deletion tty",
			tty:               true,
			opts:              &DeleteOptions{RepoArg: "OWNER/REPO"},
			wantConfirmPrompt: "Delete OWNER/REPO and its forks?",
			wantStderr:        "! This will also delete all 3 forks since this is a private repository.\n",
			wantErr:           true,
			errMsg:            "prompt failed",
			prompterStubs: func(p *prompter.PrompterMock) {
				p.ConfirmFunc = func(_ string, _ bool) (bool, error) {
					return false, errors.New("prompt failed")
				}
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO"),
					httpmock.StringResponse(`{"private":true,"fork":false,"forks_count":3}`))
			},
		},
		{
			name:    "private repo with forks requires accept flag notty",
			opts:    &DeleteOptions{RepoArg: "OWNER/REPO", Confirmed: true},
			wantErr: true,
			errMsg:  "--accept-cascading-deletion-of-private-forks required when not running interactively: deleting OWNER/REPO will also delete all 3 forks since this is a private repository",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO"),
					httpmock.StringResponse(`{"private":true,"fork":false,"forks_count":3}`))
			},
		},
		{
			// The REST API reports internal repositories as private.
			name:    "internal repo with forks requires accept flag notty",
			opts:    &DeleteOptions{RepoArg: "OWNER/REPO", Confirmed: true},
			wantErr: true,
			errMsg:  "--accept-cascading-deletion-of-private-forks required when not running interactively: deleting OWNER/REPO will also delete all 2 forks since this is a private repository",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO"),
					httpmock.StringResponse(`{"private":true,"visibility":"internal","fork":false,"forks_count":2}`))
			},
		},
		{
			name: "accept flag skips fork lookup notty",
			opts: &DeleteOptions{RepoArg: "OWNER/REPO", Confirmed: true, AcceptCascadingDeletionOfPrivateForks: true},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(204, "{}"))
			},
		},
		{
			name:                "accept flag skips fork prompt tty",
			tty:                 true,
			opts:                &DeleteOptions{RepoArg: "OWNER/REPO", AcceptCascadingDeletionOfPrivateForks: true},
			wantConfirmDeletion: "OWNER/REPO",
			wantStdout:          "✓ Deleted repository OWNER/REPO\n",
			prompterStubs: func(p *prompter.PrompterMock) {
				p.ConfirmDeletionFunc = func(_ string) error {
					return nil
				}
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(204, "{}"))
			},
		},
		{
			name: "enterprise repo is looked up and deleted on its own host notty",
			opts: &DeleteOptions{RepoArg: "example.com/OWNER/REPO", Confirmed: true},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("GET", "api/v3/repos/OWNER/REPO"), "example.com"),
					httpmock.StringResponse(`{"private":true,"fork":false,"forks_count":0}`))
				reg.Register(
					httpmock.WithHost(httpmock.REST("DELETE", "api/v3/repos/OWNER/REPO"), "example.com"),
					httpmock.StatusStringResponse(204, "{}"))
			},
		},
		{
			// Deleting a fork does not delete the fork's own forks.
			name: "private fork with forks does not need fork confirmation notty",
			opts: &DeleteOptions{RepoArg: "OWNER/REPO", Confirmed: true},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO"),
					httpmock.StringResponse(`{"private":true,"fork":true,"forks_count":2}`))
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(204, "{}"))
			},
		},
		{
			name: "public repo with forks does not need fork confirmation notty",
			opts: &DeleteOptions{RepoArg: "OWNER/REPO", Confirmed: true},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO"),
					httpmock.StringResponse(`{"private":false,"fork":false,"forks_count":5}`))
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(204, "{}"))
			},
		},
		{
			name: "private repo without forks does not need fork confirmation notty",
			opts: &DeleteOptions{RepoArg: "OWNER/REPO", Confirmed: true},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO"),
					httpmock.StringResponse(`{"private":true,"fork":false,"forks_count":0}`))
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(204, "{}"))
			},
		},
		{
			// The lookup is the first request, so it must carry the DELETE's scope hint.
			name:                 "fork lookup failure stops deletion",
			opts:                 &DeleteOptions{RepoArg: "OWNER/REPO", Confirmed: true},
			wantErr:              true,
			errMsg:               "HTTP 404: Not Found (https://api.github.com/repos/OWNER/REPO)",
			wantScopesSuggestion: `This API operation needs the "delete_repo" scope. To request it, run:  gh auth refresh -h github.com -s delete_repo`,
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO"),
					httpmock.WithHeader(
						httpmock.StatusJSONResponse(404, map[string]string{"message": "Not Found"}),
						"X-Oauth-Scopes", "repo, read:org"))
			},
		},
		{
			name:    "malformed fork lookup response stops deletion",
			opts:    &DeleteOptions{RepoArg: "OWNER/REPO", Confirmed: true},
			wantErr: true,
			errMsg:  "unexpected EOF",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO"),
					httpmock.StringResponse(`{"private":true,`))
			},
		},
		{
			// GET requests for a renamed repository are redirected with a 301.
			name:       "repo moved before fork lookup",
			opts:       &DeleteOptions{RepoArg: "OWNER/REPO", Confirmed: true},
			wantErr:    true,
			errMsg:     "SilentError",
			wantStderr: "X Failed to delete repository: OWNER/REPO has changed name or transferred ownership\n",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO"),
					httpmock.WithHeader(
						httpmock.StatusStringResponse(301, "{}"),
						"Location", "https://api.github.com/repositories/123"))
			},
		},
	}
	for _, tt := range tests {
		pm := &prompter.PrompterMock{}
		if tt.prompterStubs != nil {
			tt.prompterStubs(pm)
		}
		tt.opts.Prompter = pm

		tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		}

		reg := &httpmock.Registry{}
		if tt.httpStubs != nil {
			tt.httpStubs(reg)
		}
		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}

		ios, _, stdout, stderr := iostreams.Test()
		ios.SetStdinTTY(tt.tty)
		ios.SetStdoutTTY(tt.tty)
		tt.opts.IO = ios

		t.Run(tt.name, func(t *testing.T) {
			defer reg.Verify(t)
			err := deleteRun(tt.opts)
			if tt.wantErr {
				require.EqualError(t, err, tt.errMsg)
			} else {
				require.NoError(t, err)
			}
			if tt.wantScopesSuggestion != "" {
				var httpErr api.HTTPError
				require.ErrorAs(t, err, &httpErr)
				assert.Equal(t, tt.wantScopesSuggestion, httpErr.ScopesSuggestion())
			}
			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
			if tt.wantConfirmPrompt == "" {
				assert.Empty(t, pm.ConfirmCalls())
			} else {
				calls := pm.ConfirmCalls()
				require.Len(t, calls, 1)
				assert.Equal(t, tt.wantConfirmPrompt, calls[0].Prompt)
				assert.False(t, calls[0].DefaultValue)
			}
			if tt.wantConfirmDeletion == "" {
				assert.Empty(t, pm.ConfirmDeletionCalls())
			} else {
				calls := pm.ConfirmDeletionCalls()
				require.Len(t, calls, 1)
				assert.Equal(t, tt.wantConfirmDeletion, calls[0].RequiredValue)
			}
		})
	}
}
