package itemlist

import (
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"

	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

func TestNewCmdList(t *testing.T) {
	tests := []struct {
		name          string
		cli           string
		wants         listOpts
		wantsErr      bool
		wantsErrMsg   string
		wantsExporter bool
	}{
		{
			name:        "not-a-number",
			cli:         "x",
			wantsErr:    true,
			wantsErrMsg: "invalid number: x",
		},
		{
			name: "number",
			cli:  "123",
			wants: listOpts{
				number: 123,
				limit:  30,
			},
		},
		{
			name: "owner",
			cli:  "--owner monalisa",
			wants: listOpts{
				owner: "monalisa",
				limit: 30,
			},
		},
		{
			name: "json",
			cli:  "--format json",
			wants: listOpts{
				limit: 30,
			},
			wantsExporter: true,
		},
	}

	t.Setenv("GH_TOKEN", "auth-token")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts listOpts
			cmd := NewCmdList(f, func(config listConfig) error {
				gotOpts = config.opts
				return nil
			})

			cmd.SetArgs(argv)
			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				assert.Error(t, err)
				assert.Equal(t, tt.wantsErrMsg, err.Error())
				return
			}
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.number, gotOpts.number)
			assert.Equal(t, tt.wants.owner, gotOpts.owner)
			assert.Equal(t, tt.wantsExporter, gotOpts.exporter != nil)
			assert.Equal(t, tt.wants.limit, gotOpts.limit)
		})
	}
}

func TestRunList_User_tty(t *testing.T) {
	defer gock.Off()
	// gock.Observe(gock.DumpRequest)

	// get user ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserOrgOwner.*",
			"variables": map[string]interface{}{
				"login": "monalisa",
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"id": "an ID",
				},
			},
			"errors": []interface{}{
				map[string]interface{}{
					"type": "NOT_FOUND",
					"path": []string{"organization"},
				},
			},
		})

	// list project items
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]interface{}{
			"query": "query UserProjectWithItems.*",
			"variables": map[string]interface{}{
				"firstItems":  queries.LimitDefault,
				"afterItems":  nil,
				"firstFields": queries.LimitMax,
				"afterFields": nil,
				"login":       "monalisa",
				"number":      1,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"items": map[string]interface{}{
							"nodes": []map[string]interface{}{
								{
									"id": "issue ID",
									"content": map[string]interface{}{
										"__typename": "Issue",
										"title":      "an issue",
										"number":     1,
										"repository": map[string]string{
											"nameWithOwner": "cli/go-gh",
										},
									},
								},
								{
									"id": "pull request ID",
									"content": map[string]interface{}{
										"__typename": "PullRequest",
										"title":      "a pull request",
										"number":     2,
										"repository": map[string]string{
											"nameWithOwner": "cli/go-gh",
										},
									},
								},
								{
									"id": "draft issue ID",
									"content": map[string]interface{}{
										"id":         "draft issue ID",
										"title":      "draft issue",
										"__typename": "DraftIssue",
									},
								},
							},
						},
					},
				},
			},
		})

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	config := listConfig{
		opts: listOpts{
			number: 1,
			owner:  "monalisa",
		},
		client: client,
		io:     ios,
	}

	err := runList(config)
	assert.NoError(t, err)
	assert.Equal(t, heredoc.Doc(`
		TYPE         TITLE           NUMBER  REPOSITORY  ID
		Issue        an issue        1       cli/go-gh   issue ID
		PullRequest  a pull request  2       cli/go-gh   pull request ID
		DraftIssue   draft issue                         draft issue ID
  `), stdout.String())
}

