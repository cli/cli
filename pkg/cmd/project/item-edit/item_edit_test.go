package itemedit

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

func TestNewCmdeditItem(t *testing.T) {
	tests := []struct {
		name          string
		cli           string
		wants         editItemOpts
		wantsErr      bool
		wantsErrMsg   string
		wantsExporter bool
	}{
		{
			name:        "missing-id",
			cli:         "",
			wantsErr:    true,
			wantsErrMsg: "required flag(s) \"id\" not set",
		},
		{
			name:        "invalid-flags",
			cli:         "--id 123 --text t --date 2023-01-01",
			wantsErr:    true,
			wantsErrMsg: "only one of `--text`, `--number`, `--date`, `--single-select-option-id` or `--iteration-id` may be used",
		},
		{
			name: "item-id",
			cli:  "--id 123",
			wants: editItemOpts{
				itemID: "123",
			},
		},
		{
			name: "number",
			cli:  "--number 456 --id 123",
			wants: editItemOpts{
				number: 456,
				itemID: "123",
			},
		},
		{
			name: "field-id",
			cli:  "--field-id FIELD_ID --id 123",
			wants: editItemOpts{
				fieldID: "FIELD_ID",
				itemID:  "123",
			},
		},
		{
			name: "project-id",
			cli:  "--project-id PROJECT_ID --id 123",
			wants: editItemOpts{
				projectID: "PROJECT_ID",
				itemID:    "123",
			},
		},
		{
			name: "text",
			cli:  "--text t --id 123",
			wants: editItemOpts{
				text:   "t",
				itemID: "123",
			},
		},
		{
			name: "date",
			cli:  "--date 2023-01-01 --id 123",
			wants: editItemOpts{
				date:   "2023-01-01",
				itemID: "123",
			},
		},
		{
			name: "single-select-option-id",
			cli:  "--single-select-option-id OPTION_ID --id 123",
			wants: editItemOpts{
				singleSelectOptionID: "OPTION_ID",
				itemID:               "123",
			},
		},
		{
			name: "iteration-id",
			cli:  "--iteration-id ITERATION_ID --id 123",
			wants: editItemOpts{
				iterationID: "ITERATION_ID",
				itemID:      "123",
			},
		},
		{
			name: "clear",
			cli:  "--id 123 --field-id FIELD_ID --project-id PROJECT_ID --clear",
			wants: editItemOpts{
				itemID:    "123",
				fieldID:   "FIELD_ID",
				projectID: "PROJECT_ID",
				clear:     true,
			},
		},
		{
			name: "json",
			cli:  "--format json --id 123",
			wants: editItemOpts{
				itemID: "123",
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

			var gotOpts editItemOpts
			cmd := NewCmdEditItem(f, func(config editItemConfig) error {
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
			assert.Equal(t, tt.wants.itemID, gotOpts.itemID)
			assert.Equal(t, tt.wantsExporter, gotOpts.exporter != nil)
			assert.Equal(t, tt.wants.title, gotOpts.title)
			assert.Equal(t, tt.wants.fieldID, gotOpts.fieldID)
			assert.Equal(t, tt.wants.projectID, gotOpts.projectID)
			assert.Equal(t, tt.wants.text, gotOpts.text)
			assert.Equal(t, tt.wants.number, gotOpts.number)
			assert.Equal(t, tt.wants.date, gotOpts.date)
			assert.Equal(t, tt.wants.singleSelectOptionID, gotOpts.singleSelectOptionID)
			assert.Equal(t, tt.wants.iterationID, gotOpts.iterationID)
			assert.Equal(t, tt.wants.clear, gotOpts.clear)
		})
	}
}

func TestRunItemEdit_Draft(t *testing.T) {
	defer gock.Off()
	// gock.Observe(gock.DumpRequest)

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

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)

	config := editItemConfig{
		io: ios,
		opts: editItemOpts{
			title:  "a title",
			body:   "a new body",
			itemID: "DI_item_id",
		},
		client: client,
	}

	err := runEditItem(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Edited draft issue \"a title\"\n",
		stdout.String())
}

func TestRunItemEdit_Text(t *testing.T) {
	defer gock.Off()
	// gock.Observe(gock.DumpRequest)

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

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	config := editItemConfig{
		io: ios,
		opts: editItemOpts{
			text:      "item text",
			itemID:    "item_id",
			projectID: "project_id",
			fieldID:   "field_id",
		},
		client: client,
	}

	err := runEditItem(config)
	assert.NoError(t, err)
	assert.Equal(t, "", stdout.String())
}

func TestRunItemEdit_Number(t *testing.T) {
	defer gock.Off()
	// gock.Observe(gock.DumpRequest)

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

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)

	config := editItemConfig{
		io: ios,
		opts: editItemOpts{
			number:    2,
			itemID:    "item_id",
			projectID: "project_id",
			fieldID:   "field_id",
		},
		client: client,
	}

	err := runEditItem(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Edited item \"title\"\n",
		stdout.String())
}

