package featuredetection

import (
	"net/http"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
				StateReason:       true,
				ActorIsAssignable: true,
			},
			wantErr: false,
		},
		{
			name:     "ghec data residency (ghe.com)",
			hostname: "stampname.ghe.com",
			wantFeatures: IssueFeatures{
				StateReason:       true,
				ActorIsAssignable: true,
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
				StateReason:       false,
				ActorIsAssignable: false,
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
			name:     "github.com with all features",
			hostname: "github.com",
			queryResponse: map[string]string{
				`query PullRequest_fields\b`: heredoc.Doc(`
				{
					"data": {
						"PullRequest": {
							"fields": [
								{"name": "isInMergeQueue"},
								{"name": "isMergeQueueEnabled"}
							]
						},
						"StatusCheckRollupContextConnection": {
							"fields": [
								{"name": "checkRunCount"},
								{"name": "checkRunCountsByState"},
								{"name": "statusContextCount"},
								{"name": "statusContextCountsByState"}
							]
						}
					}
				}`),
				`query PullRequest_fields2\b`: heredoc.Doc(`
				{
					"data": {
						"WorkflowRun": {
							"fields": [
								{"name": "event"}
							]
						}
					}
				}`),
			},
			wantFeatures: PullRequestFeatures{
				MergeQueue:                     true,
				CheckRunAndStatusContextCounts: true,
				CheckRunEvent:                  true,
			},
			wantErr: false,
		},
		{
			name:     "github.com with no merge queue",
			hostname: "github.com",
			queryResponse: map[string]string{
				`query PullRequest_fields\b`: heredoc.Doc(`
				{
					"data": {
						"PullRequest": {
							"fields": []
						},
						"StatusCheckRollupContextConnection": {
							"fields": [
								{"name": "checkRunCount"},
								{"name": "checkRunCountsByState"},
								{"name": "statusContextCount"},
								{"name": "statusContextCountsByState"}
							]
						}
					}
				}`),
				`query PullRequest_fields2\b`: heredoc.Doc(`
				{
					"data": {
						"WorkflowRun": {
							"fields": [
								{"name": "event"}
							]
						}
					}
				}`),
			},
			wantFeatures: PullRequestFeatures{
				MergeQueue:                     false,
				CheckRunAndStatusContextCounts: true,
				CheckRunEvent:                  true,
			},
			wantErr: false,
		},
		{
			name:     "GHE with all features",
			hostname: "git.my.org",
			queryResponse: map[string]string{
				`query PullRequest_fields\b`: heredoc.Doc(`
				{
					"data": {
						"PullRequest": {
							"fields": [
								{"name": "isInMergeQueue"},
								{"name": "isMergeQueueEnabled"}
							]
						},
						"StatusCheckRollupContextConnection": {
							"fields": [
								{"name": "checkRunCount"},
								{"name": "checkRunCountsByState"},
								{"name": "statusContextCount"},
								{"name": "statusContextCountsByState"}
							]
						}
					}
				}`),
				`query PullRequest_fields2\b`: heredoc.Doc(`
				{
					"data": {
						"WorkflowRun": {
							"fields": [
								{"name": "event"}
							]
						}
					}
				}`),
			},
			wantFeatures: PullRequestFeatures{
				MergeQueue:                     true,
				CheckRunAndStatusContextCounts: true,
				CheckRunEvent:                  true,
			},
			wantErr: false,
		},
		{
			name:     "GHE with no features",
			hostname: "git.my.org",
			queryResponse: map[string]string{
				`query PullRequest_fields\b`: heredoc.Doc(`
				{
					"data": {
						"PullRequest": {
							"fields": []
						},
						"StatusCheckRollupContextConnection": {
							"fields": []
						}
					}
				}`),
				`query PullRequest_fields2\b`: heredoc.Doc(`
				{
					"data": {
						"WorkflowRun": {
							"fields": []
						}
					}
				}`),
			},
			wantFeatures: PullRequestFeatures{
				MergeQueue:                     false,
				CheckRunAndStatusContextCounts: false,
				CheckRunEvent:                  false,
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
			name:     "ghec data residency (ghe.com)",
			hostname: "stampname.ghe.com",
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

func TestProjectV1Support(t *testing.T) {
	tests := []struct {
		name         string
		hostname     string
		httpStubs    func(*httpmock.Registry)
		wantFeatures gh.ProjectsV1Support
	}{
		{
			name:         "github.com",
			hostname:     "github.com",
			wantFeatures: gh.ProjectsV1Unsupported,
		},
		{
			name:         "ghec data residency (ghe.com)",
			hostname:     "stampname.ghe.com",
			wantFeatures: gh.ProjectsV1Unsupported,
		},
		{
			name:     "GHE 3.16.0",
			hostname: "git.my.org",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "api/v3/meta"),
					httpmock.StringResponse(`{"installed_version":"3.16.0"}`),
				)
			},
			wantFeatures: gh.ProjectsV1Supported,
		},
		{
			name:     "GHE 3.16.1",
			hostname: "git.my.org",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "api/v3/meta"),
					httpmock.StringResponse(`{"installed_version":"3.16.1"}`),
				)
			},
			wantFeatures: gh.ProjectsV1Supported,
		},
		{
			name:     "GHE 3.17",
			hostname: "git.my.org",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "api/v3/meta"),
					httpmock.StringResponse(`{"installed_version":"3.17.0"}`),
				)
			},
			wantFeatures: gh.ProjectsV1Unsupported,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}
			httpClient := &http.Client{}
			httpmock.ReplaceTripper(httpClient, reg)

			detector := NewDetector(httpClient, tt.hostname)
			require.Equal(t, tt.wantFeatures, detector.ProjectsV1())
		})
	}
}

