package context

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
)

// cap the number of git remotes looked up, since the user might have an
// unusually large number of git remotes
const maxRemotesForLookup = 5

// ResolveRemotesToRepos takes in a list of git remotes and fetches more information about the repositories they map to.
// Only the git remotes belonging to the same hostname are ever looked up; all others are ignored.
func ResolveRemotesToRepos(remotes Remotes, client *api.Client, base string) (ResolvedRemotes, error) {
	sort.Stable(remotes)

	result := ResolvedRemotes{
		Remotes:   remotes,
		apiClient: client,
	}

	var baseOverride ghrepo.Interface
	if base != "" {
		var err error
		baseOverride, err = ghrepo.FromFullName(base)
		if err != nil {
			return result, err
		}
		result.BaseOverride = baseOverride
	}

	foundBaseOverride := false
	var hostname string
	var repos []ghrepo.Interface
	for i, r := range remotes {
		if i == 0 {
			hostname = r.RepoHost()
		} else if !strings.EqualFold(r.RepoHost(), hostname) {
			// ignore all remotes for a hostname different to that of the 1st remote
			continue
		}
		repos = append(repos, r)
		if baseOverride != nil && ghrepo.IsSame(r, baseOverride) {
			foundBaseOverride = true
		}
		if len(repos) == maxRemotesForLookup {
			break
		}
	}
	if baseOverride != nil && !foundBaseOverride {
		// additionally, look up the explicitly specified base repo if it's not
		// already covered by git remotes
		repos = append(repos, baseOverride)
	}

	networkResult, err := api.RepoNetwork(client, repos)
	if err != nil {
		return result, err
	}
	result.Network = networkResult
	return result, nil
}

type ResolvedRemotes struct {
	BaseOverride ghrepo.Interface
	Remotes      Remotes
	Network      api.RepoNetworkResult
	apiClient    *api.Client
}

// BaseRepo is the first found repository in the "upstream", "github", "origin"
// git remote order, resolved to the parent repo if the git remote points to a fork
func (r ResolvedRemotes) BaseRepo() (*api.Repository, error) {
	if r.BaseOverride != nil {
		for _, repo := range r.Network.Repositories {
			if repo != nil && ghrepo.IsSame(repo, r.BaseOverride) {
				return repo, nil
			}
		}
		return nil, fmt.Errorf("failed looking up information about the '%s' repository",
			ghrepo.FullName(r.BaseOverride))
	}

	for _, repo := range r.Network.Repositories {
		if repo == nil {
			continue
		}
		if repo.IsFork() {
			return repo.Parent, nil
		}
		return repo, nil
	}

	return nil, errors.New("not found")
}

// HeadRepo is a fork of base repo (if any), or the first found repository that
// has push access
func (r ResolvedRemotes) HeadRepo() (*api.Repository, error) {
	baseRepo, err := r.BaseRepo()
	if err != nil {
		return nil, err
	}

	// try to find a pushable fork among existing remotes
	for _, repo := range r.Network.Repositories {
		if repo != nil && repo.Parent != nil && repo.ViewerCanPush() && ghrepo.IsSame(repo.Parent, baseRepo) {
			return repo, nil
		}
	}

	// a fork might still exist on GitHub, so let's query for it
	var notFound *api.NotFoundError
	if repo, err := api.RepoFindFork(r.apiClient, baseRepo); err == nil {
		return repo, nil
	} else if !errors.As(err, &notFound) {
		return nil, err
	}

	// fall back to any listed repository that has push access
	for _, repo := range r.Network.Repositories {
		if repo != nil && repo.ViewerCanPush() {
			return repo, nil
		}
	}
	return nil, errors.New("none of the repositories have push access")
}

// RemoteForRepo finds the git remote that points to a repository
func (r ResolvedRemotes) RemoteForRepo(repo ghrepo.Interface) (*Remote, error) {
	for i, remote := range r.Remotes {
		if ghrepo.IsSame(remote, repo) ||
			// additionally, look up the resolved repository name in case this
			// git remote points to this repository via a redirect
			(r.Network.Repositories[i] != nil && ghrepo.IsSame(r.Network.Repositories[i], repo)) {
			return remote, nil
		}
	}
	return nil, errors.New("not found")
}
