package api

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoIssueFields(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.GraphQL(`query RepositoryIssueFields\b`),
		httpmock.GraphQLQuery(`{
			"data": {"repository": {"issueFields": {
				"nodes": [{"id":"IF_1","name":"Priority","dataType":"SINGLE_SELECT","options":[{"id":"OPT_1","name":"High"}]}],
				"pageInfo": {"hasNextPage": true, "endCursor": "CURSOR_1"}
			}}}
		}`, func(_ string, variables map[string]interface{}) {
			assert.Equal(t, map[string]interface{}{
				"owner": "OWNER", "name": "REPO",
			}, variables)
		}),
	)
	reg.Register(
		httpmock.GraphQL(`query RepositoryIssueFields\b`),
		httpmock.GraphQLQuery(`{
			"data": {"repository": {"issueFields": {
				"nodes": [{"id":"IF_2","name":"Due date","dataType":"DATE"}],
				"pageInfo": {"hasNextPage": false, "endCursor": "CURSOR_2"}
			}}}
		}`, func(_ string, variables map[string]interface{}) {
			assert.Equal(t, map[string]interface{}{
				"owner": "OWNER", "name": "REPO", "endCursor": "CURSOR_1",
			}, variables)
		}),
	)

	client := NewClientFromHTTP(&http.Client{Transport: reg})
	fields, err := RepoIssueFields(client, ghrepo.New("OWNER", "REPO"), true)
	require.NoError(t, err)
	require.Len(t, fields, 2)
	assert.Equal(t, "Priority", fields[0].Name)
	assert.Equal(t, []IssueFieldOption{{ID: "OPT_1", Name: "High"}}, fields[0].Options)
	assert.Equal(t, "Due date", fields[1].Name)
}

func TestRepoIssueFieldsWithoutMultiSelect(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.GraphQL(`query RepositoryIssueFields\b`),
		httpmock.GraphQLQuery(`{"data":{"repository":{"issueFields":{"nodes":[],"pageInfo":{"hasNextPage":false}}}}}`, func(query string, _ map[string]interface{}) {
			assert.NotContains(t, query, "IssueFieldMultiSelect")
		}),
	)

	client := NewClientFromHTTP(&http.Client{Transport: reg})
	fields, err := RepoIssueFields(client, ghrepo.New("OWNER", "REPO"), false)
	require.NoError(t, err)
	assert.Empty(t, fields)
}
