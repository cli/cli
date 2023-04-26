package fieldlist

import (
	"os"
	"testing"

	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

func TestNewCmdList(t *testing.T) {
	tests := []struct {
		name        string
		cli         string
		wants       listOpts
		wantsErr    bool
		wantsErrMsg string
	}{
		{
			name:        "user-and-org",
			cli:         "--user monalisa --org github",
			wantsErr:    true,
			wantsErrMsg: "if any flags in the group [user org] are set none of the others can be; [org user] were all set",
		},
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
			},
		},
		{
			name: "user",
			cli:  "--user monalisa",
			wants: listOpts{
				userOwner: "monalisa",
			},
		},
		{
			name: "org",
			cli:  "--org github",
			wants: listOpts{
				orgOwner: "github",
			},
		},
		{
			name: "limit",
			cli:  "--limit all",
			wants: listOpts{
				limit: "all",
			},
		},
		{
			name: "json",
			cli:  "--format json",
			wants: listOpts{
				format: "json",
			},
		},
	}

	os.Setenv("GH_TOKEN", "auth-token")
	defer os.Unsetenv("GH_TOKEN")

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
			assert.Equal(t, tt.wants.userOwner, gotOpts.userOwner)
			assert.Equal(t, tt.wants.orgOwner, gotOpts.orgOwner)
			assert.Equal(t, tt.wants.limit, gotOpts.limit)
			assert.Equal(t, tt.wants.format, gotOpts.format)
		})
	}
}

func TestRunList_User(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// get user ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserLogin.*",
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
		})

	// list project fields
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]interface{}{
			"query": "query UserProject.*",
			"variables": map[string]interface{}{
				"login":       "monalisa",
				"number":      1,
				"firstItems":  100,
				"afterItems":  nil,
				"firstFields": 100,
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

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	ios, _, stdout, _ := iostreams.Test()
	config := listConfig{
		tp: tableprinter.New(ios),
		opts: listOpts{
			number:    1,
			userOwner: "monalisa",
		},
		client: client,
	}

	err = runList(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Name\tDataType\tID\nFieldTitle\tProjectV2Field\tfield ID\nStatus\tProjectV2SingleSelectField\tstatus ID\nIterations\tProjectV2IterationField\titeration ID\n",
		stdout.String())
}

func TestRunList_Org(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)
	// get org ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query OrgLogin.*",
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
		})

	// list project fields
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]interface{}{
			"query": "query OrgProject.*",
			"variables": map[string]interface{}{
				"login":       "github",
				"number":      1,
				"firstItems":  100,
				"afterItems":  nil,
				"firstFields": 100,
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

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	ios, _, stdout, _ := iostreams.Test()
	config := listConfig{
		tp: tableprinter.New(ios),
		opts: listOpts{
			number:   1,
			orgOwner: "github",
		},
		client: client,
	}

	err = runList(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Name\tDataType\tID\nFieldTitle\tProjectV2Field\tfield ID\nStatus\tProjectV2SingleSelectField\tstatus ID\nIterations\tProjectV2IterationField\titeration ID\n",
		stdout.String())
}

func TestRunList_Me(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// get viewer ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query ViewerLogin.*",
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
				"firstItems":  100,
				"afterItems":  nil,
				"firstFields": 100,
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

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	ios, _, stdout, _ := iostreams.Test()
	config := listConfig{
		tp: tableprinter.New(ios),
		opts: listOpts{
			number:    1,
			userOwner: "@me",
		},
		client: client,
	}

	err = runList(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Name\tDataType\tID\nFieldTitle\tProjectV2Field\tfield ID\nStatus\tProjectV2SingleSelectField\tstatus ID\nIterations\tProjectV2IterationField\titeration ID\n",
		stdout.String())
}

func TestRunList_Empty(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// get viewer ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query ViewerLogin.*",
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
				"firstItems":  100,
				"afterItems":  nil,
				"firstFields": 100,
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

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	ios, _, stdout, _ := iostreams.Test()
	config := listConfig{
		tp: tableprinter.New(ios),
		opts: listOpts{
			number:    1,
			userOwner: "@me",
		},
		client: client,
	}

	err = runList(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Project 1 for login @me has no fields\n",
		stdout.String())
}
