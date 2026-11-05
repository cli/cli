package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
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
		Query     string          `json:"query"`
		Variables json.RawMessage `json:"variables"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Query == "" {
		return nil
	}
	vars := payload.Variables
	if trimmed := bytes.TrimSpace(vars); len(trimmed) == 0 || string(trimmed) == "null" {
		vars = nil
	}
	kind, name := operationName(payload.Query)
	return &graphQLRequest{Kind: kind, Operation: name, Query: payload.Query, Variables: vars}
}

// operationName returns the kind and name of the first operation in a GraphQL
// document. It skips fragment definitions, comments and strings, and treats a
// bare selection set as an anonymous query.
func operationName(doc string) (kind, name string) {
	depth := 0
	fragmentPending := false
	for i := 0; i < len(doc); {
		c := doc[i]
		switch {
		case c == '#':
			for i < len(doc) && doc[i] != '\n' {
				i++
			}
			continue
		case c == '"':
			i = skipString(doc, i)
			continue
		case c == '{':
			if depth == 0 {
				if !fragmentPending {
					return "query", ""
				}
				fragmentPending = false
			}
			depth++
		case c == '}':
			depth--
		case isNameStart(c):
			j := i
			for j < len(doc) && isNameChar(doc[j]) {
				j++
			}
			if depth == 0 {
				switch word := doc[i:j]; word {
				case "query", "mutation", "subscription":
					k := skipIgnored(doc, j)
					m := k
					if m < len(doc) && isNameStart(doc[m]) {
						for m < len(doc) && isNameChar(doc[m]) {
							m++
						}
					}
					return word, doc[k:m]
				case "fragment":
					fragmentPending = true
				}
			}
			i = j
			continue
		}
		i++
	}
	return "query", ""
}

// skipString returns the index just past the string starting at doc[i],
// handling escapes and """block strings""".
func skipString(doc string, i int) int {
	if strings.HasPrefix(doc[i:], `"""`) {
		if end := strings.Index(doc[i+3:], `"""`); end >= 0 {
			return i + 3 + end + 3
		}
		return len(doc)
	}
	for j := i + 1; j < len(doc); j++ {
		switch doc[j] {
		case '\\':
			j++
		case '"':
			return j + 1
		}
	}
	return len(doc)
}

// skipIgnored skips whitespace, commas and comments.
func skipIgnored(doc string, i int) int {
	for i < len(doc) {
		switch doc[i] {
		case ' ', '\t', '\n', '\r', ',':
			i++
		case '#':
			for i < len(doc) && doc[i] != '\n' {
				i++
			}
		default:
			return i
		}
	}
	return i
}

func isNameStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isNameChar(c byte) bool {
	return isNameStart(c) || (c >= '0' && c <= '9')
}
