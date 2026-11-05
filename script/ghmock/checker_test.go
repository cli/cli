package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckRequest(t *testing.T) {
	chk := testChecker(t)

	gql := func(query string) loggedRequest {
		return loggedRequest{Method: "POST", Host: apiHost, Path: "/graphql", GraphQL: &graphQLRequest{Query: query}}
	}
	rest := func(method, path string) loggedRequest {
		return loggedRequest{Method: method, Host: apiHost, Path: path}
	}
	reply := func(status int, body string) *response {
		return &response{Status: status, Body: json.RawMessage(body)}
	}
	const issueQuery = `query { repository(owner: "o", name: "r") { issue(number: 1) { number title state assignees { login } } } }`
	const unionQuery = `query { repository(owner: "o", name: "r") { issueOrPullRequest(number: 1) { __typename ... on Issue { title } ... on PullRequest { number } } } }`

	tests := []struct {
		name string
		req  loggedRequest
		resp *response
		want checkResult
	}{
		{
			name: "a GraphQL response that matches the query",
			req:  gql(`query { viewer { login name } }`),
			resp: reply(200, `{"data":{"viewer":{"login":"monalisa","name":null}}}`),
		},
		{
			name: "a query the schema rejects",
			req:  gql(`query { viewer { login nope } }`),
			resp: reply(200, `{"data":{"viewer":{"login":"monalisa"}}}`),
			want: checkResult{queryProblems: []string{`line 1, column 24: Cannot query field "nope" on type "User". Did you mean "name"?`}},
		},
		{
			name: "a query that can't be parsed",
			req:  gql(`query {`),
			resp: reply(200, `{"data":{}}`),
			want: checkResult{queryProblems: []string{"line 1, column 8: Expected Name, found <EOF>"}, note: "gh's query couldn't be read"},
		},
		{
			name: "fragments can ask for one field name with different types",
			req:  gql(`query { repository(owner: "o", name: "r") { issueOrPullRequest(number: 1) { ... on Issue { state } ... on PullRequest { state } } } }`),
			resp: reply(200, `{"data":{"repository":{"issueOrPullRequest":{"state":"MERGED"}}}}`),
		},
		{
			name: "missing fields, nulls, wrong types and enum values",
			req:  gql(issueQuery),
			resp: reply(200, `{"data":{"repository":{"issue":{"number":"1","title":null,"state":"open"}}}}`),
			want: checkResult{responseProblems: []string{
				`data.repository.issue.number: should be a whole number (Int), but is a string`,
				`data.repository.issue.title: is null, but GitHub's schema says it can't be (String!)`,
				`data.repository.issue.state: "open" isn't a value of IssueState; it can be CLOSED, OPEN`,
				`data.repository.issue.assignees: missing, but gh asked for it`,
			}},
		},
		{
			name: "lists and their items",
			req:  gql(issueQuery),
			resp: reply(200, `{"data":{"repository":{"issue":{"number":3000000000,"title":"Bug","state":"OPEN","assignees":[{"login":"a"},null,{"login":1}]}}}}`),
			want: checkResult{responseProblems: []string{
				`data.repository.issue.number: 3000000000 isn't a 32-bit whole number (Int)`,
				`data.repository.issue.assignees.1: is null, but GitHub's schema says it can't be (User!)`,
				`data.repository.issue.assignees.2.login: should be a string (String), but is a number`,
			}},
		},
		{
			name: "a list that isn't a list",
			req:  gql(issueQuery),
			resp: reply(200, `{"data":{"repository":{"issue":{"number":1,"title":"Bug","state":"OPEN","assignees":{}}}}}`),
			want: checkResult{responseProblems: []string{`data.repository.issue.assignees: should be a list ([User!]!), but is an object`}},
		},
		{
			name: "aliases name the response keys",
			req:  gql(`query { viewer { handle: login } }`),
			resp: reply(200, `{"data":{"viewer":{"login":"monalisa"}}}`),
			want: checkResult{responseProblems: []string{`data.viewer.handle: missing, but gh asked for it`}},
		},
		{
			name: "fragment fields apply only to the type in __typename",
			req:  gql(unionQuery),
			resp: reply(200, `{"data":{"repository":{"issueOrPullRequest":{"__typename":"Issue","title":"Bug"}}}}`),
		},
		{
			name: "fragment fields of the type in __typename are required",
			req:  gql(unionQuery),
			resp: reply(200, `{"data":{"repository":{"issueOrPullRequest":{"__typename":"PullRequest","title":"Bug"}}}}`),
			want: checkResult{responseProblems: []string{`data.repository.issueOrPullRequest.number: missing, but gh asked for it`}},
		},
		{
			name: "a __typename the field can't have",
			req:  gql(unionQuery),
			resp: reply(200, `{"data":{"repository":{"issueOrPullRequest":{"__typename":"Commit"}}}}`),
			want: checkResult{responseProblems: []string{`data.repository.issueOrPullRequest.__typename: "Commit" isn't a type IssueOrPullRequest can be; it can be Issue, PullRequest`}},
		},
		{
			name: "without __typename, fragment fields aren't required",
			req:  gql(`query { repository(owner: "o", name: "r") { issueOrPullRequest(number: 1) { ... on Issue { title } } } }`),
			resp: reply(200, `{"data":{"repository":{"issueOrPullRequest":{}}}}`),
		},
		{
			name: "a __typename that doesn't match the object",
			req:  gql(`query { viewer { __typename } }`),
			resp: reply(200, `{"data":{"viewer":{"__typename":"Bot"}}}`),
			want: checkResult{responseProblems: []string{`data.viewer.__typename: is "Bot", but this object's type is User`}},
		},
		{
			name: "fields under @include aren't required",
			req:  gql(`query ($x: Boolean!) { viewer { login name @include(if: $x) } }`),
			resp: reply(200, `{"data":{"viewer":{"login":"monalisa"}}}`),
		},
		{
			name: "a mutation",
			req:  gql(`mutation { closeIssue(input: {issueId: "1"}) { issue { title } } }`),
			resp: reply(200, `{"data":{"closeIssue":{"issue":{"title":"Bug"}}}}`),
		},
		{
			name: "errors without data",
			req:  gql(`query { viewer { login } }`),
			resp: reply(200, `{"errors":[{"message":"Not Found"}]}`),
		},
		{
			name: "errors that aren't a list",
			req:  gql(`query { viewer { login } }`),
			resp: reply(200, `{"errors":{"message":"Not Found"}}`),
			want: checkResult{responseProblems: []string{"errors: should be a list, but is an object"}},
		},
		{
			name: "neither data nor errors",
			req:  gql(`query { viewer { login } }`),
			resp: reply(200, `{}`),
			want: checkResult{responseProblems: []string{"body: has neither data nor errors"}},
		},
		{
			name: "an empty GraphQL body",
			req:  gql(`query { viewer { login } }`),
			resp: reply(200, ``),
			want: checkResult{responseProblems: []string{"body: missing, but a GraphQL response needs data or errors"}},
		},
		{
			name: "a GraphQL error status isn't checked",
			req:  gql(`query { viewer { login } }`),
			resp: reply(502, `{"message":"bad gateway"}`),
			want: checkResult{note: "status 502 has no GraphQL result to check"},
		},
		{
			name: "an off-schema response isn't checked, but the query is",
			req:  gql(`query { viewer { nope } }`),
			resp: &response{Body: json.RawMessage(`{}`), OffSchema: true},
			want: checkResult{queryProblems: []string{`line 1, column 18: Cannot query field "nope" on type "User". Did you mean "name"?`}},
		},
		{
			name: "without a response, only the query is checked",
			req:  gql(`query { viewer { nope } }`),
			want: checkResult{queryProblems: []string{`line 1, column 18: Cannot query field "nope" on type "User". Did you mean "name"?`}},
		},
		{
			name: "a REST response that matches the description",
			req:  rest("GET", "/repos/o/r/issues/1"),
			resp: reply(200, `{"id":1,"number":1,"title":"Bug","body":null,"state":"open","user":null,"labels":["bug",{"name":"x","color":null}],"created_at":"2026-01-01T00:00:00Z"}`),
		},
		{
			name: "REST problems use the same paths as set --at",
			req:  rest("GET", "/repos/o/r/issues/1"),
			resp: reply(200, `{"id":"1","number":1,"title":"Bug","state":"opened","user":{"login":"a"},"labels":[]}`),
			want: checkResult{responseProblems: []string{
				"id: value must be an integer",
				`state: value is not one of the allowed values ["open","closed"]`,
				"user.id: missing, but GitHub's description requires it",
				"created_at: missing, but GitHub's description requires it",
			}},
		},
		{
			name: "properties from allOf are required",
			req:  rest("GET", "/repos/o/r"),
			resp: reply(200, `{"id":1,"name":"r","owner":{"login":"o","id":2}}`),
			want: checkResult{responseProblems: []string{"subscribers_count: missing, but GitHub's description requires it"}},
		},
		{
			name: "a literal path segment beats a parameter",
			req:  rest("GET", "/repos/o/r/issues/comments"),
			resp: reply(200, `[{"login":"a"}]`),
			want: checkResult{responseProblems: []string{"0.id: missing, but GitHub's description requires it"}},
		},
		{
			name: "a path parameter can take the rest of the path",
			req:  rest("GET", "/repos/o/r/contents/docs/README.md"),
			resp: reply(200, `{"type":"file","path":"docs/README.md","content":""}`),
		},
		{
			name: "a documented error",
			req:  rest("GET", "/repos/o/r"),
			resp: reply(404, `{"message":"Not Found"}`),
		},
		{
			name: "a documented status without a body",
			req:  rest("GET", "/repos/o/r/issues/1"),
			resp: reply(304, ``),
		},
		{
			name: "a body where GitHub documents none",
			req:  rest("DELETE", "/repos/o/r"),
			resp: reply(204, `{}`),
			want: checkResult{note: "GitHub documents no JSON body for status 204"},
		},
		{
			name: "no body where GitHub documents one",
			req:  rest("GET", "/repos/o/r"),
			resp: reply(200, ``),
			want: checkResult{responseProblems: []string{"body: missing, but GitHub documents a JSON body for status 200"}},
		},
		{
			name: "a REST body that isn't JSON",
			req:  rest("GET", "/repos/o/r"),
			resp: reply(200, `nope`),
			want: checkResult{responseProblems: []string{"body: not valid JSON: invalid character 'o' in literal null (expecting 'u')"}},
		},
		{
			name: "a status GitHub doesn't document",
			req:  rest("GET", "/repos/o/r"),
			resp: reply(418, `{}`),
			want: checkResult{note: "GitHub doesn't document status 418 for GET /repos/{owner}/{repo}"},
		},
		{
			name: "a method GitHub doesn't document",
			req:  rest("PATCH", "/repos/o/r"),
			resp: reply(200, `{}`),
			want: checkResult{note: "GitHub documents no PATCH for /repos/{owner}/{repo}"},
		},
		{
			name: "a path GitHub doesn't document",
			req:  rest("GET", "/nope"),
			resp: reply(200, `{}`),
			want: checkResult{note: "not in GitHub's REST description"},
		},
		{
			name: "another host",
			req:  loggedRequest{Method: "POST", Host: "uploads." + mockHost, Path: "/upload"},
			resp: reply(201, `{}`),
			want: checkResult{note: "not a REST API host"},
		},
		{
			name: "an off-schema REST response isn't checked",
			req:  rest("GET", "/repos/o/r"),
			resp: &response{Body: json.RawMessage(`{}`), OffSchema: true},
		},
		{
			name: "a REST request without a response isn't checked",
			req:  rest("GET", "/repos/o/r"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, chk.checkRequest(tt.req, tt.resp))
		})
	}
}

