package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseScaffoldArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    scaffoldOptions
		wantErr string
	}{
		{
			name: "request name",
			args: []string{"graphql IssueByNumber"},
			want: scaffoldOptions{name: "graphql IssueByNumber"},
		},
		{
			name: "--missing",
			args: []string{"--missing"},
			want: scaffoldOptions{missing: true},
		},
		{
			name:    "neither",
			wantErr: `pass a request name, such as "graphql IssueByNumber", or --missing`,
		},
		{
			name:    "both",
			args:    []string{"GET /", "--missing"},
			wantErr: "use either a request name or --missing, not both",
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
			name:    "unknown flag",
			args:    []string{"--nope"},
			wantErr: "flag provided but not defined: -nope",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseScaffoldArgs(tt.args)
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

func gqlEntry(run, operation, query, vars string) loggedRequest {
	e := loggedRequest{
		Run: run, Binary: "gh", Method: "POST", Host: apiHost, Path: "/graphql", Name: "graphql " + operation,
		GraphQL: &graphQLRequest{Kind: "query", Operation: operation, Query: query},
	}
	if vars != "" {
		e.GraphQL.Variables = json.RawMessage(vars)
	}
	return e
}

func restEntry(run, name string) loggedRequest {
	method, path, _ := strings.Cut(name, " ")
	return loggedRequest{Run: run, Binary: "gh", Method: method, Host: apiHost, Path: path, Name: name}
}

func TestCmdScaffold(t *testing.T) {
	const repoName = `query RepoName { repository(owner: "o", name: "r") { name } }`
	const exampleIssue = `{"id":1,"number":1347,"title":"Found a bug","body":"I'm having a problem with this.","state":"open",` +
		`"user":{"login":"octocat","id":1},"labels":[{"name":"bug","color":"f29513"}],"created_at":"2011-04-22T13:33:48Z"}`

	tests := []struct {
		name       string
		noSchemas  bool
		responses  string
		logged     []loggedRequest
		runs       []runRecord
		args       []string
		wantStdout string
		wantErr    string
		want       string
	}{
		{
			name:    "cyclic fragments stop before scaffolding",
			logged:  []loggedRequest{gqlEntry("r1", "Loop", `query Loop { viewer { ...Fields } } fragment Fields on User { login ...Fields }`, "")},
			args:    []string{"graphql Loop"},
			wantErr: "gh's query couldn't be read; ghmock log --full shows it",
			want:    `{}`,
		},
		{
			name: "scaffolding uses the selected second operation",
			logged: []loggedRequest{gqlEntry("r1", "Second",
				`query First { viewer { login } } query Second { viewer { name } }`, "")},
			args:       []string{"graphql Second"},
			wantStdout: "graphql Second: added response 1 of 1 with placeholder values\n",
			want:       `{"graphql Second":[{"body":{"data":{"viewer":{"name":"name"}}}}]}`,
		},
		{
			name: "candidate validation retains explicit overrides of variable defaults",
			logged: []loggedRequest{gqlEntry("r1", "Default",
				`query Default($show:Boolean = true) { viewer { login name @include(if:$show) } }`, `{"show":false}`)},
			args:       []string{"graphql Default"},
			wantStdout: "graphql Default: added response 1 of 1 with placeholder values\n",
			want:       `{"graphql Default":[{"body":{"data":{"viewer":{"login":"login"}}}}]}`,
		},
		{
			name:      "an existing typename alias chooses the object to repair",
			responses: `{"graphql Kind":[{"body":{"data":{"repository":{"issueOrPullRequest":{"kind":"Issue"}}}}}]}`,
			logged: []loggedRequest{gqlEntry("r1", "Kind",
				`query Kind { repository(owner:"o",name:"r") { issueOrPullRequest(number:1) { kind:__typename ...on PullRequest { number } ...on Issue { title } } } }`, "")},
			args:       []string{"graphql Kind"},
			wantStdout: "graphql Kind: response 1 of 1: added data.repository.issueOrPullRequest.title\n",
			want:       `{"graphql Kind":[{"body":{"data":{"repository":{"issueOrPullRequest":{"kind":"Issue","title":"title"}}}}}]}`,
		},
		{
			name: "--missing uses the completed run rather than a later incomplete query",
			logged: []loggedRequest{
				gqlEntry("r1", "Same", `query Same { viewer { login } }`, ""),
				gqlEntry("r2", "Same", `query Same { viewer { name } }`, ""),
			},
			runs:       []runRecord{{ID: "r1"}},
			args:       []string{"--missing"},
			wantStdout: "graphql Same: added response 1 of 1 with placeholder values\n",
			want:       `{"graphql Same":[{"body":{"data":{"viewer":{"login":"login"}}}}]}`,
		},
		{
			name:    "--missing does not reuse requests from before a request-free run",
			logged:  []loggedRequest{restEntry("r1", "GET /repos/o/r")},
			runs:    []runRecord{{ID: "r1"}, {ID: "r2"}},
			args:    []string{"--missing"},
			wantErr: "no requests logged in the latest run; run gh with ghmock run first",
			want:    `{}`,
		},
		{
			name: "same query with different directive variables is not deduplicated",
			logged: []loggedRequest{
				gqlEntry("r1", "Include", `query Include($show: Boolean!) { viewer { login @include(if: $show) } }`, `{"show":false}`),
				gqlEntry("r1", "Include", `query Include($show: Boolean!) { viewer { login @include(if: $show) } }`, `{"show":true}`),
			},
			args:       []string{"graphql Include"},
			wantStdout: "graphql Include: added response 1 of 1 with placeholder values\n",
			want:       `{"graphql Include":[{"body":{"data":{"viewer":{"login":"login"}}}}]}`,
		},
		{
			name:       "operation defaults control generated selections",
			logged:     []loggedRequest{gqlEntry("r1", "Default", `query Default($show: Boolean = true) { viewer { login @include(if: $show) } }`, "")},
			args:       []string{"graphql Default"},
			wantStdout: "graphql Default: added response 1 of 1 with placeholder values\n",
			want:       `{"graphql Default":[{"body":{"data":{"viewer":{"login":"login"}}}}]}`,
		},
		{
			name:      "existing introspection values and nulls survive missing-field repair",
			responses: `{"graphql Type":[{"body":{"data":{"Type":{"name":"Keep","fields":[{"name":"title","description":null}]}}}}]}`,
			logged: []loggedRequest{gqlEntry("r1", "Type",
				`query Type { Type: __type(name:"Issue") { name kind fields { name description type { kind } } } }`, "")},
			args:       []string{"graphql Type"},
			wantStdout: "graphql Type: response 1 of 1: added data.Type.kind, data.Type.fields.*.type\n",
			want:       `{"graphql Type":[{"body":{"data":{"Type":{"name":"Keep","kind":"OBJECT","fields":[{"name":"title","description":null,"type":{"kind":"NON_NULL"}}]}}}}]}`,
		},
		{
			name: "different introspection query variants fill one generated object",
			logged: []loggedRequest{
				gqlEntry("r1", "Type", `query Type { Type: __type(name:"Issue") { name } }`, ""),
				gqlEntry("r1", "Type", `query Type { Type: __type(name:"Issue") { kind } }`, ""),
			},
			args:       []string{"graphql Type"},
			wantStdout: "graphql Type: added response 1 of 1 from GitHub's schema\n",
			want:       `{"graphql Type":[{"body":{"data":{"Type":{"name":"Issue","kind":"OBJECT"}}}}]}`,
		},
		{
			name:      "introspection list identity does not depend on selecting name",
			responses: `{"graphql Type":[{"body":{"data":{"Type":{"fields":[{"name":"title"}]}}}}]}`,
			logged: []loggedRequest{gqlEntry("r1", "Type",
				`query Type { Type: __type(name:"Issue") { fields { type { kind } } } }`, "")},
			args:       []string{"graphql Type"},
			wantStdout: "graphql Type: response 1 of 1: added data.Type.fields.*.type\n",
			want:       `{"graphql Type":[{"body":{"data":{"Type":{"fields":[{"name":"title","type":{"kind":"NON_NULL"}}]}}}}]}`,
		},
		{
			name:      "introspection list identity survives an aliased name selection",
			responses: `{"graphql Type":[{"body":{"data":{"Type":{"fields":[{"label":"title"}]}}}}]}`,
			logged: []loggedRequest{gqlEntry("r1", "Type",
				`query Type { Type: __type(name:"Issue") { fields { label: name type { kind ofType { name } } } } }`, "")},
			args:       []string{"graphql Type"},
			wantStdout: "graphql Type: response 1 of 1: added data.Type.fields.*.type\n",
			want:       `{"graphql Type":[{"body":{"data":{"Type":{"fields":[{"label":"title","type":{"kind":"NON_NULL","ofType":{"name":"String"}}}]}}}}]}`,
		},
		{
			name: "a GraphQL response gets a placeholder for every field gh asked for",
			logged: []loggedRequest{gqlEntry("r1", "RepoIssue", `query RepoIssue($n: Int!) {
				repository(owner: "o", name: "r") {
					id name nameWithOwner url hasIssuesEnabled
					issue(number: $n) { number title state createdAt assignees { login } author { login } }
					labels { nodes { name } totalCount }
				}
			}`, `{"n": 1}`)},
			args:       []string{"graphql RepoIssue"},
			wantStdout: "graphql RepoIssue: added response 1 of 1 with placeholder values\n",
			want: `{"graphql RepoIssue":[{"body":{"data":{"repository":{
				"id":"Repository_1","name":"name","nameWithOwner":"owner/name","url":"https://github.localhost/owner/name","hasIssuesEnabled":true,
				"issue":{"number":1,"title":"title","state":"OPEN","createdAt":"2026-01-01T00:00:00Z","assignees":[{"login":"login"}],"author":{"login":"login"}},
				"labels":{"nodes":[{"name":"name"}],"totalCount":1}
			}}}}]}`,
		},
		{
			name: "abstract types use the first fragment's type, and @include and @skip read gh's variables",
			logged: []loggedRequest{gqlEntry("r1", "Mixed", `query Mixed($withBody: Boolean!, $skipTitle: Boolean!) {
				repository(owner: "o", name: "r") {
					item: issueOrPullRequest(number: 1) { __typename ... on PullRequest { number state } ... on Issue { title } }
					issue(number: 1) {
						body @include(if: $withBody)
						title @skip(if: $skipTitle)
						author { __typename login ... on User { id } }
					}
				}
			}`, `{"withBody": false, "skipTitle": false}`)},
			args:       []string{"graphql Mixed"},
			wantStdout: "graphql Mixed: added response 1 of 1 with placeholder values\n",
			want: `{"graphql Mixed":[{"body":{"data":{"repository":{
				"item":{"__typename":"PullRequest","number":1,"state":"OPEN"},
				"issue":{"title":"title","author":{"__typename":"User","login":"login","id":"User_1"}}
			}}}}]}`,
		},
		{
			name: "introspection gets GitHub's real answers",
			logged: []loggedRequest{gqlEntry("r1", "Issue_fields", `query Issue_fields {
				Issue: __type(name: "Issue") { __typename name kind fields { name } }
				Deprecated: __type(name: "Issue") { fields(includeDeprecated: true) { name isDeprecated deprecationReason } }
				Wrapped: __type(name: "LabelConnection") { fields { name type { kind name ofType { kind name } } } }
				State: __type(name: "IssueState") { enumValues { name description } }
				Nope: __type(name: "Nope") { name }
			}`, "")},
			args:       []string{"graphql Issue_fields"},
			wantStdout: "graphql Issue_fields: added response 1 of 1 from GitHub's schema\n",
			want: `{"graphql Issue_fields":[{"body":{"data":{
				"Issue":{"__typename":"__Type","name":"Issue","kind":"OBJECT","fields":[
					{"name":"id"},{"name":"number"},{"name":"title"},{"name":"body"},{"name":"state"},
					{"name":"createdAt"},{"name":"author"},{"name":"assignees"}]},
				"Deprecated":{"fields":[
					{"name":"id","isDeprecated":false,"deprecationReason":null},
					{"name":"number","isDeprecated":false,"deprecationReason":null},
					{"name":"title","isDeprecated":false,"deprecationReason":null},
					{"name":"body","isDeprecated":false,"deprecationReason":null},
					{"name":"state","isDeprecated":false,"deprecationReason":null},
					{"name":"createdAt","isDeprecated":false,"deprecationReason":null},
					{"name":"author","isDeprecated":false,"deprecationReason":null},
					{"name":"assignees","isDeprecated":false,"deprecationReason":null},
					{"name":"databaseId","isDeprecated":true,"deprecationReason":"Use fullDatabaseId instead."}]},
				"Wrapped":{"fields":[
					{"name":"nodes","type":{"kind":"LIST","name":null,"ofType":{"kind":"OBJECT","name":"Label"}}},
					{"name":"totalCount","type":{"kind":"NON_NULL","name":null,"ofType":{"kind":"SCALAR","name":"Int"}}}]},
				"State":{"enumValues":[
					{"name":"CLOSED","description":"An issue that has been closed"},
					{"name":"OPEN","description":"An issue that is still open"}]},
				"Nope":null
			}}}]}`,
		},
		{
			name:      "fields a stored response lacks are added, and null stays null",
			responses: `{"graphql RepoName":[{"body":{"data":{"repository":{"name":"hello","issue":null}}}}]}`,
			logged: []loggedRequest{gqlEntry("r1", "RepoName",
				`query RepoName { repository(owner: "o", name: "r") { name nameWithOwner issue(number: 1) { title } labels { totalCount } } }`, "")},
			args:       []string{"graphql RepoName"},
			wantStdout: "graphql RepoName: response 1 of 1: added data.repository.nameWithOwner, data.repository.labels\n",
			want: `{"graphql RepoName":[{"body":{"data":{"repository":{
				"name":"hello","nameWithOwner":"owner/name","issue":null,"labels":{"totalCount":1}
			}}}}]}`,
		},
		{
			name:      "a field added to every list item is listed once",
			responses: `{"graphql Labels":[{"body":{"data":{"repository":{"labels":{"nodes":[{"name":"a"},{"name":"b"}],"totalCount":2}}}}}]}`,
			logged: []loggedRequest{gqlEntry("r1", "Labels",
				`query Labels { repository(owner: "o", name: "r") { labels { nodes { name color } totalCount } } }`, "")},
			args:       []string{"graphql Labels"},
			wantStdout: "graphql Labels: response 1 of 1: added data.repository.labels.nodes.*.color\n",
			want: `{"graphql Labels":[{"body":{"data":{"repository":{
				"labels":{"nodes":[{"name":"a","color":"color"},{"name":"b","color":"color"}],"totalCount":2}
			}}}}]}`,
		},
		{
			name: "error statuses, errors without data and off-schema responses are left as is",
			responses: `{"graphql RepoName":[
				{"status":502},
				{"body":{"errors":[{"message":"Not Found"}]}},
				{"body":{"data":{"repository":{}}},"off_schema":true}
			]}`,
			logged:     []loggedRequest{gqlEntry("r1", "RepoName", repoName, "")},
			args:       []string{"graphql RepoName"},
			wantStdout: "graphql RepoName: response 3 of 3 is off-schema, so it was left as is; it lacks data.repository.name\n",
		},
		{
			name:       "nothing to add",
			responses:  `{"graphql RepoName":[{"body":{"data":{"repository":{"name":"hello"}}}}]}`,
			logged:     []loggedRequest{gqlEntry("r1", "RepoName", repoName, "")},
			args:       []string{"graphql RepoName"},
			wantStdout: "graphql RepoName: nothing to add; every stored response has every field gh asked for\n",
		},
		{
			name: "a response gets the fields of every query in the run, such as from two gh versions",
			logged: []loggedRequest{
				gqlEntry("r1", "RepoName", repoName, ""),
				gqlEntry("r1", "RepoName", `query RepoName { repository(owner: "o", name: "r") { name nameWithOwner } }`, ""),
			},
			args:       []string{"graphql RepoName"},
			wantStdout: "graphql RepoName: added response 1 of 1 with placeholder values\n",
			want:       `{"graphql RepoName":[{"body":{"data":{"repository":{"name":"name","nameWithOwner":"owner/name"}}}}]}`,
		},
		{
			name: "only the most recent run that sent the request counts",
			logged: []loggedRequest{
				gqlEntry("r1", "RepoName", `query RepoName { repository(owner: "o", name: "r") { name nameWithOwner } }`, ""),
				gqlEntry("r2", "RepoName", repoName, ""),
			},
			args:       []string{"graphql RepoName"},
			wantStdout: "graphql RepoName: added response 1 of 1 with placeholder values\n",
			want:       `{"graphql RepoName":[{"body":{"data":{"repository":{"name":"name"}}}}]}`,
		},
		{
			name:    "a GraphQL request gh hasn't sent",
			args:    []string{"graphql Nope"},
			wantErr: "gh hasn't sent graphql Nope, so there's no query to build it from; run gh first",
		},
		{
			name:       "a REST response from GitHub's example",
			args:       []string{"GET /repos/o/r/issues/1"},
			wantStdout: "GET /repos/o/r/issues/1: added response 1 of 1 from GitHub's example\n",
			want:       `{"GET /repos/o/r/issues/1":[{"body":` + exampleIssue + `}]}`,
		},
		{
			name:       "an example's gaps are filled",
			args:       []string{"GET /repos/o/r"},
			wantStdout: "GET /repos/o/r: added response 1 of 1 from GitHub's example, with 1 value added or fixed\n",
			want:       `{"GET /repos/o/r":[{"body":{"id":1,"name":"Hello-World","owner":{"login":"octocat","id":1},"subscribers_count":1}}]}`,
		},
		{
			name:       "an example gets the discriminator its schema needs",
			args:       []string{"GET /user"},
			wantStdout: "GET /user: added response 1 of 1 from GitHub's example, with 2 values added or fixed\n",
			want:       `{"GET /user":[{"body":{"login":"octocat","id":1,"plan":{"name":"pro","space":1},"user_view_type":"private"}}]}`,
		},
		{
			name:       "file contents in an example are base64 encoded",
			args:       []string{"GET /repos/o/r/contents/README.md"},
			wantStdout: "GET /repos/o/r/contents/README.md: added response 1 of 1 from GitHub's example, with 1 value added or fixed\n",
			want:       `{"GET /repos/o/r/contents/README.md":[{"body":{"type":"file","encoding":"base64","path":"README.md","content":"ZW5jb2RlZCBjb250ZW50IC4uLg=="}}]}`,
		},
		{
			name:       "without an example, the body gets placeholders",
			args:       []string{"GET /"},
			wantStdout: "GET /: added response 1 of 1 with placeholder values\n",
			want:       `{"GET /":[{"body":{"current_user_url":"https://github.localhost/owner/name"}}]}`,
		},
		{
			name:       "the first documented success status",
			args:       []string{"POST /repos/o/r/issues"},
			wantStdout: "POST /repos/o/r/issues: added response 1 of 1 from GitHub's example, status 201\n",
			want:       `{"POST /repos/o/r/issues":[{"status":201,"body":` + exampleIssue + `}]}`,
		},
		{
			name:       "a status without a body",
			args:       []string{"DELETE /repos/o/r"},
			wantStdout: "DELETE /repos/o/r: added response 1 of 1 with no body, status 204\n",
			want:       `{"DELETE /repos/o/r":[{"status":204}]}`,
		},
		{
			name:       "a REST request that already has a response",
			responses:  `{"GET /repos/o/r":[{"status":404}]}`,
			args:       []string{"GET /repos/o/r"},
			wantStdout: "GET /repos/o/r: already has a response, so nothing was added\n",
		},
		{
			name:    "a body that isn't JSON",
			args:    []string{"GET /zen"},
			wantErr: "GitHub's response for status 200 is text/plain, and ghmock can only store JSON bodies",
		},
		{
			name:    "a path GitHub doesn't document",
			args:    []string{"GET /nope"},
			wantErr: "GitHub's REST description has no path matching /nope",
		},
		{
			name:    "another host",
			args:    []string{"POST uploads.github.localhost/upload"},
			wantErr: "uploads.github.localhost/upload isn't on the REST API host, so GitHub's REST description doesn't cover it",
		},
		{
			name:      "--missing scaffolds what the last run lacked and reports what it can't",
			responses: `{"GET /repos/o/r":[{"status":404}]}`,
			logged: []loggedRequest{
				gqlEntry("r1", "Old", `query Old { viewer { login } }`, ""),
				gqlEntry("r2", "RepoName", repoName, ""),
				restEntry("r2", "GET /repos/o/r"),
				restEntry("r2", "GET /zen"),
				{Run: "r2", Binary: "gh", Method: "CONNECT", Host: "api.github.com", Blocked: true},
				restEntry("r2", "GET /repos/o/r/issues/1"),
			},
			args: []string{"--missing"},
			wantStdout: "graphql RepoName: added response 1 of 1 with placeholder values\n" +
				"GET /zen: can't scaffold: GitHub's response for status 200 is text/plain, and ghmock can only store JSON bodies\n" +
				"GET /repos/o/r/issues/1: added response 1 of 1 from GitHub's example\n",
			want: `{
				"graphql RepoName":[{"body":{"data":{"repository":{"name":"name"}}}}],
				"GET /repos/o/r":[{"status":404}],
				"GET /repos/o/r/issues/1":[{"body":` + exampleIssue + `}]
			}`,
		},
		{
			name:       "--missing with nothing to add",
			responses:  `{"graphql RepoName":[{"body":{"data":{"repository":{"name":"hello"}}}}]}`,
			logged:     []loggedRequest{gqlEntry("r1", "RepoName", repoName, "")},
			args:       []string{"--missing"},
			wantStdout: "Nothing to add: every request in the last run has a response, with every field gh asked for.\n",
		},
		{
			name:    "--missing before any run",
			args:    []string{"--missing"},
			wantErr: "no requests logged yet; run gh with ghmock run first",
		},
		{
			name:      "without GitHub's schema",
			noSchemas: true,
			logged:    []loggedRequest{gqlEntry("r1", "RepoName", repoName, "")},
			args:      []string{"graphql RepoName"},
			wantErr: "scaffolding needs GitHub's GraphQL schema: downloading GitHub's GraphQL schema from " + graphQLSource.url +
				`: Get "` + graphQLSource.url + `": downloads are off in tests (without network access, set GHMOCK_GRAPHQL_SCHEMA to a local copy)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("GHMOCK_DIR", dir)
			if !tt.noSchemas {
				useTestSchemas(t)
			}
			st := &state{dir: dir}
			if tt.responses != "" {
				responses := responseSet{}
				require.NoError(t, json.Unmarshal([]byte(tt.responses), &responses))
				require.NoError(t, st.saveResponses(responses))
			}
			require.NoError(t, st.appendLog(tt.logged))
			for _, run := range tt.runs {
				require.NoError(t, st.appendRun(run))
			}

			var stdout bytes.Buffer
			err := cmdScaffold(tt.args, &stdout, io.Discard)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantStdout, stdout.String())

			want := tt.want
			if want == "" {
				want = tt.responses
			}
			if want == "" {
				want = "{}"
			}
			stored, err := st.loadResponses()
			require.NoError(t, err)
			got, err := json.Marshal(stored)
			require.NoError(t, err)
			assert.JSONEq(t, want, string(got))
		})
	}
}
