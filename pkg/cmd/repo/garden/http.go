package garden

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
)

func getCommits(client *http.Client, repo ghrepo.Interface, maxCommits int) ([]*Commit, error) {
	type Item struct {
		Author struct {
			Login string
		}
		Sha string
	}

	type Result []Item

	commits := []*Commit{}

	pathF := func(page int) string {
		return fmt.Sprintf("repos/%s/%s/commits?per_page=100&page=%d", repo.RepoOwner(), repo.RepoName(), page)
	}

	page := 1
	paginating := true
	for paginating {
		if len(commits) >= maxCommits {
			break
		}
		result := Result{}
		links, err := getResponse(client, repo.RepoHost(), pathF(page), &result)
		if err != nil {
			return nil, err
		}
		for _, r := range result {
			colorFunc := shaToColorFunc(r.Sha)
			handle := r.Author.Login
			if handle == "" {
				handle = "a mysterious stranger"
			}
			commits = append(commits, &Commit{
				Handle: handle,
				Sha:    r.Sha,
				Char:   colorFunc(string(handle[0])),
			})
		}
		if len(links) == 0 || !strings.Contains(links[0], "last") {
			paginating = false
		}
		page++
		time.Sleep(500)
	}

	// reverse to get older commits first
	for i, j := 0, len(commits)-1; i < j; i, j = i+1, j-1 {
		commits[i], commits[j] = commits[j], commits[i]
	}

	return commits, nil
}

// getResponse performs the API call and returns the response's link header values.
// If the "Link" header is missing, the returned slice will be nil.
func getResponse(client *http.Client, host, path string, data interface{}) ([]string, error) {
	url := ghinstance.RESTPrefix(host) + path
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !success {
		return nil, errors.New("api call failed")
	}

	links := resp.Header["Link"]

	if resp.StatusCode == http.StatusNoContent {
		return links, nil
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(b, &data)
	if err != nil {
		return nil, err
	}

	return links, nil
}
