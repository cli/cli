package featuredetection

import (
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/hashicorp/go-version"
	"golang.org/x/sync/errgroup"

	ghauth "github.com/cli/go-gh/v2/pkg/auth"
)

type Detector interface {
	IssueFeatures() (IssueFeatures, error)
	PullRequestFeatures() (PullRequestFeatures, error)
	RepositoryFeatures() (RepositoryFeatures, error)
	ProjectsV1() gh.ProjectsV1Support
	SearchFeatures() (SearchFeatures, error)
	ReleaseFeatures() (ReleaseFeatures, error)
}

type IssueFeatures struct {
	StateReason       bool
	ActorIsAssignable bool
}

var allIssueFeatures = IssueFeatures{
	StateReason:       true,
	ActorIsAssignable: true,
}

type PullRequestFeatures struct {
	MergeQueue bool
	// CheckRunAndStatusContextCounts indicates whether the API supports
	// the checkRunCount, checkRunCountsByState, statusContextCount and statusContextCountsByState
	// fields on the StatusCheckRollupContextConnection
	CheckRunAndStatusContextCounts bool
	CheckRunEvent                  bool
}

var allPullRequestFeatures = PullRequestFeatures{
	MergeQueue:                     true,
	CheckRunAndStatusContextCounts: true,
	CheckRunEvent:                  true,
}

type RepositoryFeatures struct {
	PullRequestTemplateQuery bool
	VisibilityField          bool
	AutoMerge                bool
}

var allRepositoryFeatures = RepositoryFeatures{
	PullRequestTemplateQuery: true,
	VisibilityField:          true,
	AutoMerge:                true,
}

type SearchFeatures struct {
	// AdvancedIssueSearch indicates whether the host supports advanced issue
	// search via API calls.
	AdvancedIssueSearchAPI bool
	// AdvancedIssueSearchOptIn indicates whether the host supports advanced
	// issue search as an opt-in feature, which has to be explicitly enabled in
	// API calls.
	AdvancedIssueSearchAPIOptIn bool

	// TODO advancedSearchFuture
	// When advanced issue search is supported in Pull Requests tab, or in
	// global search we can introduce more fields to reflect the support status.
}

// advancedIssueSearchNotSupported mimics GHE <3.18 where advanced issue search
// is either not supported or is not meant to be used due to not being stable
// enough (i.e. in preview).
var advancedIssueSearchNotSupported = SearchFeatures{
	AdvancedIssueSearchAPI: false,
}

// advancedIssueSearchSupportedAsOptIn mimics github.com and GHE >=3.18 before
// the full cleanup of temp types (i.e. ISSUE_ADVANCED search type is still
// present on the schema).
var advancedIssueSearchSupportedAsOptIn = SearchFeatures{
	AdvancedIssueSearchAPI:      true,
	AdvancedIssueSearchAPIOptIn: true,
}

// advancedIssueSearchSupportedAsOnlyBackend mimics github.com and GHE >=3.18
// after the full cleanup of temp types (i.e. ISSUE_ADVANCED search type is
// removed from the schema).
var advancedIssueSearchSupportedAsOnlyBackend = SearchFeatures{
	AdvancedIssueSearchAPI:      true,
	AdvancedIssueSearchAPIOptIn: false,
}

type ReleaseFeatures struct {
	ImmutableReleases bool
}

type detector struct {
	host       string
	httpClient *http.Client
}

func NewDetector(httpClient *http.Client, host string) Detector {
	return &detector{
		httpClient: httpClient,
		host:       host,
	}
}

func (d *detector) IssueFeatures() (IssueFeatures, error) {
	if !ghauth.IsEnterprise(d.host) {
		return allIssueFeatures, nil
	}

	features := IssueFeatures{
		StateReason:       false,
		ActorIsAssignable: false, // replaceActorsForAssignable GraphQL mutation unavailable on GHES
	}

	var featureDetection struct {
		Issue struct {
			Fields []struct {
				Name string
			} `graphql:"fields(includeDeprecated: true)"`
		} `graphql:"Issue: __type(name: \"Issue\")"`
	}

	gql := api.NewClientFromHTTP(d.httpClient)
	err := gql.Query(d.host, "Issue_fields", &featureDetection, nil)
	if err != nil {
		return features, err
	}

	for _, field := range featureDetection.Issue.Fields {
		if field.Name == "stateReason" {
			features.StateReason = true
		}
	}

	return features, nil
}