func TestAdvancedIssueSearchSupport(t *testing.T) {
	withIssueAdvanced := `{"data":{"SearchType":{"enumValues":[{"name":"ISSUE"},{"name":"ISSUE_ADVANCED"},{"name":"REPOSITORY"},{"name":"USER"},{"name":"DISCUSSION"}]}}}`
	withoutIssueAdvanced := `{"data":{"SearchType":{"enumValues":[{"name":"ISSUE"},{"name":"REPOSITORY"},{"name":"USER"},{"name":"DISCUSSION"}]}}}`

	tests := []struct {
		name         string
		hostname     string
		httpStubs    func(*httpmock.Registry)
		wantFeatures SearchFeatures
	}{
		{
			name:     "github.com, before ISSUE_ADVANCED cleanup",
			hostname: "github.com",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query SearchType_enumValues\b`),
					httpmock.StringResponse(withIssueAdvanced),
				)
			},
			wantFeatures: advancedIssueSearchSupportedAsOptIn,
		},
		{
			name:     "github.com, after ISSUE_ADVANCED cleanup",
			hostname: "github.com",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query SearchType_enumValues\b`),
					httpmock.StringResponse(withoutIssueAdvanced),
				)
			},
			wantFeatures: advancedIssueSearchSupportedAsOnlyBackend,
		},
		{
			name:     "ghec data residency (ghe.com), before ISSUE_ADVANCED cleanup",
			hostname: "stampname.ghe.com",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query SearchType_enumValues\b`),
					httpmock.StringResponse(withIssueAdvanced),
				)
			},
			wantFeatures: advancedIssueSearchSupportedAsOptIn,
		},
		{
			name:     "ghec data residency (ghe.com), after ISSUE_ADVANCED cleanup",
			hostname: "stampname.ghe.com",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query SearchType_enumValues\b`),
					httpmock.StringResponse(withoutIssueAdvanced),
				)
			},
			wantFeatures: advancedIssueSearchSupportedAsOnlyBackend,
		},
		{
			name:     "GHE 3.18, before ISSUE_ADVANCED cleanup",
			hostname: "git.my.org",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "api/v3/meta"),
					httpmock.StringResponse(`{"installed_version":"3.18.0"}`),
				)
				reg.Register(
					httpmock.GraphQL(`query SearchType_enumValues\b`),
					httpmock.StringResponse(withIssueAdvanced),
				)
			},
			wantFeatures: advancedIssueSearchSupportedAsOptIn,
		},
		{
			name:     "GHE 3.18, after ISSUE_ADVANCED cleanup",
			hostname: "git.my.org",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "api/v3/meta"),
					httpmock.StringResponse(`{"installed_version":"3.18.0"}`),
				)
				reg.Register(
					httpmock.GraphQL(`query SearchType_enumValues\b`),
					httpmock.StringResponse(withoutIssueAdvanced),
				)
			},
			wantFeatures: advancedIssueSearchSupportedAsOnlyBackend,
		},
		{
			name:     "GHE >3.18, before ISSUE_ADVANCED cleanup",
			hostname: "git.my.org",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "api/v3/meta"),
					httpmock.StringResponse(`{"installed_version":"3.18.1"}`),
				)
				reg.Register(
					httpmock.GraphQL(`query SearchType_enumValues\b`),
					httpmock.StringResponse(withIssueAdvanced),
				)
			},
			wantFeatures: advancedIssueSearchSupportedAsOptIn,
		},
		{
			name:     "GHE >3.18, after ISSUE_ADVANCED cleanup",
			hostname: "git.my.org",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "api/v3/meta"),
					httpmock.StringResponse(`{"installed_version":"3.18.1"}`),
				)
				reg.Register(
					httpmock.GraphQL(`query SearchType_enumValues\b`),
					httpmock.StringResponse(withoutIssueAdvanced),
				)
			},
			wantFeatures: advancedIssueSearchSupportedAsOnlyBackend,
		},
		{
			name:     "GHE <3.18 (no advanced issue search support)",
			hostname: "git.my.org",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "api/v3/meta"),
					httpmock.StringResponse(`{"installed_version":"3.17.999"}`),
				)
			},
			wantFeatures: advancedIssueSearchNotSupported,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}
			httpClient := &http.Client{}
			httpmock.ReplaceTripper(httpClient, reg)

			detector := NewDetector(httpClient, tt.hostname)

			features, err := detector.SearchFeatures()
			require.NoError(t, err)
			require.Equal(t, tt.wantFeatures, features)
		})
	}
}

