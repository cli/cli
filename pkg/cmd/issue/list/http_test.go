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
		Variables map[string]any
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
				httpmock.GraphQLQuery(`{"data":{}}`, func(query string, vars map[string]any) {
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

// TestSearchIssues_paginationLimit tests that the per-page limit shrinks on
// subsequent pages when using --search with --limit > 100 (issue #13906).
func TestSearchIssues_paginationLimit(t *testing.T) {
	reg := &httpmock.Registry{}
	httpClient := &http.Client{}
	httpmock.ReplaceTripper(httpClient, reg)
	client := api.NewClientFromHTTP(httpClient)

	// First page: 100 issues returned, hasNextPage=true
		reg.Register(
			httpmock.GraphQL(`query IssueSearch\b`),
			httpmock.StringResponse(`{
				"data": {
					"repository": {"hasIssuesEnabled": true},
					"search": {
						"issueCount": 150,
						"nodes": [{"title": "issue1"},{"title": "issue2"},{"title": "issue3"},{"title": "issue4"},{"title": "issue5"},{"title": "issue6"},{"title": "issue7"},{"title": "issue8"},{"title": "issue9"},{"title": "issue10"},{"title": "issue11"},{"title": "issue12"},{"title": "issue13"},{"title": "issue14"},{"title": "issue15"},{"title": "issue16"},{"title": "issue17"},{"title": "issue18"},{"title": "issue19"},{"title": "issue20"},{"title": "issue21"},{"title": "issue22"},{"title": "issue23"},{"title": "issue24"},{"title": "issue25"},{"title": "issue26"},{"title": "issue27"},{"title": "issue28"},{"title": "issue29"},{"title": "issue30"},{"title": "issue31"},{"title": "issue32"},{"title": "issue33"},{"title": "issue34"},{"title": "issue35"},{"title": "issue36"},{"title": "issue37"},{"title": "issue38"},{"title": "issue39"},{"title": "issue40"},{"title": "issue41"},{"title": "issue42"},{"title": "issue43"},{"title": "issue44"},{"title": "issue45"},{"title": "issue46"},{"title": "issue47"},{"title": "issue48"},{"title": "issue49"},{"title": "issue50"},{"title": "issue51"},{"title": "issue52"},{"title": "issue53"},{"title": "issue54"},{"title": "issue55"},{"title": "issue56"},{"title": "issue57"},{"title": "issue58"},{"title": "issue59"},{"title": "issue60"},{"title": "issue61"},{"title": "issue62"},{"title": "issue63"},{"title": "issue64"},{"title": "issue65"},{"title": "issue66"},{"title": "issue67"},{"title": "issue68"},{"title": "issue69"},{"title": "issue70"},{"title": "issue71"},{"title": "issue72"},{"title": "issue73"},{"title": "issue74"},{"title": "issue75"},{"title": "issue76"},{"title": "issue77"},{"title": "issue78"},{"title": "issue79"},{"title": "issue80"},{"title": "issue81"},{"title": "issue82"},{"title": "issue83"},{"title": "issue84"},{"title": "issue85"},{"title": "issue86"},{"title": "issue87"},{"title": "issue88"},{"title": "issue89"},{"title": "issue90"},{"title": "issue91"},{"title": "issue92"},{"title": "issue93"},{"title": "issue94"},{"title": "issue95"},{"title": "issue96"},{"title": "issue97"},{"title": "issue98"},{"title": "issue99"},{"title": "issue100"}],
						"pageInfo": {"hasNextPage": true, "endCursor": "ENDCURSOR"}
					}
				}
			}`),
		)

	// Second page: should request last: 50 (not 100)
	reg.Register(
		httpmock.GraphQL(`query IssueSearch\b`),
		httpmock.GraphQLQuery(`{"data":{"repository":{"hasIssuesEnabled":true},"search":{"issueCount":150,"nodes":[],"pageInfo":{"hasNextPage":false}}}}`,
			func(query string, vars map[string]any) {
				if vars["limit"] != float64(50) {
					t.Errorf("expected second page limit=50, got %v", vars["limit"])
				}
			}),
	)

	repo := ghrepo.New("OWNER", "REPO")
	_, err := searchIssues(client, fd.AdvancedIssueSearchSupportedAsOnlyBackend(), repo, prShared.FilterOptions{Search: "repo:OWNER/REPO state:open"}, 150)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
