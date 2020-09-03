package shared

import (
	"encoding/json"
	"errors"
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
	CreatedAt    time.Time `json:"created_at"`
	PublishedAt  time.Time `json:"published_at"`

	APIURL    string `json:"url"`
	UploadURL string `json:"upload_url"`
	HTMLURL   string `json:"html_url"`
	Assets    []ReleaseAsset

	Author struct {
		Login string
	}
}

type ReleaseAsset struct {
	Name   string
	Size   int64
	State  string
	APIURL string `json:"url"`
}

// FetchRelease finds a repository release by its tagName.
func FetchRelease(httpClient *http.Client, baseRepo ghrepo.Interface, tagName string) (*Release, error) {
	path := fmt.Sprintf("repos/%s/%s/releases/tags/%s", baseRepo.RepoOwner(), baseRepo.RepoName(), tagName)
	url := ghinstance.RESTPrefix(baseRepo.RepoHost()) + path
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		if canPush, err := api.CanPushToRepo(httpClient, baseRepo); err == nil && canPush {
			return FindDraftRelease(httpClient, baseRepo, tagName)
		} else if err != nil {
			return nil, err
		}
	}

	if resp.StatusCode > 299 {
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

// FetchLatestRelease finds the latest published release for a repository.
func FetchLatestRelease(httpClient *http.Client, baseRepo ghrepo.Interface) (*Release, error) {
	path := fmt.Sprintf("repos/%s/%s/releases/latest", baseRepo.RepoOwner(), baseRepo.RepoName())
	url := ghinstance.RESTPrefix(baseRepo.RepoHost()) + path
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
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

// FindDraftRelease returns the latest draft release that matches tagName.
func FindDraftRelease(httpClient *http.Client, baseRepo ghrepo.Interface, tagName string) (*Release, error) {
	path := fmt.Sprintf("repos/%s/%s/releases", baseRepo.RepoOwner(), baseRepo.RepoName())
	url := ghinstance.RESTPrefix(baseRepo.RepoHost()) + path

	perPage := 100
	page := 1
	for {
		req, err := http.NewRequest("GET", fmt.Sprintf("%s?per_page=%d&page=%d", url, perPage, page), nil)
		if err != nil {
			return nil, err
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode > 299 {
			return nil, api.HandleHTTPError(resp)
		}

		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		var releases []Release
		err = json.Unmarshal(b, &releases)
		if err != nil {
			return nil, err
		}

		for _, r := range releases {
			if r.IsDraft && r.TagName == tagName {
				return &r, nil
			}
		}

		if len(releases) < perPage {
			break
		}
		page++
	}

	return nil, errors.New("release not found")
}
