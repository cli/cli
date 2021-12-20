package api

import (
	"encoding/json"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
)

func TestBranchDeleteRemote(t *testing.T) {
	var tests = []struct {
		name           string
		responseStatus int
		responseBody   string
		expectError    bool
	}{
		{
			name:           "success",
			responseStatus: 204,
			responseBody:   "",
			expectError:    false,
		},
		{
			name:           "error",
			responseStatus: 500,
			responseBody:   `{"message": "oh no"}`,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			http := &httpmock.Registry{}
			http.Register(
				httpmock.REST("DELETE", "repos/OWNER/REPO/git/refs/heads/branch"),
				httpmock.StatusStringResponse(tt.responseStatus, tt.responseBody))

			client := NewClient(ReplaceTripper(http))
			repo, _ := ghrepo.FromFullName("OWNER/REPO")

			err := BranchDeleteRemote(client, repo, "branch")
			if (err != nil) != tt.expectError {
				t.Fatalf("unexpected result: %v", err)
			}
		})
	}
}

func Test_determinePullRequestFeatures(t *testing.T) {
	tests := []struct {
		name           string
		hostname       string
		queryResponse  map[string]string
		wantPrFeatures pullRequestFeature
		wantErr        bool
	}{
		{
			name:     "github.com",
			hostname: "github.com",
			wantPrFeatures: pullRequestFeature{
				HasReviewDecision:       true,
				HasStatusCheckRollup:    true,
				HasBranchProtectionRule: true,
			},
			wantErr: false,
		},
		{
			name:     "GHE empty response",
			hostname: "git.my.org",
			queryResponse: map[string]string{
				`query PullRequest_fields\b`:  `{"data": {}}`,
				`query PullRequest_fields2\b`: `{"data": {}}`,
			},
			wantPrFeatures: pullRequestFeature{
				HasReviewDecision:       false,
				HasStatusCheckRollup:    false,
				HasBranchProtectionRule: false,
			},
			wantErr: false,
		},
		{
			name:     "GHE has reviewDecision",
			hostname: "git.my.org",
			queryResponse: map[string]string{
				`query PullRequest_fields\b`: heredoc.Doc(`
					{ "data": { "PullRequest": { "fields": [
						{"name": "foo"},
						{"name": "reviewDecision"}
					] } } }
				`),
				`query PullRequest_fields2\b`: `{"data": {}}`,
			},
			wantPrFeatures: pullRequestFeature{
				HasReviewDecision:       true,
				HasStatusCheckRollup:    false,
				HasBranchProtectionRule: false,
			},
			wantErr: false,
		},
		{
			name:     "GHE has statusCheckRollup",
			hostname: "git.my.org",
			queryResponse: map[string]string{
				`query PullRequest_fields\b`: heredoc.Doc(`
					{ "data": { "Commit": { "fields": [
						{"name": "foo"},
						{"name": "statusCheckRollup"}
					] } } }
				`),
				`query PullRequest_fields2\b`: `{"data": {}}`,
			},
			wantPrFeatures: pullRequestFeature{
				HasReviewDecision:       false,
				HasStatusCheckRollup:    true,
				HasBranchProtectionRule: false,
			},
			wantErr: false,
		},
		{
			name:     "GHE has branchProtectionRule",
			hostname: "git.my.org",
			queryResponse: map[string]string{
				`query PullRequest_fields\b`: `{"data": {}}`,
				`query PullRequest_fields2\b`: heredoc.Doc(`
					{ "data": { "Ref": { "fields": [
						{"name": "foo"},
						{"name": "branchProtectionRule"}
					] } } }
				`),
			},
			wantPrFeatures: pullRequestFeature{
				HasReviewDecision:       false,
				HasStatusCheckRollup:    false,
				HasBranchProtectionRule: true,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeHTTP := &httpmock.Registry{}
			httpClient := NewHTTPClient(ReplaceTripper(fakeHTTP))

			for query, resp := range tt.queryResponse {
				fakeHTTP.Register(httpmock.GraphQL(query), httpmock.StringResponse(resp))
			}

			gotPrFeatures, err := determinePullRequestFeatures(httpClient, tt.hostname)
			if tt.wantErr {
				assert.Error(t, err)
				return
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantPrFeatures, gotPrFeatures)
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