func TestRunItemEdit_Date(t *testing.T) {
	defer gock.Off()
	// gock.Observe(gock.DumpRequest)

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

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	config := editItemConfig{
		io: ios,
		opts: editItemOpts{
			date:      "2023-01-01",
			itemID:    "item_id",
			projectID: "project_id",
			fieldID:   "field_id",
		},
		client: client,
	}

	err := runEditItem(config)
	assert.NoError(t, err)
	assert.Equal(t, "", stdout.String())
}

func TestRunItemEdit_SingleSelect(t *testing.T) {
	defer gock.Off()
	// gock.Observe(gock.DumpRequest)

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

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	config := editItemConfig{
		io: ios,
		opts: editItemOpts{
			singleSelectOptionID: "option_id",
			itemID:               "item_id",
			projectID:            "project_id",
			fieldID:              "field_id",
		},
		client: client,
	}

	err := runEditItem(config)
	assert.NoError(t, err)
	assert.Equal(t, "", stdout.String())
}

func TestRunItemEdit_Iteration(t *testing.T) {
	defer gock.Off()
	// gock.Observe(gock.DumpRequest)

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

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)

	config := editItemConfig{
		io: ios,
		opts: editItemOpts{
			iterationID: "option_id",
			itemID:      "item_id",
			projectID:   "project_id",
			fieldID:     "field_id",
		},
		client: client,
	}

	err := runEditItem(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Edited item \"title\"\n",
		stdout.String())
}

func TestRunItemEdit_NoChanges(t *testing.T) {
	defer gock.Off()
	// gock.Observe(gock.DumpRequest)

	client := queries.NewTestClient()

	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(true)

	config := editItemConfig{
		io:     ios,
		opts:   editItemOpts{},
		client: client,
	}

	err := runEditItem(config)
	assert.Error(t, err, "SilentError")
	assert.Equal(t, "", stdout.String())
	assert.Equal(t, "error: no changes to make\n", stderr.String())
}

func TestRunItemEdit_InvalidID(t *testing.T) {
	defer gock.Off()
	// gock.Observe(gock.DumpRequest)

	client := queries.NewTestClient()
	config := editItemConfig{
		opts: editItemOpts{
			title:  "a title",
			body:   "a new body",
			itemID: "item_id",
		},
		client: client,
	}

	err := runEditItem(config)
	assert.Error(t, err, "ID must be the ID of the draft issue content which is prefixed with `DI_`")
}

func TestRunItemEdit_Clear(t *testing.T) {
	defer gock.Off()
	// gock.Observe(gock.DumpRequest)

	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation ClearItemFieldValue.*","variables":{"input":{"projectId":"project_id","itemId":"item_id","fieldId":"field_id"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"clearProjectV2ItemFieldValue": map[string]interface{}{
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

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)

	config := editItemConfig{
		io: ios,
		opts: editItemOpts{
			itemID:    "item_id",
			projectID: "project_id",
			fieldID:   "field_id",
			clear:     true,
		},
		client: client,
	}

	err := runEditItem(config)
	assert.NoError(t, err)
	assert.Equal(t, "Edited item \"title\"\n", stdout.String())
}

func TestRunItemEdit_JSON(t *testing.T) {
	defer gock.Off()
	// gock.Observe(gock.DumpRequest)

	// edit item
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation EditDraftIssueItem.*","variables":{"input":{"draftIssueId":"DI_item_id","title":"a title","body":"a new body"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"updateProjectV2DraftIssue": map[string]interface{}{
					"draftIssue": map[string]interface{}{
						"id":    "DI_item_id",
						"title": "a title",
						"body":  "a new body",
					},
				},
			},
		})

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	config := editItemConfig{
		io: ios,
		opts: editItemOpts{
			title:    "a title",
			body:     "a new body",
			itemID:   "DI_item_id",
			exporter: cmdutil.NewJSONExporter(),
		},
		client: client,
	}

	err := runEditItem(config)
	assert.NoError(t, err)
	assert.JSONEq(
		t,
		`{"id":"DI_item_id","title":"a title","body":"a new body","type":"DraftIssue"}`,
		stdout.String())
}
