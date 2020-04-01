package api

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strings"
)

// FakeHTTP provides a mechanism by which to stub HTTP responses through
type FakeHTTP struct {
	// Requests stores references to sequential requests that RoundTrip has received
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
	f.StubRepoResponseWithPermission(owner, repo, "WRITE")
}

func (f *FakeHTTP) StubRepoResponseWithPermission(owner, repo, permission string) {
	body := bytes.NewBufferString(fmt.Sprintf(`
		{ "data": { "repo_000": {
			"id": "REPOID",
			"name": "%s",
			"owner": {"login": "%s"},
			"defaultBranchRef": {
				"name": "master"
			},
			"viewerPermission": "%s"
		} } }
	`, repo, owner, permission))
	resp := &http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(body),
	}
	f.responseStubs = append(f.responseStubs, resp)
}

func (f *FakeHTTP) StubForkedRepoResponse(forkFullName, parentFullName string) {
	forkRepo := strings.SplitN(forkFullName, "/", 2)
	parentRepo := strings.SplitN(parentFullName, "/", 2)
	body := bytes.NewBufferString(fmt.Sprintf(`
		{ "data": { "repo_000": {
			"id": "REPOID2",
			"name": "%s",
			"owner": {"login": "%s"},
			"defaultBranchRef": {
				"name": "master"
			},
			"viewerPermission": "ADMIN",
			"parent": {
				"id": "REPOID1",
				"name": "%s",
				"owner": {"login": "%s"},
				"defaultBranchRef": {
					"name": "master"
				},
				"viewerPermission": "READ"
			}
		} } }
	`, forkRepo[1], forkRepo[0], parentRepo[1], parentRepo[0]))
	resp := &http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(body),
	}
	f.responseStubs = append(f.responseStubs, resp)
}
