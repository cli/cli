package view

import (
	"context"
	"net/http"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/cli/v2/pkg/cmd/ruleset/shared"
)

func viewRepoRuleset(ctx context.Context, client *githubrest.Client, repo ghrepo.Interface, databaseId string) (*shared.RulesetREST, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "rulesets", databaseId)
	if err != nil {
		return nil, err
	}
	return viewRuleset(ctx, client, path)
}

func viewOrgRuleset(ctx context.Context, client *githubrest.Client, orgLogin string, databaseId string) (*shared.RulesetREST, error) {
	path, err := safeurl.JoinPath("orgs", orgLogin, "rulesets", databaseId)
	if err != nil {
		return nil, err
	}
	return viewRuleset(ctx, client, path)
}

func viewRuleset(ctx context.Context, client *githubrest.Client, path safeurl.SafeURL) (*shared.RulesetREST, error) {
	result := shared.RulesetREST{}

	req, err := client.NewRequest(ctx, http.MethodGet, path.String(), nil)
	if err != nil {
		return nil, err
	}
	if _, err := client.Do(req, &result); err != nil {
		return nil, err
	}

	return &result, nil
}
