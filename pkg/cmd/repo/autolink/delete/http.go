package delete

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
)

type AutolinkDeleter struct {
	HTTPClient *http.Client
}

func (a *AutolinkDeleter) Delete(repo ghrepo.Interface, id string) error {
	path := fmt.Sprintf("repos/%s/%s/autolinks/%s", repo.RepoOwner(), repo.RepoName(), id)
	err := api.NewClientFromHTTP(a.HTTPClient).REST(repo.RepoHost(), http.MethodDelete, path, nil, nil)
	if err != nil {
		var httpErr api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return fmt.Errorf("error deleting autolink: HTTP 404: Perhaps you are missing admin rights to the repository? (https://api.github.com/%s)", path)
		}
		return err
	}
	return nil
}
