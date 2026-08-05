package garden

import (
	"context"
	"errors"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/githubrest"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
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

	pathF := func(page int) (*safeurl.MutableSafeURL, error) {
		u, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "commits")
		if err != nil {
			return nil, err
		}
		u.SetQuery("per_page", "100")
		u.SetQuery("page", strconv.Itoa(page))
		return u, nil
	}

	page := 1
	paginating := true
	for paginating {
		if len(commits) >= maxCommits {
			break
		}
		result := Result{}
		path, err := pathF(page)
		if err != nil {
			return nil, err
		}
		links, err := getResponse(client, repo.RepoHost(), path, &result)
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
func getResponse(client *http.Client, hostname string, url safeurl.SafeURL, data interface{}) ([]string, error) {
	restClient, err := api.NewRESTClient(client, hostname)
	if err != nil {
		return nil, err
	}

	req, err := restClient.NewRequest(context.Background(), http.MethodGet, url.String(), nil,
		githubrest.WithHeader("Content-Type", "application/json; charset=utf-8"))
	if err != nil {
		return nil, err
	}

	resp, err := restClient.Do(req, data)
	if err != nil {
		// The message deliberately stays this vague, as it was before.
		return nil, errors.New("api call failed")
	}

	return resp.Header["Link"], nil
}
