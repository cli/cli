package shared

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/internal/ghrepo"
)

type Release struct {
	TagName      string    `json:"tag_name"`
	Name         string    `json:"name"`
	Body         string    `json:"body"`
	IsDraft      bool      `json:"draft"`
	IsPrerelease bool      `json:"prerelease"`
	PublishedAt  time.Time `json:"published_at"`

	URL       string `json:"url"`
	UploadURL string `json:"upload_url"`
	HTMLURL   string `json:"html_url"`
	Assets    []ReleaseAsset

	Author struct {
		Login string
	}
}

type ReleaseAsset struct {
	Name  string
	Size  int64
	State string
	URL   string
}

func FetchRelease(httpClient *http.Client, baseRepo ghrepo.Interface, tagName string) (*Release, error) {
	// FIXME: this doesn't find draft releases
	path := fmt.Sprintf("repos/%s/%s/releases/tags/%s", baseRepo.RepoOwner(), baseRepo.RepoName(), tagName)
	url := ghinstance.RESTPrefix(baseRepo.RepoHost()) + path
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !success {
		return nil, api.HandleHTTPError(resp)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var release Release
	err = json.Unmarshal(b, &release)
	if err != nil {
		return nil, err
	}

	return &release, nil
}
