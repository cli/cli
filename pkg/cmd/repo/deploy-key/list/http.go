package list

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
)

type deployKey struct {
	ID        int64     `json:"id"`
	Key       string    `json:"key"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	ReadOnly  bool      `json:"read_only"`
}

func repoKeys(httpClient *http.Client, repo ghrepo.Interface) ([]deployKey, error) {
	path := fmt.Sprintf("repos/%s/%s/keys?per_page=100", repo.RepoOwner(), repo.RepoName())
	var keys []deployKey
	if err := api.NewClientFromHTTP(httpClient).REST(repo.RepoHost(), "GET", path, nil, &keys); err != nil {
		return nil, err
	}
	return keys, nil
}
