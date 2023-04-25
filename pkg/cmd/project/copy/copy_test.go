package copy

import (
	"bytes"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

func TestRunCopy_User(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// get user project ID
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
					"projectV2": map[string]string{
						"id": "an ID",
					},
				},
			},
		})

	// get source user ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserLogin.*",
			"variables": map[string]string{
				"login": "monalisa",
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"id":    "an ID",
					"login": "monalisa",
				},
			},
		})

	// get target user ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserLogin.*",
			"variables": map[string]string{
				"login": "monalisa",
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"id":    "an ID",
					"login": "monalisa",
				},
			},
		})

	// Copy project
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CopyProjectV2.*","variables":{"afterFields":null,"afterItems":null,"firstFields":0,"firstItems":0,"input":{"projectId":"an ID","ownerId":"an ID","title":"a title","includeDraftIssues":false}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"copyProjectV2": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"title": "a title",
						"url":   "http://a-url.com",
						"owner": map[string]string{
							"login": "monalisa",
						},
					},
				},
			},
		})

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	buf := bytes.Buffer{}
	config := copyConfig{
		tp: tableprinter.New(&buf, false, 0),
		opts: copyOpts{
			title:           "a title",
			sourceUserOwner: "monalisa",
			targetUserOwner: "monalisa",
			number:          1,
		},
		client: client,
	}

	err = runCopy(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Created project copy 'a title'\nhttp://a-url.com\n",
		buf.String())
}

func TestRunCopy_Org(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// get org project ID
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
					"projectV2": map[string]string{
						"id": "an ID",
					},
				},
			},
		})
	// get source org ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query OrgLogin.*",
			"variables": map[string]string{
				"login": "github",
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"organization": map[string]interface{}{
					"id":    "an ID",
					"login": "github",
				},
			},
		})

	// get target source org ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query OrgLogin.*",
			"variables": map[string]string{
				"login": "github",
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"organization": map[string]interface{}{
					"id":    "an ID",
					"login": "github",
				},
			},
		})

	// Copy project
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CopyProjectV2.*","variables":{"afterFields":null,"afterItems":null,"firstFields":0,"firstItems":0,"input":{"projectId":"an ID","ownerId":"an ID","title":"a title","includeDraftIssues":false}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"copyProjectV2": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"title": "a title",
						"url":   "http://a-url.com",
						"owner": map[string]string{
							"login": "monalisa",
						},
					},
				},
			},
		})

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	buf := bytes.Buffer{}
	config := copyConfig{
		tp: tableprinter.New(&buf, false, 0),
		opts: copyOpts{
			title:          "a title",
			sourceOrgOwner: "github",
			targetOrgOwner: "github",
			number:         1,
		},
		client: client,
	}

	err = runCopy(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Created project copy 'a title'\nhttp://a-url.com\n",
		buf.String())
}

func TestRunCopy_Me(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)
	// get viewer project ID
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
					"projectV2": map[string]string{
						"id": "an ID",
					},
				},
			},
		})

	// get source viewer ID
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
					"id":    "an ID",
					"login": "me",
				},
			},
		})

	// get target viewer ID
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
					"id":    "an ID",
					"login": "me",
				},
			},
		})

	// Copy project
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CopyProjectV2.*","variables":{"afterFields":null,"afterItems":null,"firstFields":0,"firstItems":0,"input":{"projectId":"an ID","ownerId":"an ID","title":"a title","includeDraftIssues":false}}}`).Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"copyProjectV2": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"title": "a title",
						"url":   "http://a-url.com",
						"owner": map[string]string{
							"login": "me",
						},
					},
				},
			},
		})

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	buf := bytes.Buffer{}
	config := copyConfig{
		tp: tableprinter.New(&buf, false, 0),
		opts: copyOpts{
			title:           "a title",
			sourceUserOwner: "@me",
			targetUserOwner: "@me",
			number:          1,
		},
		client: client,
	}

	err = runCopy(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Created project copy 'a title'\nhttp://a-url.com\n",
		buf.String())
}
