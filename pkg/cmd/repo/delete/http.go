package delete

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
)

// deleteRepo issues the repository deletion request.
//
// This flow keeps its own http.Client so it can disable redirect following and
// detect a renamed or transferred repository. go-gh's REST client always follows
// redirects and does not expose a redirect policy, so `gh repo delete` cannot
// route through it yet. It therefore applies the api_host routing and the
// canonical-host token binding directly here, mirroring what api.Client does for
// commands that can use go-gh.
func deleteRepo(client *http.Client, cfg gh.Config, repo ghrepo.Interface) error {
	oldClient := *client
	client = &oldClient
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	host := repo.RepoHost()
	prefix := ghinstance.RESTPrefix(host)

	// Route the request to the configured api_host, if any, by swapping the host
	// of the derived endpoint while preserving the scheme and path.
	apiHost := config.ResolveAPIHost(cfg, host)
	if apiHost != "" {
		if u, err := url.Parse(prefix); err == nil {
			u.Host = apiHost
			prefix = u.String()
		}
	}

	endpoint := fmt.Sprintf("%srepos/%s", prefix, ghrepo.FullName(repo))

	request, err := http.NewRequest("DELETE", endpoint, nil)
	if err != nil {
		return err
	}

	// The request is addressed to the override host, so bind the token to the
	// canonical host explicitly. The transport would otherwise select it for the
	// override host and send nothing.
	if apiHost != "" {
		if token, _ := cfg.Authentication().ActiveToken(host); token != "" {
			request.Header.Set("Authorization", "token "+token)
		}
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
