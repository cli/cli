package list

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
)

func TestLabelList_pagination(t *testing.T) {
	http := &httpmock.Registry{}
	client := api.NewClient(api.ReplaceTripper(http))

	http.Register(
		httpmock.GraphQL(`query LabelList\b`),
		httpmock.StringResponse(`
		{
			"data": {
				"repository": {
					"labels": {
						"totalCount": 2,
						"nodes": [
							{
								"name": "bug",
								"color": "d73a4a",
								"description": "This is a bug label"
							}
						],
						"pageInfo": {
							"hasNextPage": true,
							"endCursor": "Y3Vyc29yOnYyOpK5MjAxOS0xMC0xMVQwMTozODowMyswODowMM5f3HZq"
						}
					}
				}
			}
		}`),
	)

	http.Register(
		httpmock.GraphQL(`query LabelList\b`),
		httpmock.StringResponse(`
		{
			"data": {
				"repository": {
					"labels": {
						"totalCount": 2,
						"nodes": [
							{
								"name": "docs",
								"color": "ffa8da",
								"description": "This is a docs label"
							}
						],
						"pageInfo": {
							"hasNextPage": false,
							"endCursor": "Y3Vyc29yOnYyOpK5MjAyMi0wMS0zMVQxODo1NTo1MiswODowMM7hiAL3"
						}
					}
				}
			}
		}`),
	)

	repo := ghrepo.New("OWNER", "REPO")
	labels, totalCount, err := listLabels(client, repo, 10)
	if err != nil {
		t.Fatalf("LabelList() error = %v", err)
	}

	assert.Equal(t, 2, totalCount)
	assert.Equal(t, 2, len(labels))

	assert.Equal(t, "bug", labels[0].Name)
	assert.Equal(t, "d73a4a", labels[0].Color)
	assert.Equal(t, "This is a bug label", labels[0].Description)

	assert.Equal(t, "docs", labels[1].Name)
	assert.Equal(t, "ffa8da", labels[1].Color)
	assert.Equal(t, "This is a docs label", labels[1].Description)

	var reqBody struct {
		Query     string
		Variables map[string]interface{}
	}

	bodyBytes, _ := ioutil.ReadAll(http.Requests[0].Body)
	_ = json.Unmarshal(bodyBytes, &reqBody)

	assert.Nil(t, reqBody.Variables["endCursor"])

	bodyBytes, _ = ioutil.ReadAll(http.Requests[1].Body)
	_ = json.Unmarshal(bodyBytes, &reqBody)

	assert.Equal(t, "Y3Vyc29yOnYyOpK5MjAxOS0xMC0xMVQwMTozODowMyswODowMM5f3HZq", reqBody.Variables["endCursor"].(string))
}
