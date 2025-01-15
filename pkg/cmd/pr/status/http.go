package status

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/set"

	ghauth "github.com/cli/go-gh/v2/pkg/auth"
)

type requestOptions struct {
	CurrentPR      int
	HeadRef        string
	Username       string
	Fields         []string
	ConflictStatus bool

	CheckRunAndStatusContextCountsSupported bool
}

type pullRequestsPayload struct {
	ViewerCreated   api.PullRequestAndTotalCount
	ReviewRequested api.PullRequestAndTotalCount
	CurrentPR       *api.PullRequest
	DefaultBranch   string
}

func pullRequestStatus(httpClient *http.Client, repo ghrepo.Interface, options requestOptions) (*pullRequestsPayload, error) {
	apiClient := api.NewClientFromHTTP(httpClient)
	type edges struct {
		TotalCount int
		Edges      []struct {
			Node api.PullRequest
		}
	}

	type response struct {
		Repository struct {
			DefaultBranchRef struct {
				Name string
			}
			PullRequests edges
			PullRequest  *api.PullRequest
		}
		ViewerCreated   edges
		ReviewRequested edges
	}

	var fragments string
	if len(options.Fields) > 0 {
		fields := set.NewStringSet()
		fields.AddValues(options.Fields)
		// these are always necessary to find the PR for the current branch
		fields.AddValues([]string{"isCrossRepository", "headRepositoryOwner", "headRefName"})
		gr := api.PullRequestGraphQL(fields.ToSlice())
		fragments = fmt.Sprintf("fragment pr on PullRequest{%s}fragment prWithReviews on PullRequest{...pr}", gr)
	} else {
		var err error
		fragments, err = pullRequestFragment(options.ConflictStatus, options.CheckRunAndStatusContextCountsSupported)
		if err != nil {
			return nil, err
		}
	}

	queryPrefix := `
	query PullRequestStatus($owner: String!, $repo: String!, $headRefName: String!, $viewerQuery: String!, $reviewerQuery: String!, $per_page: Int = 10) {
		repository(owner: $owner, name: $repo) {
			defaultBranchRef {
				name
			}
			pullRequests(headRefName: $headRefName, first: $per_page, orderBy: { field: CREATED_AT, direction: DESC }) {
				totalCount
				edges {
					node {
						...prWithReviews
					}
				}
			}
		}
	`
	if options.CurrentPR > 0 {
		queryPrefix = `
		query PullRequestStatus($owner: String!, $repo: String!, $number: Int!, $viewerQuery: String!, $reviewerQuery: String!, $per_page: Int = 10) {
			repository(owner: $owner, name: $repo) {
				defaultBranchRef {
					name
				}
				pullRequest(number: $number) {
					...prWithReviews
					baseRef {
						branchProtectionRule {
							requiredApprovingReviewCount
						}
					}
				}
			}
		`
	}

	query := fragments + queryPrefix + `
			viewerCreated: search(query: $viewerQuery, type: ISSUE, first: $per_page) {
				totalCount: issueCount
				edges {
					node {
					...prWithReviews
					}
				}
			}
			reviewRequested: search(query: $reviewerQuery, type: ISSUE, first: $per_page) {
				totalCount: issueCount
				edges {
					node {
					...pr
					}
				}
			}
		}
	`

	currentUsername, err := getCurrentUsername(options.Username, repo.RepoHost(), apiClient)

	if err != nil {
		return nil, err
	}

	viewerQuery := fmt.Sprintf("repo:%s state:open is:pr author:%s", ghrepo.FullName(repo), currentUsername)
	reviewerQuery := fmt.Sprintf("repo:%s state:open review-requested:%s", ghrepo.FullName(repo), currentUsername)

	currentPRHeadRef := options.HeadRef
	branchWithoutOwner := currentPRHeadRef
	if idx := strings.Index(currentPRHeadRef, ":"); idx >= 0 {
		branchWithoutOwner = currentPRHeadRef[idx+1:]
	}

	variables := map[string]interface{}{
		"viewerQuery":   viewerQuery,
		"reviewerQuery": reviewerQuery,
		"owner":         repo.RepoOwner(),
		"repo":          repo.RepoName(),
		"headRefName":   branchWithoutOwner,
		"number":        options.CurrentPR,
	}

	var resp response
	if err := apiClient.GraphQL(repo.RepoHost(), query, variables, &resp); err != nil {
		return nil, err
	}

	var viewerCreated []api.PullRequest
	for _, edge := range resp.ViewerCreated.Edges {
		viewerCreated = append(viewerCreated, edge.Node)
	}

	var reviewRequested []api.PullRequest
	for _, edge := range resp.ReviewRequested.Edges {
		reviewRequested = append(reviewRequested, edge.Node)
	}

	var currentPR = resp.Repository.PullRequest
	if currentPR == nil {
		for _, edge := range resp.Repository.PullRequests.Edges {
			if edge.Node.HeadLabel() == currentPRHeadRef {
				currentPR = &edge.Node
				break // Take the most recent PR for the current branch
			}
		}
	}

	payload := pullRequestsPayload{
		ViewerCreated: api.PullRequestAndTotalCount{
			PullRequests: viewerCreated,
			TotalCount:   resp.ViewerCreated.TotalCount,
		},
		ReviewRequested: api.PullRequestAndTotalCount{
			PullRequests: reviewRequested,
			TotalCount:   resp.ReviewRequested.TotalCount,
		},
		CurrentPR:     currentPR,
		DefaultBranch: resp.Repository.DefaultBranchRef.Name,
	}

	return &payload, nil
}

func getCurrentUsername(username string, hostname string, apiClient *api.Client) (string, error) {
	if username == "@me" && ghauth.IsEnterprise(hostname) {
		var err error
		username, err = api.CurrentLoginName(apiClient, hostname)
		if err != nil {
			return "", err
		}
	}
	return username, nil
}

func pullRequestFragment(conflictStatus bool, statusCheckRollupWithCountByState bool) (string, error) {
	fields := []string{
		"number", "title", "state", "url", "isDraft", "isCrossRepository",
		"headRefName", "headRepositoryOwner", "mergeStateStatus",
		"requiresStrictStatusChecks", "autoMergeRequest",
	}

	if conflictStatus {
		fields = append(fields, "mergeable")
	}

	if statusCheckRollupWithCountByState {
		fields = append(fields, "statusCheckRollupWithCountByState")
	} else {
		fields = append(fields, "statusCheckRollup")
	}

	reviewFields := []string{"reviewDecision", "latestReviews"}
	fragments := fmt.Sprintf(`
	fragment pr on PullRequest {%s}
	fragment prWithReviews on PullRequest {...pr,%s}
	`, api.PullRequestGraphQL(fields), api.PullRequestGraphQL(reviewFields))
	return fragments, nil
}
