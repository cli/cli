package list

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
)

const fragments = `
	fragment issue on Issue {
		number
		title
		url
		state
		updatedAt
		labels(first: 100) {
			nodes {
				name
			}
			totalCount
		}
	}
`

func IssueList(client *api.Client, repo ghrepo.Interface, state string, assigneeString string, limit int, authorString string, mentionString string, milestoneString string) (*api.IssuesAndTotalCount, error) {
	var states []string
	switch state {
	case "open", "":
		states = []string{"OPEN"}
	case "closed":
		states = []string{"CLOSED"}
	case "all":
		states = []string{"OPEN", "CLOSED"}
	default:
		return nil, fmt.Errorf("invalid state: %s", state)
	}

	query := fragments + `
	query IssueList($owner: String!, $repo: String!, $limit: Int, $endCursor: String, $states: [IssueState!] = OPEN, $assignee: String, $author: String, $mention: String, $milestone: String) {
		repository(owner: $owner, name: $repo) {
			hasIssuesEnabled
			issues(first: $limit, after: $endCursor, orderBy: {field: CREATED_AT, direction: DESC}, states: $states, filterBy: {assignee: $assignee, createdBy: $author, mentioned: $mention, milestone: $milestone}) {
				totalCount
				nodes {
					...issue
				}
				pageInfo {
					hasNextPage
					endCursor
				}
			}
		}
	}
	`

	variables := map[string]interface{}{
		"owner":  repo.RepoOwner(),
		"repo":   repo.RepoName(),
		"states": states,
	}
	if assigneeString != "" {
		variables["assignee"] = assigneeString
	}
	if authorString != "" {
		variables["author"] = authorString
	}
	if mentionString != "" {
		variables["mention"] = mentionString
	}

	if milestoneString != "" {
		var milestone *api.RepoMilestone
		if milestoneNumber, err := strconv.ParseInt(milestoneString, 10, 32); err == nil {
			milestone, err = api.MilestoneByNumber(client, repo, int32(milestoneNumber))
			if err != nil {
				return nil, err
			}
		} else {
			milestone, err = api.MilestoneByTitle(client, repo, "all", milestoneString)
			if err != nil {
				return nil, err
			}
		}

		milestoneRESTID, err := milestoneNodeIdToDatabaseId(milestone.ID)
		if err != nil {
			return nil, err
		}
		variables["milestone"] = milestoneRESTID
	}

	type responseData struct {
		Repository struct {
			Issues struct {
				TotalCount int
				Nodes      []api.Issue
				PageInfo   struct {
					HasNextPage bool
					EndCursor   string
				}
			}
			HasIssuesEnabled bool
		}
	}

	var issues []api.Issue
	var totalCount int
	pageLimit := min(limit, 100)

loop:
	for {
		var response responseData
		variables["limit"] = pageLimit
		err := client.GraphQL(repo.RepoHost(), query, variables, &response)
		if err != nil {
			return nil, err
		}
		if !response.Repository.HasIssuesEnabled {
			return nil, fmt.Errorf("the '%s' repository has disabled issues", ghrepo.FullName(repo))
		}
		totalCount = response.Repository.Issues.TotalCount

		for _, issue := range response.Repository.Issues.Nodes {
			issues = append(issues, issue)
			if len(issues) == limit {
				break loop
			}
		}

		if response.Repository.Issues.PageInfo.HasNextPage {
			variables["endCursor"] = response.Repository.Issues.PageInfo.EndCursor
			pageLimit = min(pageLimit, limit-len(issues))
		} else {
			break
		}
	}

	res := api.IssuesAndTotalCount{Issues: issues, TotalCount: totalCount}
	return &res, nil
}

func IssueSearch(client *api.Client, repo ghrepo.Interface, searchQuery string, limit int) (*api.IssuesAndTotalCount, error) {
	query := fragments +
		`query IssueSearch($repo: String!, $owner: String!, $type: SearchType!, $limit: Int, $after: String, $query: String!) {
			repository(name: $repo, owner: $owner) {
				hasIssuesEnabled
			}
			search(type: $type, last: $limit, after: $after, query: $query) {
				issueCount
				nodes { ...issue }
				pageInfo {
					hasNextPage
					endCursor
				}
			}
		}`

	type response struct {
		Repository struct {
			HasIssuesEnabled bool
		}
		Search struct {
			IssueCount int
			Nodes      []api.Issue
			PageInfo   struct {
				HasNextPage bool
				EndCursor   string
			}
		}
	}

	perPage := min(limit, 100)
	searchQuery = fmt.Sprintf("repo:%s/%s %s", repo.RepoOwner(), repo.RepoName(), searchQuery)

	variables := map[string]interface{}{
		"owner": repo.RepoOwner(),
		"repo":  repo.RepoName(),
		"type":  "ISSUE",
		"limit": perPage,
		"query": searchQuery,
	}

	ic := api.IssuesAndTotalCount{}

loop:
	for {
		var resp response
		err := client.GraphQL(repo.RepoHost(), query, variables, &resp)
		if err != nil {
			return nil, err
		}

		if !resp.Repository.HasIssuesEnabled {
			return nil, fmt.Errorf("the '%s' repository has disabled issues", ghrepo.FullName(repo))
		}

		ic.TotalCount = resp.Search.IssueCount

		for _, issue := range resp.Search.Nodes {
			ic.Issues = append(ic.Issues, issue)
			if len(ic.Issues) == limit {
				break loop
			}
		}

		if !resp.Search.PageInfo.HasNextPage {
			break
		}
		variables["after"] = resp.Search.PageInfo.EndCursor
		variables["perPage"] = min(perPage, limit-len(ic.Issues))
	}

	return &ic, nil
}

// milestoneNodeIdToDatabaseId extracts the REST Database ID from the GraphQL Node ID
// This conversion is necessary since the GraphQL API requires the use of the milestone's database ID
// for querying the related issues.
func milestoneNodeIdToDatabaseId(nodeId string) (string, error) {
	// The Node ID is Base64 obfuscated, with an underlying pattern:
	// "09:Milestone12345", where "12345" is the database ID
	decoded, err := base64.StdEncoding.DecodeString(nodeId)
	if err != nil {
		return "", err
	}
	splitted := strings.Split(string(decoded), "Milestone")
	if len(splitted) != 2 {
		return "", fmt.Errorf("couldn't get database id from node id")
	}
	return splitted[1], nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