func TestReleaseFeatures(t *testing.T) {
	withImmutableReleaseSupport := `{"data":{"Release":{"fields":[{"name":"author"},{"name":"name"},{"name":"immutable"}]}}}`
	withoutImmutableReleaseSupport := `{"data":{"Release":{"fields":[{"name":"author"},{"name":"name"}]}}}`

	tests := []struct {
		name         string
		hostname     string
		httpStubs    func(*httpmock.Registry)
		wantFeatures ReleaseFeatures
	}{
		{
			// This is not a real case as `github.com` supports immutable releases.
			name:     "github.com, immutable releases unsupported",
			hostname: "github.com",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query Release_fields\b`),
					httpmock.StringResponse(withoutImmutableReleaseSupport),
				)
			},
			wantFeatures: ReleaseFeatures{
				ImmutableReleases: false,
			},
		},
		{
			name:     "github.com, immutable releases supported",
			hostname: "github.com",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query Release_fields\b`),
					httpmock.StringResponse(withImmutableReleaseSupport),
				)
			},
			wantFeatures: ReleaseFeatures{
				ImmutableReleases: true,
			},
		},
		{
			// This is not a real case as `github.com` supports immutable releases.
			name:     "ghec data residency (ghe.com), immutable releases unsupported",
			hostname: "stampname.ghe.com",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query Release_fields\b`),
					httpmock.StringResponse(withoutImmutableReleaseSupport),
				)
			},
			wantFeatures: ReleaseFeatures{
				ImmutableReleases: false,
			},
		},
		{
			name:     "ghec data residency (ghe.com), immutable releases supported",
			hostname: "stampname.ghe.com",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query Release_fields\b`),
					httpmock.StringResponse(withImmutableReleaseSupport),
				)
			},
			wantFeatures: ReleaseFeatures{
				ImmutableReleases: true,
			},
		},
		{
			name:     "GHE, immutable releases unsupported",
			hostname: "git.my.org",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query Release_fields\b`),
					httpmock.StringResponse(withoutImmutableReleaseSupport),
				)
			},
			wantFeatures: ReleaseFeatures{
				ImmutableReleases: false,
			},
		},
		{
			name:     "GHE, immutable releases supported",
			hostname: "git.my.org",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query Release_fields\b`),
					httpmock.StringResponse(withImmutableReleaseSupport),
				)
			},
			wantFeatures: ReleaseFeatures{
				ImmutableReleases: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reg := &httpmock.Registry{}
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}
			httpClient := &http.Client{}
			httpmock.ReplaceTripper(httpClient, reg)

			detector := NewDetector(httpClient, tt.hostname)

			features, err := detector.ReleaseFeatures()
			require.NoError(t, err)
			require.Equal(t, tt.wantFeatures, features)
		})
	}
}
