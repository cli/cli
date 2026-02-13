package factory

import (
	"strings"

	ghContext "github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
)

type dynamicRepoTokenGetter struct {
	authCfg gh.AuthConfig
	repo    ghrepo.Interface
	mapper  gh.RepositoryUserResolver
}

func withRepositoryTokenMapping(authCfg gh.AuthConfig, repo ghrepo.Interface) interface{ ActiveToken(string) (string, string) } {
	if repo == nil {
		return authCfg
	}

	mapper, ok := authCfg.(gh.RepositoryUserResolver)
	if !ok {
		return authCfg
	}

	return dynamicRepoTokenGetter{
		authCfg: authCfg,
		repo:    repo,
		mapper:  mapper,
	}
}

func (t dynamicRepoTokenGetter) ActiveToken(hostname string) (string, string) {
	defaultToken, defaultSource := t.authCfg.ActiveToken(hostname)

	if strings.HasSuffix(defaultSource, "_TOKEN") {
		return defaultToken, defaultSource
	}

	if !strings.EqualFold(hostname, t.repo.RepoHost()) {
		return defaultToken, defaultSource
	}

	mappedUser, ok := t.mapper.UserForRepository(hostname, t.repo.RepoOwner(), t.repo.RepoName())
	if !ok {
		return defaultToken, defaultSource
	}

	mappedToken, mappedSource, err := t.authCfg.TokenForUser(hostname, mappedUser)
	if err != nil || mappedToken == "" {
		return defaultToken, defaultSource
	}

	return mappedToken, mappedSource
}
func repoContextFromRemotes(remotes ghContext.Remotes) (ghrepo.Interface, bool) {
	return remotes.RepoContext()
}
