package label

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
)

const (
	maxPageSize = 100

	defaultOrder = "asc"
	defaultSort  = "created"
)

var defaultFields = []string{
	"color",
	"description",
	"name",
}

type listLabelsResponseData struct {
	Repository struct {
		Labels struct {
			TotalCount int
			Nodes      []label
			PageInfo   struct {
				HasNextPage bool
				EndCursor   string
			}
		}
	}
}

type listQueryOptions struct {
	Limit int
	Query string
	Order string
	Sort  string

	fields []string
}

func (opts listQueryOptions) OrderBy() map[string]string {
	// Set on copy to keep original intact.
	if opts.Order == "" {
		opts.Order = defaultOrder
	}
	if opts.Sort == "" {
		opts.Sort = defaultSort
	}

	field := strings.ToUpper(opts.Sort)
	if opts.Sort == "created" {
		field = "CREATED_AT"
	}
	return map[string]string{
		"direction": strings.ToUpper(opts.Order),
		"field":     field,
	}
}

// listLabels lists the labels in the given repo. Pass -1 for limit to list all labels;
// otherwise, only that number of labels is returned for any number of pages.
func listLabels(client *http.Client, repo ghrepo.Interface, opts listQueryOptions) ([]label, int, error) {
	if len(opts.fields) == 0 {
		opts.fields = defaultFields
	}

	apiClient := api.NewClientFromHTTP(client)
	fragment := fmt.Sprintf("fragment label on Label{%s}", strings.Join(opts.fields, ","))
	query := fragment + `
	query LabelList($owner: String!, $repo: String!, $limit: Int!, $endCursor: String, $query: String, $orderBy: LabelOrder) {
		repository(owner: $owner, name: $repo) {
			labels(first: $limit, after: $endCursor, query: $query, orderBy: $orderBy) {
				totalCount,
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
		"owner":   repo.RepoOwner(),
		"repo":    repo.RepoName(),
		"orderBy": opts.OrderBy(),
		"query":   opts.Query,
	}

	var labels []label
	var totalCount int

loop:
	for {
		var response listLabelsResponseData
		variables["limit"] = determinePageSize(opts.Limit - len(labels))
		err := apiClient.GraphQL(repo.RepoHost(), query, variables, &response)
		if err != nil {
			return nil, 0, err
		}

		totalCount = response.Repository.Labels.TotalCount

		for _, label := range response.Repository.Labels.Nodes {
			labels = append(labels, label)
			if len(labels) == opts.Limit {
				break loop
			}
		}

		if response.Repository.Labels.PageInfo.HasNextPage {
			variables["endCursor"] = response.Repository.Labels.PageInfo.EndCursor
		} else {
			break
		}
	}

	return labels, totalCount, nil
}

func determinePageSize(numRequestedItems int) int {
	// If numRequestedItems is -1 then retrieve maxPageSize
	if numRequestedItems < 0 || numRequestedItems > maxPageSize {
		return maxPageSize
	}
	return numRequestedItems
}
