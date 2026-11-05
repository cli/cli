package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestName(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		host      string
		path      string
		query     string
		variables string
		rawBody   string
		wantName  string
		wantKind  string
		wantOp    string
		wantVars  string
	}{
		{
			name:     "REST call to the API host",
			method:   "GET",
			host:     "api.github.localhost",
			path:     "/repos/octo/hello",
			wantName: "GET /repos/octo/hello",
		},
		{
			name:     "REST call to the API root",
			method:   "GET",
			host:     "api.github.localhost",
			wantName: "GET /",
		},
		{
			name:     "host names are case-insensitive",
			method:   "GET",
			host:     "API.github.localhost",
			path:     "/user",
			wantName: "GET /user",
		},
		{
			name:     "another mock host keeps the host in the name",
			method:   "POST",
			host:     "uploads.github.localhost",
			path:     "/upload",
			wantName: "POST uploads.github.localhost/upload",
		},
		{
			name:      "named query",
			method:    "POST",
			host:      apiHost,
			path:      "/graphql",
			query:     "query IssueByNumber($n: Int!) { repository { issue(number: $n) { title } } }",
			variables: `{"n": 1}`,
			wantName:  "graphql IssueByNumber",
			wantKind:  "query",
			wantOp:    "IssueByNumber",
			wantVars:  `{"n":1}`,
		},
		{
			name:     "mutation",
			method:   "POST",
			host:     apiHost,
			path:     "/graphql",
			query:    "mutation CommentCreate($input: AddCommentInput!) { addComment(input: $input) { clientMutationId } }",
			wantName: "graphql CommentCreate",
			wantKind: "mutation",
			wantOp:   "CommentCreate",
		},
		{
			name:     "fragments before the operation",
			method:   "POST",
			host:     apiHost,
			path:     "/graphql",
			query:    "fragment issue on Issue { title } query IssueByNumber { repository { issue(number: 1) { ...issue } } }",
			wantName: "graphql IssueByNumber",
			wantKind: "query",
			wantOp:   "IssueByNumber",
		},
		{
			name:     "comments and strings that look like operations",
			method:   "POST",
			host:     apiHost,
			path:     "/graphql",
			query:    "# query Commented {\nquery Real { search(query: \"} query Quoted {\") { issueCount } }",
			wantName: "graphql Real",
			wantKind: "query",
			wantOp:   "Real",
		},
		{
			name:     "shorthand query is anonymous",
			method:   "POST",
			host:     apiHost,
			path:     "/graphql",
			query:    "{ viewer { login } }",
			wantName: "graphql <anonymous>",
			wantKind: "query",
		},
		{
			name:     "unnamed query is anonymous",
			method:   "POST",
			host:     apiHost,
			path:     "/graphql",
			query:    "query { viewer { login } }",
			wantName: "graphql <anonymous>",
			wantKind: "query",
		},
		{
			name:      "null variables are dropped",
			method:    "POST",
			host:      apiHost,
			path:      "/graphql",
			query:     "query Viewer { viewer { login } }",
			variables: "null",
			wantName:  "graphql Viewer",
			wantKind:  "query",
			wantOp:    "Viewer",
		},
		{
			name:     "a body that isn't GraphQL is named like REST",
			method:   "POST",
			host:     apiHost,
			path:     "/graphql",
			rawBody:  "not json",
			wantName: "POST /graphql",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(tt.rawBody)
			if tt.query != "" {
				payload := map[string]any{"query": tt.query}
				if tt.variables != "" {
					payload["variables"] = json.RawMessage(tt.variables)
				}
				var err error
				body, err = json.Marshal(payload)
				require.NoError(t, err)
			}

			name, gql := requestName(tt.method, tt.host, tt.path, body)

			assert.Equal(t, tt.wantName, name)
			if tt.wantKind == "" {
				assert.Nil(t, gql)
				return
			}
			require.NotNil(t, gql)
			assert.Equal(t, tt.wantKind, gql.Kind)
			assert.Equal(t, tt.wantOp, gql.Operation)
			assert.Equal(t, tt.query, gql.Query)
			if tt.wantVars == "" {
				assert.Nil(t, gql.Variables)
			} else {
				assert.Equal(t, tt.wantVars, compactJSON(gql.Variables))
			}
		})
	}
}
