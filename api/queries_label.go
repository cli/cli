package api

import (
	"github.com/cli/cli/internal/ghrepo"
	"time"
)

type Label struct {
	Color        string
	CreatedAt    time.Time
	Description  string
	Id           string
	IsDefault    bool
	Name         string
	ResourcePath string
	UpdatedAt    time.Time
	Url          string
}

type LabelsAndTotalCount struct {
	TotalCount int
	Labels     []Label
}

func LabelList(client *Client, repo ghrepo.Interface, first int) (*LabelsAndTotalCount, error) {
	query := `
    query($repo: String!, $first: Int, $endCursor: String) {
    	repository(owner: $owner, name: $repo) {
            labels(first: $first, after: $endCursor, orderBy: {field: CREATED_AT, direction: DESC}) {
                totalCount
                nodes {
                    ...label
                }
                pageInfo {
					hasNextPage
					endCursor
				}
            }
    	}
    }`

	variables := map[string]interface{}{
		"owner": repo.RepoOwner(),
		"repo":  repo.RepoName(),
	}

	var response struct {
		Repository struct {
			Labels struct {
				TotalCount int
				Nodes      []Label
				PageInfo   struct {
					HasNextPage bool
					EndCursor   string
				}
			}
		}
	}

	var labels []Label
	pageLimit := min(first, 100)

loop:
	for {
		variables["first"] = pageLimit
		err := client.GraphQL(query, variables, &response)
		if err != nil {
			return nil, err
		}

		for _, label := range response.Repository.Labels.Nodes {
			labels = append(labels, label)
			if len(labels) == first {
				break loop
			}
		}

		if response.Repository.Labels.PageInfo.HasNextPage {
			variables["endCursor"] = response.Repository.Labels.PageInfo.EndCursor
			pageLimit = min(pageLimit, first-len(labels))
		} else {
			break
		}
	}

	res := LabelsAndTotalCount{Labels: labels, TotalCount: response.Repository.Labels.TotalCount}
	return &res, nil
}
