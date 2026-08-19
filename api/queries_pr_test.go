package api

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
)

func TestBranchDeleteRemote(t *testing.T) {
	var tests = []struct {
		name        string
		branch      string
		httpStubs   func(*httpmock.Registry)
		expectError bool
	}{
		{
			name:   "success",
			branch: "owner/branch#123",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO/git/refs/heads%2Fowner%2Fbranch%23123"),
					httpmock.StatusStringResponse(204, ""))
			},
			expectError: false,
		},
		{
			name:   "error",
			branch: "my-branch",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("DELETE", "repos/OWNER/REPO/git/refs/heads%2Fmy-branch"),
					httpmock.StatusStringResponse(500, `{"message": "oh no"}`))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			http := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(http)
			}

			client := newTestClient(http)
			repo, _ := ghrepo.FromFullName("OWNER/REPO")

			err := BranchDeleteRemote(client, repo, tt.branch)
			if (err != nil) != tt.expectError {
				t.Fatalf("unexpected result: %v", err)
			}
		})
	}
}

func Test_Logins(t *testing.T) {
	rr := ReviewRequests{}
	var tests = []struct {
		name             string
		requestedReviews string
		want             []string
	}{
		{
			name:             "no requested reviewers",
			requestedReviews: `{"nodes": []}`,
			want:             []string{},
		},
		{
			name: "user",
			requestedReviews: `{"nodes": [
				{
					"requestedreviewer": {
						"__typename": "User", "login": "testuser"
					}
				}
			]}`,
			want: []string{"testuser"},
		},
		{
			name: "team",
			requestedReviews: `{"nodes": [
				{
					"requestedreviewer": {
						"__typename": "Team",
						"name": "Test Team",
						"slug": "test-team",
						"organization": {"login": "myorg"}
					}
				}
			]}`,
			want: []string{"myorg/test-team"},
		},
		{
			name: "multiple users and teams",
			requestedReviews: `{"nodes": [
				{
					"requestedreviewer": {
						"__typename": "User", "login": "user1"
					}
				},
				{
					"requestedreviewer": {
						"__typename": "User", "login": "user2"
					}
				},
				{
					"requestedreviewer": {
						"__typename": "Team",
						"name": "Test Team",
						"slug": "test-team",
						"organization": {"login": "myorg"}
					}
				},
				{
					"requestedreviewer": {
						"__typename": "Team",
						"name": "Dev Team",
						"slug": "dev-team",
						"organization": {"login": "myorg"}
					}
				}
			]}`,
			want: []string{"user1", "user2", "myorg/test-team", "myorg/dev-team"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := json.Unmarshal([]byte(tt.requestedReviews), &rr)
			assert.NoError(t, err, "Failed to unmarshal json string as ReviewRequests")
			logins := rr.Logins()
			assert.Equal(t, tt.want, logins)
		})
	}
}

// mockReviewerResponse generates a GraphQL response for SuggestedReviewerActors tests.
// It creates suggestions (s1, s2...), collaborators (c1, c2...), and teams (team1, team2...).
// totalCollabs and totalTeams set the unfiltered TotalCount fields (for "more results" calculation).
func mockReviewerResponse(suggestions, collabs, teams, totalCollabs, totalTeams int) string {
	var suggestionNodes, collabNodes, teamNodes []string

	for i := 1; i <= suggestions; i++ {
		suggestionNodes = append(suggestionNodes,
			fmt.Sprintf(`{"isAuthor": false, "reviewer": {"__typename": "User", "login": "s%d", "name": "S%d"}}`, i, i))
	}
	for i := 1; i <= collabs; i++ {
		collabNodes = append(collabNodes,
			fmt.Sprintf(`{"login": "c%d", "name": "C%d"}`, i, i))
	}
	for i := 1; i <= teams; i++ {
		teamNodes = append(teamNodes,
			fmt.Sprintf(`{"slug": "team%d"}`, i))
	}

	return fmt.Sprintf(`{
		"data": {
			"node": {"author": {"login": "testauthor"}, "suggestedReviewerActors": {"nodes": [%s]}},
			"repository": {
				"owner": {"__typename": "Organization", "teams": {"nodes": [%s]}, "teamsTotalCount": {"totalCount": %d}},
				"collaborators": {"nodes": [%s]},
				"collaboratorsTotalCount": {"totalCount": %d}
			}
		}
	}`, strings.Join(suggestionNodes, ","),
		strings.Join(teamNodes, ","), totalTeams,
		strings.Join(collabNodes, ","), totalCollabs)
}

