package listcmd

import (
	"fmt"

	"github.com/cli/cli/v2/api"
	"github.com/shurcooL/githubv4"
)

type GQLSponsorClient struct {
	Hostname  string
	APIClient *api.Client
}

func (c GQLSponsorClient) ListSponsors(user User) (Sponsors, error) {
	var listSponsorsQuery struct {
		User struct {
			Sponsors struct {
				Nodes []struct {
					Organization struct {
						Login string
					} `graphql:"... on Organization"`
					User struct {
						Login string
					} `graphql:"... on User"`
				}
			} `graphql:"sponsors(first: 30)"`
		} `graphql:"user(login: $login)"`
	}

	if err := c.APIClient.Query(c.Hostname, "ListSponsors", &listSponsorsQuery, map[string]interface{}{
		"login": githubv4.String(user),
	}); err != nil {
		return Sponsors{}, fmt.Errorf("list sponsors: %v", err)
	}

	var sponsors Sponsors
	for _, sponsor := range listSponsorsQuery.User.Sponsors.Nodes {
		if sponsor.Organization.Login != "" {
			sponsors = append(sponsors, Sponsor{Login(sponsor.Organization.Login)})
		} else if sponsor.User.Login != "" {
			sponsors = append(sponsors, Sponsor{Login(sponsor.User.Login)})
		}
	}

	return sponsors, nil
}
