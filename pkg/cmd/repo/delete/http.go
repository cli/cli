package delete

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
)

func deleteRepo(client *http.Client, repo ghrepo.Interface) error {
	noRedirectClient := &http.Client{
		Transport: client.Transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	url := fmt.Sprintf("%srepos/%s",
		ghinstance.RESTPrefix(repo.RepoHost()),
		ghrepo.FullName(repo))

	request, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}

	resp, err := noRedirectClient.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		return api.HandleHTTPError(api.EndpointNeedsScopes(resp, "delete_repo"))
	}

	return nil
}
