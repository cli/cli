package develop

import (
	"errors"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
	"github.com/cli/cli/v2/test"
	"github.com/stretchr/testify/assert"
)

func Test_developRun(t *testing.T) {
	featureEnabledPayload := `{
		"data": {
			"LinkedBranch": {
				"fields": [
					{
						"name": "id"
					},
					{
						"name": "ref"
					}
				]
			}
		}
	}`

	featureDisabledPayload := `{ "data": { "LinkedBranch": null } }`

	tests := []struct {
		name           string
		setup          func(*DevelopOptions, *testing.T) func()
		cmdStubs       func(*run.CommandStubber)
		runStubs       func(*run.CommandStubber)
		remotes        map[string]string
		askStubs       func(*prompt.AskStubber) // TODO eventually migrate to PrompterMock
		httpStubs      func(*httpmock.Registry, *testing.T)
		expectedOut    string
		expectedErrOut string
		expectedBrowse string
		wantErr        string
		tty            bool
	}{
		{name: "list branches for an issue",
			setup: func(opts *DevelopOptions, t *testing.T) func() {
				opts.IssueSelector = "42"
				opts.List = true
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query LinkedBranch_fields\b`),
					httpmock.StringResponse(featureEnabledPayload),
				)
				reg.Register(
					httpmock.GraphQL(`query BranchIssueReferenceListLinkedBranches\b`),
					httpmock.GraphQLQuery(`{
						"data": {
							"repository": {
								"issue": {
									"linkedBranches": {
										"edges": [
										{
											"node": {
												"ref": {
													"name": "foo"
												}
											}
										},
										{
											"node": {
												"ref": {
													"name": "bar"
												}
											}
										}
									]
								}
							}
						}
					}
					}
					`, func(query string, inputs map[string]interface{}) {
						assert.Equal(t, float64(42), inputs["issueNumber"])
						assert.Equal(t, "OWNER", inputs["repositoryOwner"])
						assert.Equal(t, "REPO", inputs["repositoryName"])
					}))
			},
			expectedOut: "foo\nbar\n",
		},
		{name: "list branches for an issue in tty",
			setup: func(opts *DevelopOptions, t *testing.T) func() {
				opts.IssueSelector = "42"
				opts.List = true
				return func() {}
			},
			tty: true,
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query LinkedBranch_fields\b`),
					httpmock.StringResponse(featureEnabledPayload),
				)
				reg.Register(
					httpmock.GraphQL(`query BranchIssueReferenceListLinkedBranches\b`),
					httpmock.GraphQLQuery(`{
						"data": {
							"repository": {
								"issue": {
									"linkedBranches": {
										"edges": [
										{
											"node": {
												"ref": {
													"name": "foo",
													"repository": {
														"url": "http://github.localhost/OWNER/REPO"
													}
												}
											}
										},
										{
											"node": {
												"ref": {
													"name": "bar",
													"repository": {
														"url": "http://github.localhost/OWNER/OTHER-REPO"
													}
												}
											}
										}
									]
								}
							}
						}
					}
					}
					`, func(query string, inputs map[string]interface{}) {
						assert.Equal(t, float64(42), inputs["issueNumber"])
						assert.Equal(t, "OWNER", inputs["repositoryOwner"])
						assert.Equal(t, "REPO", inputs["repositoryName"])
					}))
			},
			expectedOut: "\nShowing linked branches for OWNER/REPO#42\n\nfoo  http://github.localhost/OWNER/REPO/tree/foo\nbar  http://github.localhost/OWNER/OTHER-REPO/tree/bar\n",
		},
		{name: "list branches for an issue providing an issue url",
			setup: func(opts *DevelopOptions, t *testing.T) func() {
				opts.IssueSelector = "https://github.com/cli/test-repo/issues/42"
				opts.List = true
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query LinkedBranch_fields\b`),
					httpmock.StringResponse(featureEnabledPayload),
				)
				reg.Register(
					httpmock.GraphQL(`query BranchIssueReferenceListLinkedBranches\b`),
					httpmock.GraphQLQuery(`{
						"data": {
							"repository": {
								"issue": {
									"linkedBranches": {
										"edges": [
										{
											"node": {
												"ref": {
													"name": "foo"
												}
											}
										},
										{
											"node": {
												"ref": {
													"name": "bar"
												}
											}
										}
									]
								}
							}
						}
					}
					}
					`, func(query string, inputs map[string]interface{}) {
						assert.Equal(t, float64(42), inputs["issueNumber"])
						assert.Equal(t, "cli", inputs["repositoryOwner"])
						assert.Equal(t, "test-repo", inputs["repositoryName"])
					}))
			},
			expectedOut: "foo\nbar\n",
		},
		{name: "list branches for an issue providing an issue repo",
			setup: func(opts *DevelopOptions, t *testing.T) func() {
				opts.IssueSelector = "42"
				opts.IssueRepoSelector = "cli/test-repo"
				opts.List = true
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query LinkedBranch_fields\b`),
					httpmock.StringResponse(featureEnabledPayload),
				)
				reg.Register(
					httpmock.GraphQL(`query BranchIssueReferenceListLinkedBranches\b`),
					httpmock.GraphQLQuery(`{
						"data": {
							"repository": {
								"issue": {
									"linkedBranches": {
										"edges": [
										{
											"node": {
												"ref": {
													"name": "foo"
												}
											}
										},
										{
											"node": {
												"ref": {
													"name": "bar"
												}
											}
										}
									]
								}
							}
						}
					}
					}
					`, func(query string, inputs map[string]interface{}) {
						assert.Equal(t, float64(42), inputs["issueNumber"])
						assert.Equal(t, "cli", inputs["repositoryOwner"])
						assert.Equal(t, "test-repo", inputs["repositoryName"])
					}))
			},
			expectedOut: "foo\nbar\n",
		},
		{name: "list branches for an issue providing an issue url and specifying the same repo works",
			setup: func(opts *DevelopOptions, t *testing.T) func() {
				opts.IssueSelector = "https://github.com/cli/test-repo/issues/42"
				opts.IssueRepoSelector = "cli/test-repo"
				opts.List = true
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query LinkedBranch_fields\b`),
					httpmock.StringResponse(featureEnabledPayload),
				)
				reg.Register(
					httpmock.GraphQL(`query BranchIssueReferenceListLinkedBranches\b`),
					httpmock.GraphQLQuery(`{
					"data": {
						"repository": {
							"issue": {
								"linkedBranches": {
									"edges": [
									{
										"node": {
											"ref": {
												"name": "foo"
											}
										}
									},
									{
										"node": {
											"ref": {
												"name": "bar"
											}
										}
									}
								]
							}
						}
					}
				}
				}
				`, func(query string, inputs map[string]interface{}) {
						assert.Equal(t, float64(42), inputs["issueNumber"])
						assert.Equal(t, "cli", inputs["repositoryOwner"])
						assert.Equal(t, "test-repo", inputs["repositoryName"])
					}))
			},
			expectedOut: "foo\nbar\n",
		},
		{name: "list branches for an issue providing an issue url and specifying a different repo returns an error",
			setup: func(opts *DevelopOptions, t *testing.T) func() {
				opts.IssueSelector = "https://github.com/cli/test-repo/issues/42"
				opts.IssueRepoSelector = "cli/other"
				opts.List = true
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query LinkedBranch_fields\b`),
					httpmock.StringResponse(featureEnabledPayload),
				)
			},
			wantErr: "issue repo in url cli/test-repo does not match the repo from --issue-repo cli/other",
		},
		{name: "returns an error when the feature isn't enabled in the GraphQL API",
			setup: func(opts *DevelopOptions, t *testing.T) func() {
				opts.IssueSelector = "https://github.com/cli/test-repo/issues/42"
				opts.List = true
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query LinkedBranch_fields\b`),
					httpmock.StringResponse(featureDisabledPayload),
				)
			},
			wantErr: "the `gh issue develop` command is not currently available",
		},
		{name: "develop new branch with a name provided",
			setup: func(opts *DevelopOptions, t *testing.T) func() {
				opts.Name = "my-branch"
				opts.BaseBranch = "main"
				opts.IssueSelector = "123"
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query LinkedBranch_fields\b`),
					httpmock.StringResponse(featureEnabledPayload),
				)
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`
						{ "data": { "repository": {
							"id": "REPOID",
							"hasIssuesEnabled": true
						} } }`),
				)
				reg.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{ "hasIssuesEnabled": true, "issue":{"id": "yar", "number":123, "title":"my issue"} }}}`))
				reg.Register(
					httpmock.GraphQL(`query BranchIssueReferenceFindBaseOid\b`),
					httpmock.StringResponse(`{"data":{"repository":{"ref":{"target":{"oid":"123"}}}}}`))

				reg.Register(
					httpmock.GraphQL(`(?s)mutation CreateLinkedBranch\b.*issueId: \$issueId,\s+name: \$name,\s+oid: \$oid,`),
					httpmock.GraphQLQuery(`{ "data": { "createLinkedBranch": { "linkedBranch": {"id": "2", "ref": {"name": "my-branch"} } } } }`,
						func(query string, inputs map[string]interface{}) {
							assert.Equal(t, "REPOID", inputs["repositoryId"])
							assert.Equal(t, "my-branch", inputs["name"])
							assert.Equal(t, "yar", inputs["issueId"])
						}),
				)

			},
			expectedOut: "github.com/OWNER/REPO/tree/my-branch\n",
		},
		{name: "develop new branch without a name provided omits the param from the mutation",
			setup: func(opts *DevelopOptions, t *testing.T) func() {
				opts.Name = ""
				opts.BaseBranch = "main"
				opts.IssueSelector = "123"
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query LinkedBranch_fields\b`),
					httpmock.StringResponse(featureEnabledPayload),
				)
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`
						{ "data": { "repository": {
							"id": "REPOID",
							"hasIssuesEnabled": true
						} } }`),
				)
				reg.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{ "hasIssuesEnabled": true, "issue":{"id": "yar", "number":123, "title":"my issue"} }}}`))
				reg.Register(
					httpmock.GraphQL(`query BranchIssueReferenceFindBaseOid\b`),
					httpmock.StringResponse(`{"data":{"repository":{"ref":{"target":{"oid":"123"}}}}}`))

				reg.Register(
					httpmock.GraphQL(`(?s)mutation CreateLinkedBranch\b.*\$oid: GitObjectID!, \$repositoryId:.*issueId: \$issueId,\s+oid: \$oid,`),
					httpmock.GraphQLQuery(`{ "data": { "createLinkedBranch": { "linkedBranch": {"id": "2", "ref": {"name": "my-issue-1"} } } } }`,
						func(query string, inputs map[string]interface{}) {
							assert.Equal(t, "REPOID", inputs["repositoryId"])
							assert.Equal(t, "", inputs["name"])
							assert.Equal(t, "yar", inputs["issueId"])
						}),
				)

			},
			expectedOut: "github.com/OWNER/REPO/tree/my-issue-1\n",
		},
		{name: "develop providing an issue url and specifying a different repo returns an error",
			setup: func(opts *DevelopOptions, t *testing.T) func() {
				opts.IssueSelector = "https://github.com/cli/test-repo/issues/42"
				opts.IssueRepoSelector = "cli/other"
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query LinkedBranch_fields\b`),
					httpmock.StringResponse(featureEnabledPayload),
				)
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`
						{ "data": { "repository": {
							"id": "REPOID",
							"hasIssuesEnabled": true
						} } }`),
				)
			},
			wantErr: "issue repo in url cli/test-repo does not match the repo from --issue-repo cli/other",
		},
		{name: "develop new branch with checkout when the branch exists locally",
			setup: func(opts *DevelopOptions, t *testing.T) func() {
				opts.Name = "my-branch"
				opts.BaseBranch = "main"
				opts.IssueSelector = "123"
				opts.Checkout = true
				return func() {}
			},
			remotes: map[string]string{
				"origin": "OWNER/REPO",
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query LinkedBranch_fields\b`),
					httpmock.StringResponse(featureEnabledPayload),
				)
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`
						{ "data": { "repository": {
							"id": "REPOID",
							"hasIssuesEnabled": true
						} } }`),
				)
				reg.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{ "hasIssuesEnabled": true, "issue":{"id": "yar", "number":123, "title":"my issue"} }}}`))
				reg.Register(
					httpmock.GraphQL(`query BranchIssueReferenceFindBaseOid\b`),
					httpmock.StringResponse(`{"data":{"repository":{"ref":{"target":{"oid":"123"}}}}}`))

				reg.Register(
					httpmock.GraphQL(`mutation CreateLinkedBranch\b`),
					httpmock.GraphQLQuery(`{ "data": { "createLinkedBranch": { "linkedBranch": {"id": "2", "ref": {"name": "my-branch"} } } } }`,
						func(query string, inputs map[string]interface{}) {
							assert.Equal(t, "REPOID", inputs["repositoryId"])
							assert.Equal(t, "my-branch", inputs["name"])
							assert.Equal(t, "yar", inputs["issueId"])
						}),
				)

			},
			runStubs: func(cs *run.CommandStubber) {
				cs.Register(`git rev-parse --verify refs/heads/my-branch`, 0, "")
				cs.Register(`git checkout my-branch`, 0, "")
				cs.Register(`git pull --ff-only origin my-branch`, 0, "")
			},
			expectedOut: "github.com/OWNER/REPO/tree/my-branch\n",
		},
		{name: "develop new branch with checkout when the branch does not exist locally",
			setup: func(opts *DevelopOptions, t *testing.T) func() {
				opts.Name = "my-branch"
				opts.BaseBranch = "main"
				opts.IssueSelector = "123"
				opts.Checkout = true
				return func() {}
			},
			remotes: map[string]string{
				"origin": "OWNER/REPO",
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query LinkedBranch_fields\b`),
					httpmock.StringResponse(featureEnabledPayload),
				)
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`
						{ "data": { "repository": {
							"id": "REPOID",
							"hasIssuesEnabled": true
						} } }`),
				)
				reg.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{ "hasIssuesEnabled": true, "issue":{"id": "yar", "number":123, "title":"my issue"} }}}`))
				reg.Register(
					httpmock.GraphQL(`query BranchIssueReferenceFindBaseOid\b`),
					httpmock.StringResponse(`{"data":{"repository":{"ref":{"target":{"oid":"123"}}}}}`))

				reg.Register(
					httpmock.GraphQL(`mutation CreateLinkedBranch\b`),
					httpmock.GraphQLQuery(`{ "data": { "createLinkedBranch": { "linkedBranch": {"id": "2", "ref": {"name": "my-branch"} } } } }`,
						func(query string, inputs map[string]interface{}) {
							assert.Equal(t, "REPOID", inputs["repositoryId"])
							assert.Equal(t, "my-branch", inputs["name"])
							assert.Equal(t, "yar", inputs["issueId"])
						}),
				)

			},
			runStubs: func(cs *run.CommandStubber) {
				cs.Register(`git rev-parse --verify refs/heads/my-branch`, 1, "")
				cs.Register(`git fetch origin \+refs/heads/my-branch:refs/remotes/origin/my-branch`, 0, "")
				cs.Register(`git checkout -b my-branch --track origin/my-branch`, 0, "")
				cs.Register(`git pull --ff-only origin my-branch`, 0, "")
			},
			expectedOut: "github.com/OWNER/REPO/tree/my-branch\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg, t)
			}

			opts := DevelopOptions{}

			ios, _, stdout, stderr := iostreams.Test()

			ios.SetStdoutTTY(tt.tty)
			ios.SetStdinTTY(tt.tty)
			ios.SetStderrTTY(tt.tty)
			opts.IO = ios

			opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.New("OWNER", "REPO"), nil
			}
			opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			opts.Config = func() (config.Config, error) {
				return config.NewBlankConfig(), nil
			}

			opts.Remotes = func() (context.Remotes, error) {
				if len(tt.remotes) == 0 {
					return nil, errors.New("no remotes")
				}
				var remotes context.Remotes
				for name, repo := range tt.remotes {
					r, err := ghrepo.FromFullName(repo)
					if err != nil {
						return remotes, err
					}
					remotes = append(remotes, &context.Remote{
						Remote: &git.Remote{Name: name},
						Repo:   r,
					})
				}
				return remotes, nil
			}

			opts.GitClient = &git.Client{
				GhPath:  "some/path/gh",
				GitPath: "some/path/git",
			}

			cmdStubs, cmdTeardown := run.Stub()
			defer cmdTeardown(t)
			if tt.runStubs != nil {
				tt.runStubs(cmdStubs)
			}

			cleanSetup := func() {}
			if tt.setup != nil {
				cleanSetup = tt.setup(&opts, t)
			}
			defer cleanSetup()

			var err error
			if opts.List {
				err = developRunList(&opts)
			} else {

				err = developRunCreate(&opts)
			}
			output := &test.CmdOut{
				OutBuf: stdout,
				ErrBuf: stderr,
			}
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedOut, output.String())
				assert.Equal(t, tt.expectedErrOut, output.Stderr())
			}
		})
	}
}
