package browse

import (
	"context"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/pkg/cmd/repo/view"
)

type readmeGetter struct {
	client *githubrest.Client

	// ctx is stored on the struct because Get is called from the interactive
	// browse UI, which has no request context of its own to thread through.
	ctx context.Context
}

func newReadmeGetter(ctx context.Context, client *githubrest.Client) *readmeGetter {
	return &readmeGetter{
		client: client,
		ctx:    ctx,
	}
}

func (g *readmeGetter) Get(repoFullName string) (string, error) {
	repo, err := ghrepo.FromFullName(repoFullName)
	if err != nil {
		return "", err
	}
	readme, err := view.RepositoryReadme(g.ctx, g.client, repo, "")
	if err != nil {
		return "", err
	}
	return readme.Content, nil
}
