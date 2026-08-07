package garden

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/v2/api"
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
func getResponse(client *http.Client, hostname string, path safeurl.SafeURL, data interface{}) ([]string, error) {
	// The Content-Type this site used to set explicitly is the one the client already sends,
	// so it is no longer stated here.
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	resp, err := api.NewClientFromHTTP(client).Request(hostname, http.MethodGet, path.String(), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

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
