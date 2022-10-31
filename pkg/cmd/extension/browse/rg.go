package browse

import (
	"net/http"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/repo/view"
)

type readmeGetter interface {
	Get(string) (string, error)
}

type cachingReadmeGetter struct {
	client *http.Client
}

func newReadmeGetter(client *http.Client, cacheTTL time.Duration) readmeGetter {
	cachingClient := api.NewCachedHTTPClient(client, cacheTTL)
	return &cachingReadmeGetter{
		client: cachingClient,
	}
}

func (g *cachingReadmeGetter) Get(repoFullName string) (string, error) {
	repo, err := ghrepo.FromFullName(repoFullName)
	if err != nil {
		return "", err
	}
	readme, err := view.RepositoryReadme(g.client, repo, "")
	if err != nil {
		return "", err
	}
	return readme.Content, nil
}
