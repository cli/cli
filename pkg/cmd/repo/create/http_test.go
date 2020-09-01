package create

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/cli/cli/api"
	"github.com/cli/cli/pkg/httpmock"
)

func Test_RepoCreate(t *testing.T) {
	reg := &httpmock.Registry{}
	httpClient := api.NewHTTPClient(api.ReplaceTripper(reg))

	reg.StubResponse(200, bytes.NewBufferString(`{}`))

	input := repoCreateInput{
		Description: "roasted chesnuts",
		HomepageURL: "http://example.com",
	}

	_, err := repoCreate(httpClient, "github.com", input, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(reg.Requests) != 1 {
		t.Fatalf("expected 1 HTTP request, seen %d", len(reg.Requests))
	}

	var reqBody struct {
		Query     string
		Variables struct {
			Input map[string]interface{}
		}
	}

	bodyBytes, _ := ioutil.ReadAll(reg.Requests[0].Body)
	_ = json.Unmarshal(bodyBytes, &reqBody)
	if description := reqBody.Variables.Input["description"].(string); description != "roasted chesnuts" {
		t.Errorf("expected description to be %q, got %q", "roasted chesnuts", description)
	}
	if homepage := reqBody.Variables.Input["homepageUrl"].(string); homepage != "http://example.com" {
		t.Errorf("expected homepageUrl to be %q, got %q", "http://example.com", homepage)
	}
}
