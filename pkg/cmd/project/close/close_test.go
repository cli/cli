package close

import (
	"bytes"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

func TestRunClose_User(t *testing.T) {
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

	// close project
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CloseProjectV2.*"variables":{"afterFields":null,"afterItems":null,"firstFields":0,"firstItems":0,"input":{"projectId":"an ID","closed":true}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"updateProjectV2": map[string]interface{}{
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
	config := closeConfig{
		tp: tableprinter.New(&buf, false, 0),
		opts: closeOpts{
			number:    1,
			userOwner: "monalisa",
		},
		client: client,
	}

	err = runClose(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Closed project http://a-url.com\n",
		buf.String())
}

func TestRunClose_Org(t *testing.T) {
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

	// close project
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CloseProjectV2.*"variables":{"afterFields":null,"afterItems":null,"firstFields":0,"firstItems":0,"input":{"projectId":"an ID","closed":true}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"updateProjectV2": map[string]interface{}{
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
	config := closeConfig{
		tp: tableprinter.New(&buf, false, 0),
		opts: closeOpts{
			number:   1,
			orgOwner: "github",
		},
		client: client,
	}

	err = runClose(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Closed project http://a-url.com\n",
		buf.String())
}

func TestRunClose_Me(t *testing.T) {
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

	// close project
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CloseProjectV2.*"variables":{"afterFields":null,"afterItems":null,"firstFields":0,"firstItems":0,"input":{"projectId":"an ID","closed":true}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"updateProjectV2": map[string]interface{}{
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
	config := closeConfig{
		tp: tableprinter.New(&buf, false, 0),
		opts: closeOpts{
			number:    1,
			userOwner: "@me",
		},
		client: client,
	}

	err = runClose(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Closed project http://a-url.com\n",
		buf.String())
}

func TestRunClose_Reopen(t *testing.T) {
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

	// close project
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CloseProjectV2.*"variables":{"afterFields":null,"afterItems":null,"firstFields":0,"firstItems":0,"input":{"projectId":"an ID","closed":false}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"updateProjectV2": map[string]interface{}{
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
	config := closeConfig{
		tp: tableprinter.New(&buf, false, 0),
		opts: closeOpts{
			number:    1,
			userOwner: "monalisa",
			reopen:    true,
		},
		client: client,
	}

	err = runClose(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Reopened project http://a-url.com\n",
		buf.String())
}
