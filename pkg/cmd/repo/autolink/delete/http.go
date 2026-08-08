package delete

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
)

type AutolinkDeleter struct {
	HTTPClient *http.Client
}

func (a *AutolinkDeleter) Delete(repo ghrepo.Interface, id string) error {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "autolinks", id)
	if err != nil {
		return err
	}

	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	err = api.NewClientFromHTTP(a.HTTPClient).REST(repo.RepoHost(), http.MethodDelete, path.String(), nil, nil)
	if err != nil {
		var httpErr api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return fmt.Errorf("error deleting autolink: HTTP 404: Perhaps you are missing admin rights to the repository? (%s)", httpErr.RequestURL)
		}
		return err
	}

	return nil
}
