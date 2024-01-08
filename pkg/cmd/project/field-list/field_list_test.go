package fieldlist

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
				assert.Equal(t, tt.wantsErrMsg, err.Error())
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.number, gotOpts.number)
			assert.Equal(t, tt.wants.owner, gotOpts.owner)
			assert.Equal(t, tt.wants.limit, gotOpts.limit)
			assert.Equal(t, tt.wantsExporter, gotOpts.exporter != nil)
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

	// list project fields
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]interface{}{
			"query": "query UserProject.*",
			"variables": map[string]interface{}{
				"login":       "monalisa",
				"number":      1,
				"firstItems":  queries.LimitMax,
				"afterItems":  nil,
				"firstFields": queries.LimitDefault,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"fields": map[string]interface{}{
							"nodes": []map[string]interface{}{
								{
									"__typename": "ProjectV2Field",
									"name":       "FieldTitle",
									"id":         "field ID",
								},
								{
									"__typename": "ProjectV2SingleSelectField",
									"name":       "Status",
									"id":         "status ID",
								},
								{
									"__typename": "ProjectV2IterationField",
									"name":       "Iterations",
									"id":         "iteration ID",
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
		NAME        DATA TYPE                   ID
		FieldTitle  ProjectV2Field              field ID
		Status      ProjectV2SingleSelectField  status ID
		Iterations  ProjectV2IterationField     iteration ID
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

	// list project fields
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]interface{}{
			"query": "query UserProject.*",
			"variables": map[string]interface{}{
				"login":       "monalisa",
				"number":      1,
				"firstItems":  queries.LimitMax,
				"afterItems":  nil,
				"firstFields": queries.LimitDefault,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"fields": map[string]interface{}{
							"nodes": []map[string]interface{}{
								{
									"__typename": "ProjectV2Field",
									"name":       "FieldTitle",
									"id":         "field ID",
								},
								{
									"__typename": "ProjectV2SingleSelectField",
									"name":       "Status",
									"id":         "status ID",
								},
								{
									"__typename": "ProjectV2IterationField",
									"name":       "Iterations",
									"id":         "iteration ID",
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
		"FieldTitle\tProjectV2Field\tfield ID\nStatus\tProjectV2SingleSelectField\tstatus ID\nIterations\tProjectV2IterationField\titeration ID\n",
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

	// list project fields
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]interface{}{
			"query": "query OrgProject.*",
			"variables": map[string]interface{}{
				"login":       "github",
				"number":      1,
				"firstItems":  queries.LimitMax,
				"afterItems":  nil,
				"firstFields": queries.LimitDefault,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"organization": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"fields": map[string]interface{}{
							"nodes": []map[string]interface{}{
								{
									"__typename": "ProjectV2Field",
									"name":       "FieldTitle",
									"id":         "field ID",
								},
								{
									"__typename": "ProjectV2SingleSelectField",
									"name":       "Status",
									"id":         "status ID",
								},
								{
									"__typename": "ProjectV2IterationField",
									"name":       "Iterations",
									"id":         "iteration ID",
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
		"FieldTitle\tProjectV2Field\tfield ID\nStatus\tProjectV2SingleSelectField\tstatus ID\nIterations\tProjectV2IterationField\titeration ID\n",
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

	// list project fields
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]interface{}{
			"query": "query ViewerProject.*",
			"variables": map[string]interface{}{
				"number":      1,
				"firstItems":  queries.LimitMax,
				"afterItems":  nil,
				"firstFields": queries.LimitDefault,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"viewer": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"fields": map[string]interface{}{
							"nodes": []map[string]interface{}{
								{
									"__typename": "ProjectV2Field",
									"name":       "FieldTitle",
									"id":         "field ID",
								},
								{
									"__typename": "ProjectV2SingleSelectField",
									"name":       "Status",
									"id":         "status ID",
								},
								{
									"__typename": "ProjectV2IterationField",
									"name":       "Iterations",
									"id":         "iteration ID",
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
		"FieldTitle\tProjectV2Field\tfield ID\nStatus\tProjectV2SingleSelectField\tstatus ID\nIterations\tProjectV2IterationField\titeration ID\n",
		stdout.String())
}

func TestRunList_Empty(t *testing.T) {
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

	// list project fields
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]interface{}{
			"query": "query ViewerProject.*",
			"variables": map[string]interface{}{
				"number":      1,
				"firstItems":  queries.LimitMax,
				"afterItems":  nil,
				"firstFields": queries.LimitDefault,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"viewer": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"fields": map[string]interface{}{
							"nodes": nil,
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
			owner:  "@me",
		},
		client: client,
		io:     ios,
	}

	err := runList(config)
	assert.EqualError(
		t,
		err,
		"Project 1 for owner @me has no fields")
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

	// list project fields
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]interface{}{
			"query": "query UserProject.*",
			"variables": map[string]interface{}{
				"login":       "monalisa",
				"number":      1,
				"firstItems":  queries.LimitMax,
				"afterItems":  nil,
				"firstFields": queries.LimitDefault,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"fields": map[string]interface{}{
							"nodes": []map[string]interface{}{
								{
									"__typename": "ProjectV2Field",
									"name":       "FieldTitle",
									"id":         "field ID",
								},
								{
									"__typename": "ProjectV2SingleSelectField",
									"name":       "Status",
									"id":         "status ID",
								},
								{
									"__typename": "ProjectV2IterationField",
									"name":       "Iterations",
									"id":         "iteration ID",
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
		`{"fields":[{"id":"field ID","name":"FieldTitle","type":"ProjectV2Field"},{"id":"status ID","name":"Status","type":"ProjectV2SingleSelectField"},{"id":"iteration ID","name":"Iterations","type":"ProjectV2IterationField"}],"totalCount":3}`,
		stdout.String())
}
