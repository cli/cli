package api

import (
	"fmt"

	"github.com/github/gh-cli/context"
)

type PullRequestsPayload struct {
	ViewerCreated   []PullRequest
	ReviewRequested []PullRequest
	CurrentPR       *PullRequest
}

type PullRequest struct {
	Number      int
	Title       string
	URL         string
	HeadRefName string
	Reviews     struct {
		Nodes []struct {
			State  string
			Author struct {
				Login string
			}
		}
	}
	Commits struct {
		Nodes []struct {
			Commit struct {
				Status struct {
					State string
				}
				CheckSuites struct {
					Nodes []struct {
						Conclusion string
					}
				}
			}
		}
	}
}

func (pr *PullRequest) ChangesRequested() bool {
	reviewMap := map[string]string{}
	// Reviews will include every review on record, including consecutive ones
	// from the same actor. Consolidate them into latest state per reviewer.
	for _, review := range pr.Reviews.Nodes {
		reviewMap[review.Author.Login] = review.State
	}
	for _, state := range reviewMap {
		if state == "CHANGES_REQUESTED" {
			return true
		}
	}
	return false
}

// in the order of severity
var checksResultMap = map[string]int{
	"PENDING":         0,
	"NEUTRAL":         1,
	"SUCCESS":         2,
	"EXPECTED":        3,
	"CANCELLED":       4,
	"TIMED_OUT":       5,
	"ERROR":           6,
	"FAILURE":         7,
	"ACTION_REQUIRED": 8,
}

func (pr *PullRequest) ChecksStatus() string {
	if len(pr.Commits.Nodes) == 0 {
		return ""
	}
	commit := pr.Commits.Nodes[0].Commit
	// EXPECTED, ERROR, FAILURE, PENDING, SUCCESS
	conclusion := commit.Status.State
	for _, checkSuite := range commit.CheckSuites.Nodes {
		// ACTION_REQUIRED, TIMED_OUT, CANCELLED, FAILURE, SUCCESS, NEUTRAL
		if checksResultMap[checkSuite.Conclusion] > checksResultMap[conclusion] {
			conclusion = checkSuite.Conclusion
		}
	}
	return conclusion
}

func PullRequests() (*PullRequestsPayload, error) {
	type edges struct {
		Edges []struct {
			Node PullRequest
		}
		PageInfo struct {
			HasNextPage bool
			EndCursor   string
		}
	}

	type response struct {
		Repository struct {
			PullRequests edges
		}
		ViewerCreated   edges
		ReviewRequested edges
	}

	query := `
    fragment pr on PullRequest {
		number
		title
		url
		headRefName
		commits(last: 1) {
			nodes {
				commit {
					status {
						state
					}
					checkSuites(first: 50) {
						nodes {
							conclusion
						}
					}
				}
			}
		}
	}
	fragment prWithReviews on PullRequest {
		...pr
		reviews(last: 20) {
			nodes {
				state
				author {
					login
				}
			}
		}
	}

    query($owner: String!, $repo: String!, $headRefName: String!, $viewerQuery: String!, $reviewerQuery: String!, $per_page: Int = 10) {
      repository(owner: $owner, name: $repo) {
        pullRequests(headRefName: $headRefName, first: 1) {
          edges {
            node {
              ...prWithReviews
            }
          }
        }
      }
      viewerCreated: search(query: $viewerQuery, type: ISSUE, first: $per_page) {
        edges {
          node {
            ...prWithReviews
          }
        }
        pageInfo {
          hasNextPage
        }
      }
      reviewRequested: search(query: $reviewerQuery, type: ISSUE, first: $per_page) {
        edges {
          node {
            ...pr
          }
        }
        pageInfo {
          hasNextPage
        }
      }
    }
  `

	ghRepo, err := context.Current().BaseRepo()
	if err != nil {
		return nil, err
	}
	currentBranch, err := context.Current().Branch()
	if err != nil {
		return nil, err
	}
	currentUsername, err := context.Current().AuthLogin()
	if err != nil {
		return nil, err
	}

	owner := ghRepo.Owner
	repo := ghRepo.Name

	viewerQuery := fmt.Sprintf("repo:%s/%s state:open is:pr author:%s", owner, repo, currentUsername)
	reviewerQuery := fmt.Sprintf("repo:%s/%s state:open review-requested:%s", owner, repo, currentUsername)

	variables := map[string]string{
		"viewerQuery":   viewerQuery,
		"reviewerQuery": reviewerQuery,
		"owner":         owner,
		"repo":          repo,
		"headRefName":   currentBranch,
	}

	var resp response
	err = GraphQL(query, variables, &resp)
	if err != nil {
		return nil, err
	}

	var viewerCreated []PullRequest
	for _, edge := range resp.ViewerCreated.Edges {
		viewerCreated = append(viewerCreated, edge.Node)
	}

	var reviewRequested []PullRequest
	for _, edge := range resp.ReviewRequested.Edges {
		reviewRequested = append(reviewRequested, edge.Node)
	}

	var currentPR *PullRequest
	for _, edge := range resp.Repository.PullRequests.Edges {
		currentPR = &edge.Node
	}

	payload := PullRequestsPayload{
		viewerCreated,
		reviewRequested,
		currentPR,
	}

	return &payload, nil
}
