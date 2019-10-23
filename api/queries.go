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
}

func GitHubRepoId() (string, error) {
	ghRepo, err := context.Current().BaseRepo()
	if err != nil {
		return "", fmt.Errorf("could not determine current GH repo: %s", err)
	}

	owner := ghRepo.Owner
	repo := ghRepo.Name

	query := `
		query FindRepoID($owner:String!, $name:String!) {
			repository(owner:$owner, name:$name) {
				id
			}
	}`
	variables := map[string]interface{}{
		"owner": owner,
		"name":  repo,
	}

	result := struct {
		Repository struct {
			Id string
		}
	}{}
	err = GraphQL(query, variables, &result)
	if err != nil {
		return "", fmt.Errorf("failed to determine GH repo ID: %s", err)
	}

	return result.Repository.Id, nil
}

func CreatePullRequest(title string, body string, draft bool, base string, head string) (string, error) {
	repoId, err := GitHubRepoId()
	if err != nil {
		return "", err
	}
	if repoId == "" {
		return "", fmt.Errorf("could not determine GH repo ID")
	}

	query := `
		mutation CreatePullRequest($input: CreatePullRequestInput!) {
			createPullRequest(input: $input) {
				pullRequest {
					url
				}
			}
	}`

	variables := map[string]interface{}{
		"input": map[string]interface{}{
			"repositoryId": repoId,
			"baseRefName":  base,
			"headRefName":  head,
			"title":        title,
			"body":         body,
			"draft":        draft,
		},
	}

	result := struct {
		CreatePullRequest struct {
			PullRequest PullRequest
		}
	}{}

	err = GraphQL(query, variables, &result)
	if err != nil {
		return "", err
	}

	return result.CreatePullRequest.PullRequest.URL, nil
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
    }

    query($owner: String!, $repo: String!, $headRefName: String!, $viewerQuery: String!, $reviewerQuery: String!, $per_page: Int = 10) {
      repository(owner: $owner, name: $repo) {
        pullRequests(headRefName: $headRefName, first: 1) {
          edges {
            node {
              ...pr
            }
          }
        }
      }
      viewerCreated: search(query: $viewerQuery, type: ISSUE, first: $per_page) {
        edges {
          node {
            ...pr
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

	variables := map[string]interface{}{
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
