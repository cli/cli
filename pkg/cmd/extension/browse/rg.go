package browse

import (
	"net/http"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/repo/view"
)

type readmeGetter interface {
	Get(string) (string, error)
}

type cachingReadmeGetter struct {
	client *http.Client
	cache  map[string]string
}

func newReadmeGetter(client *http.Client) readmeGetter {
	return &cachingReadmeGetter{
		client: client,
		cache:  map[string]string{},
	}
}

func (g *cachingReadmeGetter) Get(repoFullName string) (string, error) {
	// TODO switch to cachine to disk
	if readme, ok := g.cache[repoFullName]; ok {
		return readme, nil
	}
	repo, err := ghrepo.FromFullName(repoFullName)
	readme, err := view.RepositoryReadme(g.client, repo, "")
	if err != nil {
		return "", err
	}
	g.cache[repoFullName] = readme.Content
	return readme.Content, nil
}
