package update

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
	shared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdUpdate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		output   UpdateOptions
		wantsErr string
	}{
		{
			name:   "no argument",
			input:  "",
			output: UpdateOptions{},
		},
		{
			name:  "with argument",
			input: "23",
			output: UpdateOptions{
				SelectorArg: "23",
			},
		},
		{
			name:  "no argument, --rebase",
			input: "--rebase",
			output: UpdateOptions{
				Rebase: true,
			},
		},
		{
			name:  "with argument, --rebase",
			input: "23 --rebase",
			output: UpdateOptions{
				SelectorArg: "23",
				Rebase:      true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)

			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)

			var gotOpts *UpdateOptions
			cmd := NewCmdUpdate(f, func(opts *UpdateOptions) error {
				gotOpts = opts
				return nil
			})

			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantsErr != "" {
				assert.EqualError(t, err, tt.wantsErr)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.output.SelectorArg, gotOpts.SelectorArg)
			assert.Equal(t, tt.output.Rebase, gotOpts.Rebase)
		})
	}
}

func Test_updateRun(t *testing.T) {
	defaultInput := func() UpdateOptions {
		return UpdateOptions{
			Finder: shared.NewMockFinder("123", &api.PullRequest{
				ID:         "123",
				HeadRefOid: "head-ref-oid",
			}, ghrepo.New("OWNER", "REPO")),
		}
	}

	tests := []struct {
		name      string
		input     *UpdateOptions
		httpStubs func(*testing.T, *httpmock.Registry)
		stdout    string
		stderr    string
		wantsErr  string
	}{
		{
			name: "success, merge",
			input: &UpdateOptions{
				SelectorArg: "123",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestUpdateBranch\b`),
					httpmock.GraphQLMutation(`{
						"data": {
							"updatePullRequestBranch": {
								"pullRequest": {}
							}
						}
					}`, func(inputs map[string]interface{}) {
						assert.Equal(t, "123", inputs["pullRequestId"])
						assert.Equal(t, "head-ref-oid", inputs["expectedHeadOid"])
						assert.Equal(t, "MERGE", inputs["updateMethod"])
					}))
			},
			stdout: "",
			stderr: "✓ PR branch updated\n",
		},
		{
			name: "success, rebase",
			input: &UpdateOptions{
				SelectorArg: "123",
				Rebase:      true,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestUpdateBranch\b`),
					httpmock.GraphQLMutation(`{
						"data": {
							"updatePullRequestBranch": {
								"pullRequest": {}
							}
						}
					}`, func(inputs map[string]interface{}) {
						assert.Equal(t, "123", inputs["pullRequestId"])
						assert.Equal(t, "head-ref-oid", inputs["expectedHeadOid"])
						assert.Equal(t, "REBASE", inputs["updateMethod"])
					}))
			},
			stdout: "",
			stderr: "✓ PR branch updated\n",
		},
		{
			name: "failure, pr not found",
			input: &UpdateOptions{
				Finder:      shared.NewMockFinder("", nil, nil),
				SelectorArg: "123",
			},
			wantsErr: "no pull requests found",
		},
		{
			name: "failure, API error",
			input: &UpdateOptions{
				SelectorArg: "123",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestUpdateBranch\b`),
					httpmock.GraphQLMutation(`{
						"data": {},
						"errors": [
							{
								"message": "some error"
							}
						]
					}`, func(inputs map[string]interface{}) {
						assert.Equal(t, "123", inputs["pullRequestId"])
						assert.Equal(t, "head-ref-oid", inputs["expectedHeadOid"])
						assert.Equal(t, "MERGE", inputs["updateMethod"])
					}))
			},
			wantsErr: "GraphQL: some error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)

			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(t, reg)
			}

			tt.input.GitClient = &git.Client{
				GhPath:  "some/path/gh",
				GitPath: "some/path/git",
			}

			if tt.input.Finder == nil {
				tt.input.Finder = defaultInput().Finder
			}

			httpClient := func() (*http.Client, error) { return &http.Client{Transport: reg}, nil }

			tt.input.IO = ios
			tt.input.HttpClient = httpClient

			err := updateRun(tt.input)

			if tt.wantsErr != "" {
				assert.EqualError(t, err, tt.wantsErr)
				return
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.stdout, stdout.String())
			assert.Equal(t, tt.stderr, stderr.String())
		})
	}
}
