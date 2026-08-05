package shared

import (
	"context"
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/internal/safeurl"
)

// BranchDeleteRemote deletes the remote branch on the given repository.
func BranchDeleteRemote(ctx context.Context, client *githubrest.Client, repo ghrepo.Interface, branch string) error {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "git", "refs", fmt.Sprintf("heads/%s", branch))
	if err != nil {
		return err
	}
	req, err := client.NewRequest(ctx, http.MethodDelete, path.String(), nil)
	if err != nil {
		return err
	}
	_, err = client.Do(req, nil)
	return err
}
