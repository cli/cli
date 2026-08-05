package list

import (
	"context"
	"net/http"
	"time"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/internal/safeurl"
)

type deployKey struct {
	ID        int64     `json:"id"`
	Key       string    `json:"key"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	ReadOnly  bool      `json:"read_only"`
}

func repoKeys(ctx context.Context, client *githubrest.Client, repo ghrepo.Interface) ([]deployKey, error) {
	u, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "keys")
	if err != nil {
		return nil, err
	}
	u.SetQuery("per_page", "100")

	var keys []deployKey
	req, err := client.NewRequest(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	if _, err := client.Do(req, &keys); err != nil {
		return nil, err
	}

	return keys, nil
}
