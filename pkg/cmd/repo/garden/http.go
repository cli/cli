package garden

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
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
		resp, err := getResponse(client, repo.RepoHost(), pathF(page), &result)
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
		link := resp.Header["Link"]
		if len(link) == 0 || !strings.Contains(link[0], "last") {
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

func getResponse(client *http.Client, host, path string, data interface{}) (*http.Response, error) {
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
