package api

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/cli/cli/pkg/httpmock"
)

func Test_RepoCreate(t *testing.T) {
	http := &httpmock.Registry{}
	client := NewClient(ReplaceTripper(http))

	http.StubResponse(200, bytes.NewBufferString(`{}`))

	input := RepoCreateInput{
		Description: "roasted chesnuts",
		HomepageURL: "http://example.com",
	}

	_, err := RepoCreate(client, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(http.Requests) != 1 {
		t.Fatalf("expected 1 HTTP request, seen %d", len(http.Requests))
	}

	var reqBody struct {
		Query     string
		Variables struct {
			Input map[string]interface{}
		}
	}

	bodyBytes, _ := ioutil.ReadAll(http.Requests[0].Body)
	_ = json.Unmarshal(bodyBytes, &reqBody)
	if description := reqBody.Variables.Input["description"].(string); description != "roasted chesnuts" {
		t.Errorf("expected description to be %q, got %q", "roasted chesnuts", description)
	}
	if homepage := reqBody.Variables.Input["homepageUrl"].(string); homepage != "http://example.com" {
		t.Errorf("expected homepageUrl to be %q, got %q", "http://example.com", homepage)
	}
}
