package editfield

import (
	"io"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdEditField(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		output  EditFieldOptions
		wantErr bool
		errMsg  string
	}{
		{
			name:  "text value",
			input: "23 --field-id FIELD_ID --text hello",
			output: EditFieldOptions{
				IssueNumber: 23,
				FieldID:     "FIELD_ID",
				Text:        "hello",
			},
		},
		{
			name:  "clear",
			input: "23 --field-id FIELD_ID --clear",
			output: EditFieldOptions{
				IssueNumber: 23,
				FieldID:     "FIELD_ID",
				Clear:       true,
			},
		},
		{
			name:    "field-id required",
			input:   "23 --text hello",
			wantErr: true,
			errMsg:  "`--field-id` (the ID of the issue field) is required",
		},
		{
			name:    "no value",
			input:   "23 --field-id FIELD_ID",
			wantErr: true,
			errMsg:  "provide exactly one of `--text`, `--number`, `--date`, `--single-select-option-id`, `--multi-select-option-ids`, or `--clear`",
		},
		{
			name:    "too many values",
			input:   "23 --field-id FIELD_ID --text a --clear",
			wantErr: true,
			errMsg:  "provide exactly one of `--text`, `--number`, `--date`, `--single-select-option-id`, `--multi-select-option-ids`, or `--clear`",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}
			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)

			var gotOpts *EditFieldOptions
			cmd := NewCmdEditField(f, func(opts *EditFieldOptions) error {
				gotOpts = opts
				return nil
			})

			cmd.SetArgs(argv)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			_, err = cmd.ExecuteC()
			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.output.IssueNumber, gotOpts.IssueNumber)
			assert.Equal(t, tt.output.FieldID, gotOpts.FieldID)
			assert.Equal(t, tt.output.Text, gotOpts.Text)
			assert.Equal(t, tt.output.Clear, gotOpts.Clear)
		})
	}
}

func TestEditFieldRun(t *testing.T) {
	issueResponse := `{ "data": { "repository": {
		"hasIssuesEnabled": true,
		"issue": { "id": "THE-ID", "number": 23, "title": "An issue" }
	} } }`

	tests := []struct {
		name       string
		opts       *EditFieldOptions
		httpStubs  func(*httpmock.Registry)
		wantStderr string
		wantErr    bool
		errMsg     string
	}{
		{
			name: "set text value",
			opts: &EditFieldOptions{
				IssueNumber: 23,
				FieldID:     "FIELD_ID",
				Text:        "Platform",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.GraphQL(`query IssueByNumber\b`), httpmock.StringResponse(issueResponse))
				reg.Register(
					httpmock.GraphQL(`mutation UpdateIssueFieldValue\b`),
					httpmock.GraphQLMutation(`{ "data": { "updateIssueFieldValue": { "clientMutationId": "" } } }`,
						func(inputs map[string]interface{}) {
							assert.Equal(t, "THE-ID", inputs["issueId"])
							issueField := inputs["issueField"].(map[string]interface{})
							assert.Equal(t, "FIELD_ID", issueField["fieldId"])
							assert.Equal(t, "Platform", issueField["textValue"])
						}),
				)
			},
			wantStderr: "✓ Set issue field value on OWNER/REPO#23\n",
		},
		{
			name: "set single-select value",
			opts: &EditFieldOptions{
				IssueNumber:          23,
				FieldID:              "FIELD_ID",
				SingleSelectOptionID: "OPTION_ID",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.GraphQL(`query IssueByNumber\b`), httpmock.StringResponse(issueResponse))
				reg.Register(
					httpmock.GraphQL(`mutation UpdateIssueFieldValue\b`),
					httpmock.GraphQLMutation(`{ "data": { "updateIssueFieldValue": { "clientMutationId": "" } } }`,
						func(inputs map[string]interface{}) {
							issueField := inputs["issueField"].(map[string]interface{})
							assert.Equal(t, "FIELD_ID", issueField["fieldId"])
							assert.Equal(t, "OPTION_ID", issueField["singleSelectOptionId"])
						}),
				)
			},
			wantStderr: "✓ Set issue field value on OWNER/REPO#23\n",
		},
		{
			name: "clear value",
			opts: &EditFieldOptions{
				IssueNumber: 23,
				FieldID:     "FIELD_ID",
				Clear:       true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.GraphQL(`query IssueByNumber\b`), httpmock.StringResponse(issueResponse))
				reg.Register(
					httpmock.GraphQL(`mutation UpdateIssueFieldValue\b`),
					httpmock.GraphQLMutation(`{ "data": { "updateIssueFieldValue": { "clientMutationId": "" } } }`,
						func(inputs map[string]interface{}) {
							issueField := inputs["issueField"].(map[string]interface{})
							assert.Equal(t, "FIELD_ID", issueField["fieldId"])
							assert.Equal(t, true, issueField["delete"])
						}),
				)
			},
			wantStderr: "✓ Cleared issue field value on OWNER/REPO#23\n",
		},
	}
	for _, tt := range tests {
		reg := &httpmock.Registry{}
		if tt.httpStubs != nil {
			tt.httpStubs(reg)
		}
		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}
		ios, _, _, stderr := iostreams.Test()
		tt.opts.IO = ios
		tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
			return ghrepo.FromFullName("OWNER/REPO")
		}
		t.Run(tt.name, func(t *testing.T) {
			defer reg.Verify(t)

			err := editFieldRun(tt.opts)
			if tt.wantErr {
				assert.EqualError(t, err, tt.errMsg)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}
