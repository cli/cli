package main

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSchemaArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    schemaOptions
		wantErr string
	}{
		{
			name: "GraphQL type",
			args: []string{"Issue"},
			want: schemaOptions{name: "Issue"},
		},
		{
			name: "REST request with flags",
			args: []string{"GET /repos/octo/hello", "--at", "owner.login", "--status", "404"},
			want: schemaOptions{name: "GET /repos/octo/hello", at: "owner.login", status: 404},
		},
		{
			name: "flags first, then name",
			args: []string{"--status", "404", "GET /repos/octo/hello"},
			want: schemaOptions{name: "GET /repos/octo/hello", status: 404},
		},
		{
			name:    "missing name",
			wantErr: `missing name, such as Issue, Issue.title or "GET /repos/octo/hello"`,
		},
		{
			name:    "GraphQL request name",
			args:    []string{"graphql IssueByNumber"},
			wantErr: "look up GraphQL types and fields by name, such as Issue or Issue.title",
		},
		{
			name:    "REST flags on a GraphQL name",
			args:    []string{"Issue", "--at", "title"},
			wantErr: "--at and --status are for REST requests; look up a GraphQL field as Type.field",
		},
		{
			name:    "status out of range",
			args:    []string{"GET /", "--status", "99"},
			wantErr: "--status must be between 100 and 599",
		},
		{
			name:    "lowercase method",
			args:    []string{"get /repos"},
			wantErr: `request name "get /repos" must start with an uppercase HTTP method or "graphql"`,
		},
		{
			name:    "unquoted REST name",
			args:    []string{"GET", "/repos/octo/hello"},
			wantErr: "unexpected arguments: /repos/octo/hello; quote the request name, such as 'GET /repos/octo/hello'",
		},
		{
			name:    "extra arguments",
			args:    []string{"Issue", "extra"},
			wantErr: "unexpected arguments: extra",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSchemaArgs(tt.args)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				var ue usageError
				assert.True(t, errors.As(err, &ue), "want a usage error")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCmdSchema(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr string
	}{
		{
			name: "object",
			args: []string{"Repository"},
			want: `Repository: an object
A repository contains the content for a project.
Implements: Node
Fields (a type ending in ! is never null):
  id: ID!
  name: String!
  issue: Issue
  issueOrPullRequest: IssueOrPullRequest
  labels: LabelConnection
`,
		},
		{
			name: "object with a deprecated field",
			args: []string{"Issue"},
			want: `Issue: an object
Implements: Node
Fields (a type ending in ! is never null):
  id: ID!
  number: Int!
  title: String!
  body: String
  state: IssueState!
  createdAt: DateTime!
  author: Actor
  assignees: [User!]!
  databaseId: Int (deprecated)
`,
		},
		{
			name: "interface",
			args: []string{"Actor"},
			want: `Actor: an interface
Implemented by: Bot, User
Fields (a type ending in ! is never null):
  login: String!
`,
		},
		{
			name: "union",
			args: []string{"IssueOrPullRequest"},
			want: "IssueOrPullRequest: a union\nCan be: Issue, PullRequest\n",
		},
		{
			name: "enum",
			args: []string{"IssueState"},
			want: `IssueState: an enum
The possible states of an issue.
Values:
  OPEN    An issue that is still open
  CLOSED  An issue that has been closed
`,
		},
		{
			name: "input object",
			args: []string{"CloseIssueInput"},
			want: `CloseIssueInput: an input object
Fields (a type ending in ! is required):
  issueId: ID!
  stateReason: IssueClosedStateReason
`,
		},
		{
			name: "scalar",
			args: []string{"DateTime"},
			want: "DateTime: a scalar\nAn ISO-8601 encoded UTC date string.\nSent as a JSON string.\n",
		},
		{
			name: "field with arguments",
			args: []string{"Query.repository"},
			want: `Query.repository: Repository
Can be null.
Arguments (a type ending in ! is required):
  owner: String!
  name: String!
See also: ghmock schema Repository
`,
		},
		{
			name: "field with a default argument",
			args: []string{"Repository.labels"},
			want: `Repository.labels: LabelConnection
Can be null.
Arguments (a type ending in ! is required):
  first: Int = 30
See also: ghmock schema LabelConnection
`,
		},
		{
			name: "list field",
			args: []string{"Issue.assignees"},
			want: "Issue.assignees: [User!]!\nNever null. Items are never null.\nSee also: ghmock schema User\n",
		},
		{
			name: "deprecated scalar field",
			args: []string{"Issue.databaseId"},
			want: "Issue.databaseId: Int\nCan be null.\nDeprecated: Use fullDatabaseId instead.\n",
		},
		{
			name: "input field",
			args: []string{"CloseIssueInput.issueId"},
			want: "CloseIssueInput.issueId: ID!\nRequired.\n",
		},
		{
			name:    "unknown type",
			args:    []string{"issue"},
			wantErr: "GitHub's GraphQL schema has no type named issue; did you mean Issue, IssueState, CloseIssueInput, CloseIssuePayload, IssueOrPullRequest?",
		},
		{
			name:    "unknown type with a typo",
			args:    []string{"Nope"},
			wantErr: "GitHub's GraphQL schema has no type named Nope; did you mean Node?",
		},
		{
			name:    "unknown type with nothing close",
			args:    []string{"Zzzzzzz"},
			wantErr: "GitHub's GraphQL schema has no type named Zzzzzzz",
		},
		{
			name:    "unknown field",
			args:    []string{"Issue.titel"},
			wantErr: "Issue has no field named titel; did you mean title?",
		},
		{
			name:    "field of an enum",
			args:    []string{"IssueState.OPEN"},
			wantErr: "IssueState is an enum, so it has no fields; ghmock schema IssueState describes it",
		},
		{
			name: "REST request",
			args: []string{"GET /repos/octo/hello/issues/1"},
			want: `GET /repos/{owner}/{repo}/issues/{issue_number}
Get an issue (issues/get)
Documented statuses: 200, 304, 404
Body for 200: object (issue)
Issues are a great way to keep track of tasks.
Required properties:
  created_at: string (date-time)
  id: integer
  labels: list of (string or object)
  number: integer
  state: string, one of "open", "closed"
  title: string
  user: object (nullable-simple-user) or null
Optional properties:
  body: string or null
`,
		},
		{
			name: "REST body properties from allOf",
			args: []string{"GET /repos/octo/hello"},
			want: `GET /repos/{owner}/{repo}
Get a repository (repos/get)
Documented statuses: 200, 404
Body for 200: object (full-repository)
Required properties:
  id: integer
  name: string
  owner: object (simple-user)
  subscribers_count: integer
`,
		},
		{
			name: "REST list body",
			args: []string{"GET /repos/octo/hello/issues/comments"},
			want: `GET /repos/{owner}/{repo}/issues/comments
List issue comments for a repository (issues/list-comments-for-repo)
Documented statuses: 200
Body for 200: list of object (simple-user)
Required properties of each item:
  id: integer
  login: string
`,
		},
		{
			name: "REST path parameter with slashes",
			args: []string{"GET /repos/octo/hello/contents/docs/README.md"},
			want: `GET /repos/{owner}/{repo}/contents/{path}
Get repository content (repos/get-content)
Documented statuses: 200
Body for 200: object (content-file)
Required properties:
  content: string
  path: string
  type: string, one of "file"
`,
		},
		{
			name: "REST value inside the body",
			args: []string{"GET /repos/octo/hello/issues/1", "--at", "user"},
			want: `GET /repos/{owner}/{repo}/issues/{issue_number}
Get an issue (issues/get)
Documented statuses: 200, 304, 404
user, in the body for 200: object (nullable-simple-user) or null
Required properties:
  id: integer
  login: string
`,
		},
		{
			name: "REST list item",
			args: []string{"GET /repos/octo/hello/issues/1", "--at", "labels.0"},
			want: `GET /repos/{owner}/{repo}/issues/{issue_number}
Get an issue (issues/get)
Documented statuses: 200, 304, 404
labels.0, in the body for 200: string or object
`,
		},
		{
			name: "REST error status",
			args: []string{"GET /repos/octo/hello/issues/1", "--status", "404"},
			want: `GET /repos/{owner}/{repo}/issues/{issue_number}
Get an issue (issues/get)
Documented statuses: 200, 304, 404
Body for 404: object (basic-error)
Basic Error
Optional properties:
  documentation_url: string
  message: string
`,
		},
		{
			name: "REST status without a body",
			args: []string{"DELETE /repos/octo/hello"},
			want: "DELETE /repos/{owner}/{repo}\nDelete a repository (repos/delete)\nDocumented statuses: 204\nBody for 204: none\n",
		},
		{
			name:    "REST status GitHub doesn't document",
			args:    []string{"GET /repos/octo/hello/issues/1", "--status", "500"},
			wantErr: "GitHub doesn't document status 500 for GET /repos/{owner}/{repo}/issues/{issue_number}; it documents 200, 304, 404",
		},
		{
			name:    "REST value in a status without a body",
			args:    []string{"GET /repos/octo/hello/issues/1", "--status", "304", "--at", "user"},
			wantErr: "status 304 has no JSON body",
		},
		{
			name:    "REST list without an index",
			args:    []string{"GET /repos/octo/hello/issues/1", "--at", "labels.name"},
			wantErr: "labels is a list; put an index first, such as labels.0.name",
		},
		{
			name:    "REST property that doesn't exist",
			args:    []string{"GET /repos/octo/hello/issues/1", "--at", "usr"},
			wantErr: "body has no property named usr; did you mean user?",
		},
		{
			name:    "REST value with nothing inside",
			args:    []string{"GET /repos/octo/hello/issues/1", "--at", "title.length"},
			wantErr: "title (string) has nothing inside it",
		},
		{
			name:    "REST method GitHub doesn't document",
			args:    []string{"PATCH /repos/octo/hello"},
			wantErr: "GitHub documents no PATCH for /repos/{owner}/{repo}; it documents DELETE, GET",
		},
		{
			name:    "REST path GitHub doesn't document",
			args:    []string{"GET /nope/1"},
			wantErr: "GitHub's REST description has no path matching /nope/1",
		},
		{
			name:    "another host",
			args:    []string{"POST uploads.github.localhost/upload"},
			wantErr: "uploads.github.localhost/upload isn't on the REST API host, so GitHub's REST description doesn't cover it",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useTestSchemas(t)
			var stdout bytes.Buffer
			err := cmdSchema(tt.args, &stdout, io.Discard)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, stdout.String())
		})
	}
}
