package fielddelete

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

func TestNewCmdDeleteField(t *testing.T) {
	tests := []struct {
		name          string
		cli           string
		wants         deleteFieldOpts
		wantsErr      bool
		wantsErrMsg   string
		wantsExporter bool
	}{
		{
			name:        "no id",
			cli:         "",
			wantsErr:    true,
			wantsErrMsg: "required flag(s) \"id\" not set",
		},
		{
			name: "id",
			cli:  "--id 123",
			wants: deleteFieldOpts{
				fieldID: "123",
			},
		},
		{
			name: "json",
			cli:  "--id 123 --format json",
			wants: deleteFieldOpts{
				fieldID: "123",
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

			var gotOpts deleteFieldOpts
			cmd := NewCmdDeleteField(f, func(config deleteFieldConfig) error {
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

			assert.Equal(t, tt.wants.fieldID, gotOpts.fieldID)
			assert.Equal(t, tt.wantsExporter, gotOpts.exporter != nil)
		})
	}
}

func TestRunDeleteField(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// delete Field
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation DeleteField.*","variables":{"input":{"fieldId":"an ID"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"deleteProjectV2Field": map[string]interface{}{
					"projectV2Field": map[string]interface{}{
						"id": "Field ID",
					},
				},
			},
		})

	client := queries.NewTestClient()

	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	config := deleteFieldConfig{
		opts: deleteFieldOpts{
			fieldID: "an ID",
		},
		client: client,
		io:     ios,
	}

	err := runDeleteField(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Deleted field\n",
		stdout.String())
}

func TestRunDeleteField_JSON(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// delete Field
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation DeleteField.*","variables":{"input":{"fieldId":"an ID"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"deleteProjectV2Field": map[string]interface{}{
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
	config := deleteFieldConfig{
		opts: deleteFieldOpts{
			fieldID:  "an ID",
			exporter: cmdutil.NewJSONExporter(),
		},
		client: client,
		io:     ios,
	}

	err := runDeleteField(config)
	assert.NoError(t, err)
	assert.JSONEq(
		t,
		`{"id":"Field ID","name":"a name","type":"ProjectV2Field"}`,
		stdout.String())
}
