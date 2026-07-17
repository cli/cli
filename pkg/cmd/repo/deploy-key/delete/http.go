package delete

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
)

func deleteDeployKey(httpClient *http.Client, repo ghrepo.Interface, id string) error {
	path := fmt.Sprintf("repos/%s/%s/keys/%s", repo.RepoOwner(), repo.RepoName(), id)
	return api.NewClientFromHTTP(httpClient).REST(repo.RepoHost(), "DELETE", path, nil, nil)
}
