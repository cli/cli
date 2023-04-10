package list

import (
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

type Ruleset struct {
	Id     string
	Name   string
	Target string
	// Enforcement string
	Conditions struct {
		RefName struct {
			Include []string
			Exclude []string
		}
		RepositoryName struct {
			Include []string
			Exclude []string
		}
	}
}

type RulesetList struct {
	TotalCount int
	Rulesets   []Ruleset
}

func listRepoRulesets(httpClient *http.Client, repo ghrepo.Interface, limit int) (*RulesetList, error) {
	variables := map[string]interface{}{
		"owner": repo.RepoOwner(),
		"repo":  repo.RepoName(),
	}

	return listRulesets(httpClient, rulesetQuery(false), variables, limit, repo.RepoHost())
}

func listOrgRulesets(httpClient *http.Client, orgLogin string, limit int, host string) (*RulesetList, error) {
	variables := map[string]interface{}{
		"login": orgLogin,
	}

	return listRulesets(httpClient, rulesetQuery(true), variables, limit, host)
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

func rulesetQuery(org bool) string {
	var args string
	var level string

	if org {
		args = "$login: String!"
		level = "organization(login: $login)"
	} else {
		args = "$owner: String!, $repo: String!"
		level = "repository(owner: $owner, name: $repo)"
	}

	str := "query RulesetList($limit: Int!, $endCursor: String, " + args + ") { level: " + level + " {"

	str += `
		rulesets(first: $limit, after: $endCursor) {
			totalCount
			nodes {
				id
				#database_id
				name
				target
				#enforcement
				conditions {
					refName {
						include
						exclude
					}
					repositoryName {
						include
						exclude
						protected
					}
				}
				rules {
					totalCount
				}
			}
			pageInfo {
				hasNextPage
				endCursor
			}
		}`

	return str + "}}"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
