package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/githubrest"
)

// httpRequest issues the request the user described.
//
// The path is passed to the client verbatim, exactly as it arrived from the
// user: gh api is the one command whose whole job is to send what it was asked
// to send, so it is deliberately not routed through safeurl and not escaped.
// The client resolves a relative path against its API base URL, which is how
// hostname stops being a per-request argument; only the GraphQL endpoint still
// needs it, because it does not sit under the REST prefix.
func httpRequest(ctx context.Context, client *githubrest.Client, hostname string, method string, p string, params interface{}, headers []string) (*githubrest.Response, error) {
	isGraphQL := p == "graphql"
	requestURL := p
	if isGraphQL && !strings.Contains(p, "://") {
		requestURL = ghinstance.GraphQLEndpoint(hostname)
	}

	var body io.Reader
	var bodyIsJSON bool

	switch pp := params.(type) {
	case map[string]interface{}:
		if strings.EqualFold(method, "GET") {
			requestURL = addQuery(requestURL, pp)
		} else {
			if isGraphQL {
				pp = groupGraphQLVariables(pp)
			}
			b, err := json.Marshal(pp)
			if err != nil {
				return nil, fmt.Errorf("error serializing parameters: %w", err)
			}
			body = bytes.NewBuffer(b)
			bodyIsJSON = true
		}
	case io.Reader:
		body = pp
	case nil:
		body = nil
	default:
		return nil, fmt.Errorf("unrecognized parameters type: %v", params)
	}

	req, err := client.NewRequest(ctx, strings.ToUpper(method), requestURL, body)
	if err != nil {
		return nil, err
	}

	for _, h := range headers {
		idx := strings.IndexRune(h, ':')
		if idx == -1 {
			return nil, fmt.Errorf("header %q requires a value separated by ':'", h)
		}
		name, value := h[0:idx], strings.TrimSpace(h[idx+1:])
		if strings.EqualFold(name, "Content-Length") {
			length, err := strconv.ParseInt(value, 10, 0)
			if err != nil {
				return nil, err
			}
			req.ContentLength = length
		} else {
			req.Header.Add(name, value)
		}
	}
	if bodyIsJSON && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	// Set rather than left to the transport's default of application/vnd.github+json,
	// because gh api has always accepted anything and users rely on that for
	// diffs, patches and raw file content.
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "*/*")
	}

	return client.Send(req)
}

func groupGraphQLVariables(params map[string]interface{}) map[string]interface{} {
	topLevel := make(map[string]interface{})
	variables := make(map[string]interface{})

	for key, val := range params {
		switch key {
		case "query", "operationName":
			topLevel[key] = val
		default:
			variables[key] = val
		}
	}

	if len(variables) > 0 {
		topLevel["variables"] = variables
	}
	return topLevel
}

func addQuery(path string, params map[string]interface{}) string {
	if len(params) == 0 {
		return path
	}

	query := url.Values{}
	if err := addQueryParam(query, "", params); err != nil {
		panic(err)
	}

	sep := "?"
	if strings.ContainsRune(path, '?') {
		sep = "&"
	}
	return path + sep + query.Encode()
}

func addQueryParam(query url.Values, key string, value interface{}) error {
	switch v := value.(type) {
	case string:
		query.Add(key, v)
	case []byte:
		query.Add(key, string(v))
	case nil:
		query.Add(key, "")
	case int:
		query.Add(key, fmt.Sprintf("%d", v))
	case bool:
		query.Add(key, fmt.Sprintf("%v", v))
	case map[string]interface{}:
		for subkey, value := range v {
			// support for nested subkeys can be added here if that is ever necessary
			if err := addQueryParam(query, subkey, value); err != nil {
				return err
			}
		}
	case []interface{}:
		for _, entry := range v {
			if err := addQueryParam(query, key+"[]", entry); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unknown type %v", v)
	}
	return nil
}
