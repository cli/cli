package multigraphql

import (
	"reflect"
	"testing"
)

func TestMerge(t *testing.T) {
	type args struct {
		queries []PreparedQuery
	}
	tests := []struct {
		name       string
		args       args
		wantQuery  string
		wantValues map[string]interface{}
	}{
		{
			name: "A single query",
			args: args{
				queries: []PreparedQuery{
					PreparedQuery{
						variableValues: map[string]interface{}{
							"owner": "monalisa",
							"repo":  "hello-world",
						},
						Query: Query{
							query:     `repository(owner: $owner, name: $repo) { id }`,
							variables: map[string]string{"owner": "String!", "repo": "String"},
						},
					},
				},
			},
			wantQuery: `query(
	$owner: String!
	$repo: String
) {
	multi_000: repository(owner: $owner, name: $repo) { id }
}`,
			wantValues: map[string]interface{}{
				"owner": "monalisa",
				"repo":  "hello-world",
			},
		},
		{
			name: "Multiple queries",
			args: args{
				queries: []PreparedQuery{
					PreparedQuery{
						variableValues: map[string]interface{}{
							"owner": "monalisa",
							"repo":  "hello-world",
						},
						Query: Query{
							query:     `repository(owner: $owner, name: $repo) { id }`,
							variables: map[string]string{"owner": "String!", "repo": "String"},
						},
					},
					PreparedQuery{
						variableValues: map[string]interface{}{
							"owner": "hubot",
							"repo":  "chatops",
							"user":  "octocat",
						},
						Query: Query{
							query:     `repository(owner: $owner, name: $repo, assignee: $user) { id }`,
							variables: map[string]string{"owner": "String", "repo": "String!", "user": "String"},
						},
					},
					PreparedQuery{
						variableValues: map[string]interface{}{
							"owner": "github",
							"user":  "ghost",
						},
						Query: Query{
							query:     `repository(owner: $owner, assignee: $user) { id }`,
							variables: map[string]string{"owner": "String", "user": "String!"},
						},
					},
				},
			},
			wantQuery: `query(
	$owner: String!
	$owner_001: String
	$owner_002: String
	$repo: String
	$repo_001: String!
	$user: String
	$user_002: String!
) {
	multi_000: repository(owner: $owner, name: $repo) { id }
	multi_001: repository(owner: $owner_001, name: $repo_001, assignee: $user) { id }
	multi_002: repository(owner: $owner_002, assignee: $user_002) { id }
}`,
			wantValues: map[string]interface{}{
				"owner":     "monalisa",
				"repo":      "hello-world",
				"owner_001": "hubot",
				"repo_001":  "chatops",
				"user":      "octocat",
				"owner_002": "github",
				"user_002":  "ghost",
			},
		},
		{
			name: "Queries with fragments",
			args: args{
				queries: []PreparedQuery{
					PreparedQuery{
						variableValues: map[string]interface{}{},
						Query: Query{
							query:     `a { ...b }`,
							fragments: `fragment b on B { boo }`,
							variables: map[string]string{},
						},
					},
					PreparedQuery{
						variableValues: map[string]interface{}{},
						Query: Query{
							query:     `c { ...b }`,
							fragments: `fragment b on B { boo }`,
							variables: map[string]string{},
						},
					},
					PreparedQuery{
						variableValues: map[string]interface{}{},
						Query: Query{
							query:     `d { ...e }`,
							fragments: `fragment e on E { eeek }`,
							variables: map[string]string{},
						},
					},
				},
			},
			wantQuery: `fragment b on B { boo }
fragment e on E { eeek }
query {
	multi_000: a { ...b }
	multi_001: c { ...b }
	multi_002: d { ...e }
}`,
			wantValues: map[string]interface{}{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := Merge(tt.args.queries...)
			if got != tt.wantQuery {
				t.Errorf("Merge() got = %#v, want %#v", got, tt.wantQuery)
			}
			if !reflect.DeepEqual(got1, tt.wantValues) {
				t.Errorf("Merge() got1 = %v, want %v", got1, tt.wantValues)
			}
		})
	}
}
