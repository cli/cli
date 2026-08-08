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
						},
						"status": {
							"name": "Todo",
							"optionId": "abc123"
						}
					}
				] } }
			`),
			outputJSON: heredoc.Doc(`
				{
					"projectItems": [
						{
							"status": {
								"optionId": "abc123",
								"name": "Todo"
							},
							"title": "Some Project"
						}
					]
				}
			`),
		},
		{
			name:   "assignees",
			fields: []string{"assignees"},
			inputJSON: heredoc.Doc(`
				{ "assignees": { "nodes": [
					{
						"id": "MDQ6VXNlcjE=",
						"login": "monalisa",
						"name": "Mona Lisa",
						"databaseId": 1234
					}
				] } }
			`),
			outputJSON: heredoc.Doc(`
				{
					"assignees": [
						{
							"id": "MDQ6VXNlcjE=",
							"login": "monalisa",
							"name": "Mona Lisa",
							"databaseId": 1234
						}
					]
				}
			`),
		},
		{
			name:   "linked pull requests",
			fields: []string{"closedByPullRequestsReferences"},
			inputJSON: heredoc.Doc(`
				{ "closedByPullRequestsReferences": { "nodes": [
					{
						"id": "I_123",
						"number": 123,
						"url": "https://github.com/cli/cli/pull/123",
						"repository": {
							"id": "R_123",
							"name": "cli",
							"owner": {
								"id": "O_123",
								"login": "cli"
							}
						}
					},
					{
						"id": "I_456",
						"number": 456,
						"url": "https://github.com/cli/cli/pull/456",
						"repository": {
							"id": "R_456",
							"name": "cli",
							"owner": {
								"id": "O_456",
								"login": "cli"
							}
						}
					}
				] } }
			`),
			outputJSON: heredoc.Doc(`
				{ "closedByPullRequestsReferences": [
					{
						"id": "I_123",
						"number": 123,
						"repository": {
							"id": "R_123",
							"name": "cli",
							"owner": {
								"id": "O_123",
								"login": "cli"
							}
						},
						"url": "https://github.com/cli/cli/pull/123"
					},
					{
						"id": "I_456",
						"number": 456,
						"repository": {
							"id": "R_456",
							"name": "cli",
							"owner": {
								"id": "O_456",
								"login": "cli"
							}
						},
						"url": "https://github.com/cli/cli/pull/456"
					}
				] }
			`),
		},
		{
			name:   "issue type",
			fields: []string{"issueType"},
			inputJSON: heredoc.Doc(`
				{ "issueType": {
					"id": "IT_1",
					"name": "Bug",
					"description": "Something is not working",
					"color": "d73a4a"
				} }
			`),
			outputJSON: heredoc.Doc(`
				{
					"issueType": {
						"id": "IT_1",
						"name": "Bug",
						"description": "Something is not working",
						"color": "d73a4a"
					}
				}
			`),
		},
		{
			name:      "issue type null",
			fields:    []string{"issueType"},
			inputJSON: `{}`,
			outputJSON: heredoc.Doc(`
				{ "issueType": null }
			`),
		},
		{
			name:   "parent",
			fields: []string{"parent"},
			inputJSON: heredoc.Doc(`
				{ "parent": {
					"id": "I_100",
					"number": 100,
					"title": "Epic: Authentication overhaul",
					"url": "https://github.com/OWNER/REPO/issues/100",
					"state": "OPEN",
					"repository": {"nameWithOwner": "OWNER/REPO"}
				} }
			`),
			outputJSON: heredoc.Doc(`
				{
					"parent": {
						"id": "I_100",
						"number": 100,
						"title": "Epic: Authentication overhaul",
						"url": "https://github.com/OWNER/REPO/issues/100",
						"state": "OPEN"
					}
				}
			`),
		},
		{
			name:      "parent null",
			fields:    []string{"parent"},
			inputJSON: `{}`,
			outputJSON: heredoc.Doc(`
				{ "parent": null }
			`),
		},
		{
			name:   "sub-issues",
			fields: []string{"subIssues"},
			inputJSON: heredoc.Doc(`
				{ "subIssues": {
					"nodes": [
						{
							"id": "I_101",
							"number": 101,
							"title": "Design auth module",
							"url": "https://github.com/OWNER/REPO/issues/101",
							"state": "CLOSED",
							"repository": {"nameWithOwner": "OWNER/REPO"}
						},
						{
							"id": "I_102",
							"number": 102,
							"title": "Token refresh logic",
							"url": "https://github.com/OWNER/REPO/issues/102",
							"state": "OPEN",
							"repository": {"nameWithOwner": "OWNER/REPO"}
						}
					],
					"totalCount": 2
				} }
			`),
			outputJSON: heredoc.Doc(`
				{
					"subIssues": {
						"nodes": [
							{
								"id": "I_101",
								"number": 101,
								"title": "Design auth module",
								"url": "https://github.com/OWNER/REPO/issues/101",
								"state": "CLOSED"
							},
							{
								"id": "I_102",
								"number": 102,
								"title": "Token refresh logic",
								"url": "https://github.com/OWNER/REPO/issues/102",
								"state": "OPEN"
							}
						],
						"totalCount": 2
					}
				}
			`),
		},
		{
			name:   "sub-issues summary",
			fields: []string{"subIssuesSummary"},
			inputJSON: heredoc.Doc(`
				{ "subIssuesSummary": {
					"total": 4,
					"completed": 1,
					"percentCompleted": 25.0
				} }
			`),
			outputJSON: heredoc.Doc(`
				{
					"subIssuesSummary": {
						"total": 4,
						"completed": 1,
						"percentCompleted": 25
					}
				}
			`),
		},
		{
			name:   "blocked by",
			fields: []string{"blockedBy"},
			inputJSON: heredoc.Doc(`
				{ "blockedBy": {
					"nodes": [
						{
							"id": "I_200",
							"number": 200,
							"title": "API rate limiting",
							"url": "https://github.com/OWNER/REPO/issues/200",
							"state": "OPEN",
							"repository": {"nameWithOwner": "OWNER/REPO"}
						}
					],
					"totalCount": 1
				} }
			`),
			outputJSON: heredoc.Doc(`
				{
					"blockedBy": {
						"nodes": [
							{
								"id": "I_200",
								"number": 200,
								"title": "API rate limiting",
								"url": "https://github.com/OWNER/REPO/issues/200",
								"state": "OPEN"
							}
						],
						"totalCount": 1
					}
				}
			`),
		},
		{
			name:   "blocking",
			fields: []string{"blocking"},
			inputJSON: heredoc.Doc(`
				{ "blocking": {
					"nodes": [
						{
							"id": "I_300",
							"number": 300,
							"title": "Release v2.0",
							"url": "https://github.com/OWNER/REPO/issues/300",
							"state": "OPEN",
							"repository": {"nameWithOwner": "OWNER/REPO"}
						}
					],
					"totalCount": 1
				} }
			`),
			outputJSON: heredoc.Doc(`
				{
					"blocking": {
						"nodes": [
							{
								"id": "I_300",
								"number": 300,
								"title": "Release v2.0",
								"url": "https://github.com/OWNER/REPO/issues/300",
								"state": "OPEN"
							}
						],
						"totalCount": 1
					}
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

			var gotData interface{}
			dec = json.NewDecoder(&buf)
			require.NoError(t, dec.Decode(&gotData))
			var expectData interface{}
			require.NoError(t, json.Unmarshal([]byte(tt.outputJSON), &expectData))

			assert.Equal(t, expectData, gotData)
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
		{
			name:   "project items",
			fields: []string{"projectItems"},
			inputJSON: heredoc.Doc(`
				{ "projectItems": { "nodes": [
					{
						"id": "PVTPR_id",
						"project": {
							"id": "PVT_id",
							"title": "Some Project"
						},
						"status": {
							"name": "Todo",
							"optionId": "abc123"
						}
					}
				] } }
			`),
			outputJSON: heredoc.Doc(`
				{
					"projectItems": [
						{
							"status": {
								"optionId": "abc123",
								"name": "Todo"
							},
							"title": "Some Project"
						}
					]
				}
			`),
		},
		{
			name:   "assignees",
			fields: []string{"assignees"},
			inputJSON: heredoc.Doc(`
				{ "assignees": { "nodes": [
					{
						"id": "MDQ6VXNlcjE=",
						"login": "monalisa",
						"name": "Mona Lisa",
						"databaseId": 1234
					}
				] } }
			`),
			outputJSON: heredoc.Doc(`
				{
					"assignees": [
						{
							"id": "MDQ6VXNlcjE=",
							"login": "monalisa",
							"name": "Mona Lisa",
							"databaseId": 1234
						}
					]
				}
			`),
		},
		{
			name:   "linked issues",
			fields: []string{"closingIssuesReferences"},
			inputJSON: heredoc.Doc(`
				{ "closingIssuesReferences": { "nodes": [
					{
						"id": "I_123",
						"number": 123,
						"url": "https://github.com/cli/cli/issues/123",
						"repository": {
							"id": "R_123",
							"name": "cli",
							"owner": {
								"id": "O_123",
								"login": "cli"
							}
						}
					},
					{
						"id": "I_456",
						"number": 456,
						"url": "https://github.com/cli/cli/issues/456",
						"repository": {
							"id": "R_456",
							"name": "cli",
							"owner": {
								"id": "O_456",
								"login": "cli"
							}
						}
					}
				] } }
			`),
			outputJSON: heredoc.Doc(`
				{ "closingIssuesReferences": [
					{
						"id": "I_123",
						"number": 123,
						"repository": {
							"id": "R_123",
							"name": "cli",
							"owner": {
							"id": "O_123",
							"login": "cli"
							}
						},
						"url": "https://github.com/cli/cli/issues/123"
					},
					{
						"id": "I_456",
						"number": 456,
						"repository": {
							"id": "R_456",
							"name": "cli",
							"owner": {
							"id": "O_456",
							"login": "cli"
							}
						},
						"url": "https://github.com/cli/cli/issues/456"
					}
				] }
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
