package delete

import (
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
)

func deleteRepo(client *http.Client, repo ghrepo.Interface) error {
	oldClient := *client
	client = &oldClient
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	url, err := safeurl.JoinPathWithHostPrefix(ghinstance.RESTPrefix(repo.RepoHost()), "repos", repo.RepoOwner(), repo.RepoName())
	if err != nil {
		return err
	}

	request, err := http.NewRequest("DELETE", url.String(), nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		api.EndpointNeedsScopes(resp, "delete_repo")
		return api.HandleHTTPError(resp)
	}

	return nil
}
