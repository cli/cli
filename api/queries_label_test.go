package api

import (
	"bytes"
	"encoding/json"
	"github.com/cli/cli/internal/ghrepo"
	"io/ioutil"
	"testing"
)

func TestLabelList(t *testing.T) {
	http := &FakeHTTP{}
	client := NewClient(ReplaceTripper(http))

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": {
		"labels": {
			"totalCount": 1,
			"nodes": [
                {
                    "color": "d73a4a",
                    "createdAt": "2020-03-13T12:00:28Z",
                    "description": "Something isn't working",
                    "id": "MDU6TGFiZWwxOTA2OTM1Mzk4",
                    "isDefault": true,
                    "name": "bug",
                    "resourcePath": "/OWNER/REPO/labels/bug",
                    "updatedAt": "2020-03-13T12:00:28Z",
                    "url": "https://github.com/OWNER/REPO/labels/bug"
                }
            ]
		}
	} } }
	`))

	_, err := LabelList(client, ghrepo.FromFullName("OWNER/REPO"), 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var reqBody struct {
		Query     string
		Variables map[string]interface{}
	}

	bodyBytes, _ := ioutil.ReadAll(http.Requests[0].Body)
	json.Unmarshal(bodyBytes, &reqBody)
	if first := reqBody.Variables["first"].(float64); first != 100 {
		t.Errorf("expected 100, got %v", first)
	}
	if _, cursorPresent := reqBody.Variables["endCursor"]; cursorPresent {
		t.Error("did not expect first request to pass 'endCursor'")
	}
}