func (d *detector) PullRequestFeatures() (PullRequestFeatures, error) {
	// TODO: reinstate the short-circuit once the APIs are fully available on github.com
	// https://github.com/cli/cli/issues/5778
	//
	// if !ghinstance.IsEnterprise(d.host) {
	// 	return allPullRequestFeatures, nil
	// }

	var pullRequestFeatureDetection struct {
		PullRequest struct {
			Fields []struct {
				Name string
			} `graphql:"fields(includeDeprecated: true)"`
		} `graphql:"PullRequest: __type(name: \"PullRequest\")"`
		StatusCheckRollupContextConnection struct {
			Fields []struct {
				Name string
			} `graphql:"fields(includeDeprecated: true)"`
		} `graphql:"StatusCheckRollupContextConnection: __type(name: \"StatusCheckRollupContextConnection\")"`
	}

	// Break feature detection down into two separate queries because the platform
	// only supports two `__type` expressions in one query.
	var pullRequestFeatureDetection2 struct {
		WorkflowRun struct {
			Fields []struct {
				Name string
			} `graphql:"fields(includeDeprecated: true)"`
		} `graphql:"WorkflowRun: __type(name: \"WorkflowRun\")"`
	}

	gql := api.NewClientFromHTTP(d.httpClient)

	var wg errgroup.Group
	wg.Go(func() error {
		return gql.Query(d.host, "PullRequest_fields", &pullRequestFeatureDetection, nil)
	})
	wg.Go(func() error {
		return gql.Query(d.host, "PullRequest_fields2", &pullRequestFeatureDetection2, nil)
	})
	if err := wg.Wait(); err != nil {
		return PullRequestFeatures{}, err
	}

	features := PullRequestFeatures{}

	for _, field := range pullRequestFeatureDetection.PullRequest.Fields {
		if field.Name == "isInMergeQueue" {
			features.MergeQueue = true
		}
	}

	for _, field := range pullRequestFeatureDetection.StatusCheckRollupContextConnection.Fields {
		// We only check for checkRunCount here but it, checkRunCountsByState, statusContextCount and statusContextCountsByState
		// were all introduced in the same version of the API.
		if field.Name == "checkRunCount" {
			features.CheckRunAndStatusContextCounts = true
		}
	}

	for _, field := range pullRequestFeatureDetection2.WorkflowRun.Fields {
		if field.Name == "event" {
			features.CheckRunEvent = true
		}
	}

	return features, nil
}

func (d *detector) RepositoryFeatures() (RepositoryFeatures, error) {
	if !ghauth.IsEnterprise(d.host) {
		return allRepositoryFeatures, nil
	}

	features := RepositoryFeatures{}

	var featureDetection struct {
		Repository struct {
			Fields []struct {
				Name string
			} `graphql:"fields(includeDeprecated: true)"`
		} `graphql:"Repository: __type(name: \"Repository\")"`
	}

	gql := api.NewClientFromHTTP(d.httpClient)

	err := gql.Query(d.host, "Repository_fields", &featureDetection, nil)
	if err != nil {
		return features, err
	}

	for _, field := range featureDetection.Repository.Fields {
		if field.Name == "pullRequestTemplates" {
			features.PullRequestTemplateQuery = true
		}
		if field.Name == "visibility" {
			features.VisibilityField = true
		}
		if field.Name == "autoMergeAllowed" {
			features.AutoMerge = true
		}
	}

	return features, nil
}

const (
	enterpriseProjectsV1Removed = "3.17.0"
)

func (d *detector) ProjectsV1() gh.ProjectsV1Support {
	if !ghauth.IsEnterprise(d.host) {
		return gh.ProjectsV1Unsupported
	}

	hostVersion, hostVersionErr := resolveEnterpriseVersion(d.httpClient, d.host)
	v1ProjectCutoffVersion, v1ProjectCutoffVersionErr := version.NewVersion(enterpriseProjectsV1Removed)

	if hostVersionErr == nil && v1ProjectCutoffVersionErr == nil && hostVersion.LessThan(v1ProjectCutoffVersion) {
		return gh.ProjectsV1Supported
	}

	return gh.ProjectsV1Unsupported
}

const (
	// enterpriseAdvancedIssueSearchSupport is the minimum version of GHES that
	// supports advanced issue search and gh should use it.
	//
	// Note that advanced issue search is also available on GHES 3.17, but it's
	// at the preview stage and is not as mature as it is on github.com or later
	// GHES version.
	enterpriseAdvancedIssueSearchSupport = "3.18.0"
)

