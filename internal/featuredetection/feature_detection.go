package featuredetection

import (
	"net/http"

	"github.com/cli/cli/v2/api"
	"golang.org/x/sync/errgroup"

	ghauth "github.com/cli/go-gh/v2/pkg/auth"
)

type Detector interface {
	IssueFeatures() (IssueFeatures, error)
	PullRequestFeatures() (PullRequestFeatures, error)
	RepositoryFeatures() (RepositoryFeatures, error)
}

type IssueFeatures struct {
	StateReason bool
}

var allIssueFeatures = IssueFeatures{
	StateReason: true,
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
		StateReason: false,
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
