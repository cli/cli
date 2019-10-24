package api

import (
	"fmt"
	"time"

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

type IssuesPayload struct {
	Assigned  []Issue
	Mentioned []Issue
	Recent    []Issue
}

type Issue struct {
	Number int
	Title  string
	URL    string
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

	p := resp.Repository.ViewerPermission
	if p == "ADMIN" || p == "MAINTAIN" || p == "WRITE" {
		return true, nil
	}

	return false, nil
}

func Fork() (string, string, string, error) {
	var resp struct {
		URL      string `json:"html_url"`
		CloneURL string `json:"clone_url"`
		Parent   struct {
			CloneURL string `json:"clone_url"`
		}
	}

	ghRepo, err := context.Current().BaseRepo()
	if err != nil {
		return "", "", "", err
	}

	path := fmt.Sprintf("/repos/%s/%s/forks", ghRepo.Owner, ghRepo.Name)
	err = Post(path, map[string]string{}, &resp)
	if err != nil {
		return "", "", "", err
	}

	return resp.URL, resp.CloneURL, resp.Parent.CloneURL, nil
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

func Issues() (*IssuesPayload, error) {
	type issues struct {
		Issues struct {
			Edges []struct {
				Node Issue
			}
		}
	}

	type response struct {
		Assigned  issues
		Mentioned issues
		Recent    issues
	}

	query := `
    fragment issue on Issue {
      number
      title
      url
    }

    query($owner: String!, $repo: String!, $since: DateTime!, $viewer: String!, $per_page: Int = 10) {
      assigned: repository(owner: $owner, name: $repo) {
        issues(filterBy: {assignee: $viewer}, first: $per_page) {
          edges {
            node {
              ...issue
            }
          }
        }
			}
			mentioned: repository(owner: $owner, name: $repo) {
        issues(filterBy: {mentioned: $viewer}, first: $per_page) {
          edges {
            node {
              ...issue
            }
          }
        }
			}
			recent: repository(owner: $owner, name: $repo) {
        issues(filterBy: {since: $since}, first: $per_page) {
          edges {
            node {
              ...issue
            }
          }
        }
      }
    }
  `

	ghRepo, err := context.Current().BaseRepo()
	if err != nil {
		return nil, err
	}

	currentUsername, err := context.Current().AuthLogin()
	if err != nil {
		return nil, err
	}

	owner := ghRepo.Owner
	repo := ghRepo.Name
	since := time.Now().UTC().Add(time.Hour * -24).Format("2006-01-02T15:04:05-0700")
	variables := map[string]string{
		"owner":  owner,
		"repo":   repo,
		"viewer": currentUsername,
		"since":  since,
	}

	var resp response
	err = GraphQL(query, variables, &resp)
	if err != nil {
		return nil, err
	}

	var assigned []Issue
	for _, edge := range resp.Assigned.Issues.Edges {
		assigned = append(assigned, edge.Node)
	}

	var mentioned []Issue
	for _, edge := range resp.Mentioned.Issues.Edges {
		mentioned = append(mentioned, edge.Node)
	}

	var recent []Issue
	for _, edge := range resp.Recent.Issues.Edges {
		recent = append(recent, edge.Node)
	}

	payload := IssuesPayload{
		assigned,
		mentioned,
		recent,
	}

	return &payload, nil
}
