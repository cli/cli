package list

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/cli/cli/api"
)

type Gist struct {
	Description string
	HTMLURL     string    `json:"html_url"`
	UpdatedAt   time.Time `json:"updated_at"`
	Public      bool
}

func listGists(client *http.Client, hostname string, limit int, visibility string) ([]Gist, error) {
	result := []Gist{}

	query := url.Values{}
	if visibility == "all" {
		query.Add("per_page", fmt.Sprintf("%d", limit))
	} else {
		query.Add("per_page", "100")
	}

	apiClient := api.NewClientFromHTTP(client)
	err := apiClient.REST(hostname, "GET", "gists?"+query.Encode(), nil, &result)
	if err != nil {
		return nil, err
	}

	gists := []Gist{}

	for _, gist := range result {
		if len(gists) == limit {
			break
		}
		if visibility == "all" || (visibility == "private" && !gist.Public) || (visibility == "public" && gist.Public) {
			gists = append(gists, gist)
		}
	}

	sort.SliceStable(gists, func(i, j int) bool {
		return gists[i].UpdatedAt.After(gists[j].UpdatedAt)
	})

	return gists, nil
}
