package api

import (
	"bytes"
	"errors"
	"io/ioutil"
	"net/http"
	"reflect"
	"testing"

	"github.com/cli/cli/pkg/httpmock"
)

func eq(t *testing.T, got interface{}, expected interface{}) {
	t.Helper()
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("expected: %v, got: %v", expected, got)
	}
}

func TestGraphQL(t *testing.T) {
	http := &httpmock.Registry{}
	client := NewClient(
		ReplaceTripper(http),
		AddHeader("Authorization", "token OTOKEN"),
	)

	vars := map[string]interface{}{"name": "Mona"}
	response := struct {
		Viewer struct {
			Login string
		}
	}{}

	http.StubResponse(200, bytes.NewBufferString(`{"data":{"viewer":{"login":"hubot"}}}`))
	err := client.GraphQL("github.com", "QUERY", vars, &response)
	eq(t, err, nil)
	eq(t, response.Viewer.Login, "hubot")

	req := http.Requests[0]
	reqBody, _ := ioutil.ReadAll(req.Body)
	eq(t, string(reqBody), `{"query":"QUERY","variables":{"name":"Mona"}}`)
	eq(t, req.Header.Get("Authorization"), "token OTOKEN")
}

func TestGraphQLError(t *testing.T) {
	http := &httpmock.Registry{}
	client := NewClient(ReplaceTripper(http))

	response := struct{}{}
	http.StubResponse(200, bytes.NewBufferString(`
	{ "errors": [
		{"message":"OH NO"},
		{"message":"this is fine"}
	  ]
	}`))

	err := client.GraphQL("github.com", "", nil, &response)
	if err == nil || err.Error() != "GraphQL error: OH NO\nthis is fine" {
		t.Fatalf("got %q", err.Error())
	}
}

func TestRESTGetDelete(t *testing.T) {
	http := &httpmock.Registry{}

	client := NewClient(
		ReplaceTripper(http),
	)

	http.StubResponse(204, bytes.NewBuffer([]byte{}))

	r := bytes.NewReader([]byte(`{}`))
	err := client.REST("github.com", "DELETE", "applications/CLIENTID/grant", r, nil)
	eq(t, err, nil)
}

func TestRESTError(t *testing.T) {
	fakehttp := &httpmock.Registry{}
	client := NewClient(ReplaceTripper(fakehttp))

	fakehttp.Register(httpmock.MatchAny, func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			Request:    req,
			StatusCode: 422,
			Body:       ioutil.NopCloser(bytes.NewBufferString(`{"message": "OH NO"}`)),
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

func Test_HasMinimumScopes(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		wantErr string
	}{
		{
			name:    "no scopes",
			header:  "",
			wantErr: "",
		},
		{
			name:    "default scopes",
			header:  "repo, read:org",
			wantErr: "",
		},
		{
			name:    "admin:org satisfies read:org",
			header:  "repo, admin:org",
			wantErr: "",
		},
		{
			name:    "insufficient scope",
			header:  "repo",
			wantErr: "missing required scope 'read:org'",
		},
		{
			name:    "insufficient scopes",
			header:  "gist",
			wantErr: "missing required scopes 'repo', 'read:org'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakehttp := &httpmock.Registry{}
			client := NewClient(ReplaceTripper(fakehttp))

			fakehttp.Register(httpmock.REST("GET", ""), func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					Request:    req,
					StatusCode: 200,
					Body:       ioutil.NopCloser(&bytes.Buffer{}),
					Header: map[string][]string{
						"X-Oauth-Scopes": {tt.header},
					},
				}, nil
			})

			err := client.HasMinimumScopes("github.com")
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("error: %v", err)
				}
				return
			}
			if err.Error() != tt.wantErr {
				t.Errorf("want %q, got %q", tt.wantErr, err.Error())

			}
		})
	}

}
