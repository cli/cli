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
			name:     "GHE",
			hostname: "git.my.org",
			wantFeatures: PullRequestFeatures{
				ReviewDecision:       true,
				StatusCheckRollup:    true,
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

func TestRepositoryFeatures(t *testing.T) {
	tests := []struct {
		name          string
		hostname      string
		queryResponse map[string]string
		wantFeatures  RepositoryFeatures
		wantErr       bool
	}{
		{
			name:     "github.com",
			hostname: "github.com",
			wantFeatures: RepositoryFeatures{
				IssueTemplateMutation:    true,
				IssueTemplateQuery:       true,
				PullRequestTemplateQuery: true,
			},
			wantErr: false,
		},
		{
			name:     "GHE empty response",
			hostname: "git.my.org",
			queryResponse: map[string]string{
				`query Repository_fields\b`: `{"data": {}}`,
			},
			wantFeatures: RepositoryFeatures{
				IssueTemplateMutation:    true,
				IssueTemplateQuery:       true,
				PullRequestTemplateQuery: false,
			},
			wantErr: false,
		},
		{
			name:     "GHE has pull request template query",
			hostname: "git.my.org",
			queryResponse: map[string]string{
				`query Repository_fields\b`: heredoc.Doc(`
					{ "data": { "Repository": { "fields": [
						{"name": "pullRequestTemplates"}
					] } } }
				`),
			},
			wantFeatures: RepositoryFeatures{
				IssueTemplateMutation:    true,
				IssueTemplateQuery:       true,
				PullRequestTemplateQuery: true,
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
			gotPrFeatures, err := detector.RepositoryFeatures()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantFeatures, gotPrFeatures)
		})
	}
}
