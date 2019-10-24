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

type Repository struct {
	Name           string
	Owner          string
	URL            string
	CloneURL       string
	ParentCloneURL string
}

func HasPushPermission() (bool, error) {
	var resp struct {
		Repository struct {
			ViewerPermission string
		}
	}

	query := `
    query($owner: String!, $repoName: String!) {
      repository(owner: $owner, name: $repoName) {
        viewerPermission
      }
    }
  `

	ghRepo, err := context.Current().BaseRepo()
	if err != nil {
		return false, err
	}

	variables := map[string]string{
		"owner":    ghRepo.Owner,
		"repoName": ghRepo.Name,
	}

	err = GraphQL(query, variables, &resp)
	if err != nil {
		return false, err
	}

	switch resp.Repository.ViewerPermission {
	case "ADMIN", "MAINTAIN", "WRITE":
		return true, nil
	default:
		return false, nil
	}
}

func Fork() (*Repository, error) {
	var resp struct {
		Name     string
		URL      string `json:"html_url"`
		CloneURL string `json:"clone_url"`
		Owner    struct {
			Login string
		}
		Parent struct {
			CloneURL string `json:"clone_url"`
		}
	}

	ghRepo, err := context.Current().BaseRepo()
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("/repos/%s/%s/forks", ghRepo.Owner, ghRepo.Name)
	err = Post(path, map[string]string{}, &resp)
	if err != nil {
		return nil, err
	}

	repo := &Repository{
		Name:           resp.Name,
		Owner:          resp.Owner.Login,
		URL:            resp.URL,
		CloneURL:       resp.CloneURL,
		ParentCloneURL: resp.Parent.CloneURL,
	}
	return repo, nil
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
