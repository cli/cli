package list

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	prShared "github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/httpmock"
)

func TestIssueList(t *testing.T) {
	http := &httpmock.Registry{}
	client := api.NewClient(api.ReplaceTripper(http))

	http.Register(
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
	http.Register(
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

	if len(http.Requests) != 2 {
		t.Fatalf("expected 2 HTTP requests, seen %d", len(http.Requests))
	}
	var reqBody struct {
		Query     string
		Variables map[string]interface{}
	}

	bodyBytes, _ := ioutil.ReadAll(http.Requests[0].Body)
	_ = json.Unmarshal(bodyBytes, &reqBody)
	if reqLimit := reqBody.Variables["limit"].(float64); reqLimit != 100 {
		t.Errorf("expected 100, got %v", reqLimit)
	}
	if _, cursorPresent := reqBody.Variables["endCursor"]; cursorPresent {
		t.Error("did not expect first request to pass 'endCursor'")
	}

	bodyBytes, _ = ioutil.ReadAll(http.Requests[1].Body)
	_ = json.Unmarshal(bodyBytes, &reqBody)
	if endCursor := reqBody.Variables["endCursor"].(string); endCursor != "ENDCURSOR" {
		t.Errorf("expected %q, got %q", "ENDCURSOR", endCursor)
	}
}

func TestIssueList_pagination(t *testing.T) {
	http := &httpmock.Registry{}
	client := api.NewClient(api.ReplaceTripper(http))

	http.Register(
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

	http.Register(
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
