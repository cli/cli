package branch

import (
	"context"
	"net/http"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/shurcooL/githubv4"
)

type pullRequest struct {
	Number      int
	HeadRefName string
	State       string
	IsDraft     bool
	Author      struct {
		Login string
	}
}

func pullRequests(client *http.Client, repo ghrepo.Interface) ([]pullRequest, error) {
	var query struct {
		Repository struct {
			PullRequests struct {
				Nodes []pullRequest
			} `graphql:"pullRequests(first:30, states: [OPEN, CLOSED, MERGED], orderBy: {field: CREATED_AT, direction: DESC})"`
		} `graphql:"repository(owner:$owner, name:$repo)"`
	}

	variables := map[string]interface{}{
		"owner": githubv4.String(repo.RepoOwner()),
		"repo":  githubv4.String(repo.RepoName()),
	}

	v4 := githubv4.NewClient(client)
	err := v4.Query(context.Background(), &query, variables)
	if err != nil {
		return nil, err
	}

	return query.Repository.PullRequests.Nodes, nil
}
