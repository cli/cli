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
			name:   "milestone",
			fields: []string{"number", "milestone"},
			inputJSON: heredoc.Doc(`
				{ "number": 2345, "milestone": {"title": "The next big thing"} }
			`),
			outputJSON: heredoc.Doc(`
				{
					"milestone": {
						"number": 0,
						"title": "The next big thing",
						"description": "",
						"dueOn": null
					},
					"number": 2345
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
		{
			name:   "project items",
			fields: []string{"projectItems"},
			inputJSON: heredoc.Doc(`
				{ "projectItems": { "nodes": [
					{
						"id": "PVTI_id",
						"project": {
							"id": "PVT_id",
							"title": "Some Project"
						}
					}
				] } }
			`),
			outputJSON: heredoc.Doc(`
				{
					"projectItems": [
						{
							"title": "Some Project"
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
			name:   "milestone",
			fields: []string{"number", "milestone"},
			inputJSON: heredoc.Doc(`
				{ "number": 2345, "milestone": {"title": "The next big thing"} }
			`),
			outputJSON: heredoc.Doc(`
				{
					"milestone": {
						"number": 0,
						"title": "The next big thing",
						"description": "",
						"dueOn": null
					},
					"number": 2345
				}
			`),
		},
		{
			name:   "status checks",
			fields: []string{"statusCheckRollup"},
			inputJSON: heredoc.Doc(`
				{ "statusCheckRollup": { "nodes": [
					{ "commit": { "statusCheckRollup": { "contexts": { "nodes": [
						{
							"__typename": "CheckRun",
							"name": "mycheck",
							"checkSuite": {"workflowRun": {"workflow": {"name": "myworkflow"}}},
							"status": "COMPLETED",
							"conclusion": "SUCCESS",
							"startedAt": "2020-08-31T15:44:24+02:00",
							"completedAt": "2020-08-31T15:45:24+02:00",
							"detailsUrl": "http://example.com/details"
						},
						{
							"__typename": "StatusContext",
							"context": "mycontext",
							"state": "SUCCESS",
							"createdAt": "2020-08-31T15:44:24+02:00",
							"targetUrl": "http://example.com/details"
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
							"workflowName": "myworkflow",
							"status": "COMPLETED",
							"conclusion": "SUCCESS",
							"startedAt": "2020-08-31T15:44:24+02:00",
							"completedAt": "2020-08-31T15:45:24+02:00",
							"detailsUrl": "http://example.com/details"
						},
						{
							"__typename": "StatusContext",
							"context": "mycontext",
							"state": "SUCCESS",
							"startedAt": "2020-08-31T15:44:24+02:00",
							"targetUrl": "http://example.com/details"
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

			var gotData interface{}
			dec = json.NewDecoder(&buf)
			require.NoError(t, dec.Decode(&gotData))
			var expectData interface{}
			require.NoError(t, json.Unmarshal([]byte(tt.outputJSON), &expectData))

			assert.Equal(t, expectData, gotData)
		})
	}
}
