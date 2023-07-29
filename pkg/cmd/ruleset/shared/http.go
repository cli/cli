package shared

import (
	"errors"
	"net/http"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
)

type RulesetResponse struct {
	Level struct {
		Rulesets struct {
			TotalCount int
			Nodes      []RulesetGraphQL
			PageInfo   struct {
				HasNextPage bool
				EndCursor   string
			}
		}
	}
}

type RulesetList struct {
	TotalCount int
	Rulesets   []RulesetGraphQL
}

func ListRepoRulesets(httpClient *http.Client, repo ghrepo.Interface, limit int, includeParents bool) (*RulesetList, error) {
	variables := map[string]interface{}{
		"owner":          repo.RepoOwner(),
		"repo":           repo.RepoName(),
		"includeParents": includeParents,
	}

	return listRulesets(httpClient, rulesetsQuery(false), variables, limit, repo.RepoHost())
}

func ListOrgRulesets(httpClient *http.Client, orgLogin string, limit int, host string, includeParents bool) (*RulesetList, error) {
	variables := map[string]interface{}{
		"login":          orgLogin,
		"includeParents": includeParents,
	}

	return listRulesets(httpClient, rulesetsQuery(true), variables, limit, host)
}

func listRulesets(httpClient *http.Client, query string, variables map[string]interface{}, limit int, host string) (*RulesetList, error) {
	pageLimit := min(limit, 100)

	res := RulesetList{
		Rulesets: []RulesetGraphQL{},
	}
	client := api.NewClientFromHTTP(httpClient)

	for {
		variables["limit"] = pageLimit
		var data RulesetResponse
		err := client.GraphQL(host, query, variables, &data)
		if err != nil {
			if strings.Contains(err.Error(), "requires one of the following scopes: ['admin:org']") {
				return nil, errors.New("the 'admin:org' scope is required to view organization rulesets, try running 'gh auth refresh -s admin:org'")
			}

			return nil, err
		}

		res.TotalCount = data.Level.Rulesets.TotalCount
		res.Rulesets = append(res.Rulesets, data.Level.Rulesets.Nodes...)

		if len(res.Rulesets) >= limit {
			break
		}

		if data.Level.Rulesets.PageInfo.HasNextPage {
			variables["endCursor"] = data.Level.Rulesets.PageInfo.EndCursor
			pageLimit = min(pageLimit, limit-len(res.Rulesets))
		} else {
			break
		}
	}

	return &res, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func rulesetsQuery(org bool) string {
	if org {
		return orgGraphQLHeader + sharedGraphQLBody
	} else {
		return repoGraphQLHeader + sharedGraphQLBody
	}
}

const repoGraphQLHeader = `
query RepoRulesetList($limit: Int!, $endCursor: String, $includeParents: Boolean, $owner: String!, $repo: String!) {
	level: repository(owner: $owner, name: $repo) {
`

const orgGraphQLHeader = `
query OrgRulesetList($limit: Int!, $endCursor: String, $includeParents: Boolean, $login: String!) {
	level: organization(login: $login) {
`

const sharedGraphQLBody = `
rulesets(first: $limit, after: $endCursor, includeParents: $includeParents) {
	totalCount
	nodes {
		databaseId
		name
		target
		enforcement
		source {
			__typename
			... on Repository { owner: nameWithOwner }
			... on Organization { owner: login }
		}
		rules {
			totalCount
		}
	}
	pageInfo {
		hasNextPage
		endCursor
	}
}}}`
