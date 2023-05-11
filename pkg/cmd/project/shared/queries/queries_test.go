package queries

import (
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
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

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	owner := &Owner{
		Type:  "USER",
		Login: "monalisa",
		ID:    "user ID",
	}
	project, err := ProjectItems(client, owner, 1, LimitMax)
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

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	owner := &Owner{
		Type:  "USER",
		Login: "monalisa",
		ID:    "user ID",
	}
	project, err := ProjectItems(client, owner, 1, 2)
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

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	owner := &Owner{
		Type:  "USER",
		Login: "monalisa",
		ID:    "user ID",
	}
	project, err := ProjectItems(client, owner, 1, 0)
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

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	owner := &Owner{
		Type:  "USER",
		Login: "monalisa",
		ID:    "user ID",
	}
	project, err := ProjectFields(client, owner, 1, 2)
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

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	owner := &Owner{
		Type:  "USER",
		Login: "monalisa",
		ID:    "user ID",
	}
	project, err := ProjectFields(client, owner, 1, LimitMax)
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

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	owner := &Owner{
		Type:  "USER",
		Login: "monalisa",
		ID:    "user ID",
	}
	project, err := ProjectFields(client, owner, 1, 0)
	assert.NoError(t, err)
	assert.Len(t, project.Fields.Nodes, 3)
}
