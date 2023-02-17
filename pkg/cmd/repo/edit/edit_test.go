package edit

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/pkg/prompt"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdEdit(t *testing.T) {
	tests := []struct {
		name     string
		args     string
		wantOpts EditOptions
		wantErr  string
	}{
		{
			name: "change repo description",
			args: "--description hello",
			wantOpts: EditOptions{
				Repository: ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				Edits: EditRepositoryInput{
					Description: sp("hello"),
				},
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
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
				HttpClient: func() (*http.Client, error) {
					return nil, nil
				},
			}

			argv, err := shlex.Split(tt.args)
			assert.NoError(t, err)

			var gotOpts *EditOptions
			cmd := NewCmdEdit(f, func(opts *EditOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.Flags().BoolP("help", "x", false, "")

			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			_, err = cmd.ExecuteC()
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)

			assert.Equal(t, ghrepo.FullName(tt.wantOpts.Repository), ghrepo.FullName(gotOpts.Repository))
			assert.Equal(t, tt.wantOpts.Edits, gotOpts.Edits)
		})
	}
}

func Test_editRun(t *testing.T) {
	tests := []struct {
		name        string
		opts        EditOptions
		httpStubs   func(*testing.T, *httpmock.Registry)
		wantsStderr string
		wantsErr    string
	}{
		{
			name: "change name and description",
			opts: EditOptions{
				Repository: ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				Edits: EditRepositoryInput{
					Homepage:    sp("newURL"),
					Description: sp("hello world!"),
				},
			},
			httpStubs: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]interface{}) {
						assert.Equal(t, 2, len(payload))
						assert.Equal(t, "newURL", payload["homepage"])
						assert.Equal(t, "hello world!", payload["description"])
					}))
			},
		},
		{
			name: "add and remove topics",
			opts: EditOptions{
				Repository:   ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				AddTopics:    []string{"topic1", "topic2"},
				RemoveTopics: []string{"topic3"},
			},
			httpStubs: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/topics"),
					httpmock.StringResponse(`{"names":["topic2", "topic3", "go"]}`))
				r.Register(
					httpmock.REST("PUT", "repos/OWNER/REPO/topics"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]interface{}) {
						assert.Equal(t, 1, len(payload))
						assert.Equal(t, []interface{}{"topic2", "go", "topic1"}, payload["names"])
					}))
			},
		},
		{
			name: "allow update branch",
			opts: EditOptions{
				Repository: ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				Edits: EditRepositoryInput{
					AllowUpdateBranch: bp(true),
				},
			},
			httpStubs: func(t *testing.T, r *httpmock.Registry) {
				r.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]interface{}) {
						assert.Equal(t, 1, len(payload))
						assert.Equal(t, true, payload["allow_update_branch"])
					}))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)

			httpReg := &httpmock.Registry{}
			defer httpReg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(t, httpReg)
			}

			opts := &tt.opts
			opts.HTTPClient = &http.Client{Transport: httpReg}
			opts.IO = ios
			err := editRun(context.Background(), opts)
			if tt.wantsErr == "" {
				require.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantsErr)
				return
			}
		})
	}
}

func Test_editRun_interactive(t *testing.T) {
	tests := []struct {
		name        string
		opts        EditOptions
		askStubs    func(*prompt.AskStubber)
		httpStubs   func(*testing.T, *httpmock.Registry)
		wantsStderr string
		wantsErr    string
	}{
		{
			name: "updates repo description",
			opts: EditOptions{
				Repository:      ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				InteractiveMode: true,
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubPrompt("What do you want to edit?").AnswerWith([]string{"Description"})
				as.StubPrompt("Description of the repository").AnswerWith("awesome repo description")
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`
					{
						"data": {
							"repository": {
								"description": "old description",
								"homePageUrl": "https://url.com",
								"defaultBranchRef": {
									"name": "main"
								},
								"isInOrganization": false,
								"repositoryTopics": {
									"nodes": [{
										"topic": {
											"name": "x"
										}
									}]
								}
							}
						}
					}`))
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]interface{}) {
						assert.Equal(t, "awesome repo description", payload["description"])
					}))
			},
		},
		{
			name: "updates repo topics",
			opts: EditOptions{
				Repository:      ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				InteractiveMode: true,
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubPrompt("What do you want to edit?").AnswerWith([]string{"Description", "Topics"})
				as.StubPrompt("Description of the repository").AnswerWith("awesome repo description")
				as.StubPrompt("Add topics?(csv format)").AnswerWith("a, b,c,d ")
				as.StubPrompt("Remove Topics").AnswerWith([]string{"x"})
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`
					{
						"data": {
							"repository": {
								"description": "old description",
								"homePageUrl": "https://url.com",
								"defaultBranchRef": {
									"name": "main"
								},
								"isInOrganization": false,
								"repositoryTopics": {
									"nodes": [{
										"topic": {
											"name": "x"
										}
									}]
								}
							}
						}
					}`))
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]interface{}) {
						assert.Equal(t, "awesome repo description", payload["description"])
					}))
				reg.Register(
					httpmock.REST("PUT", "repos/OWNER/REPO/topics"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]interface{}) {
						assert.Equal(t, []interface{}{"a", "b", "c", "d"}, payload["names"])
					}))
			},
		},
		{
			name: "updates repo merge options",
			opts: EditOptions{
				Repository:      ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
				InteractiveMode: true,
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubPrompt("What do you want to edit?").AnswerWith([]string{"Merge Options"})
				as.StubPrompt("Allowed merge strategies").AnswerWith([]string{allowMergeCommits, allowRebaseMerge})
				as.StubPrompt("Enable Auto Merge?").AnswerWith(false)
				as.StubPrompt("Automatically delete head branches after merging?").AnswerWith(false)
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryInfo\b`),
					httpmock.StringResponse(`
					{
						"data": {
							"repository": {
								"description": "old description",
								"homePageUrl": "https://url.com",
								"defaultBranchRef": {
									"name": "main"
								},
								"isInOrganization": false,
								"squashMergeAllowed": false,
								"rebaseMergeAllowed": true,
								"mergeCommitAllowed": true,
								"deleteBranchOnMerge": false,
								"repositoryTopics": {
									"nodes": [{
										"topic": {
											"name": "x"
										}
									}]
								}
							}
						}
					}`))
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.RESTPayload(200, `{}`, func(payload map[string]interface{}) {
						assert.Equal(t, true, payload["allow_merge_commit"])
						assert.Equal(t, false, payload["allow_squash_merge"])
						assert.Equal(t, true, payload["allow_rebase_merge"])
					}))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)

			httpReg := &httpmock.Registry{}
			defer httpReg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(t, httpReg)
			}

			opts := &tt.opts
			opts.HTTPClient = &http.Client{Transport: httpReg}
			opts.IO = ios
			//nolint:staticcheck // SA1019: prompt.NewAskStubber is deprecated: use PrompterMock
			as := prompt.NewAskStubber(t)
			if tt.askStubs != nil {
				tt.askStubs(as)
			}

			err := editRun(context.Background(), opts)
			if tt.wantsErr == "" {
				require.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.wantsErr)
				return
			}
		})
	}
}

func sp(v string) *string {
	return &v
}

func bp(b bool) *bool {
	return &b
}
