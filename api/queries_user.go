package api

import (
	"context"
	"fmt"
	"net/http"
)

func CurrentLoginNameFor(client *Client, hostname string, token string) (string, error) {
	if token != "" {
		originalTr := client.http.Transport
		client.http = &http.Client{
			Transport: &funcTripper{
				roundTrip: func(req *http.Request) (*http.Response, error) {
					req.Header.Set("Authorization", fmt.Sprintf("token %s", token))
					return originalTr.RoundTrip(req)
				},
			},
		}
	}

	var query struct {
		Viewer struct {
			Login string
		}
	}
	gql := graphQLClient(client.http, hostname)
	err := gql.QueryNamed(context.Background(), "UserCurrent", &query, nil)
	return query.Viewer.Login, err
}

func CurrentLoginName(client *Client, hostname string) (string, error) {
	return CurrentLoginNameFor(client, hostname, "")
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
