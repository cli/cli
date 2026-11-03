package delete

import (
	"encoding/json"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
)

// forkInfo holds the fields that decide whether deleting a repository also
// deletes its forks. ForksCount includes forks of forks and forks the caller
// cannot see, unlike listing the forks, which only returns visible direct forks.
type forkInfo struct {
	Private    bool `json:"private"`
	Fork       bool `json:"fork"`
	ForksCount int  `json:"forks_count"`
}

// fetchForkInfo uses REST rather than GraphQL so that a missing or moved
// repository fails with the same error the DELETE would have returned.
func fetchForkInfo(httpClient *http.Client, repo ghrepo.Interface) (forkInfo, error) {
	var info forkInfo

	p, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName())
	if err != nil {
		return info, err
	}

	// Match the DELETE's scope hint and redirect policy, so that a missing,
	// under-scoped, or renamed repository fails here exactly as the DELETE would.
	resp, err := api.NewClientFromHTTP(httpClient).Request(repo.RepoHost(), http.MethodGet, p.String(), nil,
		api.WithEndpointScopes("delete_repo"),
		api.WithoutFollowingRedirects(),
	)
	if err != nil {
		return info, err
	}
	defer resp.Body.Close()

	err = json.NewDecoder(resp.Body).Decode(&info)
	return info, err
}

func deleteRepo(httpClient *http.Client, repo ghrepo.Interface) error {
	p, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName())
	if err != nil {
		return err
	}

	// If the repo is renamed (and we're using the old name), the API will return
	// an HTTP 307 (temporary redirect), which is a METHOD-preserving redirection.
	// That is, upon receiving a 307, the HTTP client will repeat the same HTTP
	// method (in this case, DELETE) but against the new location. We don't want
	// such an implicit behaviour, so we pass the `api.WithoutFollowingRedirects`
	// to make sure an error is returned.
	resp, err := api.NewClientFromHTTP(httpClient).Request(repo.RepoHost(), http.MethodDelete, p.String(), nil,
		api.WithEndpointScopes("delete_repo"),
		api.WithoutFollowingRedirects(),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}
