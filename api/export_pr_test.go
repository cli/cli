package api

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIssue_ExportData(t *testing.T) {
	tests := []struct {
		name       string
		fields     []string
		inputJSON  string
		outputJSON string
	}{
		{
			name:   "simple",
			fields: []string{"number", "title"},
			inputJSON: heredoc.Doc(`
				{ "title": "Bugs hugs", "number": 2345 }
			`),
			outputJSON: heredoc.Doc(`
				{
					"number": 2345,
					"title": "Bugs hugs"
				}
			`),
		},
		{
			name:   "project cards",
			fields: []string{"projectCards"},
			inputJSON: heredoc.Doc(`
				{ "projectCards": { "nodes": [
					{
						"project": { "name": "Rewrite" },
						"column": { "name": "TO DO" }
					}
				] } }
			`),
			outputJSON: heredoc.Doc(`
				{
					"projectCards": [
						{
							"project": {
								"name": "Rewrite"
							},
							"column": {
								"name": "TO DO"
							}
						}
					]
				}
			`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var issue Issue
			dec := json.NewDecoder(strings.NewReader(tt.inputJSON))
			require.NoError(t, dec.Decode(&issue))

			exported := issue.ExportData(tt.fields)

			buf := bytes.Buffer{}
			enc := json.NewEncoder(&buf)
			enc.SetIndent("", "\t")
			require.NoError(t, enc.Encode(exported))
			assert.Equal(t, tt.outputJSON, buf.String())
		})
	}
}

func TestPullRequest_ExportData(t *testing.T) {
	tests := []struct {
		name       string
		fields     []string
		inputJSON  string
		outputJSON string
	}{
		{
			name:   "simple",
			fields: []string{"number", "title"},
			inputJSON: heredoc.Doc(`
				{ "title": "Bugs hugs", "number": 2345 }
			`),
			outputJSON: heredoc.Doc(`
				{
					"number": 2345,
					"title": "Bugs hugs"
				}
			`),
		},
		{
			name:   "status checks",
			fields: []string{"statusCheckRollup"},
			inputJSON: heredoc.Doc(`
				{ "commits": { "nodes": [
					{ "commit": { "statusCheckRollup": { "contexts": { "nodes": [
						{
							"__typename": "CheckRun",
							"name": "mycheck",
							"status": "COMPLETED",
							"conclusion": "SUCCESS",
							"startedAt": "2020-08-31T15:44:24+02:00",
							"completedAt": "2020-08-31T15:45:24+02:00",
							"detailsUrl": "http://example.com/details"
						}
					] } } } }
				] } }
			`),
			outputJSON: heredoc.Doc(`
				{
					"statusCheckRollup": [
						{
							"__typename": "CheckRun",
							"name": "mycheck",
							"status": "COMPLETED",
							"conclusion": "SUCCESS",
							"startedAt": "2020-08-31T15:44:24+02:00",
							"completedAt": "2020-08-31T15:45:24+02:00",
							"detailsUrl": "http://example.com/details"
						}
					]
				}
			`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pr PullRequest
			dec := json.NewDecoder(strings.NewReader(tt.inputJSON))
			require.NoError(t, dec.Decode(&pr))

			exported := pr.ExportData(tt.fields)

			buf := bytes.Buffer{}
			enc := json.NewEncoder(&buf)
			enc.SetIndent("", "\t")
			require.NoError(t, enc.Encode(exported))
			assert.Equal(t, tt.outputJSON, buf.String())
		})
	}
}
