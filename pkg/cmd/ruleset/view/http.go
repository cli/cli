package view

import (
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/cli/cli/v2/pkg/cmd/ruleset/shared"
)

func viewRepoRuleset(httpClient *http.Client, repo ghrepo.Interface, databaseId string) (*shared.RulesetREST, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "rulesets", databaseId)
	if err != nil {
		return nil, err
	}
	return viewRuleset(httpClient, repo.RepoHost(), path)
}

func viewOrgRuleset(httpClient *http.Client, orgLogin string, databaseId string, host string) (*shared.RulesetREST, error) {
	path, err := safeurl.JoinPath("orgs", orgLogin, "rulesets", databaseId)
	if err != nil {
		return nil, err
	}
	return viewRuleset(httpClient, host, path)
}

func viewRuleset(httpClient *http.Client, hostname string, path safeurl.SafeURL) (*shared.RulesetREST, error) {
	apiClient := api.NewClientFromHTTP(httpClient)
	result := shared.RulesetREST{}

	err := apiClient.REST(hostname, "GET", path.String(), nil, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}
