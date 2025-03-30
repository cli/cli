package api

import (
	"fmt"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/shurcooL/githubv4"
)

// OrganizationProjects fetches all open projects for an organization.
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

	var projects []RepoProject
	for {
		var query responseData
		err := client.Query(repo.RepoHost(), "OrganizationProjectList", &query, variables)
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

// OrganizationTeam fetch the team in an organization with the given slug
func OrganizationTeam(client *Client, hostname string, org string, teamSlug string) (*OrgTeam, error) {
	type responseData struct {
		Organization struct {
			Team OrgTeam `graphql:"team(slug: $teamSlug)"`
		} `graphql:"organization(login: $owner)"`
	}

	variables := map[string]interface{}{
		"owner":    githubv4.String(org),
		"teamSlug": githubv4.String(teamSlug),
	}

	var query responseData
	err := client.Query(hostname, "OrganizationTeam", &query, variables)
	if err != nil {
		return nil, err
	}
	if query.Organization.Team.ID == "" {
		return nil, fmt.Errorf("could not resolve to a Team with the slug of '%s'", teamSlug)
	}

	return &query.Organization.Team, nil
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

	var teams []OrgTeam
	for {
		var query responseData
		err := client.Query(repo.RepoHost(), "OrganizationTeamList", &query, variables)
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
