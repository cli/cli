package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

const (
	// mockHost is the GH_HOST value ghmock gives gh.
	mockHost = "github.localhost"
	// apiHost is where gh sends REST and GraphQL calls for mockHost.
	apiHost = "api." + mockHost
)

// graphQLRequest is the decoded body of a GraphQL call.
type graphQLRequest struct {
	Kind      string          `json:"kind"`
	Operation string          `json:"operation,omitempty"`
	Query     string          `json:"query"`
	Variables json.RawMessage `json:"variables,omitempty"`
}

func (g *graphQLRequest) operationLabel() string {
	if g.Operation == "" {
		return "<anonymous>"
	}
	return g.Operation
}

// isMockHost reports whether hostname is github.localhost or one of its subdomains.
func isMockHost(hostname string) bool {
	h := strings.ToLower(hostname)
	return h == mockHost || strings.HasSuffix(h, "."+mockHost)
}

// requestName returns the name responses are stored under: "graphql Operation"
// for GraphQL calls and "METHOD /path" for REST calls to the API host. Calls to
// any other mock host keep the host in the name, such as
// "POST uploads.github.localhost/upload".
func requestName(method, hostname, path string, body []byte) (string, *graphQLRequest) {
	hostname = strings.ToLower(hostname)
	if path == "" {
		path = "/"
	}
	if method == http.MethodPost && hostname == apiHost && path == "/graphql" {
		if gql := parseGraphQL(body); gql != nil {
			return "graphql " + gql.operationLabel(), gql
		}
	}
	if hostname == apiHost {
		return method + " " + path, nil
	}
	return method + " " + hostname + path, nil
}

func parseGraphQL(body []byte) *graphQLRequest {
	var payload struct {
		Query         string          `json:"query"`
		Variables     json.RawMessage `json:"variables"`
		OperationName string          `json:"operationName"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Query == "" {
		return nil
	}
	vars := payload.Variables
	if trimmed := bytes.TrimSpace(vars); len(trimmed) == 0 || string(trimmed) == "null" {
		vars = nil
	}
	kind, name := operationName(payload.Query, payload.OperationName)
	return &graphQLRequest{Kind: kind, Operation: name, Query: payload.Query, Variables: vars}
}

// operationName resolves the requested operation, or the document's sole
// operation when no name is supplied. Invalid selections retain the supplied
// name without claiming a kind or selecting a different operation.
func operationName(query, selected string) (kind, name string) {
	doc, err := parser.ParseQuery(&ast.Source{Input: query})
	if err != nil {
		return "", selected
	}
	op := doc.Operations.ForName(selected)
	if op == nil {
		return "", selected
	}
	return string(op.Operation), op.Name
}
