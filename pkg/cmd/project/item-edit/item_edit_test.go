package itemedit

import (
	"bytes"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

func TestRunItemEdit_Draft(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// edit item
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation EditDraftIssueItem.*","variables":{"input":{"draftIssueId":"DI_item_id","title":"a title","body":"a new body"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"updateProjectV2DraftIssue": map[string]interface{}{
					"draftIssue": map[string]interface{}{
						"title": "a title",
						"body":  "a new body",
					},
				},
			},
		})

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	buf := bytes.Buffer{}
	config := editItemConfig{
		tp: tableprinter.New(&buf, false, 0),
		opts: editItemOpts{
			title:  "a title",
			body:   "a new body",
			itemID: "DI_item_id",
		},
		client: client,
	}

	err = runEditItem(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Title\tBody\na title\ta new body\n",
		buf.String())
}

func TestRunItemEdit_Text(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// edit item
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation UpdateItemValues.*","variables":{"input":{"projectId":"project_id","itemId":"item_id","fieldId":"field_id","value":{"text":"item text"}}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"updateProjectV2ItemFieldValue": map[string]interface{}{
					"projectV2Item": map[string]interface{}{
						"ID": "item_id",
						"content": map[string]interface{}{
							"__typename": "Issue",
							"body":       "body",
							"title":      "title",
							"number":     1,
							"repository": map[string]interface{}{
								"nameWithOwner": "my-repo",
							},
						},
					},
				},
			},
		})

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	buf := bytes.Buffer{}
	config := editItemConfig{
		tp: tableprinter.New(&buf, false, 0),
		opts: editItemOpts{
			text:      "item text",
			itemID:    "item_id",
			projectID: "project_id",
			fieldID:   "field_id",
		},
		client: client,
	}

	err = runEditItem(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Type\tTitle\tNumber\tRepository\tID\nIssue\ttitle\t1\tmy-repo\titem_id\n",
		buf.String())
}

func TestRunItemEdit_Number(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// edit item
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation UpdateItemValues.*","variables":{"input":{"projectId":"project_id","itemId":"item_id","fieldId":"field_id","value":{"number":2}}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"updateProjectV2ItemFieldValue": map[string]interface{}{
					"projectV2Item": map[string]interface{}{
						"ID": "item_id",
						"content": map[string]interface{}{
							"__typename": "Issue",
							"body":       "body",
							"title":      "title",
							"number":     1,
							"repository": map[string]interface{}{
								"nameWithOwner": "my-repo",
							},
						},
					},
				},
			},
		})

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	buf := bytes.Buffer{}
	config := editItemConfig{
		tp: tableprinter.New(&buf, false, 0),
		opts: editItemOpts{
			number:    2,
			itemID:    "item_id",
			projectID: "project_id",
			fieldID:   "field_id",
		},
		client: client,
	}

	err = runEditItem(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Type\tTitle\tNumber\tRepository\tID\nIssue\ttitle\t1\tmy-repo\titem_id\n",
		buf.String())
}

func TestRunItemEdit_Date(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// edit item
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation UpdateItemValues.*","variables":{"input":{"projectId":"project_id","itemId":"item_id","fieldId":"field_id","value":{"date":"2023-01-01T00:00:00Z"}}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"updateProjectV2ItemFieldValue": map[string]interface{}{
					"projectV2Item": map[string]interface{}{
						"ID": "item_id",
						"content": map[string]interface{}{
							"__typename": "Issue",
							"body":       "body",
							"title":      "title",
							"number":     1,
							"repository": map[string]interface{}{
								"nameWithOwner": "my-repo",
							},
						},
					},
				},
			},
		})

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	buf := bytes.Buffer{}
	config := editItemConfig{
		tp: tableprinter.New(&buf, false, 0),
		opts: editItemOpts{
			date:      "2023-01-01",
			itemID:    "item_id",
			projectID: "project_id",
			fieldID:   "field_id",
		},
		client: client,
	}

	err = runEditItem(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Type\tTitle\tNumber\tRepository\tID\nIssue\ttitle\t1\tmy-repo\titem_id\n",
		buf.String())
}

func TestRunItemEdit_SingleSelect(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// edit item
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation UpdateItemValues.*","variables":{"input":{"projectId":"project_id","itemId":"item_id","fieldId":"field_id","value":{"singleSelectOptionId":"option_id"}}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"updateProjectV2ItemFieldValue": map[string]interface{}{
					"projectV2Item": map[string]interface{}{
						"ID": "item_id",
						"content": map[string]interface{}{
							"__typename": "Issue",
							"body":       "body",
							"title":      "title",
							"number":     1,
							"repository": map[string]interface{}{
								"nameWithOwner": "my-repo",
							},
						},
					},
				},
			},
		})

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	buf := bytes.Buffer{}
	config := editItemConfig{
		tp: tableprinter.New(&buf, false, 0),
		opts: editItemOpts{
			singleSelectOptionID: "option_id",
			itemID:               "item_id",
			projectID:            "project_id",
			fieldID:              "field_id",
		},
		client: client,
	}

	err = runEditItem(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Type\tTitle\tNumber\tRepository\tID\nIssue\ttitle\t1\tmy-repo\titem_id\n",
		buf.String())
}

func TestRunItemEdit_Iteration(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// edit item
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation UpdateItemValues.*","variables":{"input":{"projectId":"project_id","itemId":"item_id","fieldId":"field_id","value":{"iterationId":"option_id"}}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"updateProjectV2ItemFieldValue": map[string]interface{}{
					"projectV2Item": map[string]interface{}{
						"ID": "item_id",
						"content": map[string]interface{}{
							"__typename": "Issue",
							"body":       "body",
							"title":      "title",
							"number":     1,
							"repository": map[string]interface{}{
								"nameWithOwner": "my-repo",
							},
						},
					},
				},
			},
		})

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	buf := bytes.Buffer{}
	config := editItemConfig{
		tp: tableprinter.New(&buf, false, 0),
		opts: editItemOpts{
			iterationID: "option_id",
			itemID:      "item_id",
			projectID:   "project_id",
			fieldID:     "field_id",
		},
		client: client,
	}

	err = runEditItem(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Type\tTitle\tNumber\tRepository\tID\nIssue\ttitle\t1\tmy-repo\titem_id\n",
		buf.String())
}

func TestRunItemEdit_NoChanges(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	buf := bytes.Buffer{}
	config := editItemConfig{
		tp:     tableprinter.New(&buf, false, 0),
		opts:   editItemOpts{},
		client: client,
	}

	err = runEditItem(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"No changes to make",
		buf.String())
}

func TestRunItemEdit_InvalidID(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	buf := bytes.Buffer{}
	config := editItemConfig{
		tp: tableprinter.New(&buf, false, 0),
		opts: editItemOpts{
			title:  "a title",
			body:   "a new body",
			itemID: "item_id",
		},
		client: client,
	}

	err = runEditItem(config)
	assert.Error(t, err, "ID must be the ID of the draft issue content which is prefixed with `DI_`")
}
