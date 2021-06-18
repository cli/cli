package shared

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/internal/ghrepo"
)

var ReleaseFields = []string{
	"url",
	"apiUrl",
	"uploadUrl",
	"tarballUrl",
	"zipballUrl",
	"id",
	"tagName",
	"name",
	"body",
	"isDraft",
	"isPrerelease",
	"createdAt",
	"publishedAt",
	"targetCommitish",
	"author",
	"assets",
}

type Release struct {
	ID           string     `json:"node_id"`
	TagName      string     `json:"tag_name"`
	Name         string     `json:"name"`
	Body         string     `json:"body"`
	IsDraft      bool       `json:"draft"`
	IsPrerelease bool       `json:"prerelease"`
	CreatedAt    time.Time  `json:"created_at"`
	PublishedAt  *time.Time `json:"published_at"`

	TargetCommitish string `json:"target_commitish"`

	APIURL     string `json:"url"`
	UploadURL  string `json:"upload_url"`
	TarballURL string `json:"tarball_url"`
	ZipballURL string `json:"zipball_url"`
	URL        string `json:"html_url"`
	Assets     []ReleaseAsset

	Author struct {
		ID    string `json:"node_id"`
		Login string `json:"login"`
	}
}

type ReleaseAsset struct {
	ID     string `json:"node_id"`
	Name   string
	Label  string
	Size   int64
	State  string
	APIURL string `json:"url"`

	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	DownloadCount      int       `json:"download_count"`
	ContentType        string    `json:"content_type"`
	BrowserDownloadURL string    `json:"browser_download_url"`
}

func (rel *Release) ExportData(fields []string) *map[string]interface{} {
	v := reflect.ValueOf(rel).Elem()
	fieldByName := func(v reflect.Value, field string) reflect.Value {
		return v.FieldByNameFunc(func(s string) bool {
			return strings.EqualFold(field, s)
		})
	}
	data := map[string]interface{}{}

	for _, f := range fields {
		switch f {
		case "author":
			data[f] = map[string]interface{}{
				"id":    rel.Author.ID,
				"login": rel.Author.Login,
			}
		case "assets":
			assets := make([]interface{}, 0, len(rel.Assets))
			for _, a := range rel.Assets {
				assets = append(assets, map[string]interface{}{
					"url":           a.BrowserDownloadURL,
					"apiUrl":        a.APIURL,
					"id":            a.ID,
					"name":          a.Name,
					"label":         a.Label,
					"size":          a.Size,
					"state":         a.State,
					"createdAt":     a.CreatedAt,
					"updatedAt":     a.UpdatedAt,
					"downloadCount": a.DownloadCount,
					"contentType":   a.ContentType,
				})
			}
			data[f] = assets
		default:
			sf := fieldByName(v, f)
			data[f] = sf.Interface()
		}
	}

	return &data
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
		return FindDraftRelease(httpClient, baseRepo, tagName)
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
		//nolint:staticcheck
		break
	}

	return nil, errors.New("release not found")
}
