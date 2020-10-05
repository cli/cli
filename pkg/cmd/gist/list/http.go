package list

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/pkg/cmd/gist/shared"
	"github.com/shurcooL/githubv4"
	"github.com/shurcooL/graphql"
)

func listGists(client *http.Client, hostname string, limit int, visibility string) ([]shared.Gist, error) {
	type response struct {
		Viewer struct {
			Gists struct {
				Nodes []struct {
					Description string
					Files       []struct {
						Name string
					}
					IsPublic  bool
					Name      string
					UpdatedAt time.Time
				}
				PageInfo struct {
					HasNextPage bool
					EndCursor   string
				}
			} `graphql:"gists(first: $per_page, after: $endCursor, privacy: $visibility, orderBy: {field: CREATED_AT, direction: DESC})"`
		}
	}

	perPage := limit
	if perPage > 100 {
		perPage = 100
	}

	variables := map[string]interface{}{
		"per_page":   githubv4.Int(perPage),
		"endCursor":  (*githubv4.String)(nil),
		"visibility": githubv4.GistPrivacy(strings.ToUpper(visibility)),
	}

	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(hostname), client)

	gists := []shared.Gist{}
pagination:
	for {
		var result response
		err := gql.QueryNamed(context.Background(), "GistList", &result, variables)
		if err != nil {
			return nil, err
		}

		for _, gist := range result.Viewer.Gists.Nodes {
			files := map[string]*shared.GistFile{}
			for _, file := range gist.Files {
				files[file.Name] = &shared.GistFile{
					Filename: file.Name,
				}
			}

			gists = append(
				gists,
				shared.Gist{
					ID:          gist.Name,
					Description: gist.Description,
					Files:       files,
					UpdatedAt:   gist.UpdatedAt,
					Public:      gist.IsPublic,
				},
			)
			if len(gists) == limit {
				break pagination
			}
		}

		if !result.Viewer.Gists.PageInfo.HasNextPage {
			break
		}
		variables["endCursor"] = githubv4.String(result.Viewer.Gists.PageInfo.EndCursor)
	}

	return gists, nil
}
