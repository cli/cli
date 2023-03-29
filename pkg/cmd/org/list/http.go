package list

import (
	"net/http"

	"github.com/cli/cli/v2/api"
)

type Organization struct {
	Login string
}

func listOrgs(httpClient *http.Client, hostname string, limit int) (*[]Organization, error) {
	type response struct {
		User struct {
			Login         string
			Organizations struct {
				Nodes    []Organization
				PageInfo struct {
					HasNextPage bool
					EndCursor   string
				}
			} `graphql:"organizations(first: $limit, after: $endCursor)"`
		}
	}

	query := `query OrganizationList($user: String!, $limit: Int!, $endCursor: String) {
		user(login: $user) {
			login
			organizations(first: $limit, after: $endCursor) {
				nodes {
					login
				}
				pageInfo {
					hasNextPage
					endCursor
				}
			}
		}
	}`

	client := api.NewClientFromHTTP(httpClient)

	user, err := api.CurrentLoginName(client, hostname)
	if err != nil {
		return nil, err
	}

	orgs := []Organization{}
	pageLimit := min(limit, 100)
	variables := map[string]interface{}{
		"user": user,
	}

loop:
	for {
		variables["limit"] = pageLimit
		var data response
		err := client.GraphQL(hostname, query, variables, &data)
		if err != nil {
			return nil, err
		}

		for _, org := range data.User.Organizations.Nodes {
			orgs = append(orgs, org)
			if len(orgs) == limit {
				break loop
			}
		}

		if data.User.Organizations.PageInfo.HasNextPage {
			variables["endCursor"] = data.User.Organizations.PageInfo.EndCursor
			pageLimit = min(pageLimit, limit-len(orgs))
		} else {
			break
		}
	}

	return &orgs, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
