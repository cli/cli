package api

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"unicode"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
)

func TestUpdateProjectV2Items(t *testing.T) {
	var tests = []struct {
		name        string
		httpStubs   func(*httpmock.Registry)
		expectError bool
	}{
		{
			name: "updates project items",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`mutation UpdateProjectV2Items\b`),
					httpmock.GraphQLQuery(`{"data":{"add_000":{"item":{"id":"1"}},"delete_001":{"item":{"id":"2"}}}}`,
						func(mutations string, inputs map[string]interface{}) {
							expectedMutations := `
                mutation UpdateProjectV2Items(
                  $input_000: AddProjectV2ItemByIdInput!
                  $input_001: AddProjectV2ItemByIdInput!
                  $input_002: DeleteProjectV2ItemInput!
                  $input_003: DeleteProjectV2ItemInput!
                ) {
                  add_000: addProjectV2ItemById(input: $input_000) { item { id } }
                  add_001: addProjectV2ItemById(input: $input_001) { item { id } }
                  delete_002: deleteProjectV2Item(input: $input_002) { deletedItemId }
                  delete_003: deleteProjectV2Item(input: $input_003) { deletedItemId }
                }`
							assert.Equal(t, stripSpace(expectedMutations), stripSpace(mutations))
							if len(inputs) != 4 {
								t.Fatalf("expected 4 inputs, got %d", len(inputs))
							}
							i0 := inputs["input_000"].(map[string]interface{})
							i1 := inputs["input_001"].(map[string]interface{})
							i2 := inputs["input_002"].(map[string]interface{})
							i3 := inputs["input_003"].(map[string]interface{})
							adds := []string{
								fmt.Sprintf("%v -> %v", i0["contentId"], i0["projectId"]),
								fmt.Sprintf("%v -> %v", i1["contentId"], i1["projectId"]),
							}
							removes := []string{
								fmt.Sprintf("%v x %v", i2["itemId"], i2["projectId"]),
								fmt.Sprintf("%v x %v", i3["itemId"], i3["projectId"]),
							}
							sort.Strings(adds)
							sort.Strings(removes)
							assert.Equal(t, []string{"item1 -> project1", "item2 -> project2"}, adds)
							assert.Equal(t, []string{"item3 x project3", "item4 x project4"}, removes)
						}))
			},
		},
		{
			name: "fails to update project items",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`mutation UpdateProjectV2Items\b`),
					httpmock.GraphQLMutation(`{"data":{}, "errors": [{"message": "some gql error"}]}`, func(inputs map[string]interface{}) {}),
				)
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}
			client := newTestClient(reg)
			repo, _ := ghrepo.FromFullName("OWNER/REPO")
			addProjectItems := map[string]string{"project1": "item1", "project2": "item2"}
			deleteProjectItems := map[string]string{"project3": "item3", "project4": "item4"}
			err := UpdateProjectV2Items(client, repo, addProjectItems, deleteProjectItems)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProjectsV2ItemsForIssue(t *testing.T) {
	var tests = []struct {
		name        string
		httpStubs   func(*httpmock.Registry)
		expectItems ProjectItems
		expectError bool
	}{
		{
			name: "retrieves project items for issue",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query IssueProjectItems\b`),
					httpmock.GraphQLQuery(`{"data":{"repository":{"issue":{"projectItems":{"nodes": [{"id":"projectItem1"},{"id":"projectItem2"}]}}}}}`,
						func(query string, inputs map[string]interface{}) {}),
				)
			},
			expectItems: ProjectItems{
				Nodes: []*ProjectV2Item{
					{ID: "projectItem1"},
					{ID: "projectItem2"},
				},
			},
		},
		{
			name: "fails to retrieve project items for issue",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query IssueProjectItems\b`),
					httpmock.GraphQLQuery(`{"data":{}, "errors": [{"message": "some gql error"}]}`,
						func(query string, inputs map[string]interface{}) {}),
				)
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}
			client := newTestClient(reg)
			repo, _ := ghrepo.FromFullName("OWNER/REPO")
			issue := &Issue{Number: 1}
			err := ProjectsV2ItemsForIssue(client, repo, issue)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectItems, issue.ProjectItems)
		})
	}
}

func TestProjectsV2ItemsForPullRequest(t *testing.T) {
	var tests = []struct {
		name        string
		httpStubs   func(*httpmock.Registry)
		expectItems ProjectItems
		expectError bool
	}{
		{
			name: "retrieves project items for pull request",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query PullRequestProjectItems\b`),
					httpmock.GraphQLQuery(`{"data":{"repository":{"pullRequest":{"projectItems":{"nodes": [{"id":"projectItem3"},{"id":"projectItem4"}]}}}}}`,
						func(query string, inputs map[string]interface{}) {}),
				)
			},
			expectItems: ProjectItems{
				Nodes: []*ProjectV2Item{
					{ID: "projectItem3"},
					{ID: "projectItem4"},
				},
			},
		},
		{
			name: "fails to retrieve project items for pull request",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query PullRequestProjectItems\b`),
					httpmock.GraphQLQuery(`{"data":{}, "errors": [{"message": "some gql error"}]}`,
						func(query string, inputs map[string]interface{}) {}),
				)
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}
			client := newTestClient(reg)
			repo, _ := ghrepo.FromFullName("OWNER/REPO")
			pr := &PullRequest{Number: 1}
			err := ProjectsV2ItemsForPullRequest(client, repo, pr)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectItems, pr.ProjectItems)
		})
	}
}

func TestProjectsV2IgnorableError(t *testing.T) {
	var tests = []struct {
		name      string
		errMsg    string
		expectOut bool
	}{
		{
			name:      "read scope error",
			errMsg:    "field requires one of the following scopes: ['read:project']",
			expectOut: true,
		},
		{
			name:      "repository projectsV2 field error",
			errMsg:    "Field 'projectsV2' doesn't exist on type 'Repository'",
			expectOut: true,
		},
		{
			name:      "organization projectsV2 field error",
			errMsg:    "Field 'projectsV2' doesn't exist on type 'Organization'",
			expectOut: true,
		},
		{
			name:      "issue projectItems field error",
			errMsg:    "Field 'projectItems' doesn't exist on type 'Issue'",
			expectOut: true,
		},
		{
			name:      "pullRequest projectItems field error",
			errMsg:    "Field 'projectItems' doesn't exist on type 'PullRequest'",
			expectOut: true,
		},
		{
			name:      "other error",
			errMsg:    "some other graphql error message",
			expectOut: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := errors.New(tt.errMsg)
			out := ProjectsV2IgnorableError(err)
			assert.Equal(t, tt.expectOut, out)
		})
	}
}

func stripSpace(str string) string {
	var b strings.Builder
	b.Grow(len(str))
	for _, ch := range str {
		if !unicode.IsSpace(ch) {
			b.WriteRune(ch)
		}
	}
	return b.String()
}