func TestSuggestedReviewerActors(t *testing.T) {
	tests := []struct {
		name           string
		httpStubs      func(*httpmock.Registry)
		expectedCount  int
		expectedLogins []string
		expectedMore   int
		expectError    bool
	}{
		{
			name: "all sources plentiful - 5 each from cascading quota",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query SuggestedReviewerActors\b`),
					httpmock.StringResponse(mockReviewerResponse(6, 6, 6, 20, 10)))
			},
			expectedCount:  15,
			expectedLogins: []string{"s1", "s2", "s3", "s4", "s5", "c1", "c2", "c3", "c4", "c5", "OWNER/team1", "OWNER/team2", "OWNER/team3", "OWNER/team4", "OWNER/team5"},
			expectedMore:   30,
		},
		{
			name: "few suggestions - collaborators fill gap",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query SuggestedReviewerActors\b`),
					httpmock.StringResponse(mockReviewerResponse(2, 10, 6, 50, 10)))
			},
			expectedCount:  15,
			expectedLogins: []string{"s1", "s2", "c1", "c2", "c3", "c4", "c5", "c6", "c7", "c8", "OWNER/team1", "OWNER/team2", "OWNER/team3", "OWNER/team4", "OWNER/team5"},
			expectedMore:   60,
		},
		{
			name: "few suggestions and collaborators - teams fill gap",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query SuggestedReviewerActors\b`),
					httpmock.StringResponse(mockReviewerResponse(2, 3, 10, 3, 10)))
			},
			expectedCount:  15,
			expectedLogins: []string{"s1", "s2", "c1", "c2", "c3", "OWNER/team1", "OWNER/team2", "OWNER/team3", "OWNER/team4", "OWNER/team5", "OWNER/team6", "OWNER/team7", "OWNER/team8", "OWNER/team9", "OWNER/team10"},
			expectedMore:   13,
		},
		{
			name: "no suggestions or collaborators - teams only",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query SuggestedReviewerActors\b`),
					httpmock.StringResponse(mockReviewerResponse(0, 0, 10, 0, 10)))
			},
			expectedCount:  10, // max 15, but only 10 teams available
			expectedLogins: []string{"OWNER/team1", "OWNER/team2", "OWNER/team3", "OWNER/team4", "OWNER/team5", "OWNER/team6", "OWNER/team7", "OWNER/team8", "OWNER/team9", "OWNER/team10"},
			expectedMore:   10,
		},
		{
			name: "author excluded from suggestions",
			httpStubs: func(reg *httpmock.Registry) {
				// Custom response with author flag
				reg.Register(
					httpmock.GraphQL(`query SuggestedReviewerActors\b`),
					httpmock.StringResponse(`{
						"data": {
							"node": {"author": {"login": "testauthor"}, "suggestedReviewerActors": {"nodes": [
								{"isAuthor": true, "reviewer": {"__typename": "User", "login": "author", "name": "Author"}},
								{"isAuthor": false, "reviewer": {"__typename": "User", "login": "s1", "name": "S1"}},
								{"isAuthor": false, "reviewer": {"__typename": "User", "login": "s2", "name": "S2"}}
							]}},
							"repository": {
								"owner": {"__typename": "Organization", "teams": {"nodes": [{"slug": "team1"}]}, "teamsTotalCount": {"totalCount": 3}},
								"collaborators": {"nodes": [{"login": "c1", "name": "C1"}]},
								"collaboratorsTotalCount": {"totalCount": 5}
							}
						}
					}`))
			},
			expectedCount:  4,
			expectedLogins: []string{"s1", "s2", "c1", "OWNER/team1"},
			expectedMore:   8,
		},
		{
			name: "author excluded from collaborators",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query SuggestedReviewerActors\b`),
					httpmock.StringResponse(`{
						"data": {
							"node": {"author": {"login": "theauthor"}, "suggestedReviewerActors": {"nodes": [
								{"isAuthor": false, "reviewer": {"__typename": "User", "login": "s1", "name": "S1"}}
							]}},
							"repository": {
								"collaborators": {"nodes": [
									{"login": "theauthor", "name": "The Author"},
									{"login": "c1", "name": "C1"}
								]},
								"collaboratorsTotalCount": {"totalCount": 5}
							}
						}
					}`))
			},
			expectedCount:  2,
			expectedLogins: []string{"s1", "c1"},
			expectedMore:   5,
		},
		{
			name: "deduplication across sources",
			httpStubs: func(reg *httpmock.Registry) {
				// Custom response with duplicate user
				reg.Register(
					httpmock.GraphQL(`query SuggestedReviewerActors\b`),
					httpmock.StringResponse(`{
						"data": {
							"node": {"author": {"login": "testauthor"}, "suggestedReviewerActors": {"nodes": [
								{"isAuthor": false, "reviewer": {"__typename": "User", "login": "shareduser", "name": "Shared"}}
							]}},
							"repository": {
								"owner": {"__typename": "Organization", "teams": {"nodes": [{"slug": "team1"}]}, "teamsTotalCount": {"totalCount": 5}},
								"collaborators": {"nodes": [
									{"login": "shareduser", "name": "Shared"},
									{"login": "c1", "name": "C1"}
								]},
								"collaboratorsTotalCount": {"totalCount": 10}
							}
						}
					}`))
			},
			expectedCount:  3,
			expectedLogins: []string{"shareduser", "c1", "OWNER/team1"},
			expectedMore:   15,
		},
		{
			name: "personal repo - no organization teams",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query SuggestedReviewerActors\b`),
					httpmock.StringResponse(`{
						"data": {
							"node": {"author": {"login": "testauthor"}, "suggestedReviewerActors": {"nodes": [
								{"isAuthor": false, "reviewer": {"__typename": "User", "login": "s1", "name": "S1"}}
							]}},
							"repository": {
								"owner": {"__typename": "User"},
								"collaborators": {"nodes": [{"login": "c1", "name": "C1"}]},
								"collaboratorsTotalCount": {"totalCount": 3}
							}
						}
					}`))
			},
			expectedCount:  2,
			expectedLogins: []string{"s1", "c1"},
			expectedMore:   3,
		},
		{
			name: "bot reviewer included",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query SuggestedReviewerActors\b`),
					httpmock.StringResponse(`{
						"data": {
							"node": {"author": {"login": "testauthor"}, "suggestedReviewerActors": {"nodes": [
								{"isAuthor": false, "reviewer": {"__typename": "Bot", "login": "copilot-pull-request-reviewer"}},
								{"isAuthor": false, "reviewer": {"__typename": "User", "login": "s1", "name": "S1"}}
							]}},
							"repository": {
								"owner": {"__typename": "Organization", "teams": {"nodes": []}, "teamsTotalCount": {"totalCount": 0}},
								"collaborators": {"nodes": []},
								"collaboratorsTotalCount": {"totalCount": 5}
							}
						}
					}`))
			},
			expectedCount:  2,
			expectedLogins: []string{"copilot-pull-request-reviewer", "s1"},
			expectedMore:   5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}

			client := newTestClient(reg)
			repo, _ := ghrepo.FromFullName("OWNER/REPO")

			candidates, moreResults, err := SuggestedReviewerActors(client, repo, "PR_123", "")
			if tt.expectError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedCount, len(candidates), "candidate count mismatch")
			assert.Equal(t, tt.expectedMore, moreResults, "moreResults mismatch")

			logins := make([]string, len(candidates))
			for i, c := range candidates {
				logins[i] = c.Login()
			}
			assert.Equal(t, tt.expectedLogins, logins)
		})
	}
}

// mockReviewerResponseForRepo generates a GraphQL response for SuggestedReviewerActorsForRepo tests.
// It creates collaborators (c1, c2...) and teams (team1, team2...).
func mockReviewerResponseForRepo(collabs, teams, totalCollabs, totalTeams int) string {
	return mockReviewerResponseForRepoWithCopilot(collabs, teams, totalCollabs, totalTeams, false)
}

// mockReviewerResponseForRepoWithCopilot generates a GraphQL response for SuggestedReviewerActorsForRepo tests.
// If copilotAvailable is true, includes Copilot in the first open PR's suggested reviewers.
func mockReviewerResponseForRepoWithCopilot(collabs, teams, totalCollabs, totalTeams int, copilotAvailable bool) string {
	var collabNodes, teamNodes []string

	for i := 1; i <= collabs; i++ {
		collabNodes = append(collabNodes,
			fmt.Sprintf(`{"login": "c%d", "name": "C%d"}`, i, i))
	}
	for i := 1; i <= teams; i++ {
		teamNodes = append(teamNodes,
			fmt.Sprintf(`{"slug": "team%d"}`, i))
	}

	pullRequestsJSON := `"pullRequests": {"nodes": []}`
	if copilotAvailable {
		pullRequestsJSON = `"pullRequests": {"nodes": [{"suggestedReviewerActors": {"nodes": [{"reviewer": {"__typename": "Bot", "login": "copilot-pull-request-reviewer"}}]}}]}`
	}

	return fmt.Sprintf(`{
		"data": {
			"viewer": {"login": "testuser"},
			"repository": {
				%s,
				"owner": {"__typename": "Organization", "teams": {"nodes": [%s]}, "teamsTotalCount": {"totalCount": %d}},
				"collaborators": {"nodes": [%s]},
				"collaboratorsTotalCount": {"totalCount": %d}
			}
		}
	}`, pullRequestsJSON,
		strings.Join(teamNodes, ","), totalTeams,
		strings.Join(collabNodes, ","), totalCollabs)
}

func TestSuggestedReviewerActorsForRepo(t *testing.T) {
	tests := []struct {
		name           string
		httpStubs      func(*httpmock.Registry)
		expectedCount  int
		expectedLogins []string
		expectedMore   int
		expectError    bool
	}{
		{
			name: "both sources plentiful - 5 each from cascading quota",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query SuggestedReviewerActorsForRepo\b`),
					httpmock.StringResponse(mockReviewerResponseForRepo(6, 6, 20, 10)))
			},
			expectedCount:  10,
			expectedLogins: []string{"c1", "c2", "c3", "c4", "c5", "OWNER/team1", "OWNER/team2", "OWNER/team3", "OWNER/team4", "OWNER/team5"},
			expectedMore:   30,
		},
		{
			name: "few collaborators - teams fill gap",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query SuggestedReviewerActorsForRepo\b`),
					httpmock.StringResponse(mockReviewerResponseForRepo(2, 10, 2, 15)))
			},
			expectedCount:  10,
			expectedLogins: []string{"c1", "c2", "OWNER/team1", "OWNER/team2", "OWNER/team3", "OWNER/team4", "OWNER/team5", "OWNER/team6", "OWNER/team7", "OWNER/team8"},
			expectedMore:   17,
		},
		{
			name: "no collaborators - teams only",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query SuggestedReviewerActorsForRepo\b`),
					httpmock.StringResponse(mockReviewerResponseForRepo(0, 10, 0, 20)))
			},
			expectedCount:  10,
			expectedLogins: []string{"OWNER/team1", "OWNER/team2", "OWNER/team3", "OWNER/team4", "OWNER/team5", "OWNER/team6", "OWNER/team7", "OWNER/team8", "OWNER/team9", "OWNER/team10"},
			expectedMore:   20,
		},
		{
			name: "personal repo - no organization teams",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query SuggestedReviewerActorsForRepo\b`),
					httpmock.StringResponse(`{
						"data": {
							"repository": {
								"pullRequests": {"nodes": []},
								"owner": {"__typename": "User"},
								"collaborators": {"nodes": [{"login": "c1", "name": "C1"}]},
								"collaboratorsTotalCount": {"totalCount": 3}
							}
						}
					}`))
			},
			expectedCount:  1,
			expectedLogins: []string{"c1"},
			expectedMore:   3,
		},
		{
			name: "empty repo",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query SuggestedReviewerActorsForRepo\b`),
					httpmock.StringResponse(mockReviewerResponseForRepo(0, 0, 0, 0)))
			},
			expectedCount:  0,
			expectedLogins: []string{},
			expectedMore:   0,
		},
		{
			name: "copilot available - prepended to candidates",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query SuggestedReviewerActorsForRepo\b`),
					httpmock.StringResponse(mockReviewerResponseForRepoWithCopilot(3, 2, 5, 5, true)))
			},
			expectedCount:  6,
			expectedLogins: []string{"copilot-pull-request-reviewer", "c1", "c2", "c3", "OWNER/team1", "OWNER/team2"},
			expectedMore:   10,
		},
		{
			name: "copilot not available - not included",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query SuggestedReviewerActorsForRepo\b`),
					httpmock.StringResponse(mockReviewerResponseForRepoWithCopilot(3, 2, 5, 5, false)))
			},
			expectedCount:  5,
			expectedLogins: []string{"c1", "c2", "c3", "OWNER/team1", "OWNER/team2"},
			expectedMore:   10,
		},
		{
			name: "viewer excluded from collaborators",
			httpStubs: func(reg *httpmock.Registry) {
				// c1 matches the viewer login "testuser" won't be in this fixture,
				// but we can craft a response where the viewer login matches a collaborator.
				reg.Register(
					httpmock.GraphQL(`query SuggestedReviewerActorsForRepo\b`),
					httpmock.StringResponse(`{
						"data": {
							"viewer": {"login": "c2"},
							"repository": {
								"pullRequests": {"nodes": []},
								"owner": {"__typename": "Organization", "teams": {"nodes": []}, "teamsTotalCount": {"totalCount": 0}},
								"collaborators": {"nodes": [
									{"login": "c1", "name": "C1"},
									{"login": "c2", "name": "C2"},
									{"login": "c3", "name": "C3"}
								]},
								"collaboratorsTotalCount": {"totalCount": 3}
							}
						}
					}`))
			},
			expectedCount:  2,
			expectedLogins: []string{"c1", "c3"},
			expectedMore:   3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}

			client := newTestClient(reg)
			repo, _ := ghrepo.FromFullName("OWNER/REPO")

			candidates, moreResults, err := SuggestedReviewerActorsForRepo(client, repo, "")
			if tt.expectError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedCount, len(candidates), "candidate count mismatch")
			assert.Equal(t, tt.expectedMore, moreResults, "moreResults mismatch")

			logins := make([]string, len(candidates))
			for i, c := range candidates {
				logins[i] = c.Login()
			}
			assert.Equal(t, tt.expectedLogins, logins)
		})
	}
}

