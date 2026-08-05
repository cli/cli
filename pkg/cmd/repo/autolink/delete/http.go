package delete

import (
	"context"
	"errors"
	"fmt"
	"github.com/cli/cli/v2/internal/githubrest"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
)

type AutolinkDeleter struct {
	HTTPClient *http.Client
}

func (a *AutolinkDeleter) Delete(repo ghrepo.Interface, id string) error {
	client, err := api.NewRESTClient(a.HTTPClient, repo.RepoHost())
	if err != nil {
		return err
	}

	url, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "autolinks", id)
	if err != nil {
		return err
	}

	req, err := client.NewRequest(context.Background(), http.MethodDelete, url.String(), nil)
	if err != nil {
		return err
	}

	if _, err := client.Do(req, nil); err != nil {
		var httpErr *githubrest.ErrorResponse
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return fmt.Errorf("error deleting autolink: HTTP 404: Perhaps you are missing admin rights to the repository? (%s)", httpErr.RequestURL)
		}
		return err
	}

	return nil
}