func (d *detector) SearchFeatures() (SearchFeatures, error) {
	// TODO advancedIssueSearchCleanup
	// Once GHES 3.17 support ends, we don't need this and, probably, the entire search feature detection.

	// Regarding the release of advanced issue search (AIS, for short), there
	// are three time spans/periods:
	//
	// 1. Pre-deprecation: where both legacy search and AIS are available
	//    - GraphQL: `ISSUE` and `ISSUE_ADVANCED` search types in GraphQL behave differently
	//    - REST:    `advance_search=true` query parameter can be used to switch to AIS
	// 2. Deprecation: only AIS available
	//    - GraphQL: `ISSUE` and `ISSUE_ADVANCED` search types in GraphQL behave the same (AIS)
	//    - REST:    `advance_search` query parameter has no effect (AIS)
	// 3. Cleanup: only AIS available
	//    - GraphQL: `ISSUE` search type in GraphQL is the only available option (AIS)
	//    - REST:    `advance_search` query parameter has no effect (AIS)
	//
	// Since there's no schema-wise difference between pre-deprecation and
	// deprecation periods (i.e. `ISSUE_ADVANCED` is available during both),
	// we cannot figure out the exact time period. The consensus is to to use
	// the advanced search syntax during both periods.

	var feature SearchFeatures

	if ghauth.IsEnterprise(d.host) {
		enterpriseAISSupportVersion, err := version.NewVersion(enterpriseAdvancedIssueSearchSupport)
		if err != nil {
			return SearchFeatures{}, err
		}

		hostVersion, err := resolveEnterpriseVersion(d.httpClient, d.host)
		if err != nil {
			return SearchFeatures{}, err
		}

		if hostVersion.GreaterThanOrEqual(enterpriseAISSupportVersion) {
			// As of August 2025, advanced issue search is going to be available
			// on GHES 3.18+, including Issues tabs in repositories.
			feature.AdvancedIssueSearchAPI = true

			// TODO advancedSearchFuture
			// When the advanced search syntax is supported in global search or
			// Pull Requests tabs (in repositories), we can add and enable the
			// corresponding fields.
		}
	} else {
		// As of August 2025, advanced issue search is available on github.com,
		// including Issues tabs in repositories.
		feature.AdvancedIssueSearchAPI = true

		// TODO advancedSearchFuture
		// When the advanced search syntax is supported in global search or
		// Pull Requests tabs (in repositories), we can add and enable the
		// corresponding fields.
	}

	if !feature.AdvancedIssueSearchAPI {
		return feature, nil
	}

	var searchTypeFeatureDetection struct {
		SearchType struct {
			EnumValues []struct {
				Name string
			} `graphql:"enumValues(includeDeprecated: true)"`
		} `graphql:"SearchType: __type(name: \"SearchType\")"`
	}

	gql := api.NewClientFromHTTP(d.httpClient)
	if err := gql.Query(d.host, "SearchType_enumValues", &searchTypeFeatureDetection, nil); err != nil {
		return SearchFeatures{}, err
	}

	for _, enumValue := range searchTypeFeatureDetection.SearchType.EnumValues {
		if enumValue.Name == "ISSUE_ADVANCED" {
			// As long as ISSUE_ADVANCED is present on the schema, we should
			// explicitly opt-in when making API calls.
			feature.AdvancedIssueSearchAPIOptIn = true
			break
		}
	}

	return feature, nil
}

func (d *detector) ReleaseFeatures() (ReleaseFeatures, error) {
	// TODO: immutableReleaseFullSupport
	// Once all supported GHES versions fully support immutable releases, we can
	// remove this function, of course, unless there will be other release-related
	// features that are not available on all GH hosts.

	var releaseFeatureDetection struct {
		Release struct {
			Fields []struct {
				Name string
			} `graphql:"fields"`
		} `graphql:"Release: __type(name: \"Release\")"`
	}

	gql := api.NewClientFromHTTP(d.httpClient)
	if err := gql.Query(d.host, "Release_fields", &releaseFeatureDetection, nil); err != nil {
		return ReleaseFeatures{}, err
	}

	for _, field := range releaseFeatureDetection.Release.Fields {
		if field.Name == "immutable" {
			return ReleaseFeatures{
				ImmutableReleases: true,
			}, nil
		}
	}

	return ReleaseFeatures{}, nil
}

func resolveEnterpriseVersion(httpClient *http.Client, host string) (*version.Version, error) {
	var metaResponse struct {
		InstalledVersion string `json:"installed_version"`
	}

	apiClient := api.NewClientFromHTTP(httpClient)
	err := apiClient.REST(host, "GET", "meta", nil, &metaResponse)
	if err != nil {
		return nil, err
	}

	return version.NewVersion(metaResponse.InstalledVersion)
}
