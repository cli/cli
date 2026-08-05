package api

import (
	"io"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
)

func newTestClient(reg *httpmock.Registry) *Client {
	client := &http.Client{}
	httpmock.ReplaceTripper(client, reg)
	return NewClientFromHTTP(client)
}

func TestGraphQL(t *testing.T) {
	http := &httpmock.Registry{}
	client := newTestClient(http)

	vars := map[string]interface{}{"name": "Mona"}
	response := struct {
		Viewer struct {
			Login string
		}
	}{}

	http.Register(
		httpmock.GraphQL("QUERY"),
		httpmock.StringResponse(`{"data":{"viewer":{"login":"hubot"}}}`),
	)

	err := client.GraphQL("github.com", "QUERY", vars, &response)
	assert.NoError(t, err)
	assert.Equal(t, "hubot", response.Viewer.Login)

	req := http.Requests[0]
	reqBody, _ := io.ReadAll(req.Body)
	assert.Equal(t, `{"query":"QUERY","variables":{"name":"Mona"}}`, string(reqBody))
}

func TestGraphQLError(t *testing.T) {
	reg := &httpmock.Registry{}
	client := newTestClient(reg)

	response := struct{}{}

	reg.Register(
		httpmock.GraphQL(""),
		httpmock.StringResponse(`
			{ "errors": [
				{
					"type": "NOT_FOUND",
					"message": "OH NO",
					"path": ["repository", "issue"]
				},
				{
					"type": "ACTUALLY_ITS_FINE",
					"message": "this is fine",
					"path": ["repository", "issues", 0, "comments"]
				}
			  ]
			}
		`),
	)

	err := client.GraphQL("github.com", "", nil, &response)
	if err == nil || err.Error() != "GraphQL: OH NO (repository.issue), this is fine (repository.issues.0.comments)" {
		t.Fatalf("got %q", err.Error())
	}
}
