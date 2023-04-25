package fieldcreate

import (
	"bytes"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

func TestRunCreateField_User(t *testing.T) {
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

	// create Field
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CreateField.*","variables":{"input":{"projectId":"an ID","dataType":"TEXT","name":"a name"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"createProjectV2Field": map[string]interface{}{
					"projectV2Field": map[string]interface{}{
						"id": "Field ID",
					},
				},
			},
		})

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	buf := bytes.Buffer{}
	config := createFieldConfig{
		tp: tableprinter.New(&buf, false, 0),
		opts: createFieldOpts{
			name:      "a name",
			userOwner: "monalisa",
			number:    1,
			dataType:  "TEXT",
		},
		client: client,
	}

	err = runCreateField(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Created field\n",
		buf.String())
}

func TestRunCreateField_Org(t *testing.T) {
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

	// create Field
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CreateField.*","variables":{"input":{"projectId":"an ID","dataType":"TEXT","name":"a name"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"createProjectV2Field": map[string]interface{}{
					"projectV2Field": map[string]interface{}{
						"id": "Field ID",
					},
				},
			},
		})

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	buf := bytes.Buffer{}
	config := createFieldConfig{
		tp: tableprinter.New(&buf, false, 0),
		opts: createFieldOpts{
			name:     "a name",
			orgOwner: "github",
			number:   1,
			dataType: "TEXT",
		},
		client: client,
	}

	err = runCreateField(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Created field\n",
		buf.String())
}

func TestRunCreateField_Me(t *testing.T) {
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

	// create Field
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CreateField.*","variables":{"input":{"projectId":"an ID","dataType":"TEXT","name":"a name"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"createProjectV2Field": map[string]interface{}{
					"projectV2Field": map[string]interface{}{
						"id": "Field ID",
					},
				},
			},
		})

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	buf := bytes.Buffer{}
	config := createFieldConfig{
		tp: tableprinter.New(&buf, false, 0),
		opts: createFieldOpts{
			userOwner: "@me",
			number:    1,
			name:      "a name",
			dataType:  "TEXT",
		},
		client: client,
	}

	err = runCreateField(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Created field\n",
		buf.String())
}

func TestRunCreateField_TEXT(t *testing.T) {
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

	// create Field
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CreateField.*","variables":{"input":{"projectId":"an ID","dataType":"TEXT","name":"a name"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"createProjectV2Field": map[string]interface{}{
					"projectV2Field": map[string]interface{}{
						"id": "Field ID",
					},
				},
			},
		})

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	buf := bytes.Buffer{}
	config := createFieldConfig{
		tp: tableprinter.New(&buf, false, 0),
		opts: createFieldOpts{
			userOwner: "@me",
			number:    1,
			name:      "a name",
			dataType:  "TEXT",
		},
		client: client,
	}

	err = runCreateField(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Created field\n",
		buf.String())
}

func TestRunCreateField_DATE(t *testing.T) {
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

	// create Field
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CreateField.*","variables":{"input":{"projectId":"an ID","dataType":"DATE","name":"a name"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"createProjectV2Field": map[string]interface{}{
					"projectV2Field": map[string]interface{}{
						"id": "Field ID",
					},
				},
			},
		})

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	buf := bytes.Buffer{}
	config := createFieldConfig{
		tp: tableprinter.New(&buf, false, 0),
		opts: createFieldOpts{
			userOwner: "@me",
			number:    1,
			name:      "a name",
			dataType:  "DATE",
		},
		client: client,
	}

	err = runCreateField(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Created field\n",
		buf.String())
}

func TestRunCreateField_NUMBER(t *testing.T) {
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

	// create Field
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation CreateField.*","variables":{"input":{"projectId":"an ID","dataType":"NUMBER","name":"a name"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"createProjectV2Field": map[string]interface{}{
					"projectV2Field": map[string]interface{}{
						"id": "Field ID",
					},
				},
			},
		})

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	buf := bytes.Buffer{}
	config := createFieldConfig{
		tp: tableprinter.New(&buf, false, 0),
		opts: createFieldOpts{
			userOwner: "@me",
			number:    1,
			name:      "a name",
			dataType:  "NUMBER",
		},
		client: client,
	}

	err = runCreateField(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Created field\n",
		buf.String())
}

func TestRunCreateField_SingleSelectNoOptions(t *testing.T) {
	buf := bytes.Buffer{}
	config := createFieldConfig{
		tp: tableprinter.New(&buf, false, 0),
		opts: createFieldOpts{
			name:      "a name",
			userOwner: "@me",
			dataType:  "SINGLE_SELECT",
		},
	}

	err := runCreateField(config)
	assert.EqualError(t, err, "at least one single select options is required with data type is SINGLE_SELECT")
}
