package featuredetection

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	graphql "github.com/cli/shurcooL-graphql"
	"golang.org/x/sync/errgroup"
)

type Detector interface {
	IssueFeatures() (IssueFeatures, error)
	PullRequestFeatures() (PullRequestFeatures, error)
	RepositoryFeatures() (RepositoryFeatures, error)
}

type IssueFeatures struct {
	StatusReason bool
}

var allIssueFeatures = IssueFeatures{
	StatusReason: true,
}

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
	IssueTemplateMutationSupport    bool
	IssueTemplateQuerySupport       bool
	PullRequestTemplateQuerySupport bool
}

var allRepositoryFeatures = RepositoryFeatures{
	IssueTemplateMutationSupport:    true,
	IssueTemplateQuerySupport:       true,
	PullRequestTemplateQuerySupport: true,
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

	features := IssueFeatures{}

	var featureDetection struct {
		Issue struct {
			Fields []struct {
				Name string
			} `graphql:"fields(includeDeprecated: true)"`
		} `graphql:"Issue: __type(name: \"Issue\")"`
	}

	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(d.host), d.httpClient)

	err := gql.QueryNamed(context.Background(), "Issue_fields", &featureDetection, nil)
	if err != nil {
		return features, err
	}

	for _, field := range featureDetection.Issue.Fields {
		if field.Name == "statusReason" {
			features.StatusReason = true
		}
	}

	return features, nil
}

func (d *detector) PullRequestFeatures() (PullRequestFeatures, error) {
	if !ghinstance.IsEnterprise(d.host) {
		return allPullRequestFeatures, nil
	}

	features := PullRequestFeatures{}

	var featureDetection struct {
		PullRequest struct {
			Fields []struct {
				Name string
			} `graphql:"fields(includeDeprecated: true)"`
		} `graphql:"PullRequest: __type(name: \"PullRequest\")"`
		Commit struct {
			Fields []struct {
				Name string
			} `graphql:"fields(includeDeprecated: true)"`
		} `graphql:"Commit: __type(name: \"Commit\")"`
	}

	// needs to be a separate query because the backend only supports 2 `__type` expressions in one query
	var featureDetection2 struct {
		Ref struct {
			Fields []struct {
				Name string
			} `graphql:"fields(includeDeprecated: true)"`
		} `graphql:"Ref: __type(name: \"Ref\")"`
	}

	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(d.host), d.httpClient)

	g := new(errgroup.Group)
	g.Go(func() error {
		return gql.QueryNamed(context.Background(), "PullRequest_fields", &featureDetection, nil)
	})
	g.Go(func() error {
		return gql.QueryNamed(context.Background(), "PullRequest_fields2", &featureDetection2, nil)
	})

	err := g.Wait()
	if err != nil {
		return features, err
	}

	for _, field := range featureDetection.PullRequest.Fields {
		switch field.Name {
		case "reviewDecision":
			features.ReviewDecision = true
		}
	}
	for _, field := range featureDetection.Commit.Fields {
		switch field.Name {
		case "statusCheckRollup":
			features.StatusCheckRollup = true
		}
	}
	for _, field := range featureDetection2.Ref.Fields {
		switch field.Name {
		case "branchProtectionRule":
			features.BranchProtectionRule = true
		}
	}

	return features, nil
}

func (d *detector) RepositoryFeatures() (RepositoryFeatures, error) {
	if !ghinstance.IsEnterprise(d.host) {
		return allRepositoryFeatures, nil
	}

	features := RepositoryFeatures{}

	var featureDetection struct {
		Repository struct {
			Fields []struct {
				Name string
			} `graphql:"fields(includeDeprecated: true)"`
		} `graphql:"Repository: __type(name: \"Repository\")"`
		CreateIssueInput struct {
			InputFields []struct {
				Name string
			}
		} `graphql:"CreateIssueInput: __type(name: \"CreateIssueInput\")"`
	}

	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(d.host), d.httpClient)

	err := gql.QueryNamed(context.Background(), "IssueTemplates_fields", &featureDetection, nil)
	if err != nil {
		return features, err
	}

	fmt.Println("HERE")
	fmt.Println(featureDetection.Repository.Fields)

	for _, field := range featureDetection.Repository.Fields {
		if field.Name == "issueTemplates" {
			features.IssueTemplateQuerySupport = true
		}
		if field.Name == "pullRequestTemplates" {
			features.PullRequestTemplateQuerySupport = true
		}
	}
	for _, field := range featureDetection.CreateIssueInput.InputFields {
		if field.Name == "issueTemplate" {
			features.IssueTemplateMutationSupport = true
		}
	}

	return features, nil
}
