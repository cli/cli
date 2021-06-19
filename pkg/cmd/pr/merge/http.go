package merge

import (
	"context"
	"net/http"
	"strings"

	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/shurcooL/githubv4"
	"github.com/shurcooL/graphql"
)

type PullRequestMergeMethod int

const (
	PullRequestMergeMethodMerge PullRequestMergeMethod = iota
	PullRequestMergeMethodRebase
	PullRequestMergeMethodSquash
)

type mergePayload struct {
	repo             ghrepo.Interface
	pullRequestID    string
	method           PullRequestMergeMethod
	auto             bool
	commitSubject    string
	setCommitSubject bool
	commitBody       string
	setCommitBody    bool
}

// TODO: drop after githubv4 gets updated
type EnablePullRequestAutoMergeInput struct {
	githubv4.MergePullRequestInput
}

func mergePullRequest(client *http.Client, payload mergePayload) error {
	input := githubv4.MergePullRequestInput{
		PullRequestID: githubv4.ID(payload.pullRequestID),
	}

	switch payload.method {
	case PullRequestMergeMethodMerge:
		m := githubv4.PullRequestMergeMethodMerge
		input.MergeMethod = &m
	case PullRequestMergeMethodRebase:
		m := githubv4.PullRequestMergeMethodRebase
		input.MergeMethod = &m
	case PullRequestMergeMethodSquash:
		m := githubv4.PullRequestMergeMethodSquash
		input.MergeMethod = &m
	}

	if payload.setCommitSubject {
		commitHeadline := githubv4.String(payload.commitSubject)
		input.CommitHeadline = &commitHeadline
	}
	if payload.setCommitBody {
		commitBody := githubv4.String(payload.commitBody)
		input.CommitBody = &commitBody
	}

	variables := map[string]interface{}{
		"input": input,
	}

	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(payload.repo.RepoHost()), client)

	if payload.auto {
		var mutation struct {
			EnablePullRequestAutoMerge struct {
				ClientMutationId string
			} `graphql:"enablePullRequestAutoMerge(input: $input)"`
		}
		variables["input"] = EnablePullRequestAutoMergeInput{input}
		return gql.MutateNamed(context.Background(), "PullRequestAutoMerge", &mutation, variables)
	}

	var mutation struct {
		MergePullRequest struct {
			ClientMutationId string
		} `graphql:"mergePullRequest(input: $input)"`
	}
	return gql.MutateNamed(context.Background(), "PullRequestMerge", &mutation, variables)
}

func disableAutoMerge(client *http.Client, repo ghrepo.Interface, prID string) error {
	var mutation struct {
		DisablePullRequestAutoMerge struct {
			ClientMutationId string
		} `graphql:"disablePullRequestAutoMerge(input: {pullRequestId: $prID})"`
	}

	variables := map[string]interface{}{
		"prID": githubv4.ID(prID),
	}

	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(repo.RepoHost()), client)
	return gql.MutateNamed(context.Background(), "PullRequestAutoMergeDisable", &mutation, variables)
}

func getMergeText(client *http.Client, repo ghrepo.Interface, prID string, mergeMethod PullRequestMergeMethod) (string, error) {
	var method githubv4.PullRequestMergeMethod
	switch mergeMethod {
	case PullRequestMergeMethodMerge:
		method = githubv4.PullRequestMergeMethodMerge
	case PullRequestMergeMethodRebase:
		method = githubv4.PullRequestMergeMethodRebase
	case PullRequestMergeMethodSquash:
		method = githubv4.PullRequestMergeMethodSquash
	}

	var query struct {
		Node struct {
			PullRequest struct {
				ViewerMergeBodyText string `graphql:"viewerMergeBodyText(mergeType: $method)"`
			} `graphql:"...on PullRequest"`
		} `graphql:"node(id: $prID)"`
	}

	variables := map[string]interface{}{
		"prID":   githubv4.ID(prID),
		"method": method,
	}

	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(repo.RepoHost()), client)
	err := gql.QueryNamed(context.Background(), "PullRequestMergeText", &query, variables)
	if err != nil {
		// Tolerate this API missing in older GitHub Enterprise
		if strings.Contains(err.Error(), "Field 'viewerMergeBodyText' doesn't exist") {
			return "", nil
		}
		return "", err
	}

	return query.Node.PullRequest.ViewerMergeBodyText, nil
}
