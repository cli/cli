package api

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
)

// FakeHTTP provides a mechanism by which to stub HTTP responses through
type FakeHTTP struct {
	// Requests stores references to sequental requests that RoundTrip has received
	Requests      []*http.Request
	count         int
	responseStubs []*http.Response
}

// StubResponse pre-records an HTTP response
func (f *FakeHTTP) StubResponse(status int, body io.Reader) {
	resp := &http.Response{
		StatusCode: status,
		Body:       ioutil.NopCloser(body),
	}
	f.responseStubs = append(f.responseStubs, resp)
}

// RoundTrip satisfies http.RoundTripper
func (f *FakeHTTP) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(f.responseStubs) <= f.count {
		return nil, fmt.Errorf("FakeHTTP: missing response stub for request %d", f.count)
	}
	resp := f.responseStubs[f.count]
	f.count++
	resp.Request = req
	f.Requests = append(f.Requests, req)
	return resp, nil
}

func (f *FakeHTTP) StubWithFixture(status int, fixtureFileName string) func() {
	fixturePath := path.Join("../test/fixtures/", fixtureFileName)
	fixtureFile, _ := os.Open(fixturePath)
	f.StubResponse(status, fixtureFile)
	return func() { fixtureFile.Close() }
}

func (f *FakeHTTP) StubRepoResponse(owner, repo string) {
	body := bytes.NewBufferString(fmt.Sprintf(`
		{ "data": { "repo_000": {
			"id": "REPOID",
			"name": "%s",
			"owner": {"login": "%s"},
			"defaultBranchRef": {
				"name": "master",
				"target": {"oid": "deadbeef"}
			},
			"viewerPermission": "WRITE"
		} } }
	`, repo, owner))
	resp := &http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(body),
	}
	f.responseStubs = append(f.responseStubs, resp)
}
