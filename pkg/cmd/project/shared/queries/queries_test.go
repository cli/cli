package queries

import (
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestProjectMutationQuery_DoesNotRequireQueryVariable(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			assert.NoError(t, err)
			assert.NotContains(t, string(body), "$query")

			return &http.Response{
				StatusCode: 200,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				Body: io.NopCloser(strings.NewReader(`{
					"data": {
						"updateProjectV2": {
							"projectV2": {
								"id": "project ID",
								"url": "http://example.com"
							}
						}
					}
				}`)),
			}, nil
		}),
	}

	client := NewClient(httpClient, "github.com", ios)
	mutation := struct {
		UpdateProjectV2 struct {
			ProjectV2 ProjectMutationQuery `graphql:"projectV2"`
		} `graphql:"updateProjectV2(input:$input)"`
	}{}

	err := client.Mutate("UpdateProjectV2", &mutation, map[string]interface{}{
		"input": githubv4.UpdateProjectV2Input{
			ProjectID: githubv4.ID("project ID"),
		},
		"firstItems":  githubv4.Int(0),
		"afterItems":  (*githubv4.String)(nil),
		"firstFields": githubv4.Int(0),
		"afterFields": (*githubv4.String)(nil),
	})
	assert.NoError(t, err)
}

func TestProjectItems_DefaultLimit(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// list project items
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]interface{}{
			"query": "query UserProjectWithItems.*",
			"variables": map[string]interface{}{
				"firstItems":  LimitMax,
				"afterItems":  nil,
				"firstFields": LimitMax,
				"afterFields": nil,
				"login":       "monalisa",
				"number":      1,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"items": map[string]interface{}{
							"nodes": []map[string]interface{}{
								{
									"id": "issue ID",
								},
								{
									"id": "pull request ID",
								},
								{
									"id": "draft issue ID",
								},
							},
						},
					},
				},
			},
		})

	client := NewTestClient()

	owner := &Owner{
		Type:  "USER",
		Login: "monalisa",
		ID:    "user ID",
	}
	project, err := client.ProjectItems(owner, 1, LimitMax, "")
	assert.NoError(t, err)
	assert.Len(t, project.Items.Nodes, 3)
}

func TestProjectItems_LowerLimit(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// list project items
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]interface{}{
			"query": "query UserProjectWithItems.*",
			"variables": map[string]interface{}{
				"firstItems":  2,
				"afterItems":  nil,
				"firstFields": LimitMax,
				"afterFields": nil,
				"login":       "monalisa",
				"number":      1,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"items": map[string]interface{}{
							"nodes": []map[string]interface{}{
								{
									"id": "issue ID",
								},
								{
									"id": "pull request ID",
								},
							},
						},
					},
				},
			},
		})

	client := NewTestClient()

	owner := &Owner{
		Type:  "USER",
		Login: "monalisa",
		ID:    "user ID",
	}
	project, err := client.ProjectItems(owner, 1, 2, "")
	assert.NoError(t, err)
	assert.Len(t, project.Items.Nodes, 2)
}

func TestProjectItems_NoLimit(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// list project items
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]interface{}{
			"query": "query UserProjectWithItems.*",
			"variables": map[string]interface{}{
				"firstItems":  LimitDefault,
				"afterItems":  nil,
				"firstFields": LimitMax,
				"afterFields": nil,
				"login":       "monalisa",
				"number":      1,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"items": map[string]interface{}{
							"nodes": []map[string]interface{}{
								{
									"id": "issue ID",
								},
								{
									"id": "pull request ID",
								},
								{
									"id": "draft issue ID",
								},
							},
						},
					},
				},
			},
		})

	client := NewTestClient()

	owner := &Owner{
		Type:  "USER",
		Login: "monalisa",
		ID:    "user ID",
	}
	project, err := client.ProjectItems(owner, 1, 0, "")
	assert.NoError(t, err)
	assert.Len(t, project.Items.Nodes, 3)
}

