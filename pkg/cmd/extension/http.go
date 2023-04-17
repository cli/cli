package extension

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
)

func repoExists(httpClient *http.Client, repo ghrepo.Interface) (bool, error) {
	url := fmt.Sprintf("%srepos/%s/%s", ghinstance.RESTPrefix(repo.RepoHost()), repo.RepoOwner(), repo.RepoName())
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case 200:
		return true, nil
	case 404:
		return false, nil
	default:
		return false, api.HandleHTTPError(resp)
	}
}

func hasScript(httpClient *http.Client, repo ghrepo.Interface) (bool, error) {
	path := fmt.Sprintf("repos/%s/%s/contents/%s",
		repo.RepoOwner(), repo.RepoName(), repo.RepoName())
	url := ghinstance.RESTPrefix(repo.RepoHost()) + path
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return false, nil
	}

	if resp.StatusCode > 299 {
		err = api.HandleHTTPError(resp)
		return false, err
	}

	return true, nil
}

type releaseAsset struct {
	Name   string
	APIURL string `json:"url"`
}

type release struct {
	Tag    string `json:"tag_name"`
	Assets []releaseAsset
}

// downloadAsset downloads a single asset to the given file path.
func downloadAsset(httpClient *http.Client, asset releaseAsset, destPath string) (downloadErr error) {
	var req *http.Request
	if req, downloadErr = http.NewRequest("GET", asset.APIURL, nil); downloadErr != nil {
		return
	}

	req.Header.Set("Accept", "application/octet-stream")

	var resp *http.Response
	if resp, downloadErr = httpClient.Do(req); downloadErr != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		downloadErr = api.HandleHTTPError(resp)
		return
	}

	var f *os.File
	if f, downloadErr = os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755); downloadErr != nil {
		return
	}
	defer func() {
		if err := f.Close(); downloadErr == nil && err != nil {
			downloadErr = err
		}
	}()

	_, downloadErr = io.Copy(f, resp.Body)
	return
}

var commitNotFoundErr = errors.New("commit not found")
var releaseNotFoundErr = errors.New("release not found")
var repositoryNotFoundErr = errors.New("repository not found")

// fetchLatestRelease finds the latest published release for a repository.
func fetchLatestRelease(httpClient *http.Client, baseRepo ghrepo.Interface) (*release, error) {
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

	if resp.StatusCode == 404 {
		return nil, releaseNotFoundErr
	}
	if resp.StatusCode > 299 {
		return nil, api.HandleHTTPError(resp)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var r release
	err = json.Unmarshal(b, &r)
	if err != nil {
		return nil, err
	}

	return &r, nil
}

// fetchReleaseFromTag finds release by tag name for a repository
func fetchReleaseFromTag(httpClient *http.Client, baseRepo ghrepo.Interface, tagName string) (*release, error) {
	fullRepoName := fmt.Sprintf("%s/%s", baseRepo.RepoOwner(), baseRepo.RepoName())
	path := fmt.Sprintf("repos/%s/releases/tags/%s", fullRepoName, tagName)
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
		return nil, releaseNotFoundErr
	}
	if resp.StatusCode > 299 {
		return nil, api.HandleHTTPError(resp)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var r release
	err = json.Unmarshal(b, &r)
	if err != nil {
		return nil, err
	}

	return &r, nil
}

// fetchCommitSHA finds full commit SHA from a target ref in a repo
func fetchCommitSHA(httpClient *http.Client, baseRepo ghrepo.Interface, targetRef string) (string, error) {
	path := fmt.Sprintf("repos/%s/%s/commits/%s", baseRepo.RepoOwner(), baseRepo.RepoName(), targetRef)
	url := ghinstance.RESTPrefix(baseRepo.RepoHost()) + path
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", "application/vnd.github.v3.sha")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()
	if resp.StatusCode == 422 {
		return "", commitNotFoundErr
	}
	if resp.StatusCode > 299 {
		return "", api.HandleHTTPError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}
