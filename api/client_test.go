package api

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"reflect"
	"testing"
)

func eq(t *testing.T, got interface{}, expected interface{}) {
	t.Helper()
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("expected: %v, got: %v", expected, got)
	}
}

func TestGraphQL(t *testing.T) {
	http := &FakeHTTP{}
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
	http := &FakeHTTP{}
	client := NewClient(ReplaceTripper(http))

	response := struct{}{}
	http.StubResponse(200, bytes.NewBufferString(`{"errors":[{"message":"OH NO"}]}`))
	err := client.GraphQL("", nil, &response)
	eq(t, err, fmt.Errorf("graphql error: 'OH NO'"))
}
