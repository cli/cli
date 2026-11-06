package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// logSummary is the part of a logged request that tests compare.
type logSummary struct {
	Seq       int
	Name      string
	Body      string
	Status    int
	Matched   bool
	Blocked   bool
	Response  int
	Responses int
}

func summarize(entries []loggedRequest) []logSummary {
	var out []logSummary
	for _, e := range entries {
		out = append(out, logSummary{
			Seq:       e.Seq,
			Name:      e.Name,
			Body:      e.Body,
			Status:    e.Status,
			Matched:   e.Matched,
			Blocked:   e.Blocked,
			Response:  e.Response,
			Responses: e.Responses,
		})
	}
	return out
}

func graphQLBody(t *testing.T, query string, operation ...string) string {
	t.Helper()
	payload := map[string]string{"query": query}
	if len(operation) > 0 {
		payload["operationName"] = operation[0]
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	return string(data)
}

func TestMockServer(t *testing.T) {
	limitBody := strings.Repeat("x", maxRequestBody)
	type call struct {
		method string
		url    string
		body   string
	}
	type reply struct {
		err     bool
		status  int
		body    string
		headers map[string]string
	}

	tests := []struct {
		name      string
		responses responseSet
		calls     []call
		want      []reply
		wantLog   []logSummary
		wantKinds []string
	}{
		{
			name:      "request at the body limit is recorded in full",
			responses: responseSet{"POST /limit": {{Body: json.RawMessage(`{}`)}}},
			calls:     []call{{method: "POST", url: "http://api.github.localhost/limit", body: limitBody}},
			want:      []reply{{status: 200, body: `{}`}},
			wantLog:   []logSummary{{Seq: 1, Name: "POST /limit", Body: limitBody, Status: 200, Matched: true, Response: 1, Responses: 1}},
		},
		{
			name:      "oversized request is rejected before fixture matching",
			responses: responseSet{"POST /limit": {{Body: json.RawMessage(`{}`)}}},
			calls:     []call{{method: "POST", url: "http://api.github.localhost/limit", body: limitBody + "x"}},
			want:      []reply{{status: 413, body: `{"message":"ghmock: request body exceeds 10 MiB"}`}},
			wantLog:   []logSummary{{Seq: 1, Status: 413}},
		},
		{
			name: "wire operationName selects a later mutation response",
			responses: responseSet{
				"graphql First":  {{Body: json.RawMessage(`{"data":{"viewer":{"login":"wrong"}}}`)}},
				"graphql Second": {{Body: json.RawMessage(`{"data":{"closeIssue":{"issue":{"title":"Bug"}}}}`)}},
			},
			calls: []call{{method: "POST", url: "http://api.github.localhost/graphql", body: graphQLBody(t,
				`query First { viewer { login } } mutation Second { closeIssue(input:{issueId:"1"}) { issue { title } } }`, "Second")}},
			want:      []reply{{status: 200, body: `{"data":{"closeIssue":{"issue":{"title":"Bug"}}}}`}},
			wantLog:   []logSummary{{Seq: 1, Name: "graphql Second", Status: 200, Matched: true, Response: 1, Responses: 1}},
			wantKinds: []string{"mutation"},
		},
		{
			name: "REST response with status, headers and body",
			responses: responseSet{"GET /repos/octo/hello": {{
				Status:  201,
				Headers: map[string]string{"X-Oauth-Scopes": "repo, read:org"},
				Body:    json.RawMessage(`{"name":"hello"}`),
			}}},
			calls: []call{{method: "GET", url: "http://api.github.localhost/repos/octo/hello?per_page=1"}},
			want: []reply{{status: 201, body: `{"name":"hello"}`, headers: map[string]string{
				"X-Oauth-Scopes": "repo, read:org",
				"Content-Type":   "application/json; charset=utf-8",
			}}},
			wantLog: []logSummary{{Seq: 1, Name: "GET /repos/octo/hello", Status: 201, Matched: true, Response: 1, Responses: 1}},
		},
		{
			name:    "REST request without a response",
			calls:   []call{{method: "GET", url: "http://api.github.localhost/nope"}},
			want:    []reply{{status: 404, body: `{"message":"ghmock: no response for GET /nope"}`}},
			wantLog: []logSummary{{Seq: 1, Name: "GET /nope", Status: 404}},
		},
		{
			name: "GraphQL response chosen by operation name",
			responses: responseSet{"graphql Viewer": {{
				Body: json.RawMessage(`{"data":{"viewer":{"login":"monalisa"}}}`),
			}}},
			calls:   []call{{method: "POST", url: "http://api.github.localhost/graphql", body: graphQLBody(t, "query Viewer { viewer { login } }")}},
			want:    []reply{{status: 200, body: `{"data":{"viewer":{"login":"monalisa"}}}`}},
			wantLog: []logSummary{{Seq: 1, Name: "graphql Viewer", Status: 200, Matched: true, Response: 1, Responses: 1}},
		},
		{
			name:    "GraphQL request without a response",
			calls:   []call{{method: "POST", url: "http://api.github.localhost/graphql", body: graphQLBody(t, "query Nope { viewer { login } }")}},
			want:    []reply{{status: 200, body: `{"errors":[{"message":"ghmock: no response for graphql Nope"}]}`}},
			wantLog: []logSummary{{Seq: 1, Name: "graphql Nope", Status: 200}},
		},
		{
			name: "responses are used in order and the last one repeats",
			responses: responseSet{"GET /flaky": {
				{Status: 502, Body: json.RawMessage(`{"message":"bad gateway"}`)},
				{Body: json.RawMessage(`{"ok":true}`)},
			}},
			calls: []call{
				{method: "GET", url: "http://api.github.localhost/flaky"},
				{method: "GET", url: "http://api.github.localhost/flaky"},
				{method: "GET", url: "http://api.github.localhost/flaky"},
			},
			want: []reply{
				{status: 502, body: `{"message":"bad gateway"}`},
				{status: 200, body: `{"ok":true}`},
				{status: 200, body: `{"ok":true}`},
			},
			wantLog: []logSummary{
				{Seq: 1, Name: "GET /flaky", Status: 502, Matched: true, Response: 1, Responses: 2},
				{Seq: 2, Name: "GET /flaky", Status: 200, Matched: true, Response: 2, Responses: 2},
				{Seq: 3, Name: "GET /flaky", Status: 200, Matched: true, Response: 2, Responses: 2},
			},
		},
		{
			name: "request bodies of writes are recorded",
			responses: responseSet{"POST /repos/octo/hello/issues": {{
				Status: 201,
				Body:   json.RawMessage(`{"number":1}`),
			}}},
			calls:   []call{{method: "POST", url: "http://api.github.localhost/repos/octo/hello/issues", body: `{"title":"Bug"}`}},
			want:    []reply{{status: 201, body: `{"number":1}`}},
			wantLog: []logSummary{{Seq: 1, Name: "POST /repos/octo/hello/issues", Body: `{"title":"Bug"}`, Status: 201, Matched: true, Response: 1, Responses: 1}},
		},
		{
			name:    "plain HTTP to another host is blocked",
			calls:   []call{{method: "GET", url: "http://example.com/"}},
			want:    []reply{{status: 403, body: "ghmock: only github.localhost is served"}},
			wantLog: []logSummary{{Seq: 1, Status: 403, Blocked: true}},
		},
		{
			name:    "HTTPS tunnels are blocked",
			calls:   []call{{method: "GET", url: "https://api.github.com/zen"}},
			want:    []reply{{err: true}},
			wantLog: []logSummary{{Seq: 1, Status: 403, Blocked: true}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &state{dir: t.TempDir()}
			if tt.responses != nil {
				require.NoError(t, st.saveResponses(tt.responses))
			}
			srv, err := startServer(st)
			require.NoError(t, err)
			proxy, err := url.Parse(srv.proxyURL())
			require.NoError(t, err)
			client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxy)}}

			for i, c := range tt.calls {
				req, err := http.NewRequest(c.method, c.url, strings.NewReader(c.body))
				require.NoError(t, err)
				resp, err := client.Do(req)
				if tt.want[i].err {
					require.Error(t, err)
					continue
				}
				require.NoError(t, err)
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				resp.Body.Close()

				assert.Equal(t, tt.want[i].status, resp.StatusCode)
				assert.Equal(t, tt.want[i].body, strings.TrimSpace(string(body)))
				for k, v := range tt.want[i].headers {
					assert.Equal(t, v, resp.Header.Get(k), "header %s", k)
				}
			}

			logged := srv.close()
			assert.Equal(t, tt.wantLog, summarize(logged))
			if tt.wantKinds != nil {
				require.Len(t, logged, len(tt.wantKinds))
				for i, kind := range tt.wantKinds {
					require.NotNil(t, logged[i].GraphQL)
					assert.Equal(t, kind, logged[i].GraphQL.Kind)
					assert.Equal(t, kind == "mutation", logged[i].isWrite())
				}
			}
		})
	}
}
