package fieldcreate

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

func TestNewCmdCreateField(t *testing.T) {
	tests := []struct {
		name          string
		cli           string
		wants         createFieldOpts
		wantsErr      bool
		wantsErrMsg   string
		wantsExporter bool
	}{
		{
			name:        "missing-name-and-data-type",
			cli:         "",
			wantsErr:    true,
			wantsErrMsg: "required flag(s) \"data-type\", \"name\" not set",
		},
		{
			name:        "not-a-number",
			cli:         "x  --name n --data-type TEXT",
			wantsErr:    true,
			wantsErrMsg: "invalid number: x",
		},
		{
			name:        "single-select-no-options",
			cli:         "123 --name n --data-type SINGLE_SELECT",
			wantsErr:    true,
			wantsErrMsg: "passing `--single-select-options` is required for SINGLE_SELECT data type",
		},
		{
			name: "number",
			cli:  "123 --name n --data-type TEXT",
			wants: createFieldOpts{
				number:              123,
				name:                "n",
				dataType:            "TEXT",
				singleSelectOptions: []string{},
			},
		},
		{
			name: "owner",
			cli:  "--owner monalisa --name n --data-type TEXT",
			wants: createFieldOpts{
				owner:               "monalisa",
				name:                "n",
				dataType:            "TEXT",
				singleSelectOptions: []string{},
			},
		},
		{
			name: "single-select-options",
			cli:  "--name n --data-type TEXT --single-select-options a,b",
			wants: createFieldOpts{
				singleSelectOptions: []string{"a", "b"},
				name:                "n",
				dataType:            "TEXT",
			},
		},
		{
			name: "json",
			cli:  "--format json --name n --data-type TEXT ",
			wants: createFieldOpts{
				name:                "n",
				dataType:            "TEXT",
				singleSelectOptions: []string{},
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

			var gotOpts createFieldOpts
			cmd := NewCmdCreateField(f, func(config createFieldConfig) error {
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
			assert.Equal(t, tt.wants.owner, gotOpts.owner)
			assert.Equal(t, tt.wants.name, gotOpts.name)
			assert.Equal(t, tt.wants.dataType, gotOpts.dataType)
			assert.Equal(t, tt.wants.singleSelectOptions, gotOpts.singleSelectOptions)
			assert.Equal(t, tt.wantsExporter, gotOpts.exporter != nil)
		})
	}
}

func TestRunCreateField_User(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// get user ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserOrgOwner.*",
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
			"errors": []interface{}{
				map[string]interface{}{
					"type": "NOT_FOUND",
					"path": []string{"organization"},
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

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	config := createFieldConfig{
		opts: createFieldOpts{
			name:     "a name",
			owner:    "monalisa",
			number:   1,
			dataType: "TEXT",
		},
		client: client,
		io:     ios,
	}

	err := runCreateField(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Created field\n",
		stdout.String())
}

func TestRunCreateField_Org(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// get org ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserOrgOwner.*",
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
			"errors": []interface{}{
				map[string]interface{}{
					"type": "NOT_FOUND",
					"path": []string{"user"},
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

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	config := createFieldConfig{
		opts: createFieldOpts{
			name:     "a name",
			owner:    "github",
			number:   1,
			dataType: "TEXT",
		},
		client: client,
		io:     ios,
	}

	err := runCreateField(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Created field\n",
		stdout.String())
}

func TestRunCreateField_Me(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)
	// get viewer ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query ViewerOwner.*",
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

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	config := createFieldConfig{
		opts: createFieldOpts{
			owner:    "@me",
			number:   1,
			name:     "a name",
			dataType: "TEXT",
		},
		client: client,
		io:     ios,
	}

	err := runCreateField(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Created field\n",
		stdout.String())
}

func TestRunCreateField_TEXT(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)
	// get viewer ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query ViewerOwner.*",
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

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	config := createFieldConfig{
		opts: createFieldOpts{
			owner:    "@me",
			number:   1,
			name:     "a name",
			dataType: "TEXT",
		},
		client: client,
		io:     ios,
	}

	err := runCreateField(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Created field\n",
		stdout.String())
}

func TestRunCreateField_DATE(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)
	// get viewer ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query ViewerOwner.*",
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

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	config := createFieldConfig{
		opts: createFieldOpts{
			owner:    "@me",
			number:   1,
			name:     "a name",
			dataType: "DATE",
		},
		client: client,
		io:     ios,
	}

	err := runCreateField(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Created field\n",
		stdout.String())
}

func TestRunCreateField_NUMBER(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)
	// get viewer ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query ViewerOwner.*",
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

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	config := createFieldConfig{
		opts: createFieldOpts{
			owner:    "@me",
			number:   1,
			name:     "a name",
			dataType: "NUMBER",
		},
		client: client,
		io:     ios,
	}

	err := runCreateField(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Created field\n",
		stdout.String())
}

func TestRunCreateField_JSON(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// get user ID
	gock.New("https://api.github.com").
		Post("/graphql").
		MatchType("json").
		JSON(map[string]interface{}{
			"query": "query UserOrgOwner.*",
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
			"errors": []interface{}{
				map[string]interface{}{
					"type": "NOT_FOUND",
					"path": []string{"organization"},
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
						"__typename": "ProjectV2Field",
						"id":         "Field ID",
						"name":       "a name",
					},
				},
			},
		})

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	config := createFieldConfig{
		opts: createFieldOpts{
			name:     "a name",
			owner:    "monalisa",
			number:   1,
			dataType: "TEXT",
			exporter: cmdutil.NewJSONExporter(),
		},
		client: client,
		io:     ios,
	}

	err := runCreateField(config)
	assert.NoError(t, err)
	assert.JSONEq(
		t,
		`{"id":"Field ID","name":"a name","type":"ProjectV2Field"}`,
		stdout.String())
}
