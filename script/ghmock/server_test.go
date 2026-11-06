package main

import (
	"crypto/tls"
	"crypto/x509"
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
	Seq            int
	Name           string
	Body           string
	Status         int
	Matched        bool
	Blocked        bool
	TLSFailed      bool
	Response       int
	Responses      int
	OffSchema      bool
	QueryProblems  []string
	SchemaProblems []string
	SchemaNote     string
}

func summarize(entries []loggedRequest) []logSummary {
	var out []logSummary
	for _, e := range entries {
		out = append(out, logSummary{
			Seq:            e.Seq,
			Name:           e.Name,
			Body:           e.Body,
			Status:         e.Status,
			Matched:        e.Matched,
			Blocked:        e.Blocked,
			TLSFailed:      e.TLSError != "",
			Response:       e.Response,
			Responses:      e.Responses,
			OffSchema:      e.OffSchema,
			QueryProblems:  e.QueryProblems,
			SchemaProblems: e.SchemaProblems,
			SchemaNote:     e.SchemaNote,
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

func graphQLBodyWithVariables(t *testing.T, query, variables string) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{"query": query, "variables": json.RawMessage(variables)})
	require.NoError(t, err)
	return string(data)
}

func TestMockServer(t *testing.T) {
	limitBody := strings.Repeat("x", maxRequestBody)
	rawBytes := []byte{0, 255, ' ', '\n'}
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
		name         string
		responses    responseSet
		checkSchemas bool
		// dotcom gives the server a certificate authority, as without
		// --localhost, and the client trusts it. untrusted does the same, but
		// the client trusts nothing.
		dotcom    bool
		untrusted bool
		calls     []call
		want      []reply
		wantLog   []logSummary
		wantKinds []string
		// wantSent, when set, is the response body each logged request
		// records as sent to gh.
		wantSent        []string
		wantRawSent     [][]byte
		wantRawRequests [][]byte
		exactBody       bool
	}{
		{
			name: "typename aliases trim fields from other concrete fragments",
			responses: responseSet{"graphql Kind": {{Body: json.RawMessage(
				`{"data":{"repository":{"issueOrPullRequest":{"kind":"Issue","title":"Bug","number":777}}}}`)}}},
			checkSchemas: true,
			calls: []call{{method: "POST", url: "http://api.github.localhost/graphql", body: graphQLBody(t,
				`query Kind { repository(owner:"o",name:"r") { issueOrPullRequest(number:1) { kind:__typename ...on Issue { title } ...on PullRequest { number } } } }`)}},
			want:     []reply{{status: 200, body: `{"data":{"repository":{"issueOrPullRequest":{"kind":"Issue","title":"Bug"}}}}`}},
			wantLog:  []logSummary{{Seq: 1, Name: "graphql Kind", Status: 200, Matched: true, Response: 1, Responses: 1}},
			wantSent: []string{`{"data":{"repository":{"issueOrPullRequest":{"kind":"Issue","title":"Bug"}}}}`},
		},
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
			name: "HTTP no-body responses do not record undelivered JSON",
			responses: responseSet{
				"GET /no-content":   {{Status: 204, Body: json.RawMessage(`{"ignored":true}`)}},
				"GET /not-modified": {{Status: 304, Body: json.RawMessage(`{"ignored":true}`)}},
				"HEAD /matched":     {{Body: json.RawMessage(`{"ignored":true}`)}},
			},
			calls: []call{
				{method: "GET", url: "http://api.github.localhost/no-content"},
				{method: "GET", url: "http://api.github.localhost/not-modified"},
				{method: "HEAD", url: "http://api.github.localhost/matched"},
				{method: "HEAD", url: "http://api.github.localhost/missing"},
			},
			want: []reply{{status: 204}, {status: 304}, {status: 200}, {status: 404}},
			wantLog: []logSummary{
				{Seq: 1, Name: "GET /no-content", Status: 204, Matched: true, Response: 1, Responses: 1},
				{Seq: 2, Name: "GET /not-modified", Status: 304, Matched: true, Response: 1, Responses: 1},
				{Seq: 3, Name: "HEAD /matched", Status: 200, Matched: true, Response: 1, Responses: 1},
				{Seq: 4, Name: "HEAD /missing", Status: 404},
			},
			wantSent: []string{"", "", "", ""},
		},
		{
			name:      "operation defaults are used while trimming conditional fields",
			responses: responseSet{"graphql Default": {{Body: json.RawMessage(`{"data":{"viewer":{"login":"Mona","name":"Mona"}}}`)}}},
			calls: []call{{method: "POST", url: "http://api.github.localhost/graphql", body: graphQLBody(t,
				`query Default($show: Boolean = true, $hide: Boolean = false) { viewer { login @include(if:$show) name @skip(if:$hide) } }`)}},
			want:     []reply{{status: 200, body: `{"data":{"viewer":{"login":"Mona","name":"Mona"}}}`}},
			wantLog:  []logSummary{{Seq: 1, Name: "graphql Default", Status: 200, Matched: true, Response: 1, Responses: 1}},
			wantSent: []string{`{"data":{"viewer":{"login":"Mona","name":"Mona"}}}`},
		},
		{
			name:         "raw binary response is byte exact and recorded",
			responses:    responseSet{"GET /binary": {{RawBody: &rawBytes}}},
			dotcom:       true,
			checkSchemas: true,
			calls:        []call{{method: "GET", url: "https://api.github.com/binary"}},
			want:         []reply{{status: 200, body: string(rawBytes), headers: map[string]string{"Content-Type": "application/octet-stream"}}},
			wantLog:      []logSummary{{Seq: 1, Name: "GET /binary", Status: 200, Matched: true, Response: 1, Responses: 1}},
			wantSent:     []string{""},
			wantRawSent:  [][]byte{rawBytes},
			exactBody:    true,
		},
		{
			name:        "raw JSON is not compacted",
			responses:   responseSet{"GET /raw": {{RawBody: &[]byte{' ', '{', '}', '\n'}, Headers: map[string]string{"Content-Type": "application/json"}}}},
			calls:       []call{{method: "GET", url: "http://api.github.localhost/raw"}},
			want:        []reply{{status: 200, body: " {}\n"}},
			wantLog:     []logSummary{{Seq: 1, Name: "GET /raw", Status: 200, Matched: true, Response: 1, Responses: 1}},
			wantRawSent: [][]byte{[]byte(" {}\n")},
			exactBody:   true,
		},
		{
			name:        "HEAD does not send or record the configured body",
			responses:   responseSet{"HEAD /binary": {{RawBody: &rawBytes}}},
			calls:       []call{{method: "HEAD", url: "http://api.github.localhost/binary"}},
			want:        []reply{{status: 200, body: ""}},
			wantLog:     []logSummary{{Seq: 1, Name: "HEAD /binary", Status: 200, Matched: true, Response: 1, Responses: 1}},
			wantRawSent: [][]byte{nil},
		},
		{
			name:            "binary request bytes survive JSON recording",
			responses:       responseSet{"POST /binary": {{Body: json.RawMessage(`{}`)}}},
			calls:           []call{{method: "POST", url: "http://api.github.localhost/binary", body: string(rawBytes)}},
			want:            []reply{{status: 200, body: `{}`}},
			wantLog:         []logSummary{{Seq: 1, Name: "POST /binary", Status: 200, Matched: true, Response: 1, Responses: 1}},
			wantRawRequests: [][]byte{rawBytes},
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
			name:     "REST request without a response",
			calls:    []call{{method: "GET", url: "http://api.github.localhost/nope"}},
			want:     []reply{{status: 404, body: `{"message":"ghmock: no response for GET /nope"}`}},
			wantLog:  []logSummary{{Seq: 1, Name: "GET /nope", Status: 404}},
			wantSent: []string{`{"message":"ghmock: no response for GET /nope"}`},
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
			name:    "with --localhost, plain HTTP to another host is blocked",
			calls:   []call{{method: "GET", url: "http://example.com/"}},
			want:    []reply{{status: 403, body: "ghmock: only github.localhost is served"}},
			wantLog: []logSummary{{Seq: 1, Status: 403, Blocked: true}},
		},
		{
			name:    "with --localhost, HTTPS tunnels are blocked",
			calls:   []call{{method: "GET", url: "https://api.github.com/zen"}},
			want:    []reply{{err: true}},
			wantLog: []logSummary{{Seq: 1, Status: 403, Blocked: true}},
		},
		{
			name:      "HTTPS to api.github.com is answered as the API host",
			dotcom:    true,
			responses: responseSet{"GET /repos/octo/hello": {{Body: json.RawMessage(`{"name":"hello"}`)}}},
			calls:     []call{{method: "GET", url: "https://api.github.com/repos/octo/hello"}},
			want:      []reply{{status: 200, body: `{"name":"hello"}`}},
			wantLog:   []logSummary{{Seq: 1, Name: "GET /repos/octo/hello", Status: 200, Matched: true, Response: 1, Responses: 1}},
		},
		{
			name:   "GraphQL over HTTPS is named by operation",
			dotcom: true,
			responses: responseSet{"graphql Viewer": {{
				Body: json.RawMessage(`{"data":{"viewer":{"login":"monalisa"}}}`),
			}}},
			calls:   []call{{method: "POST", url: "https://api.github.com/graphql", body: graphQLBody(t, "query Viewer { viewer { login } }")}},
			want:    []reply{{status: 200, body: `{"data":{"viewer":{"login":"monalisa"}}}`}},
			wantLog: []logSummary{{Seq: 1, Name: "graphql Viewer", Status: 200, Matched: true, Response: 1, Responses: 1}},
		},
		{
			name:    "HTTPS to another host keeps the host in the name",
			dotcom:  true,
			calls:   []call{{method: "POST", url: "https://uploads.github.com/upload", body: "data"}},
			want:    []reply{{status: 404, body: `{"message":"ghmock: no response for POST uploads.github.com/upload"}`}},
			wantLog: []logSummary{{Seq: 1, Name: "POST uploads.github.com/upload", Body: "data", Status: 404}},
		},
		{
			name:      "a client that rejects ghmock's certificate is logged",
			untrusted: true,
			calls:     []call{{method: "GET", url: "https://api.github.com/zen"}},
			want:      []reply{{err: true}},
			wantLog:   []logSummary{{Seq: 1, TLSFailed: true}},
		},
		{
			name: "GraphQL responses are trimmed to the fields gh asked for",
			responses: responseSet{"graphql Viewer": {{
				Body: json.RawMessage(`{"data":{"viewer":{"name":"Mona","bio":"hi"}},"extensions":{"cost":1}}`),
			}}},
			calls:    []call{{method: "POST", url: "http://api.github.localhost/graphql", body: graphQLBody(t, "query Viewer { viewer { login name } }")}},
			want:     []reply{{status: 200, body: `{"data":{"viewer":{"name":"Mona"}},"extensions":{"cost":1}}`}},
			wantLog:  []logSummary{{Seq: 1, Name: "graphql Viewer", Status: 200, Matched: true, Response: 1, Responses: 1}},
			wantSent: []string{`{"data":{"viewer":{"name":"Mona"}},"extensions":{"cost":1}}`},
		},
		{
			name: "trimming follows lists, aliases, fragments for the object's type, and @include",
			responses: responseSet{"graphql Trim": {{
				Body: json.RawMessage(`{"data":{"repository":{"name":"r",
					"labels":{"nodes":[{"name":"bug","color":"f29513"}],"totalCount":1},
					"item":{"__typename":"PullRequest","number":2,"title":"Fix","state":"OPEN"}}}}`),
			}}},
			checkSchemas: true,
			calls: []call{{method: "POST", url: "http://api.github.localhost/graphql", body: graphQLBodyWithVariables(t,
				`query Trim($withColor: Boolean!) { repository(owner: "o", name: "r") {
					labels { nodes { name color @include(if: $withColor) } }
					item: issueOrPullRequest(number: 2) { __typename ... on Issue { title } ... on PullRequest { number } }
				} }`, `{"withColor": false}`)}},
			want:    []reply{{status: 200, body: `{"data":{"repository":{"item":{"__typename":"PullRequest","number":2},"labels":{"nodes":[{"name":"bug"}]}}}}`}},
			wantLog: []logSummary{{Seq: 1, Name: "graphql Trim", Status: 200, Matched: true, Response: 1, Responses: 1}},
		},
		{
			name: "off-schema responses and errors are sent as stored",
			responses: responseSet{
				"graphql Viewer": {{Body: json.RawMessage(`{"data":{"viewer":{"login":"monalisa","extra":true}}}`), OffSchema: true}},
				"graphql Nope":   {{Body: json.RawMessage(`{"data":null,"errors":[{"message":"Not Found"}]}`)}},
			},
			calls: []call{
				{method: "POST", url: "http://api.github.localhost/graphql", body: graphQLBody(t, "query Viewer { viewer { login } }")},
				{method: "POST", url: "http://api.github.localhost/graphql", body: graphQLBody(t, "query Nope { viewer { login } }")},
			},
			want: []reply{
				{status: 200, body: `{"data":{"viewer":{"login":"monalisa","extra":true}}}`},
				{status: 200, body: `{"data":null,"errors":[{"message":"Not Found"}]}`},
			},
			wantLog: []logSummary{
				{Seq: 1, Name: "graphql Viewer", Status: 200, Matched: true, Response: 1, Responses: 1},
				{Seq: 2, Name: "graphql Nope", Status: 200, Matched: true, Response: 1, Responses: 1},
			},
		},
		{
			name: "a response that doesn't match GitHub's schema is still served, and logged with its problems",
			responses: responseSet{"graphql IssueTitle": {{
				Body: json.RawMessage(`{"data":{"repository":{"issue":{"title":null}}}}`),
			}}},
			checkSchemas: true,
			calls: []call{{method: "POST", url: "http://api.github.localhost/graphql", body: graphQLBody(t,
				`query IssueTitle { repository(owner: "o", name: "r") { issue(number: 1) { title } } }`)}},
			want: []reply{{status: 200, body: `{"data":{"repository":{"issue":{"title":null}}}}`}},
			wantLog: []logSummary{{
				Seq: 1, Name: "graphql IssueTitle", Status: 200, Matched: true, Response: 1, Responses: 1,
				SchemaProblems: []string{"data.repository.issue.title: is null, but GitHub's schema says it can't be (String!)"},
			}},
		},
		{
			name: "an off-schema response isn't checked",
			responses: responseSet{"graphql IssueTitle": {{
				Body:      json.RawMessage(`{"data":{"repository":{"issue":{"title":null}}}}`),
				OffSchema: true,
			}}},
			checkSchemas: true,
			calls: []call{{method: "POST", url: "http://api.github.localhost/graphql", body: graphQLBody(t,
				`query IssueTitle { repository(owner: "o", name: "r") { issue(number: 1) { title } } }`)}},
			want:    []reply{{status: 200, body: `{"data":{"repository":{"issue":{"title":null}}}}`}},
			wantLog: []logSummary{{Seq: 1, Name: "graphql IssueTitle", Status: 200, Matched: true, Response: 1, Responses: 1, OffSchema: true}},
		},
		{
			name:         "gh's query is checked even without a response",
			checkSchemas: true,
			calls:        []call{{method: "POST", url: "http://api.github.localhost/graphql", body: graphQLBody(t, "query Nope { viewer { nope } }")}},
			want:         []reply{{status: 200, body: `{"errors":[{"message":"ghmock: no response for graphql Nope"}]}`}},
			wantLog: []logSummary{{
				Seq: 1, Name: "graphql Nope", Status: 200,
				QueryProblems: []string{`line 1, column 23: Cannot query field "nope" on type "User". Did you mean "name"?`},
			}},
		},
		{
			name:         "a REST request GitHub doesn't document isn't checked",
			responses:    responseSet{"GET /nope": {{Body: json.RawMessage(`{}`)}}},
			checkSchemas: true,
			calls:        []call{{method: "GET", url: "http://api.github.localhost/nope"}},
			want:         []reply{{status: 200, body: `{}`}},
			wantLog: []logSummary{{
				Seq: 1, Name: "GET /nope", Status: 200, Matched: true, Response: 1, Responses: 1,
				SchemaNote: "not in GitHub's REST description",
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &state{dir: t.TempDir()}
			if tt.responses != nil {
				require.NoError(t, st.saveResponses(tt.responses))
			}
			var chk *checker
			if tt.checkSchemas {
				chk = testChecker(t)
			}
			var ca *certAuthority
			if tt.dotcom || tt.untrusted {
				var err error
				ca, err = newCertAuthority()
				require.NoError(t, err)
			}
			srv, err := startServer(st, chk, ca, nil)
			require.NoError(t, err)
			proxy, err := url.Parse(srv.proxyURL())
			require.NoError(t, err)
			transport := &http.Transport{Proxy: http.ProxyURL(proxy)}
			if ca != nil {
				roots := x509.NewCertPool()
				if !tt.untrusted {
					roots.AppendCertsFromPEM(ca.certPEM)
				}
				transport.TLSClientConfig = &tls.Config{RootCAs: roots}
			}
			client := &http.Client{Transport: transport}

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
				if tt.exactBody {
					assert.Equal(t, []byte(tt.want[i].body), body)
				} else {
					assert.Equal(t, tt.want[i].body, strings.TrimSpace(string(body)))
				}
				for k, v := range tt.want[i].headers {
					assert.Equal(t, v, resp.Header.Get(k), "header %s", k)
				}
			}

			logged := srv.close()
			require.NoError(t, st.appendLog(logged))
			roundTrip, err := st.readLog()
			require.NoError(t, err)
			assert.Equal(t, logged, roundTrip)
			assert.Equal(t, tt.wantLog, summarize(logged))
			if tt.wantKinds != nil {
				require.Len(t, logged, len(tt.wantKinds))
				for i, kind := range tt.wantKinds {
					require.NotNil(t, logged[i].GraphQL)
					assert.Equal(t, kind, logged[i].GraphQL.Kind)
				}
			}
			for _, e := range logged {
				if e.Matched {
					assert.Equal(t, tt.responses[e.Name][e.Response-1].Headers, e.ResponseHeaders, "headers gh got for %s", e.Name)
				}
			}
			if tt.wantSent != nil {
				var sent []string
				for _, e := range logged {
					sent = append(sent, string(e.ResponseBody))
				}
				assert.Equal(t, tt.wantSent, sent)
			}
			if tt.wantRawSent != nil {
				var sent [][]byte
				for _, entry := range roundTrip {
					var data []byte
					if entry.ResponseRawBody != nil {
						data = *entry.ResponseRawBody
					}
					sent = append(sent, data)
				}
				assert.Equal(t, tt.wantRawSent, sent)
			}
			if tt.wantRawRequests != nil {
				var received [][]byte
				for _, entry := range roundTrip {
					var data []byte
					if entry.RawBody != nil {
						data = *entry.RawBody
					}
					received = append(received, data)
				}
				assert.Equal(t, tt.wantRawRequests, received)
			}
		})
	}
}
