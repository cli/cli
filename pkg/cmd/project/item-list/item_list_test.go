package itemlist

import (
	"testing"

	"github.com/MakeNowJust/heredoc"
	fd "github.com/cli/cli/v2/internal/featuredetection"
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
		{
			name: "query",
			cli:  `--query "assignee:octocat"`,
			wants: listOpts{
				limit: 30,
				query: "assignee:octocat",
			},
		},
		{
			name: "field",
			cli:  "--field Status --field Priority",
			wants: listOpts{
				limit:  30,
				fields: []string{"Status", "Priority"},
			},
		},
		{
			name: "field-id",
			cli:  "--field-id FIELD_ID",
			wants: listOpts{
				limit:    30,
				fieldIDs: []string{"FIELD_ID"},
			},
		},
		{
			name:        "field and field-id conflict",
			cli:         "--field Status --field-id FIELD_ID",
			wantsErr:    true,
			wantsErrMsg: "only one of `--field` or `--field-id` may be used",
		},
		{
			name:        "format and field conflict",
			cli:         "--format json --field Status",
			wantsErr:    true,
			wantsErrMsg: "cannot use `--format` with `--field` or `--field-id`",
		},
		{
			name:        "format and field-id conflict",
			cli:         "--format json --field-id FIELD_ID",
			wantsErr:    true,
			wantsErrMsg: "cannot use `--format` with `--field` or `--field-id`",
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
			assert.Equal(t, tt.wants.query, gotOpts.query)
			assert.Equal(t, tt.wantsExporter, gotOpts.exporter != nil)
			assert.Equal(t, tt.wants.limit, gotOpts.limit)
			assert.Equal(t, tt.wants.fields, gotOpts.fields)
			assert.Equal(t, tt.wants.fieldIDs, gotOpts.fieldIDs)
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
		JSON(map[string]any{
			"query": "query UserOrgOwner.*",
			"variables": map[string]any{
				"login": "monalisa",
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"id": "an ID",
				},
			},
			"errors": []any{
				map[string]any{
					"type": "NOT_FOUND",
					"path": []string{"organization"},
				},
			},
		})

	// list project items
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]any{
			"query": "query UserProjectWithItems.*",
			"variables": map[string]any{
				"firstItems":  queries.LimitDefault,
				"afterItems":  nil,
				"firstFields": queries.LimitMax,
				"afterFields": nil,
				"login":       "monalisa",
				"number":      1,
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"projectV2": map[string]any{
						"items": map[string]any{
							"nodes": []map[string]any{
								{
									"id": "issue ID",
									"content": map[string]any{
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
									"content": map[string]any{
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
									"content": map[string]any{
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
		JSON(map[string]any{
			"query": "query UserOrgOwner.*",
			"variables": map[string]any{
				"login": "monalisa",
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"id": "an ID",
				},
			},
			"errors": []any{
				map[string]any{
					"type": "NOT_FOUND",
					"path": []string{"organization"},
				},
			},
		})

	// list project items
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]any{
			"query": "query UserProjectWithItems.*",
			"variables": map[string]any{
				"firstItems":  queries.LimitDefault,
				"afterItems":  nil,
				"firstFields": queries.LimitMax,
				"afterFields": nil,
				"login":       "monalisa",
				"number":      1,
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"projectV2": map[string]any{
						"items": map[string]any{
							"nodes": []map[string]any{
								{
									"id": "issue ID",
									"content": map[string]any{
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
									"content": map[string]any{
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
									"content": map[string]any{
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
		JSON(map[string]any{
			"query": "query UserOrgOwner.*",
			"variables": map[string]any{
				"login": "github",
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"organization": map[string]any{
					"id": "an ID",
				},
			},
			"errors": []any{
				map[string]any{
					"type": "NOT_FOUND",
					"path": []string{"user"},
				},
			},
		})

	// list project items
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]any{
			"query": "query OrgProjectWithItems.*",
			"variables": map[string]any{
				"firstItems":  queries.LimitDefault,
				"afterItems":  nil,
				"firstFields": queries.LimitMax,
				"afterFields": nil,
				"login":       "github",
				"number":      1,
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"organization": map[string]any{
					"projectV2": map[string]any{
						"items": map[string]any{
							"nodes": []map[string]any{
								{
									"id": "issue ID",
									"content": map[string]any{
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
									"content": map[string]any{
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
									"content": map[string]any{
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
		JSON(map[string]any{
			"query": "query ViewerOwner.*",
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"viewer": map[string]any{
					"id": "an ID",
				},
			},
		})

	// list project items
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]any{
			"query": "query ViewerProjectWithItems.*",
			"variables": map[string]any{
				"firstItems":  queries.LimitDefault,
				"afterItems":  nil,
				"firstFields": queries.LimitMax,
				"afterFields": nil,
				"number":      1,
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"viewer": map[string]any{
					"projectV2": map[string]any{
						"items": map[string]any{
							"nodes": []map[string]any{
								{
									"id": "issue ID",
									"content": map[string]any{
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
									"content": map[string]any{
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
									"content": map[string]any{
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
		JSON(map[string]any{
			"query": "query UserOrgOwner.*",
			"variables": map[string]any{
				"login": "monalisa",
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"id": "an ID",
				},
			},
			"errors": []any{
				map[string]any{
					"type": "NOT_FOUND",
					"path": []string{"organization"},
				},
			},
		})

	// list project items
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]any{
			"query": "query UserProjectWithItems.*",
			"variables": map[string]any{
				"firstItems":  queries.LimitDefault,
				"afterItems":  nil,
				"firstFields": queries.LimitMax,
				"afterFields": nil,
				"login":       "monalisa",
				"number":      1,
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"projectV2": map[string]any{
						"items": map[string]any{
							"nodes": []map[string]any{
								{
									"id": "issue ID",
									"content": map[string]any{
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
									"content": map[string]any{
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
									"content": map[string]any{
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

func TestRunList_WithQuery(t *testing.T) {
	defer gock.Off()

	// get user ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]any{
			"query": "query UserOrgOwner.*",
			"variables": map[string]any{
				"login": "monalisa",
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"id": "an ID",
				},
			},
			"errors": []any{
				map[string]any{
					"type": "NOT_FOUND",
					"path": []string{"organization"},
				},
			},
		})

	// list project items with query
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]any{
			"query": "query UserProjectWithItems.*",
			"variables": map[string]any{
				"firstItems":  queries.LimitDefault,
				"afterItems":  nil,
				"firstFields": queries.LimitMax,
				"afterFields": nil,
				"login":       "monalisa",
				"number":      1,
				"query":       "assignee:octocat -status:Done",
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"projectV2": map[string]any{
						"items": map[string]any{
							"nodes": []map[string]any{
								{
									"id": "issue ID",
									"content": map[string]any{
										"__typename": "Issue",
										"title":      "an issue",
										"number":     1,
										"repository": map[string]string{
											"nameWithOwner": "cli/go-gh",
										},
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
			query:  "assignee:octocat -status:Done",
		},
		client:   client,
		detector: &fd.EnabledDetectorMock{},
		io:       ios,
	}

	err := runList(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Issue\tan issue\t1\tcli/go-gh\tissue ID\n",
		stdout.String())
}

func TestRunList_QueryUnsupported(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	config := listConfig{
		opts: listOpts{
			number: 1,
			owner:  "monalisa",
			query:  "assignee:octocat",
		},
		detector: &fd.DisabledDetectorMock{},
		io:       ios,
	}

	err := runList(config)
	assert.EqualError(t, err, "the `--query` flag is not supported on this GitHub host")
}

func TestRunList_FieldColumn(t *testing.T) {
	defer gock.Off()

	// get user ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]any{
			"query": "query UserOrgOwner.*",
			"variables": map[string]any{
				"login": "monalisa",
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"id": "an ID",
				},
			},
			"errors": []any{
				map[string]any{
					"type": "NOT_FOUND",
					"path": []string{"organization"},
				},
			},
		})

	// list project items with fields
	gock.New("https://api.github.com").
		Post("/graphql").
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"projectV2": map[string]any{
						"fields": map[string]any{
							"nodes": []map[string]any{
								{
									"__typename": "ProjectV2SingleSelectField",
									"id":         "status ID",
									"name":       "Status",
									"dataType":   "SINGLE_SELECT",
								},
								{
									"__typename": "ProjectV2Field",
									"id":         "est ID",
									"name":       "Est",
									"dataType":   "NUMBER",
								},
								{
									"__typename": "ProjectV2Field",
									"id":         "tags ID",
									"name":       "Tags",
									"dataType":   "LABELS",
								},
								{
									"__typename": "ProjectV2IterationField",
									"id":         "iter ID",
									"name":       "Iter",
									"dataType":   "ITERATION",
								},
							},
						},
						"items": map[string]any{
							"nodes": []map[string]any{
								{
									"id": "issue ID",
									"content": map[string]any{
										"__typename": "Issue",
										"title":      "an issue",
										"number":     1,
										"repository": map[string]string{
											"nameWithOwner": "cli/go-gh",
										},
									},
									"fieldValues": map[string]any{
										"nodes": []map[string]any{
											{
												"__typename": "ProjectV2ItemFieldSingleSelectValue",
												"name":       "In Progress",
												"field": map[string]any{
													"__typename": "ProjectV2SingleSelectField",
													"id":         "status ID",
												},
											},
											{
												"__typename": "ProjectV2ItemFieldNumberValue",
												"number":     5,
												"field": map[string]any{
													"__typename": "ProjectV2Field",
													"id":         "est ID",
												},
											},
											{
												"__typename": "ProjectV2ItemFieldLabelValue",
												"labels": map[string]any{
													"nodes": []map[string]any{
														{"name": "bug"},
														{"name": "p1"},
													},
												},
												"field": map[string]any{
													"__typename": "ProjectV2Field",
													"id":         "tags ID",
												},
											},
											{
												"__typename": "ProjectV2ItemFieldIterationValue",
												"title":      "S1",
												"field": map[string]any{
													"__typename": "ProjectV2IterationField",
													"id":         "iter ID",
												},
											},
										},
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
			fields: []string{"Status", "Est", "Tags", "Iter"},
		},
		client: client,
		io:     ios,
	}

	err := runList(config)
	assert.NoError(t, err)
	assert.Equal(t, heredoc.Doc(`
		TYPE   TITLE     NUMBER  REPOSITORY  ID        STATUS       EST  TAGS     ITER
		Issue  an issue  1       cli/go-gh   issue ID  In Progress  5    bug, p1  S1
  `), stdout.String())
}

func TestRunList_FieldColumn_UnknownName(t *testing.T) {
	defer gock.Off()

	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]any{
			"query": "query UserOrgOwner.*",
			"variables": map[string]any{
				"login": "monalisa",
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"id": "an ID",
				},
			},
			"errors": []any{
				map[string]any{
					"type": "NOT_FOUND",
					"path": []string{"organization"},
				},
			},
		})

	gock.New("https://api.github.com").
		Post("/graphql").
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"projectV2": map[string]any{
						"fields": map[string]any{
							"nodes": []map[string]any{
								{
									"__typename": "ProjectV2SingleSelectField",
									"id":         "status ID",
									"name":       "Status",
									"dataType":   "SINGLE_SELECT",
								},
							},
						},
						"items": map[string]any{
							"nodes": []map[string]any{
								{
									"id": "issue ID",
									"content": map[string]any{
										"__typename": "Issue",
										"title":      "an issue",
										"number":     1,
										"repository": map[string]string{
											"nameWithOwner": "cli/go-gh",
										},
									},
								},
							},
						},
					},
				},
			},
		})

	client := queries.NewTestClient()

	ios, _, _, _ := iostreams.Test()
	config := listConfig{
		opts: listOpts{
			number: 1,
			owner:  "monalisa",
			fields: []string{"Statuss"},
		},
		client: client,
		io:     ios,
	}

	err := runList(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), `field "Statuss" not found`)
	assert.Contains(t, err.Error(), "available fields: Status")
}

func TestRunList_FieldColumn_UnknownID(t *testing.T) {
	defer gock.Off()

	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]any{
			"query": "query UserOrgOwner.*",
			"variables": map[string]any{
				"login": "monalisa",
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"id": "an ID",
				},
			},
			"errors": []any{
				map[string]any{
					"type": "NOT_FOUND",
					"path": []string{"organization"},
				},
			},
		})

	gock.New("https://api.github.com").
		Post("/graphql").
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"projectV2": map[string]any{
						"fields": map[string]any{
							"nodes": []map[string]any{
								{
									"__typename": "ProjectV2SingleSelectField",
									"id":         "status ID",
									"name":       "Status",
									"dataType":   "SINGLE_SELECT",
								},
							},
						},
						"items": map[string]any{
							"nodes": []map[string]any{
								{
									"id": "issue ID",
									"content": map[string]any{
										"__typename": "Issue",
										"title":      "an issue",
										"number":     1,
										"repository": map[string]string{
											"nameWithOwner": "cli/go-gh",
										},
									},
								},
							},
						},
					},
				},
			},
		})

	client := queries.NewTestClient()

	ios, _, _, _ := iostreams.Test()
	config := listConfig{
		opts: listOpts{
			number:   1,
			owner:    "monalisa",
			fieldIDs: []string{"missing ID"},
		},
		client: client,
		io:     ios,
	}

	err := runList(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), `field with ID "missing ID" not found`)
	assert.Contains(t, err.Error(), "available fields: Status (status ID)")
}

func TestRunList_FieldColumn_PaginatesFields(t *testing.T) {
	defer gock.Off()

	// get user ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]any{
			"query": "query UserOrgOwner.*",
			"variables": map[string]any{
				"login": "monalisa",
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"id": "an ID",
				},
			},
			"errors": []any{
				map[string]any{
					"type": "NOT_FOUND",
					"path": []string{"organization"},
				},
			},
		})

	// list project items; the fields connection is paginated, so "Priority" is not
	// on the first page of fields returned alongside the items.
	gock.New("https://api.github.com").
		Post("/graphql").
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"projectV2": map[string]any{
						"fields": map[string]any{
							"totalCount": 2,
							"pageInfo": map[string]any{
								"hasNextPage": true,
								"endCursor":   "STATUSCURSOR",
							},
							"nodes": []map[string]any{
								{
									"__typename": "ProjectV2SingleSelectField",
									"id":         "status ID",
									"name":       "Status",
									"dataType":   "SINGLE_SELECT",
								},
							},
						},
						"items": map[string]any{
							"nodes": []map[string]any{
								{
									"id": "issue ID",
									"content": map[string]any{
										"__typename": "Issue",
										"title":      "an issue",
										"number":     1,
										"repository": map[string]string{
											"nameWithOwner": "cli/go-gh",
										},
									},
									"fieldValues": map[string]any{
										"nodes": []map[string]any{
											{
												"__typename": "ProjectV2ItemFieldSingleSelectValue",
												"name":       "High",
												"field": map[string]any{
													"__typename": "ProjectV2SingleSelectField",
													"id":         "priority ID",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		})

	// fallback: fetch the full, paginated field list, which includes "Priority".
	gock.New("https://api.github.com").
		Post("/graphql").
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"projectV2": map[string]any{
						"fields": map[string]any{
							"totalCount": 2,
							"pageInfo": map[string]any{
								"hasNextPage": false,
								"endCursor":   "PRIORITYCURSOR",
							},
							"nodes": []map[string]any{
								{
									"__typename": "ProjectV2SingleSelectField",
									"id":         "status ID",
									"name":       "Status",
									"dataType":   "SINGLE_SELECT",
								},
								{
									"__typename": "ProjectV2SingleSelectField",
									"id":         "priority ID",
									"name":       "Priority",
									"dataType":   "SINGLE_SELECT",
								},
							},
						},
						"items": map[string]any{
							"nodes": []map[string]any{},
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
			fields: []string{"Priority"},
		},
		client: client,
		io:     ios,
	}

	err := runList(config)
	assert.NoError(t, err)
	assert.Equal(t, heredoc.Doc(`
		TYPE   TITLE     NUMBER  REPOSITORY  ID        PRIORITY
		Issue  an issue  1       cli/go-gh   issue ID  High
	`), stdout.String())
}
