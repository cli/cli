package delete

import (
	"bytes"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

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

	buf := bytes.Buffer{}
	config := deleteConfig{
		tp: tableprinter.New(&buf, false, 0),
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
		buf.String())
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

	buf := bytes.Buffer{}
	config := deleteConfig{
		tp: tableprinter.New(&buf, false, 0),
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
		buf.String())
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

	buf := bytes.Buffer{}
	config := deleteConfig{
		tp: tableprinter.New(&buf, false, 0),
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
		buf.String())
}
