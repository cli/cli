package remoterepo

import (
	"context"
	"errors"
	"net/http"

	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	graphql "github.com/cli/shurcooL-graphql"
	"github.com/shurcooL/githubv4"
)

type Client struct {
	// TODO: accept interfaces instead of funcs
	Repo       func() (ghrepo.Interface, error)
	HTTPClient func() (*http.Client, error)
}

var ErrNotImplemented = errors.New("not implemented")

func (c *Client) PathPrefix(ctx context.Context) (string, error) {
	return "", ErrNotImplemented
}

func (c *Client) LastCommit(ctx context.Context) (*git.Commit, error) {
	httpClient, err := c.HTTPClient()
	if err != nil {
		return nil, err
	}
	repo, err := c.Repo()
	if err != nil {
		return nil, err
	}

	var responseData struct {
		Repository struct {
			DefaultBranchRef struct {
				Target struct {
					Commit struct {
						OID             string `graphql:"oid"`
						MessageHeadline string `graphql:"messageHeadline"`
					} `graphql:"... on Commit"`
				}
			}
		} `graphql:"repository(owner: $owner, name: $repo)"`
	}

	variables := map[string]interface{}{
		"owner": githubv4.String(repo.RepoOwner()),
		"repo":  githubv4.String(repo.RepoName()),
	}

	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(repo.RepoHost()), httpClient)
	if err := gql.QueryNamed(ctx, "RepositoryLastCommit", &responseData, variables); err != nil {
		return nil, err
	}

	commit := responseData.Repository.DefaultBranchRef.Target.Commit
	return &git.Commit{
		Sha:   commit.OID,
		Title: commit.MessageHeadline,
	}, nil
}
