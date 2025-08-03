package shared

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/require"
)

func TestRequestableReviewersForCompletion(t *testing.T) {
	tests := []struct {
		name              string
		expectedReviewers []string
		httpStubs         func(*httpmock.Registry, *testing.T)
	}{
		{
			name:              "when users and teams are both available, both are listed",
			expectedReviewers: []string{"MonaLisa\tMona Display Name", "OWNER/core", "OWNER/robots", "hubot"},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data": {"viewer": {"login": "OWNER"} } }`))
				reg.Register(
					httpmock.GraphQL(`query RepositoryAssignableUsers\b`),
					httpmock.StringResponse(`
						{ "data": { "repository": { "assignableUsers": {
							"nodes": [
								{ "login": "hubot", "id": "HUBOTID", "name": "" },
								{ "login": "MonaLisa", "id": "MONAID", "name": "Mona Display Name" }
							],
							"pageInfo": { "hasNextPage": false }
						} } } }
					`))
				reg.Register(
					httpmock.GraphQL(`query OrganizationTeamList\b`),
					httpmock.StringResponse(`
						{ "data": { "organization": { "teams": {
							"nodes": [
								{ "slug": "core", "id": "COREID" },
								{ "slug": "robots", "id": "ROBOTID" }
							],
							"pageInfo": { "hasNextPage": false }
						} } } }
					`))
			},
		},
		{
			name:              "when users are available but teams aren't, users are listed",
			expectedReviewers: []string{"MonaLisa\tMona Display Name", "hubot"},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data": {"viewer": {"login": "OWNER"} } }`))
				reg.Register(
					httpmock.GraphQL(`query RepositoryAssignableUsers\b`),
					httpmock.StringResponse(`
						{ "data": { "repository": { "assignableUsers": {
							"nodes": [
								{ "login": "hubot", "id": "HUBOTID", "name": "" },
								{ "login": "MonaLisa", "id": "MONAID", "name": "Mona Display Name" }
							],
							"pageInfo": { "hasNextPage": false }
						} } } }
					`))
				reg.Register(
					httpmock.GraphQL(`query OrganizationTeamList\b`),
					httpmock.StringResponse(`
						{ "data": { "organization": { "teams": {
							"nodes": [],
							"pageInfo": { "hasNextPage": false }
						} } } }
					`))
			},
		},
		{
			name:              "when teams are available but users aren't, teams are listed",
			expectedReviewers: []string{"OWNER/core", "OWNER/robots"},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data": {"viewer": {"login": "OWNER"} } }`))
				reg.Register(
					httpmock.GraphQL(`query RepositoryAssignableUsers\b`),
					httpmock.StringResponse(`
						{ "data": { "repository": { "assignableUsers": {
							"nodes": [],
							"pageInfo": { "hasNextPage": false }
						} } } }
					`))
				reg.Register(
					httpmock.GraphQL(`query OrganizationTeamList\b`),
					httpmock.StringResponse(`
						{ "data": { "organization": { "teams": {
							"nodes": [
								{ "slug": "core", "id": "COREID" },
								{ "slug": "robots", "id": "ROBOTID" }
							],
							"pageInfo": { "hasNextPage": false }
						} } } }
					`))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg, t)
			}

			reviewers, err := RequestableReviewersForCompletion(&http.Client{Transport: reg}, ghrepo.New("OWNER", "REPO"))
			require.NoError(t, err)
			require.Equal(t, tt.expectedReviewers, reviewers)
		})
	}
}
