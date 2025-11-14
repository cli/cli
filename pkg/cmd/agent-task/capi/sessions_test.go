package capi

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListLatestSessionsForViewer(t *testing.T) {
	sampleDateString := "2025-08-29T00:00:00Z"
	sampleDate, err := time.Parse(time.RFC3339, sampleDateString)
	require.NoError(t, err)

	tests := []struct {
		name      string
		perPage   int
		limit     int
		httpStubs func(*testing.T, *httpmock.Registry)
		wantErr   string
		wantOut   []*Session
	}{
		{
			name:    "zero limit",
			limit:   0,
			wantOut: nil,
		},
		{
			name:  "no sessions",
			limit: 10,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(
						httpmock.QueryMatcher("GET", "agents/sessions", url.Values{
							"page_number": {"1"},
							"page_size":   {"50"},
						}),
						"api.githubcopilot.com",
					),
					httpmock.StringResponse(`{"sessions":[]}`),
				)
			},
			wantOut: nil,
		},
		{
			name:  "single session",
			limit: 10,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(
						httpmock.QueryMatcher("GET", "agents/sessions", url.Values{
							"page_number": {"1"},
							"page_size":   {"50"},
						}),
						"api.githubcopilot.com",
					),
					httpmock.StringResponse(heredoc.Docf(`
						{
							"sessions": [
								{
									"id": "sess1",
									"name": "Build artifacts",
									"user_id": 1,
									"agent_id": 2,
									"logs": "",
									"state": "completed",
									"owner_id": 10,
									"repo_id": 1000,
									"resource_type": "pull",
									"resource_id": 2000,
									"created_at": "%[1]s",
									"premium_requests": 0.1
								}
							]
						}`,
						sampleDateString,
					)),
				)
				// GraphQL hydration
				reg.Register(
					httpmock.GraphQL(`query FetchPRsAndUsersForAgentTaskSessions\b`),
					httpmock.GraphQLQuery(heredoc.Docf(`
						{
							"data": {
								"nodes": [
									{
										"__typename": "PullRequest",
										"id": "PR_node",
										"fullDatabaseId": "2000",
										"number": 42,
										"title": "Improve docs",
										"state": "OPEN",
										"isDraft": true,
										"url": "https://github.com/OWNER/REPO/pull/42",
										"body": "",
										"createdAt": "%[1]s",
										"updatedAt": "%[1]s",
										"repository": {
											"nameWithOwner": "OWNER/REPO"
										}
									},
									{
										"__typename": "User",
										"login": "octocat",
										"name": "Octocat",
										"databaseId": 1
									}
								]
							}
						}`,
						sampleDateString,
					), func(q string, vars map[string]interface{}) {
						assert.Equal(t, []interface{}{"PR_kwDNA-jNB9A", "U_kgAB"}, vars["ids"])
					}),
				)
			},
			wantOut: []*Session{
				{

					ID:              "sess1",
					Name:            "Build artifacts",
					UserID:          1,
					AgentID:         2,
					Logs:            "",
					State:           "completed",
					OwnerID:         10,
					RepoID:          1000,
					ResourceType:    "pull",
					ResourceID:      2000,
					CreatedAt:       sampleDate,
					PremiumRequests: 0.1,
					PullRequest: &api.PullRequest{
						ID:             "PR_node",
						FullDatabaseID: "2000",
						Number:         42,
						Title:          "Improve docs",
						State:          "OPEN",
						IsDraft:        true,
						URL:            "https://github.com/OWNER/REPO/pull/42",
						Body:           "",
						CreatedAt:      sampleDate,
						UpdatedAt:      sampleDate,
						Repository: &api.PRRepository{
							NameWithOwner: "OWNER/REPO",
						},
					},
					User: &api.GitHubUser{
						Login:      "octocat",
						Name:       "Octocat",
						DatabaseID: 1,
					},
				},
			},
		},
		{
			// This happens at the early moments of a session lifecycle, before a PR is created and associated with it.
			name:  "single session, no pull request resource",
			limit: 10,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(
						httpmock.QueryMatcher("GET", "agents/sessions", url.Values{
							"page_number": {"1"},
							"page_size":   {"50"},
						}),
						"api.githubcopilot.com",
					),
					httpmock.StringResponse(heredoc.Docf(`
						{
							"sessions": [
								{
									"id": "sess1",
									"name": "Build artifacts",
									"user_id": 1,
									"agent_id": 2,
									"logs": "",
									"state": "completed",
									"owner_id": 10,
									"repo_id": 1000,
									"resource_type": "",
									"resource_id": 0,
									"created_at": "%[1]s",
									"premium_requests": 0.1
								}
							]
						}`,
						sampleDateString,
					)),
				)
				// GraphQL hydration
				reg.Register(
					httpmock.GraphQL(`query FetchPRsAndUsersForAgentTaskSessions\b`),
					httpmock.GraphQLQuery(heredoc.Docf(`
						{
							"data": {
								"nodes": [
									{
										"__typename": "User",
										"login": "octocat",
										"name": "Octocat",
										"databaseId": 1
									}
								]
							}
						}`,
						sampleDateString,
					), func(q string, vars map[string]interface{}) {
						assert.Equal(t, []interface{}{"U_kgAB"}, vars["ids"])
					}),
				)
			},
			wantOut: []*Session{
				{

					ID:              "sess1",
					Name:            "Build artifacts",
					UserID:          1,
					AgentID:         2,
					Logs:            "",
					State:           "completed",
					OwnerID:         10,
					RepoID:          1000,
					ResourceType:    "",
					ResourceID:      0,
					CreatedAt:       sampleDate,
					PremiumRequests: 0.1,
					User: &api.GitHubUser{
						Login:      "octocat",
						Name:       "Octocat",
						DatabaseID: 1,
					},
				},
			},
		},
		{
			name:    "multiple sessions, paginated",
			perPage: 1, // to enforce pagination
			limit:   2,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(
						httpmock.QueryMatcher("GET", "agents/sessions", url.Values{
							"page_number": {"1"},
							"page_size":   {"1"},
						}),
						"api.githubcopilot.com",
					),
					httpmock.StringResponse(heredoc.Docf(`
						{
							"sessions": [
								{
									"id": "sess1",
									"name": "Build artifacts",
									"user_id": 1,
									"agent_id": 2,
									"logs": "",
									"state": "completed",
									"owner_id": 10,
									"repo_id": 1000,
									"resource_type": "pull",
									"resource_id": 2000,
									"created_at": "%[1]s",
									"premium_requests": 0.1
								}
							]
						}`,
						sampleDateString,
					)),
				)

				// Second page
				reg.Register(
					httpmock.WithHost(
						httpmock.QueryMatcher("GET", "agents/sessions", url.Values{
							"page_number": {"2"},
							"page_size":   {"1"},
						}),
						"api.githubcopilot.com",
					),
					httpmock.StringResponse(heredoc.Docf(`
						{
							"sessions": [
								{
									"id": "sess2",
									"name": "Build artifacts",
									"user_id": 1,
									"agent_id": 2,
									"logs": "",
									"state": "completed",
									"owner_id": 10,
									"repo_id": 1000,
									"resource_type": "pull",
									"resource_id": 2001,
									"created_at": "%[1]s",
									"premium_requests": 0.1
								}
							]
						}`,
						sampleDateString,
					)),
				)
				// GraphQL hydration
				reg.Register(
					httpmock.GraphQL(`query FetchPRsAndUsersForAgentTaskSessions\b`),
					httpmock.GraphQLQuery(heredoc.Docf(`
						{
							"data": {
								"nodes": [
									{
										"__typename": "PullRequest",
										"id": "PR_node",
										"fullDatabaseId": "2000",
										"number": 42,
										"title": "Improve docs",
										"state": "OPEN",
										"isDraft": true,
										"url": "https://github.com/OWNER/REPO/pull/42",
										"body": "",
										"createdAt": "%[1]s",
										"updatedAt": "%[1]s",
										"repository": {
											"nameWithOwner": "OWNER/REPO"
										}
									},
									{
										"__typename": "PullRequest",
										"id": "PR_node",
										"fullDatabaseId": "2001",
										"number": 43,
										"title": "Improve docs",
										"state": "OPEN",
										"isDraft": true,
										"url": "https://github.com/OWNER/REPO/pull/43",
										"body": "",
										"createdAt": "%[1]s",
										"updatedAt": "%[1]s",
										"repository": {
											"nameWithOwner": "OWNER/REPO"
										}
									},
									{
										"__typename": "User",
										"login": "octocat",
										"name": "Octocat",
										"databaseId": 1
									}
								]
							}
						}`,
						sampleDateString,
					), func(q string, vars map[string]interface{}) {
						assert.Equal(t, []interface{}{"PR_kwDNA-jNB9A", "PR_kwDNA-jNB9E", "U_kgAB"}, vars["ids"])
					}),
				)
			},
			wantOut: []*Session{
				{
					ID:              "sess1",
					Name:            "Build artifacts",
					UserID:          1,
					AgentID:         2,
					Logs:            "",
					State:           "completed",
					OwnerID:         10,
					RepoID:          1000,
					ResourceType:    "pull",
					ResourceID:      2000,
					CreatedAt:       sampleDate,
					PremiumRequests: 0.1,
					PullRequest: &api.PullRequest{
						ID:             "PR_node",
						FullDatabaseID: "2000",
						Number:         42,
						Title:          "Improve docs",
						State:          "OPEN",
						IsDraft:        true,
						URL:            "https://github.com/OWNER/REPO/pull/42",
						Body:           "",
						CreatedAt:      sampleDate,
						UpdatedAt:      sampleDate,
						Repository: &api.PRRepository{
							NameWithOwner: "OWNER/REPO",
						},
					},
					User: &api.GitHubUser{
						Login:      "octocat",
						Name:       "Octocat",
						DatabaseID: 1,
					},
				},
				{
					ID:              "sess2",
					Name:            "Build artifacts",
					UserID:          1,
					AgentID:         2,
					Logs:            "",
					State:           "completed",
					OwnerID:         10,
					RepoID:          1000,
					ResourceType:    "pull",
					ResourceID:      2001,
					CreatedAt:       sampleDate,
					PremiumRequests: 0.1,
					PullRequest: &api.PullRequest{
						ID:             "PR_node",
						FullDatabaseID: "2001",
						Number:         43,
						Title:          "Improve docs",
						State:          "OPEN",
						IsDraft:        true,
						URL:            "https://github.com/OWNER/REPO/pull/43",
						Body:           "",
						CreatedAt:      sampleDate,
						UpdatedAt:      sampleDate,
						Repository: &api.PRRepository{
							NameWithOwner: "OWNER/REPO",
						},
					},
					User: &api.GitHubUser{
						Login:      "octocat",
						Name:       "Octocat",
						DatabaseID: 1,
					},
				},
			},
		},
		{
			name:    "multiple pages with duplicates per PR only newest kept",
			perPage: 2,
			limit:   3,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				// Page 1 returns newest sessions (ordered newest first overall)
				reg.Register(
					httpmock.WithHost(
						httpmock.QueryMatcher("GET", "agents/sessions", url.Values{
							"page_number": {"1"},
							"page_size":   {"2"},
							"sort":        {"last_updated_at,desc"},
						}),
						"api.githubcopilot.com",
					),
					httpmock.StringResponse(heredoc.Docf(`
						{
							"sessions": [
								{
									"id": "sessA-new",
									"name": "Build artifacts",
									"user_id": 1,
									"agent_id": 2,
									"logs": "",
									"state": "completed",
									"owner_id": 10,
									"repo_id": 1000,
									"resource_type": "pull",
									"resource_id": 3000,
									"created_at": "%[1]s"
								},
								{
									"id": "sessB-new",
									"name": "Build artifacts",
									"user_id": 1,
									"agent_id": 2,
									"logs": "",
									"state": "completed",
									"owner_id": 10,
									"repo_id": 1000,
									"resource_type": "pull",
									"resource_id": 3001,
									"created_at": "%[1]s"
								}
							]
						}`,
						sampleDateString,
					)),
				)

				// Page 2 returns older duplicate sessions for 3000, plus another new PR 3002
				reg.Register(
					httpmock.WithHost(
						httpmock.QueryMatcher("GET", "agents/sessions", url.Values{
							"page_number": {"2"},
							"page_size":   {"2"},
							"sort":        {"last_updated_at,desc"},
						}),
						"api.githubcopilot.com",
					),
					httpmock.StringResponse(heredoc.Docf(`
						{
							"sessions": [
								{
									"id": "sessA-old",
									"name": "Build artifacts",
									"user_id": 1,
									"agent_id": 2,
									"logs": "",
									"state": "completed",
									"owner_id": 10,
									"repo_id": 1000,
									"resource_type": "pull",
									"resource_id": 3000,
									"created_at": "%[1]s"
								},
								{
									"id": "sessC-new",
									"name": "Build artifacts",
									"user_id": 1,
									"agent_id": 2,
									"logs": "",
									"state": "completed",
									"owner_id": 10,
									"repo_id": 1000,
									"resource_type": "pull",
									"resource_id": 3002,
									"created_at": "%[1]s"
								}
							]
						}`,
						sampleDateString,
					)),
				)

				// GraphQL hydration for PRs 3000, 3001, 3002 and user 1
				reg.Register(
					httpmock.GraphQL(`query FetchPRsAndUsersForAgentTaskSessions\b`),
					httpmock.GraphQLQuery(heredoc.Docf(`
						{
							"data": {
								"nodes": [
									{
										"__typename": "PullRequest",
										"id": "PR_node3000",
										"fullDatabaseId": "3000",
										"number": 100,
										"title": "Improve docs",
										"state": "OPEN",
										"isDraft": true,
										"url": "https://github.com/OWNER/REPO/pull/100",
										"body": "",
										"createdAt": "%[1]s",
										"updatedAt": "%[1]s",
										"repository": {"nameWithOwner": "OWNER/REPO"}
									},
									{
										"__typename": "PullRequest",
										"id": "PR_node3001",
										"fullDatabaseId": "3001",
										"number": 101,
										"title": "Improve docs",
										"state": "OPEN",
										"isDraft": true,
										"url": "https://github.com/OWNER/REPO/pull/101",
										"body": "",
										"createdAt": "%[1]s",
										"updatedAt": "%[1]s",
										"repository": {"nameWithOwner": "OWNER/REPO"}
									},
									{
										"__typename": "PullRequest",
										"id": "PR_node3002",
										"fullDatabaseId": "3002",
										"number": 102,
										"title": "Improve docs",
										"state": "OPEN",
										"isDraft": true,
										"url": "https://github.com/OWNER/REPO/pull/102",
										"body": "",
										"createdAt": "%[1]s",
										"updatedAt": "%[1]s",
										"repository": {"nameWithOwner": "OWNER/REPO"}
									},
									{
										"__typename": "User",
										"login": "octocat",
										"name": "Octocat",
										"databaseId": 1
									}
								]
							}
						}`,
						sampleDateString,
					), func(q string, vars map[string]interface{}) {
						// Expected encoded node IDs for resource IDs 3000,3001,3002 and user octocat
						assert.Equal(t, []interface{}{"PR_kwDNA-jNC7g", "PR_kwDNA-jNC7k", "PR_kwDNA-jNC7o", "U_kgAB"}, vars["ids"])
					}),
				)
			},
			wantOut: []*Session{
				{
					ID:           "sessA-new",
					Name:         "Build artifacts",
					UserID:       1,
					AgentID:      2,
					Logs:         "",
					State:        "completed",
					OwnerID:      10,
					RepoID:       1000,
					ResourceType: "pull",
					ResourceID:   3000,
					CreatedAt:    sampleDate,
					PullRequest: &api.PullRequest{
						ID:             "PR_node3000",
						FullDatabaseID: "3000",
						Number:         100,
						Title:          "Improve docs",
						State:          "OPEN",
						IsDraft:        true,
						URL:            "https://github.com/OWNER/REPO/pull/100",
						Body:           "",
						CreatedAt:      sampleDate,
						UpdatedAt:      sampleDate,
						Repository: &api.PRRepository{
							NameWithOwner: "OWNER/REPO",
						},
					},
					User: &api.GitHubUser{Login: "octocat", Name: "Octocat", DatabaseID: 1},
				},
				{
					ID:           "sessB-new",
					Name:         "Build artifacts",
					UserID:       1,
					AgentID:      2,
					Logs:         "",
					State:        "completed",
					OwnerID:      10,
					RepoID:       1000,
					ResourceType: "pull",
					ResourceID:   3001,
					CreatedAt:    sampleDate,
					PullRequest: &api.PullRequest{
						ID:             "PR_node3001",
						FullDatabaseID: "3001",
						Number:         101,
						Title:          "Improve docs",
						State:          "OPEN",
						IsDraft:        true,
						URL:            "https://github.com/OWNER/REPO/pull/101",
						Body:           "",
						CreatedAt:      sampleDate,
						UpdatedAt:      sampleDate,
						Repository: &api.PRRepository{
							NameWithOwner: "OWNER/REPO",
						},
					},
					User: &api.GitHubUser{Login: "octocat", Name: "Octocat", DatabaseID: 1},
				},
				{
					ID:           "sessC-new",
					Name:         "Build artifacts",
					UserID:       1,
					AgentID:      2,
					Logs:         "",
					State:        "completed",
					OwnerID:      10,
					RepoID:       1000,
					ResourceType: "pull",
					ResourceID:   3002,
					CreatedAt:    sampleDate,
					PullRequest: &api.PullRequest{
						ID:             "PR_node3002",
						FullDatabaseID: "3002",
						Number:         102,
						Title:          "Improve docs",
						State:          "OPEN",
						IsDraft:        true,
						URL:            "https://github.com/OWNER/REPO/pull/102",
						Body:           "",
						CreatedAt:      sampleDate,
						UpdatedAt:      sampleDate,
						Repository: &api.PRRepository{
							NameWithOwner: "OWNER/REPO",
						},
					},
					User: &api.GitHubUser{Login: "octocat", Name: "Octocat", DatabaseID: 1},
				},
			},
		},
		{
			name:    "multiple pages with zero resource IDs all kept",
			perPage: 2,
			limit:   3,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				// Page 1 returns newest sessions, one with a zero resource ID
				reg.Register(
					httpmock.WithHost(
						httpmock.QueryMatcher("GET", "agents/sessions", url.Values{
							"page_number": {"1"},
							"page_size":   {"2"},
							"sort":        {"last_updated_at,desc"},
						}),
						"api.githubcopilot.com",
					),
					httpmock.StringResponse(heredoc.Docf(`
						{
							"sessions": [
								{
									"id": "sessA-new",
									"name": "Build artifacts",
									"user_id": 1,
									"agent_id": 2,
									"logs": "",
									"state": "completed",
									"owner_id": 10,
									"repo_id": 1000,
									"resource_type": "pull",
									"resource_id": 3000,
									"created_at": "%[1]s"
								},
								{
									"id": "sessB-new",
									"name": "Build artifacts",
									"user_id": 1,
									"agent_id": 2,
									"logs": "",
									"state": "queued",
									"owner_id": 10,
									"repo_id": 1000,
									"resource_type": "",
									"resource_id": 0,
									"created_at": "%[1]s"
								}
							]
						}`,
						sampleDateString,
					)),
				)

				// Page 2 returns older duplicate sessions for 3000, plus another new session with zero resource ID
				reg.Register(
					httpmock.WithHost(
						httpmock.QueryMatcher("GET", "agents/sessions", url.Values{
							"page_number": {"2"},
							"page_size":   {"2"},
							"sort":        {"last_updated_at,desc"},
						}),
						"api.githubcopilot.com",
					),
					httpmock.StringResponse(heredoc.Docf(`
						{
							"sessions": [
								{
									"id": "sessA-old",
									"name": "Build artifacts",
									"user_id": 1,
									"agent_id": 2,
									"logs": "",
									"state": "completed",
									"owner_id": 10,
									"repo_id": 1000,
									"resource_type": "pull",
									"resource_id": 3000,
									"created_at": "%[1]s"
								},
								{
									"id": "sessC-new",
									"name": "Build artifacts",
									"user_id": 1,
									"agent_id": 2,
									"logs": "",
									"state": "queued",
									"owner_id": 10,
									"repo_id": 1000,
									"resource_type": "",
									"resource_id": 0,
									"created_at": "%[1]s"
								}
							]
						}`,
						sampleDateString,
					)),
				)

				// GraphQL hydration for PRs 3000 and user 1
				reg.Register(
					httpmock.GraphQL(`query FetchPRsAndUsersForAgentTaskSessions\b`),
					httpmock.GraphQLQuery(heredoc.Docf(`
						{
							"data": {
								"nodes": [
									{
										"__typename": "PullRequest",
										"id": "PR_node3000",
										"fullDatabaseId": "3000",
										"number": 100,
										"title": "Improve docs",
										"state": "OPEN",
										"isDraft": true,
										"url": "https://github.com/OWNER/REPO/pull/100",
										"body": "",
										"createdAt": "%[1]s",
										"updatedAt": "%[1]s",
										"repository": {"nameWithOwner": "OWNER/REPO"}
									},
									{
										"__typename": "User",
										"login": "octocat",
										"name": "Octocat",
										"databaseId": 1
									}
								]
							}
						}`,
						sampleDateString,
					), func(q string, vars map[string]interface{}) {
						// Expected encoded node IDs for resource IDs 3000 and user octocat
						assert.Equal(t, []interface{}{"PR_kwDNA-jNC7g", "U_kgAB"}, vars["ids"])
					}),
				)
			},
			wantOut: []*Session{
				{
					ID:           "sessA-new",
					Name:         "Build artifacts",
					UserID:       1,
					AgentID:      2,
					Logs:         "",
					State:        "completed",
					OwnerID:      10,
					RepoID:       1000,
					ResourceType: "pull",
					ResourceID:   3000,
					CreatedAt:    sampleDate,
					PullRequest: &api.PullRequest{
						ID:             "PR_node3000",
						FullDatabaseID: "3000",
						Number:         100,
						Title:          "Improve docs",
						State:          "OPEN",
						IsDraft:        true,
						URL:            "https://github.com/OWNER/REPO/pull/100",
						Body:           "",
						CreatedAt:      sampleDate,
						UpdatedAt:      sampleDate,
						Repository: &api.PRRepository{
							NameWithOwner: "OWNER/REPO",
						},
					},
					User: &api.GitHubUser{Login: "octocat", Name: "Octocat", DatabaseID: 1},
				},
				{
					ID:           "sessB-new",
					Name:         "Build artifacts",
					UserID:       1,
					AgentID:      2,
					Logs:         "",
					State:        "queued",
					OwnerID:      10,
					RepoID:       1000,
					ResourceType: "",
					ResourceID:   0,
					CreatedAt:    sampleDate,
					User:         &api.GitHubUser{Login: "octocat", Name: "Octocat", DatabaseID: 1},
				},
				{
					ID:           "sessC-new",
					Name:         "Build artifacts",
					UserID:       1,
					AgentID:      2,
					Logs:         "",
					State:        "queued",
					OwnerID:      10,
					RepoID:       1000,
					ResourceType: "",
					ResourceID:   0,
					CreatedAt:    sampleDate,
					User:         &api.GitHubUser{Login: "octocat", Name: "Octocat", DatabaseID: 1},
				},
			},
		},
		{
			name:  "session error is included",
			limit: 10,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(
						httpmock.QueryMatcher("GET", "agents/sessions", url.Values{
							"page_number": {"1"},
							"page_size":   {"50"},
							"sort":        {"last_updated_at,desc"},
						}),
						"api.githubcopilot.com",
					),
					httpmock.StringResponse(heredoc.Docf(`
						{
							"sessions": [
								{
									"id": "sessA",
									"name": "Build artifacts",
									"user_id": 1,
									"agent_id": 2,
									"logs": "",
									"state": "failed",
									"owner_id": 10,
									"repo_id": 1000,
									"resource_type": "pull",
									"resource_id": 3000,
									"created_at": "%[1]s",
									"error": {
										"code": "some-error-code",
										"message": "some-error-message"
									}
								}
							]
						}`,
						sampleDateString,
					)),
				)

				// GraphQL hydration
				reg.Register(
					httpmock.GraphQL(`query FetchPRsAndUsersForAgentTaskSessions\b`),
					httpmock.GraphQLQuery(heredoc.Docf(`
						{
							"data": {
								"nodes": [
									{
										"__typename": "PullRequest",
										"id": "PR_node3000",
										"fullDatabaseId": "3000",
										"number": 100,
										"title": "Improve docs",
										"state": "OPEN",
										"isDraft": true,
										"url": "https://github.com/OWNER/REPO/pull/100",
										"body": "",
										"createdAt": "%[1]s",
										"updatedAt": "%[1]s",
										"repository": {"nameWithOwner": "OWNER/REPO"}
									},
									{
										"__typename": "User",
										"login": "octocat",
										"name": "Octocat",
										"databaseId": 1
									}
								]
							}
						}`,
						sampleDateString,
					), func(q string, vars map[string]interface{}) {
						// Expected encoded node IDs for resource IDs 3000 and user octocat
						assert.Equal(t, []interface{}{"PR_kwDNA-jNC7g", "U_kgAB"}, vars["ids"])
					}),
				)
			},
			wantOut: []*Session{
				{
					ID:           "sessA",
					Name:         "Build artifacts",
					UserID:       1,
					AgentID:      2,
					Logs:         "",
					State:        "failed",
					OwnerID:      10,
					RepoID:       1000,
					ResourceType: "pull",
					ResourceID:   3000,
					CreatedAt:    sampleDate,
					Error: &SessionError{
						Code:    "some-error-code",
						Message: "some-error-message",
					},
					PullRequest: &api.PullRequest{
						ID:             "PR_node3000",
						FullDatabaseID: "3000",
						Number:         100,
						Title:          "Improve docs",
						State:          "OPEN",
						IsDraft:        true,
						URL:            "https://github.com/OWNER/REPO/pull/100",
						Body:           "",
						CreatedAt:      sampleDate,
						UpdatedAt:      sampleDate,
						Repository: &api.PRRepository{
							NameWithOwner: "OWNER/REPO",
						},
					},
					User: &api.GitHubUser{Login: "octocat", Name: "Octocat", DatabaseID: 1},
				},
			},
		},
		{
			name:  "workflow run id is included",
			limit: 10,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(
						httpmock.QueryMatcher("GET", "agents/sessions", url.Values{
							"page_number": {"1"},
							"page_size":   {"50"},
							"sort":        {"last_updated_at,desc"},
						}),
						"api.githubcopilot.com",
					),
					httpmock.StringResponse(heredoc.Docf(`
						{
							"sessions": [
								{
									"id": "sessA",
									"name": "Build artifacts",
									"user_id": 1,
									"agent_id": 2,
									"logs": "",
									"state": "failed",
									"owner_id": 10,
									"repo_id": 1000,
									"resource_type": "pull",
									"resource_id": 3000,
									"created_at": "%[1]s",
									"workflow_run_id": 9999
								}
							]
						}`,
						sampleDateString,
					)),
				)

				// GraphQL hydration
				reg.Register(
					httpmock.GraphQL(`query FetchPRsAndUsersForAgentTaskSessions\b`),
					httpmock.GraphQLQuery(heredoc.Docf(`
						{
							"data": {
								"nodes": [
									{
										"__typename": "PullRequest",
										"id": "PR_node3000",
										"fullDatabaseId": "3000",
										"number": 100,
										"title": "Improve docs",
										"state": "OPEN",
										"isDraft": true,
										"url": "https://github.com/OWNER/REPO/pull/100",
										"body": "",
										"createdAt": "%[1]s",
										"updatedAt": "%[1]s",
										"repository": {"nameWithOwner": "OWNER/REPO"}
									},
									{
										"__typename": "User",
										"login": "octocat",
										"name": "Octocat",
										"databaseId": 1
									}
								]
							}
						}`,
						sampleDateString,
					), func(q string, vars map[string]interface{}) {
						// Expected encoded node IDs for resource IDs 3000 and user octocat
						assert.Equal(t, []interface{}{"PR_kwDNA-jNC7g", "U_kgAB"}, vars["ids"])
					}),
				)
			},
			wantOut: []*Session{
				{
					ID:            "sessA",
					Name:          "Build artifacts",
					UserID:        1,
					AgentID:       2,
					Logs:          "",
					State:         "failed",
					OwnerID:       10,
					RepoID:        1000,
					ResourceType:  "pull",
					ResourceID:    3000,
					CreatedAt:     sampleDate,
					WorkflowRunID: 9999,
					PullRequest: &api.PullRequest{
						ID:             "PR_node3000",
						FullDatabaseID: "3000",
						Number:         100,
						Title:          "Improve docs",
						State:          "OPEN",
						IsDraft:        true,
						URL:            "https://github.com/OWNER/REPO/pull/100",
						Body:           "",
						CreatedAt:      sampleDate,
						UpdatedAt:      sampleDate,
						Repository: &api.PRRepository{
							NameWithOwner: "OWNER/REPO",
						},
					},
					User: &api.GitHubUser{Login: "octocat", Name: "Octocat", DatabaseID: 1},
				},
			},
		},
		{
			name:  "API error",
			limit: 10,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(
						httpmock.QueryMatcher("GET", "agents/sessions", url.Values{
							"page_number": {"1"},
							"page_size":   {"50"},
						}),
						"api.githubcopilot.com",
					),
					httpmock.StatusStringResponse(500, "{}"),
				)
			},
			wantErr: "failed to list sessions:",
		}, {
			name:  "API error at hydration",
			limit: 10,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(
						httpmock.QueryMatcher("GET", "agents/sessions", url.Values{
							"page_number": {"1"},
							"page_size":   {"50"},
						}),
						"api.githubcopilot.com",
					),
					httpmock.StringResponse(heredoc.Docf(`
						{
							"sessions": [
								{
									"id": "sess1",
									"name": "Build artifacts",
									"user_id": 1,
									"agent_id": 2,
									"logs": "",
									"state": "completed",
									"owner_id": 10,
									"repo_id": 1000,
									"resource_type": "pull",
									"resource_id": 2000,
									"created_at": "%[1]s",
									"premium_requests": 0.1
								}
							]
						}`,
						sampleDateString,
					)),
				)
				// GraphQL hydration
				reg.Register(
					httpmock.GraphQL(`query FetchPRsAndUsersForAgentTaskSessions\b`),
					httpmock.StatusStringResponse(500, `{}`),
				)
			},
			wantErr: `failed to fetch session resources: non-200 OK status code:`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(t, reg)
			}
			defer reg.Verify(t)

			httpClient := &http.Client{Transport: reg}

			capiClient := NewCAPIClient(httpClient, "", "github.com")

			if tt.perPage != 0 {
				last := defaultSessionsPerPage
				defaultSessionsPerPage = tt.perPage
				defer func() {
					defaultSessionsPerPage = last
				}()
			}

			sessions, err := capiClient.ListLatestSessionsForViewer(context.Background(), tt.limit)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Nil(t, sessions)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantOut, sessions)
		})
	}
}

