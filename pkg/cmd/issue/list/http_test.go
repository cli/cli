package list

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/api"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/ghrepo"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
)

func TestIssueList(t *testing.T) {
	reg := &httpmock.Registry{}
	httpClient := &http.Client{}
	httpmock.ReplaceTripper(httpClient, reg)
	client := api.NewClientFromHTTP(httpClient)

	reg.Register(
		httpmock.GraphQL(`query IssueList\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": {
				"hasIssuesEnabled": true,
				"issues": {
					"nodes": [],
					"pageInfo": {
						"hasNextPage": true,
						"endCursor": "ENDCURSOR"
					}
				}
			} } }
		`),
	)
	reg.Register(
		httpmock.GraphQL(`query IssueList\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": {
				"hasIssuesEnabled": true,
				"issues": {
					"nodes": [],
					"pageInfo": {
						"hasNextPage": false,
						"endCursor": "ENDCURSOR"
					}
				}
			} } }
			`),
	)

	repo, _ := ghrepo.FromFullName("OWNER/REPO")
	filters := prShared.FilterOptions{
		Entity: "issue",
		State:  "open",
	}
	_, err := listIssues(client, repo, filters, 251)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reg.Requests) != 2 {
		t.Fatalf("expected 2 HTTP requests, seen %d", len(reg.Requests))
	}
	var reqBody struct {
		Query     string
		Variables map[string]interface{}
	}

	bodyBytes, _ := io.ReadAll(reg.Requests[0].Body)
	_ = json.Unmarshal(bodyBytes, &reqBody)
	if reqLimit := reqBody.Variables["limit"].(float64); reqLimit != 100 {
		t.Errorf("expected 100, got %v", reqLimit)
	}
	if _, cursorPresent := reqBody.Variables["endCursor"]; cursorPresent {
		t.Error("did not expect first request to pass 'endCursor'")
	}

	bodyBytes, _ = io.ReadAll(reg.Requests[1].Body)
	_ = json.Unmarshal(bodyBytes, &reqBody)
	if endCursor := reqBody.Variables["endCursor"].(string); endCursor != "ENDCURSOR" {
		t.Errorf("expected %q, got %q", "ENDCURSOR", endCursor)
	}
}

func TestIssueList_pagination(t *testing.T) {
	reg := &httpmock.Registry{}
	httpClient := &http.Client{}
	httpmock.ReplaceTripper(httpClient, reg)
	client := api.NewClientFromHTTP(httpClient)

	reg.Register(
		httpmock.GraphQL(`query IssueList\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": {
				"hasIssuesEnabled": true,
				"issues": {
					"nodes": [
						{
							"title": "issue1",
							"labels": { "nodes": [ { "name": "bug" } ], "totalCount": 1 },
							"assignees": { "nodes": [ { "login": "user1" } ], "totalCount": 1 }
						}
					],
					"pageInfo": {
						"hasNextPage": true,
						"endCursor": "ENDCURSOR"
					},
					"totalCount": 2
				}
			} } }
			`),
	)

	reg.Register(
		httpmock.GraphQL(`query IssueList\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": {
				"hasIssuesEnabled": true,
				"issues": {
					"nodes": [
						{
							"title": "issue2",
							"labels": { "nodes": [ { "name": "enhancement" } ], "totalCount": 1 },
							"assignees": { "nodes": [ { "login": "user2" } ], "totalCount": 1 }
						}
					],
					"pageInfo": {
						"hasNextPage": false,
						"endCursor": "ENDCURSOR"
					},
					"totalCount": 2
				}
			} } }
			`),
	)

	repo := ghrepo.New("OWNER", "REPO")
	res, err := listIssues(client, repo, prShared.FilterOptions{}, 0)
	if err != nil {
		t.Fatalf("IssueList() error = %v", err)
	}

	assert.Equal(t, 2, res.TotalCount)
	assert.Equal(t, 2, len(res.Issues))

	getLabels := func(i api.Issue) []string {
		var labels []string
		for _, l := range i.Labels.Nodes {
			labels = append(labels, l.Name)
		}
		return labels
	}
	getAssignees := func(i api.Issue) []string {
		var logins []string
		for _, u := range i.Assignees.Nodes {
			logins = append(logins, u.Login)
		}
		return logins
	}

	assert.Equal(t, []string{"bug"}, getLabels(res.Issues[0]))
	assert.Equal(t, []string{"user1"}, getAssignees(res.Issues[0]))
	assert.Equal(t, []string{"enhancement"}, getLabels(res.Issues[1]))
	assert.Equal(t, []string{"user2"}, getAssignees(res.Issues[1]))
}

// TODO advancedIssueSearchCleanup
// Remove this test once GHES 3.17 support ends.
func TestSearchIssuesAndAdvancedSearch(t *testing.T) {
	tests := []struct {
		name           string
		detector       fd.Detector
		wantSearchType string
	}{
		{
			name:           "advanced issue search not supported",
			detector:       fd.AdvancedIssueSearchUnsupported(),
			wantSearchType: "ISSUE",
		},
		{
			name:           "advanced issue search supported as opt-in",
			detector:       fd.AdvancedIssueSearchSupportedAsOptIn(),
			wantSearchType: "ISSUE_ADVANCED",
		},
		{
			name:           "advanced issue search supported as only backend",
			detector:       fd.AdvancedIssueSearchSupportedAsOnlyBackend(),
			wantSearchType: "ISSUE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			reg.Register(
				httpmock.GraphQL(`query IssueSearch\b`),
				httpmock.GraphQLQuery(`{"data":{}}`, func(query string, vars map[string]interface{}) {
					assert.Equal(t, tt.wantSearchType, vars["type"])
					// Since no repeated usage of special search qualifiers is possible
					// with our current implementation, we can assert against the same
					// query for both search backend (i.e. legacy and advanced issue search).
					assert.Equal(t, "repo:OWNER/REPO state:open type:issue", vars["query"])
				}))

			httpClient := &http.Client{Transport: reg}
			client := api.NewClientFromHTTP(httpClient)

			searchIssues(client, tt.detector, ghrepo.New("OWNER", "REPO"), prShared.FilterOptions{State: "open"}, 30)
		})
	}
}

func TestSearchIssues_rejectsPullRequestQualifiers(t *testing.T) {
	tests := []struct {
		name   string
		search string
	}{
		{
			name:   "is:pr",
			search: "is:pr",
		},
		{
			name:   "type:pr",
			search: "type:pr",
		},
		{
			name:   "type:pull-request",
			search: "type:pull-request",
		},
		{
			name:   "type:pullrequest",
			search: "type:pullrequest",
		},
		{
			name:   "case-insensitive is:PR",
			search: "is:PR",
		},
		{
			name:   "case-insensitive TYPE:Pull-Request",
			search: "TYPE:Pull-Request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			httpClient := &http.Client{Transport: reg}
			client := api.NewClientFromHTTP(httpClient)

			_, err := searchIssues(
				client,
				fd.AdvancedIssueSearchSupportedAsOnlyBackend(),
				ghrepo.New("OWNER", "REPO"),
				prShared.FilterOptions{Search: tt.search},
				30,
			)

			assert.EqualError(t, err, "cannot use pull request search qualifiers with `gh issue list`; use `gh pr list` instead")
			assert.Len(t, reg.Requests, 0)
		})
	}
}
