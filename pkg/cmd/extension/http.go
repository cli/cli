package extension

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
)

func repoExists(httpClient *http.Client, repo ghrepo.Interface) (bool, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName())
	if err != nil {
		return false, err
	}

	// The response body is deliberately not read. Existence is decided by the status alone,
	// so Request is used rather than REST, which would try to decode the body.
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	resp, err := api.NewClientFromHTTP(httpClient).Request(repo.RepoHost(), http.MethodGet, path.String(), nil)
	if err != nil {
		var httpErr api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}
	defer resp.Body.Close()

	// Only 200 means the repository exists. Any other success status is unexpected here and is
	// reported as an error rather than being taken as existence.
	if resp.StatusCode != http.StatusOK {
		return false, api.UnexpectedStatusError(resp)
	}

	return true, nil
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
//
// assetURL is an absolute URL supplied by the API, so it is requested as given rather than
// being resolved against the host's REST endpoint. hostname is still needed so the request
// is made through a client configured for the right host.
func downloadAsset(httpClient *http.Client, hostname string, assetURL safeurl.SafeURL, destPath string) (downloadErr error) {
	var resp *http.Response
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	resp, downloadErr = api.NewClientFromHTTP(httpClient).Request(hostname, http.MethodGet, assetURL.String(), nil,
		api.WithHeader("Accept", "application/octet-stream"))
	if downloadErr != nil {
		return
	}
	defer resp.Body.Close()

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
	path, err := safeurl.JoinPath("repos", baseRepo.RepoOwner(), baseRepo.RepoName(), "commits", targetRef)
	if err != nil {
		return "", err
	}

	// The response body is a bare SHA rather than JSON, so Request is used instead of REST.
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	resp, err := api.NewClientFromHTTP(httpClient).Request(baseRepo.RepoHost(), http.MethodGet, path.String(), nil,
		api.WithHeader("Accept", "application/vnd.github.v3.sha"))
	if err != nil {
		var httpErr api.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusUnprocessableEntity {
			return "", commitNotFoundErr
		}
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}
