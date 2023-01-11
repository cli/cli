package api

import (
	"net/http"
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
							expectedVariables := map[string]interface{}{
								"input_000": map[string]interface{}{"contentId": "item1", "projectId": "project1"},
								"input_001": map[string]interface{}{"contentId": "item2", "projectId": "project2"},
								"input_002": map[string]interface{}{"itemId": "item3", "projectId": "project3"},
								"input_003": map[string]interface{}{"itemId": "item4", "projectId": "project4"},
							}
							assert.Equal(t, stripSpace(expectedMutations), stripSpace(mutations))
							assert.Equal(t, expectedVariables, inputs)
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

func TestHasProjectsV2Scope(t *testing.T) {
	var tests = []struct {
		name      string
		httpStubs func(*httpmock.Registry)
		expect    bool
	}{
		{
			name: "has project scope",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("HEAD", ""),
					httpmock.ScopesResponder("repo, read:org, project"),
				)
			},
			expect: true,
		},
		{
			name: "has read:project scope",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("HEAD", ""),
					httpmock.ScopesResponder("repo, read:org, read:project"),
				)
			},
			expect: true,
		},
		{
			name: "does not have project scope",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("HEAD", ""),
					httpmock.ScopesResponder("repo, read:org"),
				)
			},
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}
			client := &http.Client{Transport: reg}
			actual := HasProjectsV2Scope(client, "github.com")
			assert.Equal(t, tt.expect, actual)
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
