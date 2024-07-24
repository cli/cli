package api

type SponsorQuery struct {
	User User `json:"user"`
}

type User struct {
	Sponsors Sponsors `json:"sponsors"`
}

type Sponsors struct {
	TotalCount int    `json:"totalCount"`
	Nodes      []Node `json:"nodes"`
}

type Node struct {
	Login string `json:"login"`
}

func GetSponsorsList(client *Client, hostname string, username string) ([]string, error) {
	queryResult, err := querySponsorsViaReflection(client, hostname, username)
	// queryResult, err := querySponsorsViaStringManipulation(client, hostname, username)
	if err != nil {
		return nil, err
	}

	return mapQueryToSponsorsList(queryResult), nil
}

func querySponsorsViaReflection(client *Client, hostname string, username string) (SponsorQuery, error) {
	query := SponsorQuery{}
	variables := map[string]interface{}{
		username: username,
	}

	err := client.Query(hostname, "SponsorsList", &query, variables)
	if err != nil {
		return SponsorQuery{}, err
	}

	return query, nil
}

// func querySponsorsViaStringManipulation(client *Client, hostname string, username string) (SponsorQuery, error) {}

func mapQueryToSponsorsList(queryResult SponsorQuery) []string {
	sponsorsList := []string{}
	for _, node := range queryResult.User.Sponsors.Nodes {
		sponsorsList = append(sponsorsList, node.Login)
	}
	return sponsorsList
}
