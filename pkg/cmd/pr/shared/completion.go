package shared

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
)

func RequestableReviewersForCompletion(httpClient *http.Client, repo ghrepo.Interface) ([]string, error) {
	client := api.NewClientFromHTTP(api.NewCachedHTTPClient(httpClient, time.Minute*2))

	metadata, err := api.RepoMetadata(client, repo, api.RepoMetadataInput{Reviewers: true})
	if err != nil {
		return nil, err
	}

	results := []string{}
	for _, user := range metadata.AssignableUsers {
		if strings.EqualFold(user.Login, metadata.CurrentLogin) {
			continue
		}
		if user.Name != "" {
			results = append(results, fmt.Sprintf("%s\t%s", user.Login, user.Name))
		} else {
			results = append(results, user.Login)
		}
	}
	for _, team := range metadata.Teams {
		results = append(results, fmt.Sprintf("%s/%s", repo.RepoOwner(), team.Slug))
	}

	sort.Strings(results)
	return results, nil
}