func TestRunList_User(t *testing.T) {
	defer gock.Off()
	// gock.Observe(gock.DumpRequest)

	// get user ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserOrgOwner.*",
			"variables": map[string]interface{}{
				"login": "monalisa",
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"id": "an ID",
				},
			},
			"errors": []interface{}{
				map[string]interface{}{
					"type": "NOT_FOUND",
					"path": []string{"organization"},
				},
			},
		})

	// list project items
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]interface{}{
			"query": "query UserProjectWithItems.*",
			"variables": map[string]interface{}{
				"firstItems":  queries.LimitDefault,
				"afterItems":  nil,
				"firstFields": queries.LimitMax,
				"afterFields": nil,
				"login":       "monalisa",
				"number":      1,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"items": map[string]interface{}{
							"nodes": []map[string]interface{}{
								{
									"id": "issue ID",
									"content": map[string]interface{}{
										"__typename": "Issue",
										"title":      "an issue",
										"number":     1,
										"repository": map[string]string{
											"nameWithOwner": "cli/go-gh",
										},
									},
								},
								{
									"id": "pull request ID",
									"content": map[string]interface{}{
										"__typename": "PullRequest",
										"title":      "a pull request",
										"number":     2,
										"repository": map[string]string{
											"nameWithOwner": "cli/go-gh",
										},
									},
								},
								{
									"id": "draft issue ID",
									"content": map[string]interface{}{
										"id":         "draft issue ID",
										"title":      "draft issue",
										"__typename": "DraftIssue",
									},
								},
							},
						},
					},
				},
			},
		})

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	config := listConfig{
		opts: listOpts{
			number: 1,
			owner:  "monalisa",
		},
		client: client,
		io:     ios,
	}

	err := runList(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Issue\tan issue\t1\tcli/go-gh\tissue ID\nPullRequest\ta pull request\t2\tcli/go-gh\tpull request ID\nDraftIssue\tdraft issue\t\t\tdraft issue ID\n",
		stdout.String())
}

func TestRunList_Org(t *testing.T) {
	defer gock.Off()
	// gock.Observe(gock.DumpRequest)

	// get org ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserOrgOwner.*",
			"variables": map[string]interface{}{
				"login": "github",
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"organization": map[string]interface{}{
					"id": "an ID",
				},
			},
			"errors": []interface{}{
				map[string]interface{}{
					"type": "NOT_FOUND",
					"path": []string{"user"},
				},
			},
		})

	// list project items
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]interface{}{
			"query": "query OrgProjectWithItems.*",
			"variables": map[string]interface{}{
				"firstItems":  queries.LimitDefault,
				"afterItems":  nil,
				"firstFields": queries.LimitMax,
				"afterFields": nil,
				"login":       "github",
				"number":      1,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"organization": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"items": map[string]interface{}{
							"nodes": []map[string]interface{}{
								{
									"id": "issue ID",
									"content": map[string]interface{}{
										"__typename": "Issue",
										"title":      "an issue",
										"number":     1,
										"repository": map[string]string{
											"nameWithOwner": "cli/go-gh",
										},
									},
								},
								{
									"id": "pull request ID",
									"content": map[string]interface{}{
										"__typename": "PullRequest",
										"title":      "a pull request",
										"number":     2,
										"repository": map[string]string{
											"nameWithOwner": "cli/go-gh",
										},
									},
								},
								{
									"id": "draft issue ID",
									"content": map[string]interface{}{
										"id":         "draft issue ID",
										"title":      "draft issue",
										"__typename": "DraftIssue",
									},
								},
							},
						},
					},
				},
			},
		})

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	config := listConfig{
		opts: listOpts{
			number: 1,
			owner:  "github",
		},
		client: client,
		io:     ios,
	}

	err := runList(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Issue\tan issue\t1\tcli/go-gh\tissue ID\nPullRequest\ta pull request\t2\tcli/go-gh\tpull request ID\nDraftIssue\tdraft issue\t\t\tdraft issue ID\n",
		stdout.String())
}