// TestPRRepositorySelectionMatchesStruct guards the seam between the two:
// a field added to the selection but not the struct is silently dropped, and
// one added to the struct but not the selection is silently zero.
func TestPRRepositorySelectionMatchesStruct(t *testing.T) {
	selection := PullRequestGraphQL([]string{"repository"})
	inner := strings.TrimSuffix(strings.TrimPrefix(selection, "repository{"), "}")
	selected := strings.Split(inner, ",")

	var declared []string
	repositoryType := reflect.TypeOf(PRRepository{})
	for i := range repositoryType.NumField() {
		name, _, _ := strings.Cut(repositoryType.Field(i).Tag.Get("json"), ",")
		declared = append(declared, name)
	}

	assert.ElementsMatch(t, declared, selected)
}

func TestPullRequestRepositoryDatabaseID(t *testing.T) {
	tests := []struct {
		name string
		pr   PullRequest
		want int64
	}{
		{
			name: "the repository field was requested",
			pr:   PullRequest{Repository: &PRRepository{DatabaseID: 1234}},
			want: 1234,
		},
		{
			name: "the repository field was not requested",
			pr:   PullRequest{},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.pr.RepositoryDatabaseID())
		})
	}
}

func TestPullRequestRepositoryViewerPermission(t *testing.T) {
	tests := []struct {
		name string
		pr   PullRequest
		want string
	}{
		{
			name: "the repository field was requested",
			pr:   PullRequest{Repository: &PRRepository{ViewerPermission: "WRITE"}},
			want: "WRITE",
		},
		{
			name: "the repository field was not requested",
			pr:   PullRequest{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.pr.RepositoryViewerPermission())
		})
	}
}