func TestRESTMatch(t *testing.T) {
	idx := &restIndex{}
	for _, tmpl := range []string{
		"/",
		"/repos/{owner}/{repo}",
		"/repos/{owner}/{repo}/issues/{issue_number}",
		"/repos/{owner}/{repo}/issues/comments",
		"/repos/{owner}/{repo}/contents/{path}",
		"/repos/{owner}/{repo}/compare/{basehead}",
		"/files/{name}.{ext}",
	} {
		idx.routes = append(idx.routes, restRoute{template: tmpl, segments: parseTemplate(tmpl)})
	}

	tests := []struct {
		path string
		want string
	}{
		{path: "/", want: "/"},
		{path: "/repos/o/r", want: "/repos/{owner}/{repo}"},
		{path: "/repos/o/r/", want: "/repos/{owner}/{repo}"},
		{path: "/repos/o/r/issues/1", want: "/repos/{owner}/{repo}/issues/{issue_number}"},
		{path: "/repos/o/r/issues/comments", want: "/repos/{owner}/{repo}/issues/comments"},
		{path: "/repos/o/r/contents/docs/README.md", want: "/repos/{owner}/{repo}/contents/{path}"},
		{path: "/repos/o/r/compare/main...feature/x", want: "/repos/{owner}/{repo}/compare/{basehead}"},
		{path: "/files/report.txt", want: "/files/{name}.{ext}"},
		{path: "/files/report"},
		{path: "/repos/o"},
		{path: "/repos/o/r/issues"},
		{path: "/repos/o/r/issues/1/extra"},
		{path: "/repos//r"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := ""
			if route := idx.match(tt.path); route != nil {
				got = route.template
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
