package list

import (
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
)

type RepositoryRuleset struct {
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

type RepositoryRulesetList struct {
	TotalCount int
	Rulesets   []RepositoryRuleset
}

func listRepoRulesets(httpClient *http.Client, repo ghrepo.Interface, limit int) (*RepositoryRulesetList, error) {
	type response struct {
		Repository struct {
			Rulesets struct {
				TotalCount int
				Nodes      []RepositoryRuleset
			}
		}
	}

	query := `
		query RepositoryRulesetList(
			$owner: String!,
			$repo: String!,
			$limit: Int!
		) {
			repository(owner: $owner, name: $repo) {
				rulesets(first: $limit) {
					totalCount
					nodes {
						id
						name
						target
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
						},
						rules {
							totalCount
						}
					}
				}
			}
		}
	`

	variables := map[string]interface{}{
		"owner": repo.RepoOwner(),
		"repo":  repo.RepoName(),
		"limit": limit,
	}

	client := api.NewClientFromHTTP(httpClient)
	var res response
	err := client.GraphQL(repo.RepoHost(), query, variables, &res)
	if err != nil {
		return nil, err
	}

	list := RepositoryRulesetList{
		TotalCount: res.Repository.Rulesets.TotalCount,
		Rulesets:   res.Repository.Rulesets.Nodes,
	}

	return &list, nil
}