func TestProjectItems_WithQuery(t *testing.T) {
	tests := []struct {
		name      string
		owner     *Owner
		queryName string
		dataKey   string
		vars      map[string]interface{}
	}{
		{
			name: "user owner",
			owner: &Owner{
				Type:  UserOwner,
				Login: "monalisa",
				ID:    "user ID",
			},
			queryName: "UserProjectWithItems",
			dataKey:   "user",
			vars: map[string]interface{}{
				"firstItems":  LimitMax,
				"afterItems":  nil,
				"firstFields": LimitMax,
				"afterFields": nil,
				"login":       "monalisa",
				"number":      1,
				"query":       "assignee:octocat",
			},
		},
		{
			name: "org owner",
			owner: &Owner{
				Type:  OrgOwner,
				Login: "github",
				ID:    "org ID",
			},
			queryName: "OrgProjectWithItems",
			dataKey:   "organization",
			vars: map[string]interface{}{
				"firstItems":  LimitMax,
				"afterItems":  nil,
				"firstFields": LimitMax,
				"afterFields": nil,
				"login":       "github",
				"number":      1,
				"query":       "assignee:octocat",
			},
		},
		{
			name: "viewer owner",
			owner: &Owner{
				Type: ViewerOwner,
				ID:   "viewer ID",
			},
			queryName: "ViewerProjectWithItems",
			dataKey:   "viewer",
			vars: map[string]interface{}{
				"firstItems":  LimitMax,
				"afterItems":  nil,
				"firstFields": LimitMax,
				"afterFields": nil,
				"number":      1,
				"query":       "assignee:octocat",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer gock.Off()
			gock.Observe(gock.DumpRequest)

			gock.New("https://api.github.com").
				Post("/graphql").
				JSON(map[string]interface{}{
					"query":     "query " + tt.queryName + ".*",
					"variables": tt.vars,
				}).
				Reply(200).
				JSON(map[string]interface{}{
					"data": map[string]interface{}{
						tt.dataKey: map[string]interface{}{
							"projectV2": map[string]interface{}{
								"items": map[string]interface{}{
									"nodes": []map[string]interface{}{
										{
											"id": "issue ID",
										},
									},
								},
							},
						},
					},
				})

			client := NewTestClient()
			project, err := client.ProjectItems(tt.owner, 1, LimitMax, "assignee:octocat")
			assert.NoError(t, err)
			assert.Len(t, project.Items.Nodes, 1)
		})
	}
}

func TestProjectItems_NoQueryDoesNotUseQueryItems(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			assert.NoError(t, err)
			assert.NotContains(t, string(body), "$query")

			return &http.Response{
				StatusCode: 200,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				Body: io.NopCloser(strings.NewReader(`{
					"data": {
						"user": {
							"projectV2": {
								"items": {
									"nodes": [
										{"id": "issue ID"}
									]
								}
							}
						}
					}
				}`)),
			}, nil
		}),
	}

	client := NewClient(httpClient, "github.com", ios)
	owner := &Owner{
		Type:  UserOwner,
		Login: "monalisa",
		ID:    "user ID",
	}
	project, err := client.ProjectItems(owner, 1, LimitMax, "")
	assert.NoError(t, err)
	assert.Len(t, project.Items.Nodes, 1)
}

func TestProjects_ViewerQueryDoesNotUseQueryItems(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			assert.NoError(t, err)
			assert.NotContains(t, string(body), "$query")

			return &http.Response{
				StatusCode: 200,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				Body: io.NopCloser(strings.NewReader(`{
					"data": {
						"viewer": {
							"projectsV2": {
								"totalCount": 1,
								"pageInfo": {
									"hasNextPage": false,
									"endCursor": ""
								},
								"nodes": [
									{
										"number": 1,
										"title": "Roadmap"
									}
								]
							}
						}
					}
				}`)),
			}, nil
		}),
	}

	client := NewClient(httpClient, "github.com", ios)
	projects, err := client.Projects("", ViewerOwner, 1, false)
	assert.NoError(t, err)
	assert.Len(t, projects.Nodes, 1)
	assert.Equal(t, int32(1), projects.Nodes[0].Number)
	assert.Equal(t, "Roadmap", projects.Nodes[0].Title)
}

func TestProjectFields_LowerLimit(t *testing.T) {

	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// list project fields
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]interface{}{
			"query": "query UserProject.*",
			"variables": map[string]interface{}{
				"login":       "monalisa",
				"number":      1,
				"firstItems":  LimitMax,
				"afterItems":  nil,
				"firstFields": 2,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"fields": map[string]interface{}{
							"nodes": []map[string]interface{}{
								{
									"id": "field ID",
								},
								{
									"id": "status ID",
								},
							},
						},
					},
				},
			},
		})

	client := NewTestClient()
	owner := &Owner{
		Type:  "USER",
		Login: "monalisa",
		ID:    "user ID",
	}
	project, err := client.ProjectFields(owner, 1, 2)
	assert.NoError(t, err)
	assert.Len(t, project.Fields.Nodes, 2)
}

func TestProjectFields_DefaultLimit(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// list project fields
	// list project fields
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]interface{}{
			"query": "query UserProject.*",
			"variables": map[string]interface{}{
				"login":       "monalisa",
				"number":      1,
				"firstItems":  LimitMax,
				"afterItems":  nil,
				"firstFields": LimitMax,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"fields": map[string]interface{}{
							"nodes": []map[string]interface{}{
								{
									"id": "field ID",
								},
								{
									"id": "status ID",
								},
								{
									"id": "iteration ID",
								},
							},
						},
					},
				},
			},
		})

	client := NewTestClient()

	owner := &Owner{
		Type:  "USER",
		Login: "monalisa",
		ID:    "user ID",
	}
	project, err := client.ProjectFields(owner, 1, LimitMax)
	assert.NoError(t, err)
	assert.Len(t, project.Fields.Nodes, 3)
}

