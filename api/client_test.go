package api

import (
	"bytes"
	"errors"
	"github.com/cli/cli/v2/internal/githubrest"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestRESTGetDelete(t *testing.T) {
	http := &httpmock.Registry{}
	client := newTestClient(http)

	http.Register(
		httpmock.REST("DELETE", "applications/CLIENTID/grant"),
		httpmock.StatusStringResponse(204, "{}"),
	)

	r := bytes.NewReader([]byte(`{}`))
	err := client.REST("github.com", "DELETE", "applications/CLIENTID/grant", r, nil)
	assert.NoError(t, err)
}

func TestRESTWithFullURL(t *testing.T) {
	http := &httpmock.Registry{}
	client := newTestClient(http)

	http.Register(
		httpmock.REST("GET", "api/v3/user/repos"),
		httpmock.StatusStringResponse(200, "{}"))
	http.Register(
		httpmock.REST("GET", "user/repos"),
		httpmock.StatusStringResponse(200, "{}"))

	err := client.REST("example.com", "GET", "user/repos", nil, nil)
	require.NoError(t, err)
	err = client.REST("example.com", "GET", "https://example.com/user/repos", nil, nil)
	require.NoError(t, err)

	assert.Equal(t, "example.com", http.Requests[0].URL.Hostname())
	assert.Equal(t, "example.com", http.Requests[1].URL.Hostname())
}

// TestRESTRefusesForeignAbsoluteURL records a deliberate behaviour change. This
// test previously asserted that REST would follow an absolute URL to an
// unrelated host, sending that host the user's token. githubrest refuses any
// absolute URL outside the client's credentialed hosts, and the only absolute
// URLs the CLI passes to REST are Link header pages and *_url fields from API
// responses, all of which are on the API host.
func TestRESTRefusesForeignAbsoluteURL(t *testing.T) {
	http := &httpmock.Registry{}
	client := newTestClient(http)

	err := client.REST("example.com", "GET", "https://another.net/user/repos", nil, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), `refusing to send credentials to "another.net"`)
	assert.Empty(t, http.Requests)
}

func TestRESTError(t *testing.T) {
	fakehttp := &httpmock.Registry{}
	client := newTestClient(fakehttp)

	fakehttp.Register(httpmock.MatchAny, func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			Request:    req,
			StatusCode: 422,
			Body:       io.NopCloser(bytes.NewBufferString(`{"message": "OH NO"}`)),
			Header: map[string][]string{
				"Content-Type": {"application/json; charset=utf-8"},
			},
		}, nil
	})

	var httpErr *githubrest.ErrorResponse
	err := client.REST("github.com", "DELETE", "repos/branch", nil, nil)
	if err == nil || !errors.As(err, &httpErr) {
		t.Fatalf("got %v", err)
	}

	if httpErr.StatusCode != 422 {
		t.Errorf("expected status code 422, got %d", httpErr.StatusCode)
	}
	if httpErr.Error() != "HTTP 422: OH NO (https://api.github.com/repos/branch)" {
		t.Errorf("got %q", httpErr.Error())
	}
}

func TestRESTWithNextError(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	client := newTestClient(reg)

	reg.Register(httpmock.MatchAny, func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			Request:    req,
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString(`{"message": "Not Found"}`)),
			Header: http.Header{
				"Content-Type":            {"application/json"},
				"X-Accepted-Oauth-Scopes": {"repo"},
				"X-Oauth-Scopes":          {"read:user"},
			},
		}, nil
	})

	_, err := client.RESTWithNext("github.com", http.MethodGet, "repos/owner/repo/items", nil, nil)

	var httpErr *githubrest.ErrorResponse
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.StatusCode)
	assert.Contains(t, err.Error(), "HTTP 404")
	assert.Equal(t, `This API operation needs the "repo" scope. To request it, run:  gh auth refresh -h github.com -s repo`, httpErr.ScopesSuggestion())
}

func TestRESTAndRESTWithNextErrorTypeParity(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	client := newTestClient(reg)

	responder := func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			Request:    req,
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString(`{"message": "Not Found"}`)),
			Header:     http.Header{"Content-Type": {"application/json"}},
		}, nil
	}
	reg.Register(httpmock.MatchAny, responder)
	reg.Register(httpmock.MatchAny, responder)

	restErr := client.REST("github.com", http.MethodGet, "repos/owner/repo/items", nil, nil)
	_, restWithNextErr := client.RESTWithNext("github.com", http.MethodGet, "repos/owner/repo/items", nil, nil)

	require.Error(t, restErr)
	require.Error(t, restWithNextErr)
	assert.IsType(t, restErr, restWithNextErr)
}

func TestRESTWithNext(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	client := newTestClient(reg)

	reg.Register(httpmock.MatchAny, func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			Request:    req,
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(`{"name": "item"}`)),
			Header: http.Header{
				"Content-Type": {"application/json"},
				"Link":         {`<https://api.github.com/repos/owner/repo/items?page=2>; rel="next", <https://api.github.com/repos/owner/repo/items?page=3>; rel="last"`},
			},
		}, nil
	})

	response := struct {
		Name string `json:"name"`
	}{}
	next, err := client.RESTWithNext("github.com", http.MethodGet, "repos/owner/repo/items", nil, &response)

	require.NoError(t, err)
	assert.Equal(t, "item", response.Name)
	assert.Equal(t, "https://api.github.com/repos/owner/repo/items?page=2", next)
}

