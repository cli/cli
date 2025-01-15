package merge

import (
	"net/http"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/shurcooL/githubv4"
)

type PullRequestMergeMethod int

const (
	PullRequestMergeMethodMerge PullRequestMergeMethod = iota
	PullRequestMergeMethodRebase
	PullRequestMergeMethodSquash
)

const (
	MergeStateStatusBehind   = "BEHIND"
	MergeStateStatusBlocked  = "BLOCKED"
	MergeStateStatusClean    = "CLEAN"
	MergeStateStatusDirty    = "DIRTY"
	MergeStateStatusHasHooks = "HAS_HOOKS"
	MergeStateStatusMerged   = "MERGED"
	MergeStateStatusUnstable = "UNSTABLE"
)

type mergePayload struct {
	repo            ghrepo.Interface
	pullRequestID   string
	method          PullRequestMergeMethod
	auto            bool
	commitSubject   string
	commitBody      string
	setCommitBody   bool
	expectedHeadOid string
	authorEmail     string
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

	if payload.authorEmail != "" {
		authorEmail := githubv4.String(payload.authorEmail)
		input.AuthorEmail = &authorEmail
	}
	if payload.commitSubject != "" {
		commitHeadline := githubv4.String(payload.commitSubject)
		input.CommitHeadline = &commitHeadline
	}
	if payload.setCommitBody {
		commitBody := githubv4.String(payload.commitBody)
		input.CommitBody = &commitBody
	}

	if payload.expectedHeadOid != "" {
		expectedHeadOid := githubv4.GitObjectID(payload.expectedHeadOid)
		input.ExpectedHeadOid = &expectedHeadOid
	}

	variables := map[string]interface{}{
		"input": input,
	}

	gql := api.NewClientFromHTTP(client)

	if payload.auto {
		var mutation struct {
			EnablePullRequestAutoMerge struct {
				ClientMutationId string
			} `graphql:"enablePullRequestAutoMerge(input: $input)"`
		}
		variables["input"] = EnablePullRequestAutoMergeInput{input}
		return gql.Mutate(payload.repo.RepoHost(), "PullRequestAutoMerge", &mutation, variables)
	}

	var mutation struct {
		MergePullRequest struct {
			ClientMutationId string
		} `graphql:"mergePullRequest(input: $input)"`
	}
	return gql.Mutate(payload.repo.RepoHost(), "PullRequestMerge", &mutation, variables)
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

	gql := api.NewClientFromHTTP(client)
	return gql.Mutate(repo.RepoHost(), "PullRequestAutoMergeDisable", &mutation, variables)
}

func getMergeText(client *http.Client, repo ghrepo.Interface, prID string, mergeMethod PullRequestMergeMethod) (string, string, error) {
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
				ViewerMergeHeadlineText string `graphql:"viewerMergeHeadlineText(mergeType: $method)"`
				ViewerMergeBodyText     string `graphql:"viewerMergeBodyText(mergeType: $method)"`
			} `graphql:"...on PullRequest"`
		} `graphql:"node(id: $prID)"`
	}

	variables := map[string]interface{}{
		"prID":   githubv4.ID(prID),
		"method": method,
	}

	gql := api.NewClientFromHTTP(client)
	err := gql.Query(repo.RepoHost(), "PullRequestMergeText", &query, variables)
	if err != nil {
		// Tolerate this API missing in older GitHub Enterprise
		if strings.Contains(err.Error(), "Field 'viewerMergeHeadlineText' doesn't exist") ||
			strings.Contains(err.Error(), "Field 'viewerMergeBodyText' doesn't exist") {
			return "", "", nil
		}
		return "", "", err
	}

	return query.Node.PullRequest.ViewerMergeHeadlineText, query.Node.PullRequest.ViewerMergeBodyText, nil
}