func TestProjectFields_NoLimit(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// list project fields
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]interface{}{
			"query": "query UserProject.*",
			"variables": map[string]interface{}{
				"login":       "monalisa",
				"number":      1,
				"firstItems":  LimitMax,
				"afterItems":  nil,
				"firstFields": LimitDefault,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"fields": map[string]interface{}{
							"nodes": []map[string]interface{}{
								{
									"id": "field ID",
								},
								{
									"id": "status ID",
								},
								{
									"id": "iteration ID",
								},
							},
						},
					},
				},
			},
		})

	client := NewTestClient()

	owner := &Owner{
		Type:  "USER",
		Login: "monalisa",
		ID:    "user ID",
	}
	project, err := client.ProjectFields(owner, 1, 0)
	assert.NoError(t, err)
	assert.Len(t, project.Fields.Nodes, 3)
}

func Test_requiredScopesFromServerMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want []string
	}{
		{
			name: "no scopes",
			msg:  "SERVER OOPSIE",
			want: []string(nil),
		},
		{
			name: "one scope",
			msg:  "Your token has not been granted the required scopes to execute this query. The 'dataType' field requires one of the following scopes: ['read:project'], but your token has only been granted the: ['codespace', repo'] scopes. Please modify your token's scopes at: https://github.com/settings/tokens.",
			want: []string{"read:project"},
		},
		{
			name: "multiple scopes",
			msg:  "Your token has not been granted the required scopes to execute this query. The 'dataType' field requires one of the following scopes: ['read:project', 'read:discussion', 'codespace'], but your token has only been granted the: [repo'] scopes. Please modify your token's scopes at: https://github.com/settings/tokens.",
			want: []string{"read:project", "read:discussion", "codespace"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := requiredScopesFromServerMessage(tt.msg); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("requiredScopesFromServerMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewProject_nonTTY(t *testing.T) {
	client := NewTestClient()
	_, err := client.NewProject(false, &Owner{}, 0, false)
	assert.EqualError(t, err, "project number is required when not running interactively")
}

func TestNewOwner_nonTTY(t *testing.T) {
	client := NewTestClient()
	_, err := client.NewOwner(false, "")
	assert.EqualError(t, err, "owner is required when not running interactively")

}

func TestProjectItems_FieldTitle(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// list project items
	gock.New("https://api.github.com").
		Post("/graphql").
		JSON(map[string]interface{}{
			"query": "query UserProjectWithItems.*",
			"variables": map[string]interface{}{
				"firstItems":  LimitMax,
				"afterItems":  nil,
				"firstFields": LimitMax,
				"afterFields": nil,
				"login":       "monalisa",
				"number":      1,
			},
		}).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"user": map[string]interface{}{
					"projectV2": map[string]interface{}{
						"items": map[string]interface{}{
							"nodes": []map[string]interface{}{
								{
									"id": "draft issue ID",
									"fieldValues": map[string]interface{}{
										"nodes": []map[string]interface{}{
											{
												"__typename":  "ProjectV2ItemFieldIterationValue",
												"title":       "Iteration Title 1",
												"iterationId": "iterationId1",
											},
											{
												"__typename": "ProjectV2ItemFieldMilestoneValue",
												"milestone": map[string]interface{}{
													"title": "Milestone Title 1",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		})

	client := NewTestClient()

	owner := &Owner{
		Type:  "USER",
		Login: "monalisa",
		ID:    "user ID",
	}
	project, err := client.ProjectItems(owner, 1, LimitMax, "")
	assert.NoError(t, err)
	assert.Len(t, project.Items.Nodes, 1)
	assert.Len(t, project.Items.Nodes[0].FieldValues.Nodes, 2)
	assert.Equal(t, project.Items.Nodes[0].FieldValues.Nodes[0].ProjectV2ItemFieldIterationValue.Title, "Iteration Title 1")
	assert.Equal(t, project.Items.Nodes[0].FieldValues.Nodes[0].ProjectV2ItemFieldIterationValue.IterationId, "iterationId1")
	assert.Equal(t, project.Items.Nodes[0].FieldValues.Nodes[1].ProjectV2ItemFieldMilestoneValue.Milestone.Title, "Milestone Title 1")
}

func TestCamelCase(t *testing.T) {
	assert.Equal(t, "camelCase", camelCase("camelCase"))
	assert.Equal(t, "camelCase", camelCase("CamelCase"))
	assert.Equal(t, "c", camelCase("C"))
	assert.Equal(t, "", camelCase(""))
}