func TestListSessionsByResourceIDRequiresResource(t *testing.T) {
	client := &CAPIClient{}

	_, err := client.ListSessionsByResourceID(context.Background(), "", 999, 0)
	assert.EqualError(t, err, "missing resource type/ID")
	_, err = client.ListSessionsByResourceID(context.Background(), "only-resource-type", 0, 0)
	assert.EqualError(t, err, "missing resource type/ID")
	_, err = client.ListSessionsByResourceID(context.Background(), "", 0, 0)
	assert.EqualError(t, err, "missing resource type/ID")
}

func TestListSessionsByResourceID(t *testing.T) {
	sampleDateString := "2025-08-29T07:00:00Z"
	sampleDate, err := time.Parse(time.RFC3339, sampleDateString)
	require.NoError(t, err)
	sampleDateTimestamp := sampleDate.Unix()

	resourceID := int64(999)
	resourceType := "pull"

	tests := []struct {
		name      string
		perPage   int
		limit     int
		httpStubs func(*testing.T, *httpmock.Registry)
		wantErr   string
		wantOut   []*Session
	}{
		{
			name:    "zero limit",
			limit:   0,
			wantOut: nil,
		},
		{
			// If the given pull request does not exist or the pull request has no sessions,
			// the API endpoint returns 404 with different messages. We should treat them
			// the same though.
			name:  "no sessions or no pull request",
			limit: 10,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("GET", "agents/resource/pull/999"), "api.githubcopilot.com"),

					httpmock.StatusStringResponse(404, "{}"),
				)
			},
			wantErr: "failed to list sessions",
		},
		{
			name:  "single session",
			limit: 10,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("GET", "agents/resource/pull/999"), "api.githubcopilot.com"),
					httpmock.StringResponse(heredoc.Docf(`
						{
							"id": "resource:pull:2000",
							"user_id": 1,
							"resource_global_id": "PR_kwDNA-jNB9A",
							"resource_type": "pull",
							"resource_id": 2000,
							"session_count": 1,
							"last_updated_at": %[1]d,
							"state": "completed",
							"resource_state": "draft",
							"sessions": [
								{
									"id": "sess1",
									"name": "Build artifacts",
									"state": "completed",
									"last_updated_at": %[1]d
								}
							]
						}`,
						sampleDateTimestamp,
					)),
				)
				// GraphQL hydration
				reg.Register(
					httpmock.GraphQL(`query FetchPRsAndUsersForAgentTaskSessions\b`),
					httpmock.GraphQLQuery(heredoc.Docf(`
						{
							"data": {
								"nodes": [
									{
										"__typename": "PullRequest",
										"id": "PR_node",
										"fullDatabaseId": "2000",
										"number": 42,
										"title": "Improve docs",
										"state": "OPEN",
										"isDraft": true,
										"url": "https://github.com/OWNER/REPO/pull/42",
										"body": "",
										"createdAt": "%[1]s",
										"updatedAt": "%[1]s",
										"repository": {
											"nameWithOwner": "OWNER/REPO"
										}
									},
									{
										"__typename": "User",
										"login": "octocat",
										"name": "Octocat",
										"databaseId": 1
									}
								]
							}
						}`,
						sampleDateString,
					), func(q string, vars map[string]interface{}) {
						assert.Equal(t, []interface{}{"PR_kwDNA-jNB9A", "U_kgAB"}, vars["ids"])
					}),
				)
			},
			wantOut: []*Session{
				{
					ID:            "sess1",
					CreatedAt:     time.Time{},
					LastUpdatedAt: sampleDate,
					Name:          "Build artifacts",
					UserID:        1,
					State:         "completed",
					ResourceType:  "pull",
					ResourceID:    2000,
					PullRequest: &api.PullRequest{
						ID:             "PR_node",
						FullDatabaseID: "2000",
						Number:         42,
						Title:          "Improve docs",
						State:          "OPEN",
						IsDraft:        true,
						URL:            "https://github.com/OWNER/REPO/pull/42",
						Body:           "",
						CreatedAt:      sampleDate,
						UpdatedAt:      sampleDate,
						Repository: &api.PRRepository{
							NameWithOwner: "OWNER/REPO",
						},
					},
					User: &api.GitHubUser{
						Login:      "octocat",
						Name:       "Octocat",
						DatabaseID: 1,
					},
				},
			},
		},
		{
			name:    "multiple sessions",
			perPage: 1, // to enforce pagination
			limit:   2,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("GET", "agents/resource/pull/999"), "api.githubcopilot.com"),
					httpmock.StringResponse(heredoc.Docf(`
						{
							"id": "resource:pull:2000",
							"user_id": 1,
							"resource_global_id": "PR_kwDNA-jNB9A",
							"resource_type": "pull",
							"resource_id": 2000,
							"session_count": 1,
							"last_updated_at": %[1]d,
							"state": "completed",
							"resource_state": "draft",
							"sessions": [
								{
									"id": "sess1",
									"name": "Build artifacts",
									"user_id": 1,
									"agent_id": 2,
									"logs": "",
									"state": "completed",
									"owner_id": 10,
									"repo_id": 1000,
									"resource_type": "pull",
									"resource_id": 2000,
									"created_at": %[1]d,
									"premium_requests": 0.1
								},
								{
									"id": "sess2",
									"name": "Build artifacts",
									"user_id": 1,
									"agent_id": 2,
									"logs": "",
									"state": "completed",
									"owner_id": 10,
									"repo_id": 1000,
									"resource_type": "pull",
									"resource_id": 2001,
									"created_at": %[1]d,
									"premium_requests": 0.1
								}
							]
						}`,
						sampleDateTimestamp,
					)),
				)
				// GraphQL hydration
				reg.Register(
					httpmock.GraphQL(`query FetchPRsAndUsersForAgentTaskSessions\b`),
					httpmock.GraphQLQuery(heredoc.Docf(`
						{
							"data": {
								"nodes": [
									{
										"__typename": "PullRequest",
										"id": "PR_node",
										"fullDatabaseId": "2000",
										"number": 42,
										"title": "Improve docs",
										"state": "OPEN",
										"isDraft": true,
										"url": "https://github.com/OWNER/REPO/pull/42",
										"body": "",
										"createdAt": "%[1]s",
										"updatedAt": "%[1]s",
										"repository": {
											"nameWithOwner": "OWNER/REPO"
										}
									},
									{
										"__typename": "User",
										"login": "octocat",
										"name": "Octocat",
										"databaseId": 1
									}
								]
							}
						}`,
						sampleDateString,
					), func(q string, vars map[string]interface{}) {
						assert.Equal(t, []interface{}{"PR_kwDNA-jNB9A", "U_kgAB"}, vars["ids"])
					}),
				)
			},
			wantOut: []*Session{
				{
					ID:           "sess1",
					Name:         "Build artifacts",
					UserID:       1,
					State:        "completed",
					ResourceType: "pull",
					ResourceID:   2000,
					PullRequest: &api.PullRequest{
						ID:             "PR_node",
						FullDatabaseID: "2000",
						Number:         42,
						Title:          "Improve docs",
						State:          "OPEN",
						IsDraft:        true,
						URL:            "https://github.com/OWNER/REPO/pull/42",
						Body:           "",
						CreatedAt:      sampleDate,
						UpdatedAt:      sampleDate,
						Repository: &api.PRRepository{
							NameWithOwner: "OWNER/REPO",
						},
					},
					User: &api.GitHubUser{
						Login:      "octocat",
						Name:       "Octocat",
						DatabaseID: 1,
					},
				},
				{
					ID:           "sess2",
					Name:         "Build artifacts",
					UserID:       1,
					State:        "completed",
					ResourceType: "pull",
					ResourceID:   2000,
					PullRequest: &api.PullRequest{
						ID:             "PR_node",
						FullDatabaseID: "2000",
						Number:         42,
						Title:          "Improve docs",
						State:          "OPEN",
						IsDraft:        true,
						URL:            "https://github.com/OWNER/REPO/pull/42",
						Body:           "",
						CreatedAt:      sampleDate,
						UpdatedAt:      sampleDate,
						Repository: &api.PRRepository{
							NameWithOwner: "OWNER/REPO",
						},
					},
					User: &api.GitHubUser{
						Login:      "octocat",
						Name:       "Octocat",
						DatabaseID: 1,
					},
				},
			},
		},
		{
			name:  "API error",
			limit: 10,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("GET", "agents/resource/pull/999"), "api.githubcopilot.com"),
					httpmock.StatusStringResponse(500, "{}"),
				)
			},
			wantErr: "failed to list sessions:",
		}, {
			name:  "API error at hydration",
			limit: 10,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("GET", "agents/resource/pull/999"), "api.githubcopilot.com"),
					httpmock.StringResponse(heredoc.Docf(`
						{
							"sessions": [
								{
									"id": "sess1",
									"name": "Build artifacts",
									"user_id": 1,
									"agent_id": 2,
									"logs": "",
									"state": "completed",
									"owner_id": 10,
									"repo_id": 1000,
									"resource_type": "pull",
									"resource_id": 2000,
									"created_at": "%[1]s",
									"premium_requests": 0.1
								}
							]
						}`,
						sampleDateString,
					)),
				)
				// GraphQL hydration
				reg.Register(
					httpmock.GraphQL(`query FetchPRsAndUsersForAgentTaskSessions\b`),
					httpmock.StatusStringResponse(500, `{}`),
				)
			},
			wantErr: `failed to fetch session resources: non-200 OK status code:`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(t, reg)
			}
			defer reg.Verify(t)

			httpClient := &http.Client{Transport: reg}

			capiClient := NewCAPIClient(httpClient, "", "github.com")

			if tt.perPage != 0 {
				last := defaultSessionsPerPage
				defaultSessionsPerPage = tt.perPage
				defer func() {
					defaultSessionsPerPage = last
				}()
			}

			sessions, err := capiClient.ListSessionsByResourceID(context.Background(), resourceType, resourceID, tt.limit)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Nil(t, sessions)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantOut, sessions)
		})
	}
}

