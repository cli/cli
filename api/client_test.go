package api

import (
	"bytes"
	"errors"
	"io/ioutil"
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
	err := client.GraphQL("QUERY", vars, &response)
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

	err := client.GraphQL("", nil, &response)
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
	err := client.REST("DELETE", "applications/CLIENTID/grant", r, nil)
	eq(t, err, nil)
}

func TestRESTError(t *testing.T) {
	http := &httpmock.Registry{}
	client := NewClient(ReplaceTripper(http))

	http.StubResponse(422, bytes.NewBufferString(`{"message": "OH NO"}`))

	var httpErr HTTPError
	err := client.REST("DELETE", "repos/branch", nil, nil)
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