func TestRunList_Me(t *testing.T) {
	defer gock.Off()
	// gock.Observe(gock.DumpRequest)

	// get viewer ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query ViewerOwner.*",
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"viewer": map[string]interface{}{
					"id": "an ID",
				},
			},
		})

	// list project items
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]interface{}{
			"query": "query ViewerProjectWithItems.*",
			"variables": map[string]interface{}{
				"firstItems":  queries.LimitDefault,
				"afterItems":  nil,
				"firstFields": queries.LimitMax,
				"afterFields": nil,
				"number":      1,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"viewer": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"items": map[string]interface{}{
							"nodes": []map[string]interface{}{
								{
									"id": "issue ID",
									"content": map[string]interface{}{
										"__typename": "Issue",
										"title":      "an issue",
										"number":     1,
										"repository": map[string]string{
											"nameWithOwner": "cli/go-gh",
										},
									},
								},
								{
									"id": "pull request ID",
									"content": map[string]interface{}{
										"__typename": "PullRequest",
										"title":      "a pull request",
										"number":     2,
										"repository": map[string]string{
											"nameWithOwner": "cli/go-gh",
										},
									},
								},
								{
									"id": "draft issue ID",
									"content": map[string]interface{}{
										"id":         "draft issue ID",
										"title":      "draft issue",
										"__typename": "DraftIssue",
									},
								},
							},
						},
					},
				},
			},
		})

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	config := listConfig{
		opts: listOpts{
			number: 1,
			owner:  "@me",
		},
		client: client,
		io:     ios,
	}

	err := runList(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Issue\tan issue\t1\tcli/go-gh\tissue ID\nPullRequest\ta pull request\t2\tcli/go-gh\tpull request ID\nDraftIssue\tdraft issue\t\t\tdraft issue ID\n",
		stdout.String())
}

func TestRunList_JSON(t *testing.T) {
	defer gock.Off()
	// gock.Observe(gock.DumpRequest)

	// get user ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserOrgOwner.*",
			"variables": map[string]interface{}{
				"login": "monalisa",
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"id": "an ID",
				},
			},
			"errors": []interface{}{
				map[string]interface{}{
					"type": "NOT_FOUND",
					"path": []string{"organization"},
				},
			},
		})

	// list project items
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]interface{}{
			"query": "query UserProjectWithItems.*",
			"variables": map[string]interface{}{
				"firstItems":  queries.LimitDefault,
				"afterItems":  nil,
				"firstFields": queries.LimitMax,
				"afterFields": nil,
				"login":       "monalisa",
				"number":      1,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"items": map[string]interface{}{
							"nodes": []map[string]interface{}{
								{
									"id": "issue ID",
									"content": map[string]interface{}{
										"__typename": "Issue",
										"title":      "an issue",
										"number":     1,
										"repository": map[string]string{
											"nameWithOwner": "cli/go-gh",
										},
									},
								},
								{
									"id": "pull request ID",
									"content": map[string]interface{}{
										"__typename": "PullRequest",
										"title":      "a pull request",
										"number":     2,
										"repository": map[string]string{
											"nameWithOwner": "cli/go-gh",
										},
									},
								},
								{
									"id": "draft issue ID",
									"content": map[string]interface{}{
										"id":         "draft issue ID",
										"title":      "draft issue",
										"__typename": "DraftIssue",
									},
								},
							},
							"totalCount": 3,
						},
					},
				},
			},
		})

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	config := listConfig{
		opts: listOpts{
			number:   1,
			owner:    "monalisa",
			exporter: cmdutil.NewJSONExporter(),
		},
		client: client,
		io:     ios,
	}

	err := runList(config)
	assert.NoError(t, err)
	assert.JSONEq(
		t,
		`{"items":[{"content":{"type":"Issue","body":"","title":"an issue","number":1,"repository":"cli/go-gh","url":""},"id":"issue ID"},{"content":{"type":"PullRequest","body":"","title":"a pull request","number":2,"repository":"cli/go-gh","url":""},"id":"pull request ID"},{"content":{"type":"DraftIssue","body":"","title":"draft issue","id":"draft issue ID"},"id":"draft issue ID"}],"totalCount":3}`,
		stdout.String())
}
