package list

import (
	"net/http"

	"github.com/cli/cli/v2/api"
)

type SponsorQuery struct {
	User User `json:"user" graphql:"user(login: $username)"`
}

type User struct {
	Sponsors Sponsors `json:"sponsors" graphql:"sponsors(first: 30)"`
}

type Sponsors struct {
	TotalCount int    `json:"totalCount"`
	Nodes      []Node `json:"nodes"`
}

type Node struct {
	Login string `json:"login"`
}

type GetSponsorsList func(*http.Client, string, string) ([]string, error)

type SponsorsListGetter struct {
	get GetSponsorsList
}

func NewSponsorsListGetter(getFn GetSponsorsList) *SponsorsListGetter {
	return &SponsorsListGetter{get: getFn}
}

func getSponsorsList(httpClient *http.Client, hostname string, username string) ([]string, error) {
	client := api.NewClientFromHTTP(httpClient)
	//queryResult, err := querySponsorsViaReflection(client, hostname, username)
	queryResult, err := querySponsorsViaStringManipulation(client, hostname, username)
	if err != nil {
		return nil, err
	}

	return mapQueryToSponsorsList(queryResult), nil
}

// This is currently broken. I suspect the issue is that the SponsorQuery
// struct is not defining the variable $username correctly. I need to go
// find an example query with a variable passed in like this to see how its
// done. In the mean time, I'm going to move forward with the string
// manipulation version of this function.
func querySponsorsViaReflection(client *api.Client, hostname string, username string) (SponsorQuery, error) {
	query := SponsorQuery{}
	variables := map[string]interface{}{
		"username": username,
	}

	err := client.Query(hostname, "SponsorsList", &query, variables)
	if err != nil {
		return SponsorQuery{}, err
	}

	return query, nil
}

func querySponsorsViaStringManipulation(client *api.Client, hostname string, username string) (SponsorQuery, error) {
	type response struct {
		SponsorQuery
	}

	query := `query SponsorsList($username: String!) {
		user(login: $username) {
			sponsors(first: 30) {
				totalCount
				nodes {
					... on User {
						login
					}
					... on Organization {
						login
					}
				}
			}	
		}
	}`
	variables := map[string]interface{}{
		"username": username,
	}
	var data response
	err := client.GraphQL(hostname, query, variables, &data)
	if err != nil {
		return SponsorQuery{}, err
	}

	return data.SponsorQuery, nil
}

func mapQueryToSponsorsList(queryResult SponsorQuery) []string {
	sponsorsList := []string{}
	for _, node := range queryResult.User.Sponsors.Nodes {
		sponsorsList = append(sponsorsList, node.Login)
	}
	return sponsorsList
}