func TestRESTWithNextNoContent(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	client := newTestClient(reg)

	reg.Register(
		httpmock.REST(http.MethodDelete, "repos/owner/repo/items/1"),
		httpmock.StatusStringResponse(http.StatusNoContent, "not JSON"),
	)

	next, err := client.RESTWithNext("github.com", http.MethodDelete, "repos/owner/repo/items/1", nil, nil)

	require.NoError(t, err)
	assert.Empty(t, next)
}

func TestHandleHTTPError_GraphQL502(t *testing.T) {
	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp := &http.Response{
		Request:    req,
		StatusCode: 502,
		Body:       io.NopCloser(bytes.NewBufferString(`{ "data": null, "errors": [{ "message": "Something went wrong" }] }`)),
		Header:     map[string][]string{"Content-Type": {"application/json"}},
	}
	err = HandleHTTPError(resp)
	if err == nil || err.Error() != "HTTP 502: Something went wrong (https://api.github.com/user)" {
		t.Errorf("got error: %v", err)
	}
}

func TestHTTPError_ScopesSuggestion(t *testing.T) {
	makeResponse := func(s int, u, haveScopes, needScopes string) *http.Response {
		req, err := http.NewRequest("GET", u, nil)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{
			Request:    req,
			StatusCode: s,
			Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
			Header: map[string][]string{
				"Content-Type":            {"application/json"},
				"X-Oauth-Scopes":          {haveScopes},
				"X-Accepted-Oauth-Scopes": {needScopes},
			},
		}
	}

	tests := []struct {
		name string
		resp *http.Response
		want string
	}{
		{
			name: "has necessary scopes",
			resp: makeResponse(404, "https://api.github.com/gists", "repo, gist, read:org", "gist"),
			want: ``,
		},
		{
			name: "normalizes scopes",
			resp: makeResponse(404, "https://api.github.com/orgs/ORG/discussions", "admin:org, write:discussion", "read:org, read:discussion"),
			want: ``,
		},
		{
			name: "no scopes on endpoint",
			resp: makeResponse(404, "https://api.github.com/user", "repo", ""),
			want: ``,
		},
		{
			name: "missing a scope",
			resp: makeResponse(404, "https://api.github.com/gists", "repo, read:org", "gist, delete_repo"),
			want: `This API operation needs the "gist" scope. To request it, run:  gh auth refresh -h github.com -s gist`,
		},
		{
			name: "server error",
			resp: makeResponse(500, "https://api.github.com/gists", "repo", "gist"),
			want: ``,
		},
		{
			name: "no scopes on token",
			resp: makeResponse(404, "https://api.github.com/gists", "", "gist, delete_repo"),
			want: ``,
		},
		{
			name: "http code is 422",
			resp: makeResponse(422, "https://api.github.com/gists", "", "gist"),
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpError := HandleHTTPError(tt.resp)
			if got := httpError.(*githubrest.ErrorResponse).ScopesSuggestion(); got != tt.want {
				t.Errorf("ErrorResponse.ScopesSuggestion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHTTPHeaders(t *testing.T) {
	var gotReq *http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReq = r
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	ios, _, _, stderr := iostreams.Test()
	httpClient, err := NewHTTPClient(HTTPClientOptions{
		AppVersion: "v1.2.3",
		Config:     tinyConfig{"github.com:oauth_token": "MYTOKEN"},
		Log:        ios.ErrOut,
	})
	require.NoError(t, err)

	// The request is addressed to github.com and redirected to the test server
	// at the last moment. This used to be expressed by passing the test server
	// URL as both hostname and path, which worked only because go-gh's REST
	// client passed an absolute path straight through. githubrest resolves
	// paths against the client's base URL and refuses absolute URLs on other
	// hosts, so the redirection moves to the transport, below every layer whose
	// headers this test is checking.
	httpClient.Transport = redirectTransport(httpClient.Transport, ts.URL)
	client := NewClientFromHTTP(httpClient)

	err = client.REST("github.com", "GET", "user/repos", nil, nil)
	require.NoError(t, err)

	wantHeader := map[string]string{
		"Accept":               "application/vnd.github.merge-info-preview+json, application/vnd.github.nebula-preview",
		"Authorization":        "token MYTOKEN",
		"Content-Type":         "application/json; charset=utf-8",
		"User-Agent":           "GitHub CLI v1.2.3",
		"X-GitHub-Api-Version": "2022-11-28",
	}
	for name, value := range wantHeader {
		assert.Equal(t, value, gotReq.Header.Get(name), name)
	}
	assert.Equal(t, "", stderr.String())
}

// redirectTransport sends every request to target, keeping its path, so a test
// can address a request to a real GitHub hostname and still have it served
// locally.
//
// The original host is preserved on Request.Host so that the transports below
// this one, which is every transport whose headers these tests care about,
// still see the request as addressed to GitHub.
func redirectTransport(rt http.RoundTripper, target string) http.RoundTripper {
	parsed, err := url.Parse(target)
	if err != nil {
		panic(err)
	}
	return funcTripper{roundTrip: func(req *http.Request) (*http.Response, error) {
		req.Host = req.URL.Host
		req.URL.Scheme = parsed.Scheme
		req.URL.Host = parsed.Host
		return rt.RoundTrip(req)
	}}
}
