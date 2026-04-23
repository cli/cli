package view

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/ruleset/shared"
)

func viewRepoRuleset(httpClient *http.Client, repo ghrepo.Interface, databaseId string) (*shared.RulesetREST, error) {
	path := fmt.Sprintf("repos/%s/%s/rulesets/%s", repo.RepoOwner(), repo.RepoName(), url.PathEscape(databaseId))
	return viewRuleset(httpClient, repo.RepoHost(), path)
}

func viewOrgRuleset(httpClient *http.Client, orgLogin string, databaseId string, host string) (*shared.RulesetREST, error) {
	path := fmt.Sprintf("orgs/%s/rulesets/%s", url.PathEscape(orgLogin), url.PathEscape(databaseId))
	return viewRuleset(httpClient, host, path)
}

func viewRuleset(httpClient *http.Client, hostname string, path string) (*shared.RulesetREST, error) {
	apiClient := api.NewClientFromHTTP(httpClient)
	result := shared.RulesetREST{}

	err := apiClient.REST(hostname, "GET", path, nil, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}
