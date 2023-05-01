package shared

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
)

type RulesetResponse struct {
	Level struct {
		Rulesets struct {
			TotalCount int
			Nodes      []Ruleset
			PageInfo   struct {
				HasNextPage bool
				EndCursor   string
			}
		}
	}
}

type RulesetList struct {
	TotalCount int
	Rulesets   []Ruleset
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
		Rulesets: []Ruleset{},
	}
	client := api.NewClientFromHTTP(httpClient)

	for {
		variables["limit"] = pageLimit
		var data RulesetResponse
		err := client.GraphQL(host, query, variables, &data)
		if err != nil {
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

func rulesetsQuery(org bool) string {
	var args string
	var level string

	if org {
		args = "$login: String!"
		level = "organization(login: $login)"
	} else {
		args = "$owner: String!, $repo: String!"
		level = "repository(owner: $owner, name: $repo)"
	}

	str := fmt.Sprintf("query RulesetList($limit: Int!, $endCursor: String, $includeParents: Boolean, %s) { level: %s {", args, level)

	return str + `
		rulesets(first: $limit, after: $endCursor, includeParents: $includeParents) {
			totalCount
			nodes {
				id: databaseId
				name
				target
				enforcement
				source {
					... on Repository { repoOwner: nameWithOwner }
					... on Organization { orgOwner: login }
				}
				# conditions {
				# 	refName {
				# 		include
				# 		exclude
				# 	}
				# 	repositoryName {
				# 		include
				# 		exclude
				# 		protected
				# 	}
				# }
				rulesGql: rules {
					totalCount
				}
			}
			pageInfo {
				hasNextPage
				endCursor
			}
		}
	}}`
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
