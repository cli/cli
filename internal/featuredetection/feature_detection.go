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
	ProjectFeatures() (ProjectFeatures, error)
	SearchFeatures() (SearchFeatures, error)
	ReleaseFeatures() (ReleaseFeatures, error)
	ActionsFeatures() (ActionsFeatures, error)
}

type IssueFeatures struct {
	// TODO ApiActorsSupported
	// ApiActorsSupported indicates the host supports actor-based APIs. True for
	// github.com and ghe.com, false for GHES.
	//
	// The GitHub API has two generations of assignee/reviewer types:
	//
	// Legacy (GHES): Uses AssignableUser (users only) and node-ID-based mutations.
	//   - assignableUsers query returns []AssignableUser
	//   - Mutations take node IDs (assigneeIds, userReviewerIds, teamReviewerIds)
	//
	// Actor-based (github.com): Uses AssignableActor (User + Bot union) and
	// login-based mutations, enabling assignment of non-user actors like Copilot.
	//   - suggestedActors query returns []AssignableActor (User | Bot)
	//   - suggestedReviewerActors returns []ReviewerCandidate (User | Bot | Team)
	//   - Mutations take logins (replaceActorsForAssignable, requestReviewsByLogin)
	//
	// When GHES adds support for the actor-based types and mutations, this flag
	// can be removed and all // TODO ApiActorsSupported sites collapsed to the
	// actor-only path. To verify GHES support, check whether the GHES GraphQL
	// schema includes:
	//   - The suggestedActors field on Repository (assignee search)
	//   - The suggestedReviewerActors field on PullRequest (reviewer search)
	//   - The replaceActorsForAssignable mutation
	//   - The requestReviewsByLogin mutation
	ApiActorsSupported bool
}

var allIssueFeatures = IssueFeatures{
	ApiActorsSupported: true,
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

type ProjectFeatures struct {
	// ProjectItemQuery indicates support for the `query` argument on
	// ProjectV2.items (supported on github.com and GHES 3.20+).
	ProjectItemQuery bool
}

var allProjectFeatures = ProjectFeatures{
	ProjectItemQuery: true,
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

type ActionsFeatures struct {
	// DispatchRunDetails indicates whether the API supports the `return_run_details`
	// field in workflow dispatches that, when set to true, will return the details
	// of the created workflow run in the response (with status code 200).
	//
	// On older API versions (e.g. GHES 3.20 or earlier), this new field is not
	// supported and setting it will cause an error.
	DispatchRunDetails bool
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

	return IssueFeatures{
		ApiActorsSupported: false, // TODO ApiActorsSupported — actor-based mutations unavailable on GHES
	}, nil
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

func (d *detector) ProjectFeatures() (ProjectFeatures, error) {
	if !ghauth.IsEnterprise(d.host) {
		return allProjectFeatures, nil
	}

	var features ProjectFeatures

	var featureDetection struct {
		ProjectV2 struct {
			Fields []struct {
				Name string
				Args []struct {
					Name string
				}
			} `graphql:"fields(includeDeprecated: true)"`
		} `graphql:"ProjectV2: __type(name: \"ProjectV2\")"`
	}

	gql := api.NewClientFromHTTP(d.httpClient)
	err := gql.Query(d.host, "ProjectV2_fields", &featureDetection, nil)
	if err != nil {
		return features, err
	}

	for _, field := range featureDetection.ProjectV2.Fields {
		if field.Name == "items" {
			for _, arg := range field.Args {
				if arg.Name == "query" {
					features.ProjectItemQuery = true
					break
				}
			}
			break
		}
	}

	return features, nil
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

const (
	enterpriseWorkflowDispatchRunDetailsSupport = "3.21.0"
)

func (d *detector) ActionsFeatures() (ActionsFeatures, error) {
	// TODO workflowDispatchRunDetailsCleanup
	// Once GHES 3.20 support ends, we don't need feature detection for workflow dispatch (i.e. run details support).
	//
	// On github.com, workflow dispatch API now supports a new field named `return_run_details` that enabling it will
	// result in a 200 OK response with the details of the created workflow run. If not set (or set to false), the API
	// will keep the old behavior of returning a 204 No Content response.
	//
	// On GHES (current latest at 3.20), this new field is not available, and setting it will cause a 400 response.
	//
	// Once GHES 3.20 support ends, we can remove the feature detection and start using the new field in API calls.
	//
	// IMPORTANT: In the future REST API versions (i.e. breaking changes), the workflow dispatch endpoint is going to
	// always return the details of the created workflow run in the response, and the `return_run_details` field is
	// going to be ignored/removed. So, once we are migrating to the new API version we should double check the status
	// of the API.

	if !ghauth.IsEnterprise(d.host) {
		return ActionsFeatures{
			DispatchRunDetails: true,
		}, nil
	}

	minSupportedVersion, err := version.NewVersion(enterpriseWorkflowDispatchRunDetailsSupport)
	if err != nil {
		return ActionsFeatures{}, err
	}

	hostVersion, err := resolveEnterpriseVersion(d.httpClient, d.host)
	if err != nil {
		return ActionsFeatures{}, err
	}

	if hostVersion.GreaterThanOrEqual(minSupportedVersion) {
		return ActionsFeatures{
			DispatchRunDetails: true,
		}, nil
	}

	return ActionsFeatures{
		DispatchRunDetails: false,
	}, nil
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