func TestGetSessionRequiresID(t *testing.T) {
	client := &CAPIClient{}

	_, err := client.GetSession(context.Background(), "")
	assert.EqualError(t, err, "missing session ID")
}

func TestGetSession(t *testing.T) {
	sampleDateString := "2025-08-29T00:00:00Z"
	sampleDate, err := time.Parse(time.RFC3339, sampleDateString)
	require.NoError(t, err)

	tests := []struct {
		name      string
		httpStubs func(*testing.T, *httpmock.Registry)
		wantErr   string
		wantErrIs error
		wantOut   *Session
	}{
		{
			name: "session not found",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("GET", "agents/sessions/some-uuid"), "api.githubcopilot.com"),
					httpmock.StatusStringResponse(404, "{}"),
				)
			},
			wantErrIs: ErrSessionNotFound,
			wantErr:   "not found",
		},
		{
			name: "API error",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("GET", "agents/sessions/some-uuid"), "api.githubcopilot.com"),
					httpmock.StatusStringResponse(500, "some error"),
				)
			},
			wantErr: "failed to get session:",
		},
		{
			name: "invalid JSON response",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("GET", "agents/sessions/some-uuid"), "api.githubcopilot.com"),
					httpmock.StatusStringResponse(200, ""),
				)
			},
			wantErr: "failed to decode session response: EOF",
		},
		{
			name: "success",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("GET", "agents/sessions/some-uuid"), "api.githubcopilot.com"),
					httpmock.StringResponse(heredoc.Docf(`
						{
							"id": "some-uuid",
							"name": "Build artifacts",
							"user_id": 1,
							"agent_id": 2,
							"logs": "",
							"state": "completed",
							"owner_id": 10,
							"repo_id": 1000,
							"resource_type": "pull",
							"resource_id": 2000,
							"created_at": "%[1]s",
							"premium_requests": 0.1
						}`,
						sampleDateString,
					)),
				)
				// GraphQL hydration
				reg.Register(
					httpmock.GraphQL(`query FetchPRsAndUsersForAgentTaskSessions\b`),
					httpmock.GraphQLQuery(heredoc.Docf(`
						{
							"data": {
								"nodes": [
									{
										"__typename": "PullRequest",
										"id": "PR_node",
										"fullDatabaseId": "2000",
										"number": 42,
										"title": "Improve docs",
										"state": "OPEN",
										"isDraft": true,
										"url": "https://github.com/OWNER/REPO/pull/42",
										"body": "",
										"createdAt": "%[1]s",
										"updatedAt": "%[1]s",
										"repository": {
											"nameWithOwner": "OWNER/REPO"
										}
									},
									{
										"__typename": "User",
										"login": "octocat",
										"name": "Octocat",
										"databaseId": 1
									}
								]
							}
						}`,
						sampleDateString,
					), func(q string, vars map[string]interface{}) {
						assert.Equal(t, []interface{}{"PR_kwDNA-jNB9A", "U_kgAB"}, vars["ids"])
					}),
				)
			},
			wantOut: &Session{
				ID:              "some-uuid",
				Name:            "Build artifacts",
				UserID:          1,
				AgentID:         2,
				Logs:            "",
				State:           "completed",
				OwnerID:         10,
				RepoID:          1000,
				ResourceType:    "pull",
				ResourceID:      2000,
				CreatedAt:       sampleDate,
				PremiumRequests: 0.1,
				PullRequest: &api.PullRequest{
					ID:             "PR_node",
					FullDatabaseID: "2000",
					Number:         42,
					Title:          "Improve docs",
					State:          "OPEN",
					IsDraft:        true,
					URL:            "https://github.com/OWNER/REPO/pull/42",
					Body:           "",
					CreatedAt:      sampleDate,
					UpdatedAt:      sampleDate,
					Repository: &api.PRRepository{
						NameWithOwner: "OWNER/REPO",
					},
				},
				User: &api.GitHubUser{
					Login:      "octocat",
					Name:       "Octocat",
					DatabaseID: 1,
				},
			},
		},
		{
			// This happens at the early moments of a session lifecycle, before a PR is created and associated with it.
			name: "success, but no pull request resource",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("GET", "agents/sessions/some-uuid"), "api.githubcopilot.com"),
					httpmock.StringResponse(heredoc.Docf(`
						{
							"id": "some-uuid",
							"name": "Build artifacts",
							"user_id": 1,
							"agent_id": 2,
							"logs": "",
							"state": "completed",
							"owner_id": 10,
							"repo_id": 1000,
							"resource_type": "",
							"resource_id": 0,
							"created_at": "%[1]s",
							"premium_requests": 0.1
						}`,
						sampleDateString,
					)),
				)
				// GraphQL hydration
				reg.Register(
					httpmock.GraphQL(`query FetchPRsAndUsersForAgentTaskSessions\b`),
					httpmock.GraphQLQuery(heredoc.Docf(`
						{
							"data": {
								"nodes": [
									{
										"__typename": "User",
										"login": "octocat",
										"name": "Octocat",
										"databaseId": 1
									}
								]
							}
						}`,
						sampleDateString,
					), func(q string, vars map[string]interface{}) {
						assert.Equal(t, []interface{}{"U_kgAB"}, vars["ids"])
					}),
				)
			},
			wantOut: &Session{
				ID:              "some-uuid",
				Name:            "Build artifacts",
				UserID:          1,
				AgentID:         2,
				Logs:            "",
				State:           "completed",
				OwnerID:         10,
				RepoID:          1000,
				ResourceType:    "",
				ResourceID:      0,
				CreatedAt:       sampleDate,
				PremiumRequests: 0.1,
				User: &api.GitHubUser{
					Login:      "octocat",
					Name:       "Octocat",
					DatabaseID: 1,
				},
			},
		},
		{
			name: "API error at hydration",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.REST("GET", "agents/sessions/some-uuid"), "api.githubcopilot.com"),
					httpmock.StringResponse(heredoc.Docf(`
						{
							"id": "some-uuid",
							"name": "Build artifacts",
							"user_id": 1,
							"agent_id": 2,
							"logs": "",
							"state": "completed",
							"owner_id": 10,
							"repo_id": 1000,
							"resource_type": "pull",
							"resource_id": 2000,
							"created_at": "%[1]s",
							"premium_requests": 0.1
						}`,
						sampleDateString,
					)),
				)
				// GraphQL hydration
				reg.Register(
					httpmock.GraphQL(`query FetchPRsAndUsersForAgentTaskSessions\b`),
					httpmock.StatusStringResponse(500, `{}`),
				)
			},
			wantErr: `failed to fetch session resources: non-200 OK status code:`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(t, reg)
			}
			defer reg.Verify(t)

			httpClient := &http.Client{Transport: reg}

			capiClient := NewCAPIClient(httpClient, "", "github.com")

			session, err := capiClient.GetSession(context.Background(), "some-uuid")

			if tt.wantErrIs != nil {
				require.ErrorIs(t, err, tt.wantErrIs)
			}

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Nil(t, session)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantOut, session)
		})
	}
}
func TestGetPullRequestDatabaseID(t *testing.T) {
	tests := []struct {
		name           string
		httpStubs      func(*testing.T, *httpmock.Registry)
		wantErr        string
		wantDatabaseID int64
		wantURL        string
	}{
		{
			name: "graphql error",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.GraphQL(`query GetPullRequestFullDatabaseID\b`), "api.github.com"),
					httpmock.StringResponse(`{"data":{}, "errors": [{"message": "some gql error"}]}`),
				)
			},
			wantErr: "some gql error",
		},
		{
			// This never happens in practice and it's just to cover more code path
			name: "non-int database ID",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.GraphQL(`query GetPullRequestFullDatabaseID\b`), "api.github.com"),
					httpmock.StringResponse(`{"data": {"repository": {"pullRequest": {"fullDatabaseId": "non-int", "url": "some-url"}}}}`),
				)
			},
			wantErr: `strconv.ParseInt: parsing "non-int": invalid syntax`,
			wantURL: "some-url",
		},
		{
			name: "success",
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.WithHost(httpmock.GraphQL(`query GetPullRequestFullDatabaseID\b`), "api.github.com"),
					httpmock.GraphQLQuery(`{"data": {"repository": {"pullRequest": {"fullDatabaseId": "999", "url": "some-url"}}}}`, func(s string, m map[string]interface{}) {
						assert.Equal(t, "OWNER", m["owner"])
						assert.Equal(t, "REPO", m["repo"])
						assert.Equal(t, float64(42), m["number"])
					}),
				)
			},
			wantDatabaseID: 999,
			wantURL:        "some-url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(t, reg)
			}
			defer reg.Verify(t)

			httpClient := &http.Client{Transport: reg}

			capiClient := NewCAPIClient(httpClient, "", "github.com")

			databaseID, url, err := capiClient.GetPullRequestDatabaseID(context.Background(), "github.com", "OWNER", "REPO", 42)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Zero(t, databaseID)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantDatabaseID, databaseID)
			require.Equal(t, tt.wantURL, url)
		})
	}
}
