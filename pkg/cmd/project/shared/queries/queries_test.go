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

	err := client.Mutate("UpdateProjectV2", &mutation, map[string]any{
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
		JSON(map[string]any{
			"query": "query UserProjectWithItems.*",
			"variables": map[string]any{
				"firstItems":  LimitMax,
				"afterItems":  nil,
				"firstFields": LimitMax,
				"afterFields": nil,
				"login":       "monalisa",
				"number":      1,
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"projectV2": map[string]any{
						"items": map[string]any{
							"nodes": []map[string]any{
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
		JSON(map[string]any{
			"query": "query UserProjectWithItems.*",
			"variables": map[string]any{
				"firstItems":  2,
				"afterItems":  nil,
				"firstFields": LimitMax,
				"afterFields": nil,
				"login":       "monalisa",
				"number":      1,
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"projectV2": map[string]any{
						"items": map[string]any{
							"nodes": []map[string]any{
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
		JSON(map[string]any{
			"query": "query UserProjectWithItems.*",
			"variables": map[string]any{
				"firstItems":  LimitDefault,
				"afterItems":  nil,
				"firstFields": LimitMax,
				"afterFields": nil,
				"login":       "monalisa",
				"number":      1,
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"projectV2": map[string]any{
						"items": map[string]any{
							"nodes": []map[string]any{
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
		vars      map[string]any
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
			vars: map[string]any{
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
			vars: map[string]any{
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
			vars: map[string]any{
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
				JSON(map[string]any{
					"query":     "query " + tt.queryName + ".*",
					"variables": tt.vars,
				}).
				Reply(200).
				JSON(map[string]any{
					"data": map[string]any{
						tt.dataKey: map[string]any{
							"projectV2": map[string]any{
								"items": map[string]any{
									"nodes": []map[string]any{
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
		JSON(map[string]any{
			"query": "query UserProject.*",
			"variables": map[string]any{
				"login":       "monalisa",
				"number":      1,
				"firstItems":  LimitMax,
				"afterItems":  nil,
				"firstFields": 2,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"projectV2": map[string]any{
						"fields": map[string]any{
							"nodes": []map[string]any{
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
		JSON(map[string]any{
			"query": "query UserProject.*",
			"variables": map[string]any{
				"login":       "monalisa",
				"number":      1,
				"firstItems":  LimitMax,
				"afterItems":  nil,
				"firstFields": LimitMax,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"projectV2": map[string]any{
						"fields": map[string]any{
							"nodes": []map[string]any{
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
		JSON(map[string]any{
			"query": "query UserProject.*",
			"variables": map[string]any{
				"login":       "monalisa",
				"number":      1,
				"firstItems":  LimitMax,
				"afterItems":  nil,
				"firstFields": LimitDefault,
				"afterFields": nil,
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"projectV2": map[string]any{
						"fields": map[string]any{
							"nodes": []map[string]any{
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
	_, err := client.NewProject(false, &Owner{}, 0, false, nil)
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
		JSON(map[string]any{
			"query": "query UserProjectWithItems.*",
			"variables": map[string]any{
				"firstItems":  LimitMax,
				"afterItems":  nil,
				"firstFields": LimitMax,
				"afterFields": nil,
				"login":       "monalisa",
				"number":      1,
			},
		}).
		Reply(200).
		JSON(map[string]any{
			"data": map[string]any{
				"user": map[string]any{
					"projectV2": map[string]any{
						"items": map[string]any{
							"nodes": []map[string]any{
								{
									"id": "draft issue ID",
									"fieldValues": map[string]any{
										"nodes": []map[string]any{
											{
												"__typename":  "ProjectV2ItemFieldIterationValue",
												"title":       "Iteration Title 1",
												"iterationId": "iterationId1",
											},
											{
												"__typename": "ProjectV2ItemFieldMilestoneValue",
												"milestone": map[string]any{
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

func TestFieldValueNodesDisplayValue(t *testing.T) {
	textValue := func(text string) FieldValueNodes {
		v := FieldValueNodes{Type: "ProjectV2ItemFieldTextValue"}
		v.ProjectV2ItemFieldTextValue.Text = text
		return v
	}

	numberValue := FieldValueNodes{Type: "ProjectV2ItemFieldNumberValue"}
	numberValue.ProjectV2ItemFieldNumberValue.Number = 12

	singleSelectValue := FieldValueNodes{Type: "ProjectV2ItemFieldSingleSelectValue"}
	singleSelectValue.ProjectV2ItemFieldSingleSelectValue.Name = "In Progress"

	labelValue := FieldValueNodes{Type: "ProjectV2ItemFieldLabelValue"}
	labelValue.ProjectV2ItemFieldLabelValue.Labels.Nodes = []struct {
		Name string
	}{
		{Name: "bug"},
		{Name: "help wanted"},
	}

	dateValue := FieldValueNodes{Type: "ProjectV2ItemFieldDateValue"}
	dateValue.ProjectV2ItemFieldDateValue.Date = "2022-05-01"

	iterationValue := FieldValueNodes{Type: "ProjectV2ItemFieldIterationValue"}
	iterationValue.ProjectV2ItemFieldIterationValue.Title = "Sprint 1"

	milestoneValue := FieldValueNodes{Type: "ProjectV2ItemFieldMilestoneValue"}
	milestoneValue.ProjectV2ItemFieldMilestoneValue.Milestone.Title = "v1.0"

	pullRequestValue := FieldValueNodes{Type: "ProjectV2ItemFieldPullRequestValue"}
	pullRequestValue.ProjectV2ItemFieldPullRequestValue.PullRequests.Nodes = []struct {
		Url string
	}{
		{Url: "https://github.com/cli/cli/pull/1"},
		{Url: "https://github.com/cli/cli/pull/2"},
	}

	repositoryValue := FieldValueNodes{Type: "ProjectV2ItemFieldRepositoryValue"}
	repositoryValue.ProjectV2ItemFieldRepositoryValue.Repository.Url = "https://github.com/cli/cli"

	userValue := FieldValueNodes{Type: "ProjectV2ItemFieldUserValue"}
	userValue.ProjectV2ItemFieldUserValue.Users.Nodes = []struct {
		Login string
	}{
		{Login: "monalisa"},
		{Login: "hubot"},
	}

	reviewerValue := FieldValueNodes{Type: "ProjectV2ItemFieldReviewerValue"}
	reviewerValue.ProjectV2ItemFieldReviewerValue.Reviewers.Nodes = []struct {
		Type string `graphql:"__typename"`
		Team struct {
			Name string
		} `graphql:"... on Team"`
		User struct {
			Login string
		} `graphql:"... on User"`
	}{
		{
			Type: "User",
			User: struct {
				Login string
			}{Login: "monalisa"},
		},
		{
			Type: "Team",
			Team: struct {
				Name string
			}{Name: "octocat-team"},
		},
	}

	tests := []struct {
		name  string
		value FieldValueNodes
		want  string
	}{
		{
			name:  "empty when field has no value",
			value: FieldValueNodes{Type: "ProjectV2ItemFieldTextValue"},
			want:  "",
		},
		{
			name:  "unknown field type",
			value: FieldValueNodes{Type: "SomethingElse"},
			want:  "",
		},
		{
			name:  "single-line text",
			value: textValue("hello world"),
			want:  "hello world",
		},
		{
			name:  "multi-line text collapses newlines to spaces",
			value: textValue("first line\nsecond line"),
			want:  "first line second line",
		},
		{
			name:  "CRLF text collapses to a single space",
			value: textValue("first line\r\nsecond line"),
			want:  "first line second line",
		},
		{
			name:  "leading and trailing newlines",
			value: textValue("\nwrapped\n"),
			want:  " wrapped ",
		},
		{
			name:  "number",
			value: numberValue,
			want:  "12",
		},
		{
			name:  "single select",
			value: singleSelectValue,
			want:  "In Progress",
		},
		{
			name:  "labels joined with commas",
			value: labelValue,
			want:  "bug, help wanted",
		},
		{
			name:  "date",
			value: dateValue,
			want:  "2022-05-01",
		},
		{
			name:  "iteration title",
			value: iterationValue,
			want:  "Sprint 1",
		},
		{
			name:  "milestone title",
			value: milestoneValue,
			want:  "v1.0",
		},
		{
			name:  "pull requests joined with commas",
			value: pullRequestValue,
			want:  "https://github.com/cli/cli/pull/1, https://github.com/cli/cli/pull/2",
		},
		{
			name:  "repository url",
			value: repositoryValue,
			want:  "https://github.com/cli/cli",
		},
		{
			name:  "users joined with commas",
			value: userValue,
			want:  "monalisa, hubot",
		},
		{
			name:  "reviewers joined with commas for users and teams",
			value: reviewerValue,
			want:  "monalisa, octocat-team",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.value.DisplayValue())
		})
	}
}

func TestSingleLineFieldValue(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "no line breaks", in: "plain", want: "plain"},
		{name: "newline becomes space", in: "a\nb", want: "a b"},
		{name: "CRLF becomes a single space", in: "a\r\nb", want: "a b"},
		{name: "lone carriage return is dropped", in: "a\rb", want: "ab"},
		{name: "consecutive newlines", in: "a\n\nb", want: "a  b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, singleLineFieldValue(tt.in))
		})
	}
}
