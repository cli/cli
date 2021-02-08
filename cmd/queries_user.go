package api

import (
	"context"
)

func CurrentLoginName(client *Client, hostname string) (string, error) {
	var query struct {
		Viewer struct {
			Login string
		}
	}
	gql := graphQLClient(client.http, hostname)
	err := gql.QueryNamed(context.Background(), "UserCurrent", &query, nil)
	return query.Viewer.Login, err
}

func CurrentUserID(client *Client, hostname string) (string, error) {
	var query struct {
		Viewer struct {
			ID string
		}
	}
	gql := graphQLClient(client.http, hostname)
	err := gql.QueryNamed(context.Background(), "UserCurrent", &query, nil)
	return query.Viewer.ID, err
}
