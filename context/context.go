// TODO: rename this package to avoid clash with stdlib
package context

import (
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/iostreams"
)

// Cap the number of git remotes to look up, since the user might have an
// unusually large number of git remotes.
const defaultRemotesForLookup = 5

func ResolveRemotesToRepos(remotes Remotes, client *api.Client, base string) (*ResolvedRemotes, error) {
	sort.Stable(remotes)

	result := &ResolvedRemotes{
		remotes:   remotes,
		apiClient: client,
	}

	var baseOverride ghrepo.Interface
	if base != "" {
		var err error
		baseOverride, err = ghrepo.FromFullName(base)
		if err != nil {
			return result, err
		}
		result.baseOverride = baseOverride
	}

	return result, nil
}

func resolveNetwork(result *ResolvedRemotes, remotesForLookup int) error {
	var repos []ghrepo.Interface
	for _, r := range result.remotes {
		repos = append(repos, r)
		if len(repos) == remotesForLookup {
			break
		}
	}

	networkResult, err := api.RepoNetwork(result.apiClient, repos)
	result.network = &networkResult
	return err
}

type ResolvedRemotes struct {
	baseOverride ghrepo.Interface
	remotes      Remotes
	network      *api.RepoNetworkResult
	apiClient    *api.Client
}

func (r *ResolvedRemotes) BaseRepo(io *iostreams.IOStreams) (ghrepo.Interface, error) {
	if r.baseOverride != nil {
		return r.baseOverride, nil
	}

	if len(r.remotes) == 0 {
		return nil, errors.New("no git remotes")
	}

	// if any of the remotes already has a resolution, respect that
	for _, r := range r.remotes {
		if r.Resolved == "base" {
			return r, nil
		} else if r.Resolved != "" {
			repo, err := ghrepo.FromFullName(r.Resolved)
			if err != nil {
				return nil, err
			}
			return ghrepo.NewWithHost(repo.RepoOwner(), repo.RepoName(), r.RepoHost()), nil
		}
	}

	if !io.CanPrompt() {
		// we cannot prompt, so just resort to the 1st remote
		return r.remotes[0], nil
	}

	repos, err := r.NetworkRepos(defaultRemotesForLookup)
	if err != nil {
		return nil, err
	}

	if len(repos) == 0 {
		return r.remotes[0], nil
	} else if len(repos) == 1 {
		return repos[0], nil
	}

	cs := io.ColorScheme()

	fmt.Fprintf(io.ErrOut,
		"%s No default remote repository has been set for this directory.\n",
		cs.FailureIcon())

	fmt.Fprintln(io.Out)

	return nil, errors.New(
		"please run `gh repo set-default` to select a default remote repository.")
}

func (r *ResolvedRemotes) HeadRepos() ([]*api.Repository, error) {
	if r.network == nil {
		err := resolveNetwork(r, defaultRemotesForLookup)
		if err != nil {
			return nil, err
		}
	}

	var results []*api.Repository
	var ids []string // Check if repo duplicates
	for _, repo := range r.network.Repositories {
		if repo != nil && repo.ViewerCanPush() && !slices.Contains(ids, repo.ID) {
			results = append(results, repo)
			ids = append(ids, repo.ID)
		}
	}
	return results, nil
}

// NetworkRepos fetches info about remotes for the network of repos.
// Pass a value of 0 to fetch info on all remotes.
func (r *ResolvedRemotes) NetworkRepos(remotesForLookup int) ([]*api.Repository, error) {
	if r.network == nil {
		err := resolveNetwork(r, remotesForLookup)
		if err != nil {
			return nil, err
		}
	}

	var repos []*api.Repository
	repoMap := map[string]bool{}

	add := func(r *api.Repository) {
		fn := ghrepo.FullName(r)
		if _, ok := repoMap[fn]; !ok {
			repoMap[fn] = true
			repos = append(repos, r)
		}
	}

	for _, repo := range r.network.Repositories {
		if repo == nil {
			continue
		}
		if repo.Parent != nil {
			add(repo.Parent)
		}
		add(repo)
	}

	return repos, nil
}

// RemoteForRepo finds the git remote that points to a repository
func (r *ResolvedRemotes) RemoteForRepo(repo ghrepo.Interface) (*Remote, error) {
	for _, remote := range r.remotes {
		if ghrepo.IsSame(remote, repo) {
			return remote, nil
		}
	}
	return nil, errors.New("not found")
}
