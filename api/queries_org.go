package api

import (
	"context"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/shurcooL/githubv4"
)

// OrganizationProjects fetches all open projects for an organization
func OrganizationProjects(client *Client, repo ghrepo.Interface) ([]RepoProject, error) {
	type responseData struct {
		Organization struct {
			Projects struct {
				Nodes    []RepoProject
				PageInfo struct {
					HasNextPage bool
					EndCursor   string
				}
			} `graphql:"projects(states: [OPEN], first: 100, orderBy: {field: NAME, direction: ASC}, after: $endCursor)"`
		} `graphql:"organization(login: $owner)"`
	}

	variables := map[string]interface{}{
		"owner":     githubv4.String(repo.RepoOwner()),
		"endCursor": (*githubv4.String)(nil),
	}

	gql := graphQLClient(client.http, repo.RepoHost())

	var projects []RepoProject
	for {
		var query responseData
		err := gql.QueryNamed(context.Background(), "OrganizationProjectList", &query, variables)
		if err != nil {
			return nil, err
		}

		projects = append(projects, query.Organization.Projects.Nodes...)
		if !query.Organization.Projects.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(query.Organization.Projects.PageInfo.EndCursor)
	}

	return projects, nil
}

type OrgTeam struct {
	ID   string
	Slug string
}

// OrganizationTeams fetches all the teams in an organization
func OrganizationTeams(client *Client, repo ghrepo.Interface) ([]OrgTeam, error) {
	type responseData struct {
		Organization struct {
			Teams struct {
				Nodes    []OrgTeam
				PageInfo struct {
					HasNextPage bool
					EndCursor   string
				}
			} `graphql:"teams(first: 100, orderBy: {field: NAME, direction: ASC}, after: $endCursor)"`
		} `graphql:"organization(login: $owner)"`
	}

	variables := map[string]interface{}{
		"owner":     githubv4.String(repo.RepoOwner()),
		"endCursor": (*githubv4.String)(nil),
	}

	gql := graphQLClient(client.http, repo.RepoHost())

	var teams []OrgTeam
	for {
		var query responseData
		err := gql.QueryNamed(context.Background(), "OrganizationTeamList", &query, variables)
		if err != nil {
			return nil, err
		}

		teams = append(teams, query.Organization.Teams.Nodes...)
		if !query.Organization.Teams.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(query.Organization.Teams.PageInfo.EndCursor)
	}

	return teams, nil
}

type OrgRepo struct {
	Name       string
	DatabaseID int `json:"databaseId"`
}

func OrganizationRepos(client *Client, hostname, orgName string) ([]OrgRepo, error) {
	type responseData struct {
		Organization struct {
			Repositories struct {
				Nodes    []OrgRepo
				PageInfo struct {
					HasNextPage bool
					EndCursor   string
				}
			} `graphql:"repositories(first: 100, orderBy: {field: NAME, direction: ASC}, after: $endCursor)"`
		} `graphql:"organization(login: $login)"`
	}

	variables := map[string]interface{}{
		"login":     (githubv4.String)(orgName),
		"endCursor": (*githubv4.String)(nil),
	}

	gql := graphQLClient(client.http, hostname)

	var repositories []OrgRepo
	for {
		var query responseData
		err := gql.QueryNamed(context.Background(), "OrganizationRepositories", &query, variables)
		if err != nil {
			return nil, err
		}

		repositories = append(repositories, query.Organization.Repositories.Nodes...)
		if !query.Organization.Repositories.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(query.Organization.Repositories.PageInfo.EndCursor)
	}

	return repositories, nil
}
