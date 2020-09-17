package list

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/cli/cli/api"
	"github.com/cli/cli/pkg/cmd/gist/shared"
)

func listGists(client *http.Client, hostname string, limit int, visibility string) ([]shared.Gist, error) {
	type response struct {
		Viewer struct {
			Gists struct {
				Nodes []struct {
					Description string
					Files       []struct {
						Name     string
						Language struct {
							Name string
						}
						Extension string
					}
					IsPublic  bool
					Name      string
					UpdatedAt time.Time
				}
			}
		}
	}

	query := `
	query ListGists($visibility: GistPrivacy!, $per_page: Int = 10) {
		viewer {
			gists(first: $per_page, privacy: $visibility) {
				nodes {
					name
					files {
						name
						language {
							name
						}
						extension
					}
					description
					updatedAt
					isPublic
				}
			}
		}
	}`

	if visibility != "all" {
		limit = 100
	}
	variables := map[string]interface{}{
		"per_page":   limit,
		"visibility": strings.ToUpper(visibility),
	}

	apiClient := api.NewClientFromHTTP(client)
	var result response
	err := apiClient.GraphQL(hostname, query, variables, &result)
	if err != nil {
		return nil, err
	}

	gists := []shared.Gist{}
	for _, gist := range result.Viewer.Gists.Nodes {

		files := map[string]*shared.GistFile{}
		for _, file := range gist.Files {
			files[file.Name] = &shared.GistFile{
				Filename: file.Name,
				Type:     file.Extension,
				Language: file.Language.Name,
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
	}

	sort.SliceStable(gists, func(i, j int) bool {
		return gists[i].UpdatedAt.After(gists[j].UpdatedAt)
	})

	return gists, nil
}
