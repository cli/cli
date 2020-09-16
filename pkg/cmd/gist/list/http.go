package list

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"

	"github.com/cli/cli/api"
	"github.com/cli/cli/pkg/cmd/gist/shared"
)

func listGists(client *http.Client, hostname string, limit int, visibility string) ([]shared.Gist, error) {
	result := []shared.Gist{}

	query := url.Values{}
	if visibility == "all" {
		query.Add("per_page", fmt.Sprintf("%d", limit))
	} else {
		query.Add("per_page", "100")
	}

	// TODO switch to graphql
	apiClient := api.NewClientFromHTTP(client)
	err := apiClient.REST(hostname, "GET", "gists?"+query.Encode(), nil, &result)
	if err != nil {
		return nil, err
	}

	gists := []shared.Gist{}

	for _, gist := range result {
		if len(gists) == limit {
			break
		}
		if visibility == "all" || (visibility == "secret" && !gist.Public) || (visibility == "public" && gist.Public) {
			gists = append(gists, gist)
		}
	}

	sort.SliceStable(gists, func(i, j int) bool {
		return gists[i].UpdatedAt.After(gists[j].UpdatedAt)
	})

	return gists, nil
}
