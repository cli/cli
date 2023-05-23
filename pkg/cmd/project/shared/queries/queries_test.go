package queries

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

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
	project, err := client.ProjectItems(owner, 1, LimitMax)
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
	project, err := client.ProjectItems(owner, 1, 2)
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
	project, err := client.ProjectItems(owner, 1, 0)
	assert.NoError(t, err)
	assert.Len(t, project.Items.Nodes, 3)
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
