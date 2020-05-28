package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func httpRequest(client *http.Client, method string, p string, params interface{}, headers []string) (*http.Response, error) {
	// TODO: GHE support
	url := "https://api.github.com/" + p
	var body io.Reader
	var bodyIsJSON bool
	isGraphQL := p == "graphql"

	switch pp := params.(type) {
	case map[string]interface{}:
		if strings.EqualFold(method, "GET") {
			url = addQuery(url, pp)
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

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	for _, h := range headers {
		idx := strings.IndexRune(h, ':')
		if idx == -1 {
			return nil, fmt.Errorf("header %q requires a value separated by ':'", h)
		}
		req.Header.Add(h[0:idx], strings.TrimSpace(h[idx+1:]))
	}
	if bodyIsJSON && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}

	return client.Do(req)
}

func groupGraphQLVariables(params map[string]interface{}) map[string]interface{} {
	topLevel := make(map[string]interface{})
	variables := make(map[string]interface{})

	for key, val := range params {
		switch key {
		case "query":
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
	for key, value := range params {
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
		default:
			panic(fmt.Sprintf("unknown type %v", v))
		}
	}

	sep := "?"
	if strings.ContainsRune(path, '?') {
		sep = "&"
	}
	return path + sep + query.Encode()
}
