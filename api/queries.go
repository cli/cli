package api

import (
	"fmt"
	"strings"

	"github.com/github/gh-cli/git"
	"github.com/github/gh-cli/github"
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

func PullRequests() (PullRequestsPayload, error) {
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

	project := project()
	owner := project.Owner
	repo := project.Name
	currentBranch := currentBranch()

	viewerQuery := fmt.Sprintf("repo:%s/%s state:open is:pr author:%s", owner, repo, currentUsername())
	reviewerQuery := fmt.Sprintf("repo:%s/%s state:open review-requested:%s", owner, repo, currentUsername())

	variables := map[string]string{
		"viewerQuery":   viewerQuery,
		"reviewerQuery": reviewerQuery,
		"owner":         owner,
		"repo":          repo,
		"headRefName":   currentBranch,
	}

	var resp response
	err := graphQL(query, variables, &resp)
	if err != nil {
		return PullRequestsPayload{}, err
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

	return payload, nil
}

// These will be replaced by nate's context stuff
// 💥💥💥💥💥💥💥💥💥💥💥💥💥💥💥💥💥💥💥💥💥
func project() github.Project {
	remotes, error := github.Remotes()
	if error != nil {
		panic(error)
	}

	for _, remote := range remotes {
		if project, error := remote.Project(); error == nil {
			return *project
		}
	}

	panic("Could not get the project. What is a project? I don't know, it's kind of like a git repository I think?")
}

func currentBranch() string {
	currentBranch, err := git.Head()
	if err != nil {
		panic(err)
	}

	return strings.Replace(currentBranch, "refs/heads/", "", 1)
}

func currentUsername() string {
	host, err := github.CurrentConfig().DefaultHost()
	if err != nil {
		panic(err)
	}
	return host.User
}
