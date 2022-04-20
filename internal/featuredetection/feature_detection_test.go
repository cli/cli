package featuredetection

import (
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
)

func TestPullRequestFeatures(t *testing.T) {
	tests := []struct {
		name          string
		hostname      string
		queryResponse map[string]string
		wantFeatures  PullRequestFeatures
		wantErr       bool
	}{
		{
			name:     "github.com",
			hostname: "github.com",
			wantFeatures: PullRequestFeatures{
				ReviewDecision:       true,
				StatusCheckRollup:    true,
				BranchProtectionRule: true,
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
			wantFeatures: PullRequestFeatures{
				ReviewDecision:       false,
				StatusCheckRollup:    false,
				BranchProtectionRule: false,
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
			wantFeatures: PullRequestFeatures{
				ReviewDecision:       true,
				StatusCheckRollup:    false,
				BranchProtectionRule: false,
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
			wantFeatures: PullRequestFeatures{
				ReviewDecision:       false,
				StatusCheckRollup:    true,
				BranchProtectionRule: false,
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
			wantFeatures: PullRequestFeatures{
				ReviewDecision:       false,
				StatusCheckRollup:    false,
				BranchProtectionRule: true,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeHTTP := &httpmock.Registry{}
			httpClient := api.NewHTTPClient(api.ReplaceTripper(fakeHTTP))
			for query, resp := range tt.queryResponse {
				fakeHTTP.Register(httpmock.GraphQL(query), httpmock.StringResponse(resp))
			}
			detector := detector{host: tt.hostname, httpClient: httpClient}
			gotPrFeatures, err := detector.PullRequestFeatures()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantFeatures, gotPrFeatures)
		})
	}
}
