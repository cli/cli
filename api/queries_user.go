package api

import (
	"context"

	"github.com/shurcooL/githubv4"
)

func CurrentLoginName(client *Client) (string, error) {
	var query struct {
		Viewer struct {
			Login string
		}
	}
	v4 := githubv4.NewClient(client.http)
	err := v4.Query(context.Background(), &query, nil)
	return query.Viewer.Login, err
}
