package context

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
)

// Context represents the interface for querying information about the current environment
type Context interface {
	Config() (config.Config, error)
}

// cap the number of git remotes looked up, since the user might have an
// unusually large number of git remotes
const maxRemotesForLookup = 5

// ResolveRemotesToRepos takes in a list of git remotes and fetches more information about the repositories they map to.
// Only the git remotes belonging to the same hostname are ever looked up; all others are ignored.
func ResolveRemotesToRepos(remotes Remotes, client *api.Client, base string) (ResolvedRemotes, error) {
	sort.Stable(remotes)

	hasBaseOverride := base != ""
	baseOverride, _ := ghrepo.FromFullName(base)
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
		if ghrepo.IsSame(r, baseOverride) {
			foundBaseOverride = true
		}
		if len(repos) == maxRemotesForLookup {
			break
		}
	}
	if hasBaseOverride && !foundBaseOverride {
		// additionally, look up the explicitly specified base repo if it's not
		// already covered by git remotes
		repos = append(repos, baseOverride)
	}

	result := ResolvedRemotes{
		Remotes:   remotes,
		apiClient: client,
	}
	if hasBaseOverride {
		result.BaseOverride = baseOverride
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

// New initializes a Context that reads from the filesystem
func New() Context {
	return &fsContext{}
}

// A Context implementation that queries the filesystem
type fsContext struct {
	config   config.Config
	branch   string
	baseRepo ghrepo.Interface
}

func (c *fsContext) Config() (config.Config, error) {
	if c.config == nil {
		cfg, err := config.ParseDefaultConfig()
		if errors.Is(err, os.ErrNotExist) {
			cfg = config.NewBlankConfig()
		} else if err != nil {
			return nil, err
		}
		c.config = cfg
	}
	return c.config, nil
}

func (c *fsContext) Branch() (string, error) {
	if c.branch != "" {
		return c.branch, nil
	}

	currentBranch, err := git.CurrentBranch()
	if err != nil {
		return "", fmt.Errorf("could not determine current branch: %w", err)
	}

	c.branch = currentBranch
	return c.branch, nil
}

func (c *fsContext) SetBranch(b string) {
	c.branch = b
}

func (c *fsContext) SetBaseRepo(nwo string) {
	c.baseRepo, _ = ghrepo.FromFullName(nwo)
}
