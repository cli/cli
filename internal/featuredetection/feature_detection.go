package featuredetection

import (
	"context"
	"net/http"

	"github.com/cli/cli/v2/internal/ghinstance"
	graphql "github.com/cli/shurcooL-graphql"
)

type Detector interface {
	IssueFeatures() (IssueFeatures, error)
	PullRequestFeatures() (PullRequestFeatures, error)
	RepositoryFeatures() (RepositoryFeatures, error)
}

type IssueFeatures struct{}

var allIssueFeatures = IssueFeatures{}

type PullRequestFeatures struct {
	ReviewDecision       bool
	StatusCheckRollup    bool
	BranchProtectionRule bool
	MergeQueue           bool
}

var allPullRequestFeatures = PullRequestFeatures{
	ReviewDecision:       true,
	StatusCheckRollup:    true,
	BranchProtectionRule: true,
	MergeQueue:           true,
}

type RepositoryFeatures struct {
	IssueTemplateMutation    bool
	IssueTemplateQuery       bool
	PullRequestTemplateQuery bool
	VisibilityField          bool
	AutoMerge                bool
}

var allRepositoryFeatures = RepositoryFeatures{
	IssueTemplateMutation:    true,
	IssueTemplateQuery:       true,
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
	if !ghinstance.IsEnterprise(d.host) {
		return allIssueFeatures, nil
	}

	return allIssueFeatures, nil
}

func (d *detector) PullRequestFeatures() (PullRequestFeatures, error) {
	// TODO: reinstate the short-circuit once the APIs are fully available on github.com
	// https://github.com/cli/cli/issues/5778
	//
	// if !ghinstance.IsEnterprise(d.host) {
	// 	return allPullRequestFeatures, nil
	// }

	features := PullRequestFeatures{
		ReviewDecision:       true,
		StatusCheckRollup:    true,
		BranchProtectionRule: true,
	}

	var featureDetection struct {
		PullRequest struct {
			Fields []struct {
				Name string
			} `graphql:"fields(includeDeprecated: true)"`
		} `graphql:"PullRequest: __type(name: \"PullRequest\")"`
	}

	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(d.host), d.httpClient)

	err := gql.QueryNamed(context.Background(), "PullRequest_fields", &featureDetection, nil)
	if err != nil {
		return features, err
	}

	for _, field := range featureDetection.PullRequest.Fields {
		if field.Name == "isInMergeQueue" {
			features.MergeQueue = true
		}
	}

	return features, nil
}

func (d *detector) RepositoryFeatures() (RepositoryFeatures, error) {
	if !ghinstance.IsEnterprise(d.host) {
		return allRepositoryFeatures, nil
	}

	features := RepositoryFeatures{
		IssueTemplateQuery:    true,
		IssueTemplateMutation: true,
	}

	var featureDetection struct {
		Repository struct {
			Fields []struct {
				Name string
			} `graphql:"fields(includeDeprecated: true)"`
		} `graphql:"Repository: __type(name: \"Repository\")"`
	}

	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(d.host), d.httpClient)

	err := gql.QueryNamed(context.Background(), "Repository_fields", &featureDetection, nil)
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
