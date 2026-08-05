package extension

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
)

func repoExists(httpClient *http.Client, repo ghrepo.Interface) (bool, error) {
	url, err := safeurl.JoinPathWithHostPrefix(ghinstance.RESTPrefix(repo.RepoHost()), "repos", repo.RepoOwner(), repo.RepoName())
	if err != nil {
		return false, err
	}

	req, err := http.NewRequest(http.MethodGet, url.String(), nil)
	if err != nil {
		return false, err
	}

	// TODO(api-client-rollout)
	// This has been deferred from moving to api.Client due to its exact-status contract and body-blind response handling.
	resp, err := httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, api.HandleHTTPError(resp)
	}
}

func hasScript(httpClient *http.Client, repo ghrepo.Interface) (bool, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "contents", repo.RepoName())
	if err != nil {
		return false, err
	}

	// The response body is not decoded, because a script is considered present for any
	// successful response regardless of the content type reported.
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	err = api.NewClientFromHTTP(httpClient).REST(repo.RepoHost(), http.MethodGet, path.String(), nil, nil)
	if err != nil {
		var httpErr api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return false, nil
		}
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
func downloadAsset(httpClient *http.Client, assetURL safeurl.SafeURL, destPath string) (downloadErr error) {
	var req *http.Request
	if req, downloadErr = http.NewRequest("GET", assetURL.String(), nil); downloadErr != nil {
		return
	}

	req.Header.Set("Accept", "application/octet-stream")

	var resp *http.Response
	// TODO(api-client-rollout)
	// This has been deferred from moving to api.Client due to its custom Accept header and binary response streaming.
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
	path, err := safeurl.JoinPath("repos", baseRepo.RepoOwner(), baseRepo.RepoName(), "releases", "latest")
	if err != nil {
		return nil, err
	}

	var data json.RawMessage
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	err = api.NewClientFromHTTP(httpClient).REST(baseRepo.RepoHost(), http.MethodGet, path.String(), nil, &data)
	if err != nil {
		var httpErr api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return nil, releaseNotFoundErr
		}
		return nil, err
	}

	var r release
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}

	return &r, nil
}

// fetchReleaseFromTag finds release by tag name for a repository
func fetchReleaseFromTag(httpClient *http.Client, baseRepo ghrepo.Interface, tagName string) (*release, error) {
	path, err := safeurl.JoinPath("repos", baseRepo.RepoOwner(), baseRepo.RepoName(), "releases", "tags", tagName)
	if err != nil {
		return nil, err
	}

	var data json.RawMessage
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	err = api.NewClientFromHTTP(httpClient).REST(baseRepo.RepoHost(), http.MethodGet, path.String(), nil, &data)
	if err != nil {
		var httpErr api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return nil, releaseNotFoundErr
		}
		return nil, err
	}

	var r release
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}

	return &r, nil
}

// fetchCommitSHA finds full commit SHA from a target ref in a repo
func fetchCommitSHA(httpClient *http.Client, baseRepo ghrepo.Interface, targetRef string) (string, error) {
	url, err := safeurl.JoinPathWithHostPrefix(ghinstance.RESTPrefix(baseRepo.RepoHost()), "repos", baseRepo.RepoOwner(), baseRepo.RepoName(), "commits", targetRef)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", "application/vnd.github.v3.sha")
	// TODO(api-client-rollout)
	// This has been deferred from moving to api.Client due to its custom Accept header and bare SHA response body.
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
