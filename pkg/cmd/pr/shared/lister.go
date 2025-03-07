package shared

import (
	"fmt"
	"net/http"

	"github.com/cli/cli/v2/internal/ghrepo"

	api "github.com/cli/cli/v2/api"
)

type PRLister interface {
	List(opt ListOptions) (*api.PullRequestAndTotalCount, error)
}

type ListOptions struct {
	BaseRepo ghrepo.Interface

	LimitResults int

	State      string
	BaseBranch string
	HeadBranch string

	Fields []string
}

type lister struct {
	httpClient *http.Client
}

func NewLister(httpClient *http.Client) PRLister {
	return &lister{
		httpClient: httpClient,
	}
}

func (l *lister) List(opts ListOptions) (*api.PullRequestAndTotalCount, error) {
	type response struct {
		Repository struct {
			PullRequests struct {
				Nodes    []api.PullRequest
				PageInfo struct {
					HasNextPage bool
					EndCursor   string
				}
				TotalCount int
			}
		}
	}
	limit := opts.LimitResults
	fragment := fmt.Sprintf("fragment pr on PullRequest{%s}", api.PullRequestGraphQL(opts.Fields))
	query := fragment + `
		query PullRequestList(
			$owner: String!,
			$repo: String!,
			$limit: Int!,
			$endCursor: String,
			$baseBranch: String,
			$headBranch: String,
			$state: [PullRequestState!] = OPEN
		) {
			repository(owner: $owner, name: $repo) {
				pullRequests(
					states: $state,
					baseRefName: $baseBranch,
					headRefName: $headBranch,
					first: $limit,
					after: $endCursor,
					orderBy: {field: CREATED_AT, direction: DESC}
				) {
					totalCount
					nodes {
						...pr
					}
					pageInfo {
						hasNextPage
						endCursor
					}
				}
			}
		}`

	pageLimit := min(limit, 100)
	variables := map[string]interface{}{
		"owner": opts.BaseRepo.RepoOwner(),
		"repo":  opts.BaseRepo.RepoName(),
	}

	switch opts.State {
	case "open":
		variables["state"] = []string{"OPEN"}
	case "closed":
		variables["state"] = []string{"CLOSED", "MERGED"}
	case "merged":
		variables["state"] = []string{"MERGED"}
	case "all":
		variables["state"] = []string{"OPEN", "CLOSED", "MERGED"}
	default:
		return nil, fmt.Errorf("invalid state: %s", opts.State)
	}

	if opts.BaseBranch != "" {
		variables["baseBranch"] = opts.BaseBranch
	}
	if opts.HeadBranch != "" {
		variables["headBranch"] = opts.HeadBranch
	}

	res := api.PullRequestAndTotalCount{}
	var check = make(map[int]struct{})
	client := api.NewClientFromHTTP(l.httpClient)

loop:
	for {
		variables["limit"] = pageLimit
		var data response
		err := client.GraphQL(opts.BaseRepo.RepoHost(), query, variables, &data)
		if err != nil {
			return nil, err
		}
		prData := data.Repository.PullRequests
		res.TotalCount = prData.TotalCount

		for _, pr := range prData.Nodes {
			if _, exists := check[pr.Number]; exists && pr.Number > 0 {
				continue
			}
			check[pr.Number] = struct{}{}

			res.PullRequests = append(res.PullRequests, pr)
			if len(res.PullRequests) == limit {
				break loop
			}
		}

		if prData.PageInfo.HasNextPage {
			variables["endCursor"] = prData.PageInfo.EndCursor
			pageLimit = min(pageLimit, limit-len(res.PullRequests))
		} else {
			break
		}
	}

	return &res, nil
}

type mockLister struct {
	called       bool
	expectFields []string

	result *api.PullRequestAndTotalCount
	err    error
}

func NewMockLister(result *api.PullRequestAndTotalCount, err error) *mockLister {
	return &mockLister{
		result: result,
		err:    err,
	}
}

func (m *mockLister) List(opt ListOptions) (*api.PullRequestAndTotalCount, error) {
	m.called = true
	if m.err != nil {
		return nil, m.err
	}

	if m.expectFields != nil {

		if !isEqualSet(m.expectFields, opt.Fields) {
			return nil, fmt.Errorf("unexpected fields: %v", opt.Fields)
		}
	}

	return m.result, m.err
}

func (m *mockLister) ExpectFields(fields []string) {
	m.expectFields = fields
}
