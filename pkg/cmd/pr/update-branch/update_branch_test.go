package update_branch

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

func TestNewCmdUpdateBranch(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		output   UpdateBranchOptions
		wantsErr string
	}{
		{
			name:   "no argument",
			input:  "",
			output: UpdateBranchOptions{},
		},
		{
			name:  "with argument",
			input: "23",
			output: UpdateBranchOptions{
				SelectorArg: "23",
			},
		},
		{
			name:  "no argument, --rebase",
			input: "--rebase",
			output: UpdateBranchOptions{
				Rebase: true,
			},
		},
		{
			name:  "with argument, --rebase",
			input: "23 --rebase",
			output: UpdateBranchOptions{
				SelectorArg: "23",
				Rebase:      true,
			},
		},
		{
			name:     "no argument, --repo",
			input:    "--repo owner/repo",
			wantsErr: "argument required when using the --repo flag",
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

			var gotOpts *UpdateBranchOptions
			cmd := NewCmdUpdateBranch(f, func(opts *UpdateBranchOptions) error {
				gotOpts = opts
				return nil
			})

			cmd.PersistentFlags().StringP("repo", "R", "", "")

			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)

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

func Test_updateBranchRun(t *testing.T) {
	defaultInput := func() UpdateBranchOptions {
		return UpdateBranchOptions{
			Finder: shared.NewMockFinder("123", &api.PullRequest{
				ID:                  "123",
				Number:              123,
				HeadRefOid:          "head-ref-oid",
				HeadRefName:         "head-ref-name",
				HeadRepositoryOwner: api.Owner{Login: "head-repository-owner"},
				Mergeable:           api.PullRequestMergeableMergeable,
			}, ghrepo.New("OWNER", "REPO")),
		}
	}

	tests := []struct {
		name      string
		input     *UpdateBranchOptions
		httpStubs func(*testing.T, *httpmock.Registry)
		stdout    string
		stderr    string
		wantsErr  string
	}{
		{
			name: "failure, pr not found",
			input: &UpdateBranchOptions{
				Finder:      shared.NewMockFinder("", nil, nil),
				SelectorArg: "123",
			},
			wantsErr: "no pull requests found",
		},
		{
			name: "success, already up-to-date",
			input: &UpdateBranchOptions{
				SelectorArg: "123",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query ComparePullRequestBaseBranchWith\b`),
					httpmock.GraphQLQuery(`{
						"data": {
							"repository": {
								"pullRequest": {
									"baseRef": {
										"compare": {
											"aheadBy": 999,
											"behindBy": 0,
											"Status": "AHEAD"
										}
									}
								}
							}
						}
					}`, func(_ string, inputs map[string]interface{}) {
						assert.Equal(t, float64(123), inputs["pullRequestNumber"])
						assert.Equal(t, "head-repository-owner:head-ref-name", inputs["headRef"])
					}))
			},
			stdout: "✓ PR branch already up-to-date\n",
		},
		{
			name: "success, already up-to-date, PR branch on the same repo as base",
			input: &UpdateBranchOptions{
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					ID:                  "123",
					Number:              123,
					HeadRefOid:          "head-ref-oid",
					HeadRefName:         "head-ref-name",
					HeadRepositoryOwner: api.Owner{Login: "OWNER"},
					Mergeable:           api.PullRequestMergeableMergeable,
				}, ghrepo.New("OWNER", "REPO")),
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query ComparePullRequestBaseBranchWith\b`),
					httpmock.GraphQLQuery(`{
						"data": {
							"repository": {
								"pullRequest": {
									"baseRef": {
										"compare": {
											"aheadBy": 999,
											"behindBy": 0,
											"Status": "AHEAD"
										}
									}
								}
							}
						}
					}`, func(_ string, inputs map[string]interface{}) {
						assert.Equal(t, float64(123), inputs["pullRequestNumber"])
						assert.Equal(t, "head-ref-name", inputs["headRef"])
					}))
			},
			stdout: "✓ PR branch already up-to-date\n",
		},
		{
			name: "failure, not mergeable due to conflicts",
			input: &UpdateBranchOptions{
				SelectorArg: "123",
				Finder: shared.NewMockFinder("123", &api.PullRequest{
					ID:                  "123",
					Number:              123,
					HeadRefOid:          "head-ref-oid",
					HeadRefName:         "head-ref-name",
					HeadRepositoryOwner: api.Owner{Login: "OWNER"},
					Mergeable:           api.PullRequestMergeableConflicting,
				}, ghrepo.New("OWNER", "REPO")),
			},
			stderr:   "X Cannot update PR branch due to conflicts\n",
			wantsErr: cmdutil.SilentError.Error(),
		},
		{
			name: "success, merge",
			input: &UpdateBranchOptions{
				SelectorArg: "123",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query ComparePullRequestBaseBranchWith\b`),
					httpmock.GraphQLQuery(`{
						"data": {
							"repository": {
								"pullRequest": {
									"baseRef": {
										"compare": {
											"aheadBy": 0,
											"behindBy": 999,
											"Status": "BEHIND"
										}
									}
								}
							}
						}
					}`, func(_ string, inputs map[string]interface{}) {
						assert.Equal(t, float64(123), inputs["pullRequestNumber"])
						assert.Equal(t, "head-repository-owner:head-ref-name", inputs["headRef"])
					}))
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
			stdout: "✓ PR branch updated\n",
		},
		{
			name: "success, rebase",
			input: &UpdateBranchOptions{
				SelectorArg: "123",
				Rebase:      true,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query ComparePullRequestBaseBranchWith\b`),
					httpmock.GraphQLQuery(`{
						"data": {
							"repository": {
								"pullRequest": {
									"baseRef": {
										"compare": {
											"aheadBy": 0,
											"behindBy": 999,
											"Status": "BEHIND"
										}
									}
								}
							}
						}
					}`, func(_ string, inputs map[string]interface{}) {
						assert.Equal(t, float64(123), inputs["pullRequestNumber"])
						assert.Equal(t, "head-repository-owner:head-ref-name", inputs["headRef"])
					}))
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
			stdout: "✓ PR branch updated\n",
		},
		{
			name: "failure, API error on ref comparison request",
			input: &UpdateBranchOptions{
				SelectorArg: "123",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query ComparePullRequestBaseBranchWith\b`),
					httpmock.GraphQLQuery(`{
						"data": {},
						"errors": [
							{
								"message": "some error"
							}
						]
					}`, func(_ string, inputs map[string]interface{}) {
						assert.Equal(t, float64(123), inputs["pullRequestNumber"])
						assert.Equal(t, "head-repository-owner:head-ref-name", inputs["headRef"])
					}))
			},
			wantsErr: "GraphQL: some error",
		},
		{
			name: "failure, merge conflict error on update request",
			input: &UpdateBranchOptions{
				SelectorArg: "123",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query ComparePullRequestBaseBranchWith\b`),
					httpmock.GraphQLQuery(`{
						"data": {
							"repository": {
								"pullRequest": {
									"baseRef": {
										"compare": {
											"aheadBy": 999,
											"behindBy": 999,
											"Status": "BEHIND"
										}
									}
								}
							}
						}
					}`, func(_ string, inputs map[string]interface{}) {
						assert.Equal(t, float64(123), inputs["pullRequestNumber"])
						assert.Equal(t, "head-repository-owner:head-ref-name", inputs["headRef"])
					}))
				reg.Register(
					httpmock.GraphQL(`mutation PullRequestUpdateBranch\b`),
					httpmock.GraphQLMutation(`{
						"data": {},
						"errors": [
							{
								"message": "merge conflict between base and head (updatePullRequestBranch)"
							}
						]
					}`, func(inputs map[string]interface{}) {
						assert.Equal(t, "123", inputs["pullRequestId"])
						assert.Equal(t, "head-ref-oid", inputs["expectedHeadOid"])
						assert.Equal(t, "MERGE", inputs["updateMethod"])
					}))
			},
			stderr:   "X Cannot update PR branch due to conflicts\n",
			wantsErr: cmdutil.SilentError.Error(),
		},
		{
			name: "failure, API error on update request",
			input: &UpdateBranchOptions{
				SelectorArg: "123",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query ComparePullRequestBaseBranchWith\b`),
					httpmock.GraphQLQuery(`{
						"data": {
							"repository": {
								"pullRequest": {
									"baseRef": {
										"compare": {
											"aheadBy": 0,
											"behindBy": 999,
											"Status": "BEHIND"
										}
									}
								}
							}
						}
					}`, func(_ string, inputs map[string]interface{}) {
						assert.Equal(t, float64(123), inputs["pullRequestNumber"])
						assert.Equal(t, "head-repository-owner:head-ref-name", inputs["headRef"])
					}))
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

			err := updateBranchRun(tt.input)

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
