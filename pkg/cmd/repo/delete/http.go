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

	request, err := http.NewRequest(http.MethodDelete, url.String(), nil)
	if err != nil {
		return err
	}

	// DoRequest rather than Request because the CheckRedirect set above is a property of the
	// client, and Request delegates to go-gh, which builds a client of its own from the
	// transport alone and so silently drops it. Following the redirect would delete nothing
	// and report success. The cost is that this site keeps an absolute URL and so will not
	// pick up api_host resolution until go-gh can carry a redirect policy.
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	resp, err := api.NewClientFromHTTP(client).DoRequest(request, api.WithEndpointScopes("delete_repo"))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
