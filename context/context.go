// TODO: rename this package to avoid clash with stdlib
package context

import (
	"errors"
	"sort"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
)

// cap the number of git remotes looked up, since the user might have an
// unusually large number of git remotes
const maxRemotesForLookup = 5

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

func resolveNetwork(result *ResolvedRemotes) error {
	var repos []ghrepo.Interface
	for _, r := range result.remotes {
		repos = append(repos, r)
		if len(repos) == maxRemotesForLookup {
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

	// from here on, consult the API
	if r.network == nil {
		err := resolveNetwork(r)
		if err != nil {
			return nil, err
		}
	}

	var repoNames []string
	repoMap := map[string]*api.Repository{}
	add := func(r *api.Repository) {
		fn := ghrepo.FullName(r)
		if _, ok := repoMap[fn]; !ok {
			repoMap[fn] = r
			repoNames = append(repoNames, fn)
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

	if len(repoNames) == 0 {
		return r.remotes[0], nil
	}

	baseName := repoNames[0]
	if len(repoNames) > 1 {
		err := prompt.SurveyAskOne(&survey.Select{
			Message: "Which should be the base repository (used for e.g. querying issues) for this directory?",
			Options: repoNames,
		}, &baseName)
		if err != nil {
			return nil, err
		}
	}

	// determine corresponding git remote
	selectedRepo := repoMap[baseName]
	resolution := "base"
	remote, _ := r.RemoteForRepo(selectedRepo)
	if remote == nil {
		remote = r.remotes[0]
		resolution = ghrepo.FullName(selectedRepo)
	}

	// cache the result to git config
	err := git.SetRemoteResolution(remote.Name, resolution)
	return selectedRepo, err
}

func (r *ResolvedRemotes) HeadRepos() ([]*api.Repository, error) {
	if r.network == nil {
		err := resolveNetwork(r)
		if err != nil {
			return nil, err
		}
	}

	var results []*api.Repository
	for _, repo := range r.network.Repositories {
		if repo != nil && repo.ViewerCanPush() {
			results = append(results, repo)
		}
	}
	return results, nil
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
