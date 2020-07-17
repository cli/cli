package api

import (
	"context"
)

func CurrentLoginName(client *Client) (string, error) {
	var query struct {
		Viewer struct {
			Login string
		}
	}
	gql := graphQLClient(client.http)
	err := gql.QueryNamed(context.Background(), "UserCurrent", &query, nil)
	return query.Viewer.Login, err
}
