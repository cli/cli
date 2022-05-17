package featuredetection

import (
	"context"
	"net/http"
	"time"

	"github.com/cli/cli/v2/api"
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
}

var allPullRequestFeatures = PullRequestFeatures{
	ReviewDecision:       true,
	StatusCheckRollup:    true,
	BranchProtectionRule: true,
}

type RepositoryFeatures struct {
	IssueTemplateMutation    bool
	IssueTemplateQuery       bool
	PullRequestTemplateQuery bool
}

var allRepositoryFeatures = RepositoryFeatures{
	IssueTemplateMutation:    true,
	IssueTemplateQuery:       true,
	PullRequestTemplateQuery: true,
}

type detector struct {
	host       string
	httpClient *http.Client
}

func NewDetector(httpClient *http.Client, host string) Detector {
	cachedClient := api.NewCachedClient(httpClient, time.Hour*48)
	return &detector{
		httpClient: cachedClient,
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
	if !ghinstance.IsEnterprise(d.host) {
		return allPullRequestFeatures, nil
	}

	return allPullRequestFeatures, nil
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
	}

	return features, nil
}
