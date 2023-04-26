package delete

import (
	"testing"

	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

func TestNewCmdDelete(t *testing.T) {
	tests := []struct {
		name        string
		cli         string
		wants       deleteOpts
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
			wants: deleteOpts{
				number: 123,
			},
		},
		{
			name: "user",
			cli:  "--user monalisa",
			wants: deleteOpts{
				userOwner: "monalisa",
			},
		},
		{
			name: "org",
			cli:  "--org github",
			wants: deleteOpts{
				orgOwner: "github",
			},
		},
		{
			name: "json",
			cli:  "--format json",
			wants: deleteOpts{
				format: "json",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts deleteOpts
			cmd := NewCmdDelete(f, func(config deleteConfig) error {
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
			assert.Equal(t, tt.wants.userOwner, gotOpts.userOwner)
			assert.Equal(t, tt.wants.orgOwner, gotOpts.orgOwner)
			assert.Equal(t, tt.wants.format, gotOpts.format)
		})
	}
}

func TestRunDelete_User(t *testing.T) {
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

	// get project ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserProject.*",
			"variables": map[string]interface{}{
				"login":       "monalisa",
				"number":      1,
				"firstItems":  0,
				"afterItems":  nil,
				"firstFields": 0,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"id": "an ID",
					},
				},
			},
		})

	// delete project
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation DeleteProject.*","variables":{"afterFields":null,"afterItems":null,"firstFields":0,"firstItems":0,"input":{"projectId":"an ID"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"deleteProjectV2": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"id": "project ID",
					},
				},
			},
		})

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	ios, _, stdout, _ := iostreams.Test()
	config := deleteConfig{
		tp: tableprinter.New(ios),
		opts: deleteOpts{
			userOwner: "monalisa",
			number:    1,
		},
		client: client,
	}

	err = runDelete(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Deleted project\n",
		stdout.String())
}

func TestRunDelete_Org(t *testing.T) {
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

	// get project ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query OrgProject.*",
			"variables": map[string]interface{}{
				"login":       "github",
				"number":      1,
				"firstItems":  0,
				"afterItems":  nil,
				"firstFields": 0,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"organization": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"id": "an ID",
					},
				},
			},
		})

	// delete project
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation DeleteProject.*","variables":{"afterFields":null,"afterItems":null,"firstFields":0,"firstItems":0,"input":{"projectId":"an ID"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"deleteProjectV2": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"id": "project ID",
					},
				},
			},
		})

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	ios, _, stdout, _ := iostreams.Test()
	config := deleteConfig{
		tp: tableprinter.New(ios),
		opts: deleteOpts{
			orgOwner: "github",
			number:   1,
		},
		client: client,
	}

	err = runDelete(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Deleted project\n",
		stdout.String())
}

func TestRunDelete_Me(t *testing.T) {
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

	// get project ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query ViewerProject.*",
			"variables": map[string]interface{}{
				"number":      1,
				"firstItems":  0,
				"afterItems":  nil,
				"firstFields": 0,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"viewer": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"id": "an ID",
					},
				},
			},
		})

	// delete project
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation DeleteProject.*","variables":{"afterFields":null,"afterItems":null,"firstFields":0,"firstItems":0,"input":{"projectId":"an ID"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"deleteProjectV2": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"id": "project ID",
					},
				},
			},
		})

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	ios, _, stdout, _ := iostreams.Test()
	config := deleteConfig{
		tp: tableprinter.New(ios),
		opts: deleteOpts{
			userOwner: "@me",
			number:    1,
		},
		client: client,
	}

	err = runDelete(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Deleted project\n",
		stdout.String())
}
