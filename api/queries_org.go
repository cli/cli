package api

import (
	"context"
	"fmt"

	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/shurcooL/githubv4"
)

// using API v3 here because the equivalent in GraphQL needs `read:org` scope
func resolveOrganization(client *Client, orgName string) (string, error) {
	var response struct {
		NodeID string `json:"node_id"`
	}
	// TODO: GHE support
	err := client.REST(ghinstance.Default(), "GET", fmt.Sprintf("users/%s", orgName), nil, &response)
	return response.NodeID, err
}

// using API v3 here because the equivalent in GraphQL needs `read:org` scope
func resolveOrganizationTeam(client *Client, orgName, teamSlug string) (string, string, error) {
	var response struct {
		NodeID       string `json:"node_id"`
		Organization struct {
			NodeID string `json:"node_id"`
		}
	}
	// TODO: GHE support
	err := client.REST(ghinstance.Default(), "GET", fmt.Sprintf("orgs/%s/teams/%s", orgName, teamSlug), nil, &response)
	return response.Organization.NodeID, response.NodeID, err
}

// OrganizationProjects fetches all open projects for an organization
func OrganizationProjects(client *Client, repo ghrepo.Interface) ([]RepoProject, error) {
	var query struct {
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
	var query struct {
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
