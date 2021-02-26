package mergeconflict

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/internal/ghrepo"
)

// TODO gql prob faster but could not figure out how to do this via gql
// TODO ripped mostly from garden
// TODO max commits is a little silly
// TODO use cached client probs

func getShas(client *http.Client, repo ghrepo.Interface) ([]string, error) {
	type Item struct {
		Sha string
	}

	type Result []Item

	shas := []string{}

	pathF := func(page int) string {
		return fmt.Sprintf("repos/%s/%s/commits?per_page=100&page=%d", repo.RepoOwner(), repo.RepoName(), page)
	}

	// TODO
	maxCommits := 1000

	page := 1
	paginating := true
	for paginating {
		if len(shas) >= maxCommits {
			break
		}
		result := Result{}
		resp, err := getCommitsResponse(client, pathF(page), &result)
		if err != nil {
			return nil, err
		}
		for _, r := range result {
			shas = append(shas, r.Sha[0:11])
		}
		link := resp.Header["Link"]
		if len(link) == 0 || !strings.Contains(link[0], "last") {
			paginating = false
		}
		page++
		time.Sleep(500)
	}

	return shas, nil
}

func getCommitsResponse(client *http.Client, path string, data interface{}) (*http.Response, error) {
	cachedClient := api.NewCachedClient(client, time.Hour*24)

	url := ghinstance.RESTPrefix(ghinstance.OverridableDefault()) + path
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := cachedClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !success {
		return nil, errors.New("api call failed")
	}

	if resp.StatusCode == http.StatusNoContent {
		return resp, nil
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(b, &data)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func getIssues(client *http.Client, repo ghrepo.Interface) ([]string, error) {
	type Issue struct {
		Number int
		Title  string
	}

	variables := map[string]interface{}{
		"owner": repo.RepoOwner(),
		"repo":  repo.RepoName(),
	}

	query := heredoc.Doc(`
		query GetIssuesForMC($owner: String!, $repo: String!, $limit: Int, $endCursor: String) {
			repository(owner: $owner, name: $repo) {
				hasIssuesEnabled
				issues(first: $limit, after: $endCursor, states: [OPEN]) {
					nodes {
						number
						title
					}
					pageInfo {
						hasNextPage
						endCursor
					}
				}
			}
		}`)

	type responseData struct {
		Repository struct {
			HasIssuesEnabled bool
			Issues           struct {
				Nodes    []Issue
				PageInfo struct {
					HasNextPage bool
					EndCursor   string
				}
			}
		}
	}

	var issues []Issue
	pageLimit := 100

	cachedClient := api.NewCachedClient(client, time.Hour*24)
	c := api.NewClientFromHTTP(cachedClient)

	for {
		var response responseData
		variables["limit"] = pageLimit
		err := c.GraphQL(repo.RepoHost(), query, variables, &response)
		if err != nil {
			return nil, err
		}
		if !response.Repository.HasIssuesEnabled {
			return nil, fmt.Errorf("the '%s' repository has disabled issues", ghrepo.FullName(repo))
		}

		issues = append(issues, response.Repository.Issues.Nodes...)

		if response.Repository.Issues.PageInfo.HasNextPage {
			variables["endCursor"] = response.Repository.Issues.PageInfo.EndCursor
		} else {
			break
		}
	}

	res := []string{}
	for _, issue := range issues {
		res = append(res, fmt.Sprintf("#%d %s", issue.Number, issue.Title))
	}

	return res, nil
}
