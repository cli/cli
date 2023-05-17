package featuredetection

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
)

func TestIssueFeatures(t *testing.T) {
	tests := []struct {
		name          string
		hostname      string
		queryResponse map[string]string
		wantFeatures  IssueFeatures
		wantErr       bool
	}{
		{
			name:     "github.com",
			hostname: "github.com",
			wantFeatures: IssueFeatures{
				StateReason: true,
			},
			wantErr: false,
		},
		{
			name:     "GHE empty response",
			hostname: "git.my.org",
			queryResponse: map[string]string{
				`query Issue_fields\b`: `{"data": {}}`,
			},
			wantFeatures: IssueFeatures{
				StateReason: false,
			},
			wantErr: false,
		},
		{
			name:     "GHE has state reason field",
			hostname: "git.my.org",
			queryResponse: map[string]string{
				`query Issue_fields\b`: heredoc.Doc(`
					{ "data": { "Issue": { "fields": [
						{"name": "stateReason"}
					] } } }
				`),
			},
			wantFeatures: IssueFeatures{
				StateReason: true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			httpClient := &http.Client{}
			httpmock.ReplaceTripper(httpClient, reg)
			for query, resp := range tt.queryResponse {
				reg.Register(httpmock.GraphQL(query), httpmock.StringResponse(resp))
			}
			detector := detector{host: tt.hostname, httpClient: httpClient}
			gotFeatures, err := detector.IssueFeatures()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantFeatures, gotFeatures)
		})
	}
}

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
			queryResponse: map[string]string{
				`query PullRequest_fields\b`: heredoc.Doc(`
					{ "data": { "PullRequest": { "fields": [
						{"name": "isInMergeQueue"},
						{"name": "isMergeQueueEnabled"}
					] } } }
				`),
				`query StatusCheckRollupContextConnection_fields\b`: emptyIntrospectionFor("StatusCheckRollupContextConnection"),
			},
			wantFeatures: PullRequestFeatures{
				MergeQueue: true,
			},
			wantErr: false,
		},
		{
			name:     "github.com with no merge queue",
			hostname: "github.com",
			queryResponse: map[string]string{
				`query PullRequest_fields\b`: heredoc.Doc(`
					{ "data": { "PullRequest": { "fields": [
					] } } }
				`),
				`query StatusCheckRollupContextConnection_fields\b`: emptyIntrospectionFor("StatusCheckRollupContextConnection"),
			},
			wantFeatures: PullRequestFeatures{
				MergeQueue: false,
			},
			wantErr: false,
		},
		{
			name:     "GHE",
			hostname: "git.my.org",
			queryResponse: map[string]string{
				`query PullRequest_fields\b`: heredoc.Doc(`
					{ "data": { "PullRequest": { "fields": [
						{"name": "isInMergeQueue"},
						{"name": "isMergeQueueEnabled"}
					] } } }
				`),
				`query StatusCheckRollupContextConnection_fields\b`: emptyIntrospectionFor("StatusCheckRollupContextConnection"),
			},
			wantFeatures: PullRequestFeatures{
				MergeQueue: true,
			},
			wantErr: false,
		},
		{
			name:     "Any GitHub host without check run and status context count support",
			hostname: "github.com",
			queryResponse: map[string]string{
				`query PullRequest_fields\b`: emptyIntrospectionFor("PullRequest"),
				`query StatusCheckRollupContextConnection_fields\b`: heredoc.Doc(`
					{ "data": { "StatusCheckRollupContextConnection": { "fields": [
					] } } }
				`),
			},
			wantFeatures: PullRequestFeatures{
				CheckRunAndStatusContextCounts: false,
			},
			wantErr: false,
		},
		{
			name:     "Any GitHub host with check run and status context count support",
			hostname: "github.com",
			queryResponse: map[string]string{
				`query PullRequest_fields\b`: emptyIntrospectionFor("PullRequest"),
				`query StatusCheckRollupContextConnection_fields\b`: heredoc.Doc(`
					{ "data": { "StatusCheckRollupContextConnection": { "fields": [
						{"name": "checkRunCount"},
						{"name": "checkRunCountsByState"},
						{"name": "statusContextCount"},
						{"name": "statusContextCountsByState"}
					] } } }
				`),
			},
			wantFeatures: PullRequestFeatures{
				CheckRunAndStatusContextCounts: true,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			httpClient := &http.Client{}
			httpmock.ReplaceTripper(httpClient, reg)
			for query, resp := range tt.queryResponse {
				reg.Register(httpmock.GraphQL(query), httpmock.StringResponse(resp))
			}
			detector := detector{host: tt.hostname, httpClient: httpClient}
			gotFeatures, err := detector.PullRequestFeatures()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantFeatures, gotFeatures)
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
				PullRequestTemplateQuery: true,
				VisibilityField:          true,
				AutoMerge:                true,
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
				PullRequestTemplateQuery: true,
			},
			wantErr: false,
		},
		{
			name:     "GHE has visibility field",
			hostname: "git.my.org",
			queryResponse: map[string]string{
				`query Repository_fields\b`: heredoc.Doc(`
					{ "data": { "Repository": { "fields": [
						{"name": "visibility"}
					] } } }
				`),
			},
			wantFeatures: RepositoryFeatures{
				VisibilityField: true,
			},
			wantErr: false,
		},
		{
			name:     "GHE has automerge field",
			hostname: "git.my.org",
			queryResponse: map[string]string{
				`query Repository_fields\b`: heredoc.Doc(`
					{ "data": { "Repository": { "fields": [
						{"name": "autoMergeAllowed"}
					] } } }
				`),
			},
			wantFeatures: RepositoryFeatures{
				AutoMerge: true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			httpClient := &http.Client{}
			httpmock.ReplaceTripper(httpClient, reg)
			for query, resp := range tt.queryResponse {
				reg.Register(httpmock.GraphQL(query), httpmock.StringResponse(resp))
			}
			detector := detector{host: tt.hostname, httpClient: httpClient}
			gotFeatures, err := detector.RepositoryFeatures()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantFeatures, gotFeatures)
		})
	}
}

func emptyIntrospectionFor(typ string) string {
	return fmt.Sprintf(`
		{ "data": { "%s": { "fields": [
		] } } }
	`, typ)
}
