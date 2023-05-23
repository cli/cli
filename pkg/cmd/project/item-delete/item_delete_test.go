package itemdelete

import (
	"os"
	"testing"

	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

func TestNewCmdDeleteItem(t *testing.T) {
	tests := []struct {
		name        string
		cli         string
		wants       deleteItemOpts
		wantsErr    bool
		wantsErrMsg string
	}{
		{
			name:        "missing-id",
			cli:         "",
			wantsErr:    true,
			wantsErrMsg: "required flag(s) \"id\" not set",
		},

		{
			name:        "user-and-org",
			cli:         "--user monalisa --org github --id 123",
			wantsErr:    true,
			wantsErrMsg: "only one of `--user` or `--org` may be used",
		},
		{
			name:        "not-a-number",
			cli:         "x --id 123",
			wantsErr:    true,
			wantsErrMsg: "invalid number: x",
		},
		{
			name: "item-id",
			cli:  "--id 123",
			wants: deleteItemOpts{
				itemID: "123",
			},
		},
		{
			name: "number",
			cli:  "456 --id 123",
			wants: deleteItemOpts{
				number: 456,
				itemID: "123",
			},
		},
		{
			name: "user",
			cli:  "--user monalisa --id 123",
			wants: deleteItemOpts{
				userOwner: "monalisa",
				itemID:    "123",
			},
		},
		{
			name: "org",
			cli:  "--org github --id 123",
			wants: deleteItemOpts{
				orgOwner: "github",
				itemID:   "123",
			},
		},
		{
			name: "json",
			cli:  "--format json --id 123",
			wants: deleteItemOpts{
				format: "json",
				itemID: "123",
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

			var gotOpts deleteItemOpts
			cmd := NewCmdDeleteItem(f, func(config deleteItemConfig) error {
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
			assert.Equal(t, tt.wants.itemID, gotOpts.itemID)
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

	// delete item
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation DeleteProjectItem.*","variables":{"input":{"projectId":"an ID","itemId":"item ID"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"deleteProjectV2Item": map[string]interface{}{
					"deletedItemId": "item ID",
				},
			},
		})

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	config := deleteItemConfig{
		tp: tableprinter.New(ios),
		opts: deleteItemOpts{
			userOwner: "monalisa",
			number:    1,
			itemID:    "item ID",
		},
		client: client,
	}

	err := runDeleteItem(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Deleted item\n",
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

	// delete item
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation DeleteProjectItem.*","variables":{"input":{"projectId":"an ID","itemId":"item ID"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"deleteProjectV2Item": map[string]interface{}{
					"deletedItemId": "item ID",
				},
			},
		})

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	config := deleteItemConfig{
		tp: tableprinter.New(ios),
		opts: deleteItemOpts{
			orgOwner: "github",
			number:   1,
			itemID:   "item ID",
		},
		client: client,
	}

	err := runDeleteItem(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Deleted item\n",
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

	// delete item
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation DeleteProjectItem.*","variables":{"input":{"projectId":"an ID","itemId":"item ID"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"deleteProjectV2Item": map[string]interface{}{
					"deletedItemId": "item ID",
				},
			},
		})

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	config := deleteItemConfig{
		tp: tableprinter.New(ios),
		opts: deleteItemOpts{
			userOwner: "@me",
			number:    1,
			itemID:    "item ID",
		},
		client: client,
	}

	err := runDeleteItem(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Deleted item\n",
		stdout.String())
}
