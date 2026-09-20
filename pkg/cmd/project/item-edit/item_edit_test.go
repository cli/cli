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
			name:        "no item selector flags",
			cli:         "",
			wantsErr:    true,
			wantsErrMsg: "specify the item to edit with `--id` or `--url`",
		},
		{
			name:        "invalid-flags",
			cli:         "--id 123 --text t --date 2023-01-01",
			wantsErr:    true,
			wantsErrMsg: "only one of `--text`, `--number`, `--date`, `--single-select-option-id`, `--iteration-id` or `--value` may be used",
		},
		{
			name:        "field and field-id conflict",
			cli:         "--url https://github.com/o/r/issues/1 --field Status --field-id FIELD_ID",
			wantsErr:    true,
			wantsErrMsg: "only one of `--field` or `--field-id` may be used",
		},
		{
			name:        "url and id conflict",
			cli:         "--id 123 --url https://github.com/o/r/issues/1",
			wantsErr:    true,
			wantsErrMsg: "only one of `--url` or `--id` may be used",
		},
		{
			name:        "value and single-select-option-id conflict",
			cli:         "--url https://github.com/o/r/issues/1 --field Status --value Todo --single-select-option-id OPTION_ID",
			wantsErr:    true,
			wantsErrMsg: "only one of `--text`, `--number`, `--date`, `--single-select-option-id`, `--iteration-id` or `--value` may be used",
		},
		{
			name:        "value requires field",
			cli:         "1 --url https://github.com/o/r/issues/1 --value Todo",
			wantsErr:    true,
			wantsErrMsg: "`--value` requires `--field`",
		},
		{
			name:        "value and field-id conflict",
			cli:         "1 --owner monalisa --url https://github.com/o/r/issues/1 --field-id FIELD_ID --value Todo",
			wantsErr:    true,
			wantsErrMsg: "`--value` cannot be used with `--field-id`; name the field with `--field` to use `--value`",
		},
		{
			name:        "field and id conflict",
			cli:         "1 --owner monalisa --id ITEM_ID --field Status",
			wantsErr:    true,
			wantsErrMsg: "`--field` cannot be used with `--id`; use `--url` to address the item when editing by name",
		},
		{
			name:        "name-based flags require project number",
			cli:         "--owner monalisa --url https://github.com/o/r/issues/1 --field Status --value Todo",
			wantsErr:    true,
			wantsErrMsg: "provide the project number as an argument when using `--url`, `--field`, or `--value`",
		},
		{
			name: "name-based flags",
			cli:  "1 --owner monalisa --url https://github.com/o/r/issues/1 --field Status --value Todo",
			wants: editItemOpts{
				number32:     1,
				owner:        "monalisa",
				url:          "https://github.com/o/r/issues/1",
				field:        "Status",
				value:        "Todo",
				valueChanged: true,
			},
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
			name: "number with floating point value",
			cli:  "--number 123.45 --id 123",
			wants: editItemOpts{
				number: 123.45,
				itemID: "123",
			},
		},
		{
			name: "number zero",
			cli:  "--number 0 --id 123",
			wants: editItemOpts{
				number: 0,
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
		{
			name: "draft issue body only",
			cli:  "--id 123 --body foobar",
			wants: editItemOpts{
				itemID:      "123",
				body:        "foobar",
				bodyChanged: true,
			},
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
			assert.Equal(t, tt.wants.titleChanged, gotOpts.titleChanged)
			assert.Equal(t, tt.wants.bodyChanged, gotOpts.bodyChanged)
			assert.Equal(t, tt.wants.body, gotOpts.body)
			assert.Equal(t, tt.wants.owner, gotOpts.owner)
			assert.Equal(t, tt.wants.number32, gotOpts.number32)
			assert.Equal(t, tt.wants.url, gotOpts.url)
			assert.Equal(t, tt.wants.field, gotOpts.field)
			assert.Equal(t, tt.wants.value, gotOpts.value)
			assert.Equal(t, tt.wants.valueChanged, gotOpts.valueChanged)
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
		JSON(map[string]any{
			"data": map[string]any{
				"updateProjectV2DraftIssue": map[string]any{
					"draftIssue": map[string]any{
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
			title:        "a title",
			titleChanged: true,
			body:         "a new body",
			bodyChanged:  true,
			itemID:       "DI_item_id",
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

func TestRunItemEdit_DraftTitleOnly(t *testing.T) {
	defer gock.Off()

	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"query DraftIssueByID.*","variables":{"id":"DI_item_id"}}`).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"node": map[string]any{
					"id":    "DI_item_id",
					"title": "existing title",
					"body":  "existing body",
				},
			},
		})

	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation EditDraftIssueItem.*","variables":{"input":{"draftIssueId":"DI_item_id","title":"new title","body":"existing body"}}}`).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"updateProjectV2DraftIssue": map[string]any{
					"draftIssue": map[string]any{
						"title": "new title",
						"body":  "existing body",
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
			title:        "new title",
			titleChanged: true,
			bodyChanged:  false,
			itemID:       "DI_item_id",
		},
		client: client,
	}

	err := runEditItem(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Edited draft issue \"new title\"\n",
		stdout.String())
}

func TestRunItemEdit_DraftBodyOnly(t *testing.T) {
	defer gock.Off()

	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"query DraftIssueByID.*","variables":{"id":"DI_item_id"}}`).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"node": map[string]any{
					"id":    "DI_item_id",
					"title": "existing title",
					"body":  "existing body",
				},
			},
		})

	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation EditDraftIssueItem.*","variables":{"input":{"draftIssueId":"DI_item_id","title":"existing title","body":"new body"}}}`).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"updateProjectV2DraftIssue": map[string]any{
					"draftIssue": map[string]any{
						"title": "existing title",
						"body":  "new body",
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
			titleChanged: false,
			body:         "new body",
			bodyChanged:  true,
			itemID:       "DI_item_id",
		},
		client: client,
	}

	err := runEditItem(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Edited draft issue \"existing title\"\n",
		stdout.String())
}

func TestRunItemEdit_DraftFetchError(t *testing.T) {
	defer gock.Off()

	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"query DraftIssueByID.*","variables":{"id":"DI_item_id"}}`).
		Reply(200).
		JSON(map[string]any{
			"errors": []map[string]any{
				{
					"type":    "NOT_FOUND",
					"message": "Could not resolve to a node with the global id of 'DI_item_id' (node)",
				},
			},
		})

	client := queries.NewTestClient()

	ios, _, _, _ := iostreams.Test()

	config := editItemConfig{
		io: ios,
		opts: editItemOpts{
			title:        "new title",
			titleChanged: true,
			bodyChanged:  false,
			itemID:       "DI_item_id",
		},
		client: client,
	}

	err := runEditItem(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Could not resolve to a node")
}

func TestRunItemEdit_Text(t *testing.T) {
	defer gock.Off()
	// gock.Observe(gock.DumpRequest)

	// edit item
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation UpdateItemValues.*","variables":{"input":{"projectId":"project_id","itemId":"item_id","fieldId":"field_id","value":{"text":"item text"}}}}`).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"updateProjectV2ItemFieldValue": map[string]any{
					"projectV2Item": map[string]any{
						"ID": "item_id",
						"content": map[string]any{
							"body":   "body",
							"title":  "title",
							"number": 1,
							"repository": map[string]any{
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
		BodyString(`{"query":"mutation UpdateItemValues.*","variables":{"input":{"projectId":"project_id","itemId":"item_id","fieldId":"field_id","value":{"number":123.45}}}}`).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"updateProjectV2ItemFieldValue": map[string]any{
					"projectV2Item": map[string]any{
						"ID": "item_id",
						"content": map[string]any{
							"__typename": "Issue",
							"body":       "body",
							"title":      "title",
							"number":     1,
							"repository": map[string]any{
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
			number:        123.45,
			numberChanged: true,
			itemID:        "item_id",
			projectID:     "project_id",
			fieldID:       "field_id",
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

func TestRunItemEdit_NumberZero(t *testing.T) {
	defer gock.Off()
	// gock.Observe(gock.DumpRequest)

	// edit item
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation UpdateItemValues.*","variables":{"input":{"projectId":"project_id","itemId":"item_id","fieldId":"field_id","value":{"number":0}}}}`).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"updateProjectV2ItemFieldValue": map[string]any{
					"projectV2Item": map[string]any{
						"ID": "item_id",
						"content": map[string]any{
							"__typename": "Issue",
							"body":       "body",
							"title":      "title",
							"number":     1,
							"repository": map[string]any{
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
			number:        0,
			numberChanged: true,
			itemID:        "item_id",
			projectID:     "project_id",
			fieldID:       "field_id",
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
		JSON(map[string]any{
			"data": map[string]any{
				"updateProjectV2ItemFieldValue": map[string]any{
					"projectV2Item": map[string]any{
						"ID": "item_id",
						"content": map[string]any{
							"__typename": "Issue",
							"body":       "body",
							"title":      "title",
							"number":     1,
							"repository": map[string]any{
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
		JSON(map[string]any{
			"data": map[string]any{
				"updateProjectV2ItemFieldValue": map[string]any{
					"projectV2Item": map[string]any{
						"ID": "item_id",
						"content": map[string]any{
							"__typename": "Issue",
							"body":       "body",
							"title":      "title",
							"number":     1,
							"repository": map[string]any{
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
		JSON(map[string]any{
			"data": map[string]any{
				"updateProjectV2ItemFieldValue": map[string]any{
					"projectV2Item": map[string]any{
						"ID": "item_id",
						"content": map[string]any{
							"__typename": "Issue",
							"body":       "body",
							"title":      "title",
							"number":     1,
							"repository": map[string]any{
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
			title:        "a title",
			titleChanged: true,
			body:         "a new body",
			bodyChanged:  true,
			itemID:       "item_id",
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
		JSON(map[string]any{
			"data": map[string]any{
				"clearProjectV2ItemFieldValue": map[string]any{
					"projectV2Item": map[string]any{
						"ID": "item_id",
						"content": map[string]any{
							"__typename": "Issue",
							"body":       "body",
							"title":      "title",
							"number":     1,
							"repository": map[string]any{
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
		JSON(map[string]any{
			"data": map[string]any{
				"updateProjectV2DraftIssue": map[string]any{
					"draftIssue": map[string]any{
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
			title:        "a title",
			titleChanged: true,
			body:         "a new body",
			bodyChanged:  true,
			itemID:       "DI_item_id",
			exporter:     cmdutil.NewJSONExporter(),
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

func TestRunItemEdit_ByName_SingleSelect(t *testing.T) {
	defer gock.Off()

	// resolve owner
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]any{
			"query": "query UserOrgOwner.*",
			"variables": map[string]any{
				"login": "monalisa",
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"id": "user ID",
				},
			},
			"errors": []any{
				map[string]any{
					"type": "NOT_FOUND",
					"path": []string{"organization"},
				},
			},
		})

	// resolve project + fields
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]any{
			"query": "query UserProject.*",
			"variables": map[string]any{
				"login":       "monalisa",
				"number":      1,
				"firstItems":  queries.LimitMax,
				"afterItems":  nil,
				"firstFields": queries.LimitMax,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"projectV2": map[string]any{
						"id": "project ID",
						"fields": map[string]any{
							"nodes": []map[string]any{
								{
									"__typename": "ProjectV2SingleSelectField",
									"id":         "status ID",
									"name":       "Status",
									"dataType":   "SINGLE_SELECT",
									"options": []map[string]any{
										{"id": "opt_todo", "name": "Todo"},
										{"id": "opt_done", "name": "Done"},
									},
								},
							},
						},
					},
				},
			},
		})

	// resolve item by URL
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]any{
			"query": "query GetProjectItemByURL.*",
			"variables": map[string]any{
				"url":        "https://github.com/monalisa/repo/issues/1",
				"firstItems": queries.LimitMax,
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"resource": map[string]any{
					"__typename": "Issue",
					"projectItems": map[string]any{
						"nodes": []map[string]any{
							{"id": "item ID", "project": map[string]any{"id": "project ID"}},
						},
					},
				},
			},
		})

	// mutation uses the resolved option ID
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation UpdateItemValues.*","variables":{"input":{"projectId":"project ID","itemId":"item ID","fieldId":"status ID","value":{"singleSelectOptionId":"opt_done"}}}}`).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"updateProjectV2ItemFieldValue": map[string]any{
					"projectV2Item": map[string]any{
						"id": "item ID",
						"content": map[string]any{
							"__typename": "Issue",
							"title":      "an issue",
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
			owner:        "monalisa",
			number32:     1,
			url:          "https://github.com/monalisa/repo/issues/1",
			field:        "Status",
			value:        "Done",
			valueChanged: true,
		},
		client: client,
	}

	err := runEditItem(config)
	assert.NoError(t, err)
	assert.Equal(t, "Edited item \"an issue\"\n", stdout.String())
}

// TestRunItemEdit_ByName_ValueDispatch covers how --value is dispatched by the
// resolved field's data type for the non single-select field types. The error
// cases exercise the guard branches that reject a value before any write: since
// those errors are only produced after the owner/project/item lookups succeed,
// the exact error assertion proves the field-value mutation never fired.
func TestRunItemEdit_ByName_ValueDispatch(t *testing.T) {
	tests := []struct {
		name         string
		fieldNode    map[string]any
		value        string
		mutationBody string // expected mutation body; empty when no write should happen
		wantErr      string // expected error; empty for happy paths
	}{
		{
			name: "text field",
			fieldNode: map[string]any{
				"__typename": "ProjectV2Field",
				"id":         "text ID",
				"name":       "Text",
				"dataType":   "TEXT",
			},
			value:        "hello",
			mutationBody: `{"query":"mutation UpdateItemValues.*","variables":{"input":{"projectId":"project ID","itemId":"item ID","fieldId":"text ID","value":{"text":"hello"}}}}`,
		},
		{
			name: "number field",
			fieldNode: map[string]any{
				"__typename": "ProjectV2Field",
				"id":         "number ID",
				"name":       "Estimate",
				"dataType":   "NUMBER",
			},
			value:        "123.45",
			mutationBody: `{"query":"mutation UpdateItemValues.*","variables":{"input":{"projectId":"project ID","itemId":"item ID","fieldId":"number ID","value":{"number":123.45}}}}`,
		},
		{
			name: "date field",
			fieldNode: map[string]any{
				"__typename": "ProjectV2Field",
				"id":         "date ID",
				"name":       "Due",
				"dataType":   "DATE",
			},
			value:        "2023-01-01",
			mutationBody: `{"query":"mutation UpdateItemValues.*","variables":{"input":{"projectId":"project ID","itemId":"item ID","fieldId":"date ID","value":{"date":"2023-01-01T00:00:00Z"}}}}`,
		},
		{
			name: "invalid number value",
			fieldNode: map[string]any{
				"__typename": "ProjectV2Field",
				"id":         "number ID",
				"name":       "Estimate",
				"dataType":   "NUMBER",
			},
			value:   "not-a-number",
			wantErr: `invalid number value "not-a-number" for field "Estimate"`,
		},
		{
			name: "iteration field rejected",
			fieldNode: map[string]any{
				"__typename": "ProjectV2IterationField",
				"id":         "iteration ID",
				"name":       "Sprint",
				"dataType":   "ITERATION",
			},
			value:   "Sprint 1",
			wantErr: "setting an iteration field by name is not supported; use `--iteration-id`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer gock.Off()

			// resolve owner
			gock.New("https://api.github.com").
				Post("/graphql").
				JSON(map[string]any{
					"query": "query UserOrgOwner.*",
					"variables": map[string]any{
						"login": "monalisa",
					},
				}).
				Reply(200).
				JSON(map[string]any{
					"data": map[string]any{
						"user": map[string]any{
							"id": "user ID",
						},
					},
					"errors": []any{
						map[string]any{
							"type": "NOT_FOUND",
							"path": []string{"organization"},
						},
					},
				})

			// resolve project + fields
			gock.New("https://api.github.com").
				Post("/graphql").
				JSON(map[string]any{
					"query": "query UserProject.*",
					"variables": map[string]any{
						"login":       "monalisa",
						"number":      1,
						"firstItems":  queries.LimitMax,
						"afterItems":  nil,
						"firstFields": queries.LimitMax,
						"afterFields": nil,
					},
				}).
				Reply(200).
				JSON(map[string]any{
					"data": map[string]any{
						"user": map[string]any{
							"projectV2": map[string]any{
								"id": "project ID",
								"fields": map[string]any{
									"nodes": []map[string]any{tt.fieldNode},
								},
							},
						},
					},
				})

			// resolve item by URL
			gock.New("https://api.github.com").
				Post("/graphql").
				JSON(map[string]any{
					"query": "query GetProjectItemByURL.*",
					"variables": map[string]any{
						"url":        "https://github.com/monalisa/repo/issues/1",
						"firstItems": queries.LimitMax,
					},
				}).
				Reply(200).
				JSON(map[string]any{
					"data": map[string]any{
						"resource": map[string]any{
							"__typename": "Issue",
							"projectItems": map[string]any{
								"nodes": []map[string]any{
									{"id": "item ID", "project": map[string]any{"id": "project ID"}},
								},
							},
						},
					},
				})

			if tt.mutationBody != "" {
				gock.New("https://api.github.com").
					Post("/graphql").
					BodyString(tt.mutationBody).
					Reply(200).
					JSON(map[string]any{
						"data": map[string]any{
							"updateProjectV2ItemFieldValue": map[string]any{
								"projectV2Item": map[string]any{
									"id": "item ID",
									"content": map[string]any{
										"__typename": "Issue",
										"title":      "an issue",
									},
								},
							},
						},
					})
			}

			client := queries.NewTestClient()

			ios, _, stdout, _ := iostreams.Test()
			ios.SetStdoutTTY(true)

			config := editItemConfig{
				io: ios,
				opts: editItemOpts{
					owner:        "monalisa",
					number32:     1,
					url:          "https://github.com/monalisa/repo/issues/1",
					field:        tt.fieldNode["name"].(string),
					value:        tt.value,
					valueChanged: true,
				},
				client: client,
			}

			err := runEditItem(config)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				assert.True(t, gock.IsDone(), "the item should be resolved and no write attempted")
				return
			}
			assert.NoError(t, err)
			assert.True(t, gock.IsDone())
			assert.Equal(t, "Edited item \"an issue\"\n", stdout.String())
		})
	}
}

func TestRunItemEdit_ByName_CaseInsensitive(t *testing.T) {
	defer gock.Off()

	// resolve owner
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]any{
			"query": "query UserOrgOwner.*",
			"variables": map[string]any{
				"login": "monalisa",
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"id": "user ID",
				},
			},
			"errors": []any{
				map[string]any{
					"type": "NOT_FOUND",
					"path": []string{"organization"},
				},
			},
		})

	// resolve project + fields
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]any{
			"query": "query UserProject.*",
			"variables": map[string]any{
				"login":       "monalisa",
				"number":      1,
				"firstItems":  queries.LimitMax,
				"afterItems":  nil,
				"firstFields": queries.LimitMax,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"projectV2": map[string]any{
						"id": "project ID",
						"fields": map[string]any{
							"nodes": []map[string]any{
								{
									"__typename": "ProjectV2SingleSelectField",
									"id":         "status ID",
									"name":       "Status",
									"dataType":   "SINGLE_SELECT",
									"options": []map[string]any{
										{"id": "opt_todo", "name": "Todo"},
										{"id": "opt_inprog", "name": "In Progress"},
									},
								},
							},
						},
					},
				},
			},
		})

	// resolve item by URL
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]any{
			"query": "query GetProjectItemByURL.*",
			"variables": map[string]any{
				"url":        "https://github.com/monalisa/repo/issues/1",
				"firstItems": queries.LimitMax,
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"resource": map[string]any{
					"__typename": "Issue",
					"projectItems": map[string]any{
						"nodes": []map[string]any{
							{"id": "item ID", "project": map[string]any{"id": "project ID"}},
						},
					},
				},
			},
		})

	// mutation resolves the lowercase field/value inputs to their canonical IDs
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation UpdateItemValues.*","variables":{"input":{"projectId":"project ID","itemId":"item ID","fieldId":"status ID","value":{"singleSelectOptionId":"opt_inprog"}}}}`).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"updateProjectV2ItemFieldValue": map[string]any{
					"projectV2Item": map[string]any{
						"id": "item ID",
						"content": map[string]any{
							"__typename": "Issue",
							"title":      "an issue",
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
			owner:        "monalisa",
			number32:     1,
			url:          "https://github.com/monalisa/repo/issues/1",
			field:        "status",
			value:        "in progress",
			valueChanged: true,
		},
		client: client,
	}

	err := runEditItem(config)
	assert.NoError(t, err)
	assert.Equal(t, "Edited item \"an issue\"\n", stdout.String())
	assert.True(t, gock.IsDone())
}

func TestRunItemEdit_ByName_FieldNotFound(t *testing.T) {
	defer gock.Off()

	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]any{
			"query": "query UserOrgOwner.*",
			"variables": map[string]any{
				"login": "monalisa",
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"id": "user ID",
				},
			},
			"errors": []any{
				map[string]any{
					"type": "NOT_FOUND",
					"path": []string{"organization"},
				},
			},
		})

	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]any{
			"query": "query UserProject.*",
			"variables": map[string]any{
				"login":       "monalisa",
				"number":      1,
				"firstItems":  queries.LimitMax,
				"afterItems":  nil,
				"firstFields": queries.LimitMax,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"projectV2": map[string]any{
						"id": "project ID",
						"fields": map[string]any{
							"nodes": []map[string]any{
								{
									"__typename": "ProjectV2Field",
									"id":         "title ID",
									"name":       "Title",
									"dataType":   "TITLE",
								},
							},
						},
					},
				},
			},
		})

	client := queries.NewTestClient()

	ios, _, _, _ := iostreams.Test()

	config := editItemConfig{
		io: ios,
		opts: editItemOpts{
			owner:        "monalisa",
			number32:     1,
			url:          "https://github.com/monalisa/repo/issues/1",
			field:        "Status",
			value:        "Done",
			valueChanged: true,
		},
		client: client,
	}

	// The field resolves against the project fields, so no item lookup or
	// mutation should fire; the error must list candidate field names.
	err := runEditItem(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), `field "Status" not found`)
	assert.Contains(t, err.Error(), "available fields: Title")
	assert.True(t, gock.IsDone())
}

func TestRunItemEdit_ByName_WrongFieldType(t *testing.T) {
	defer gock.Off()

	// resolve owner
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]any{
			"query": "query UserOrgOwner.*",
			"variables": map[string]any{
				"login": "monalisa",
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"id": "user ID",
				},
			},
			"errors": []any{
				map[string]any{
					"type": "NOT_FOUND",
					"path": []string{"organization"},
				},
			},
		})

	// resolve project + fields: the project has a built-in Title field, which
	// updateProjectV2ItemFieldValue does not support with --value.
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]any{
			"query": "query UserProject.*",
			"variables": map[string]any{
				"login":       "monalisa",
				"number":      1,
				"firstItems":  queries.LimitMax,
				"afterItems":  nil,
				"firstFields": queries.LimitMax,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"projectV2": map[string]any{
						"id": "project ID",
						"fields": map[string]any{
							"nodes": []map[string]any{
								{
									"__typename": "ProjectV2Field",
									"id":         "title ID",
									"name":       "Title",
									"dataType":   "TITLE",
								},
							},
						},
					},
				},
			},
		})

	// resolve item by URL: this fires before the value dispatch, but no
	// mutation should follow.
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]any{
			"query": "query GetProjectItemByURL.*",
			"variables": map[string]any{
				"url":        "https://github.com/monalisa/repo/issues/1",
				"firstItems": queries.LimitMax,
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"resource": map[string]any{
					"__typename": "Issue",
					"projectItems": map[string]any{
						"nodes": []map[string]any{
							{"id": "item ID", "project": map[string]any{"id": "project ID"}},
						},
					},
				},
			},
		})

	client := queries.NewTestClient()

	ios, _, _, _ := iostreams.Test()

	config := editItemConfig{
		io: ios,
		opts: editItemOpts{
			owner:        "monalisa",
			number32:     1,
			url:          "https://github.com/monalisa/repo/issues/1",
			field:        "Title",
			value:        "a new title",
			valueChanged: true,
		},
		client: client,
	}

	// The Title field resolves but its data type is not writable with --value,
	// so a clean error is returned and no mutation fires.
	err := runEditItem(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), `field "Title" has data type "TITLE"`)
	assert.Contains(t, err.Error(), "not supported with `--value`")
	assert.True(t, gock.IsDone())
}
