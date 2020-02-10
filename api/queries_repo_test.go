package api

import (
	"bytes"
	"testing"
)

func TestCreateRepoCheckUrlPath(t *testing.T) {
	testParams := []struct {
		org_name string
		path     string
	}{
		{org_name: "fake_organization", path: "/orgs/fake_organization/repos"},
		{org_name: "", path: "/user/repos"},
	}
	for _, p := range testParams {
		http := &FakeHTTP{}
		client := NewClient(ReplaceTripper(http))

		http.StubResponse(200, bytes.NewBufferString(`{}`))
		params := make(map[string]string)

		_, err := CreateRepo(client, p.org_name, params)

		if err != nil {
			t.Fatalf("got %q", err.Error())
		}

		if len(http.Requests) == 0 {
			t.Fatalf("There is not http request for create repo")
		}

		path := http.Requests[0].URL.Path
		expectedPath := p.path
		if path != expectedPath {
			t.Fatalf("expected path: '%s' is not equal fact path: '%s'", expectedPath, path)
		}
	}
}
