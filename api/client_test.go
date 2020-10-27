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

func Test_CheckScopes(t *testing.T) {
	tests := []struct {
		name           string
		wantScope      string
		responseApp    string
		responseScopes string
		responseError  error
		expectCallback bool
	}{
		{
			name:           "missing read:org",
			wantScope:      "read:org",
			responseApp:    "APPID",
			responseScopes: "repo, gist",
			expectCallback: true,
		},
		{
			name:           "has read:org",
			wantScope:      "read:org",
			responseApp:    "APPID",
			responseScopes: "repo, read:org, gist",
			expectCallback: false,
		},
		{
			name:           "has admin:org",
			wantScope:      "read:org",
			responseApp:    "APPID",
			responseScopes: "repo, admin:org, gist",
			expectCallback: false,
		},
		{
			name:           "no scopes in response",
			wantScope:      "read:org",
			responseApp:    "",
			responseScopes: "",
			expectCallback: false,
		},
		{
			name:           "errored response",
			wantScope:      "read:org",
			responseApp:    "",
			responseScopes: "",
			responseError:  errors.New("Network Failed"),
			expectCallback: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := &httpmock.Registry{}
			tr.Register(httpmock.MatchAny, func(*http.Request) (*http.Response, error) {
				if tt.responseError != nil {
					return nil, tt.responseError
				}
				if tt.responseScopes == "" {
					return &http.Response{StatusCode: 200}, nil
				}
				return &http.Response{
					StatusCode: 200,
					Header: http.Header{
						"X-Oauth-Client-Id": []string{tt.responseApp},
						"X-Oauth-Scopes":    []string{tt.responseScopes},
					},
				}, nil
			})

			callbackInvoked := false
			var gotAppID string
			fn := CheckScopes(tt.wantScope, func(appID string) error {
				callbackInvoked = true
				gotAppID = appID
				return nil
			})

			rt := fn(tr)
			req, err := http.NewRequest("GET", "https://api.github.com/hello", nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			issuedScopesWarning = false
			_, err = rt.RoundTrip(req)
			if err != nil && !errors.Is(err, tt.responseError) {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.expectCallback != callbackInvoked {
				t.Fatalf("expected CheckScopes callback: %v", tt.expectCallback)
			}
			if tt.expectCallback && gotAppID != tt.responseApp {
				t.Errorf("unexpected app ID: %q", gotAppID)
			}
		})
	}
}
