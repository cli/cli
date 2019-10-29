package api

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
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
