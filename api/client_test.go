package api

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
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
	assert.NoError(t, err)
	err = client.REST("example.com", "GET", "https://another.net/user/repos", nil, nil)
	assert.NoError(t, err)

	assert.Equal(t, "example.com", http.Requests[0].URL.Hostname())
	assert.Equal(t, "another.net", http.Requests[1].URL.Hostname())
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

	var httpErr HTTPError
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
			if got := httpError.(HTTPError).ScopesSuggestion(); got != tt.want {
				t.Errorf("HTTPError.ScopesSuggestion() = %v, want %v", got, tt.want)
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
		Config:     tinyConfig{ts.URL[7:] + ":oauth_token": "MYTOKEN"},
		Log:        ios.ErrOut,
	})
	assert.NoError(t, err)
	client := NewClientFromHTTP(httpClient)

	err = client.REST(ts.URL, "GET", ts.URL+"/user/repos", nil, nil)
	assert.NoError(t, err)

	wantHeader := map[string]string{
		"Accept":        "application/vnd.github.merge-info-preview+json, application/vnd.github.nebula-preview",
		"Authorization": "token MYTOKEN",
		"Content-Type":  "application/json; charset=utf-8",
		"User-Agent":    "GitHub CLI v1.2.3",
	}
	for name, value := range wantHeader {
		assert.Equal(t, value, gotReq.Header.Get(name), name)
	}
	assert.Equal(t, "", stderr.String())
}
