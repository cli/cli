package api

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/httpmock"
)

func TestIssueList(t *testing.T) {
	http := &httpmock.Registry{}
	client := NewClient(ReplaceTripper(http))

	http.StubResponse(200, bytes.NewBufferString(`
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
	`))
	http.StubResponse(200, bytes.NewBufferString(`
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
	`))

	repo, _ := ghrepo.FromFullName("OWNER/REPO")
	_, err := IssueList(client, repo, "open", []string{}, "", 251, "")
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
