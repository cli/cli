package extension

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/internal/safeurl"
)

// statusIs reports whether err is a REST error carrying the given status.
//
// Send and Do turn every non-2xx into an error, so the handful of call sites
// that treat a particular status as a normal outcome have to look inside the
// error rather than at a status code.
func statusIs(err error, status int) bool {
	var errResp *githubrest.ErrorResponse
	return errors.As(err, &errResp) && errResp.StatusCode == status
}

func repoExists(httpClient *http.Client, repo ghrepo.Interface) (bool, error) {
	client, err := api.NewRESTClient(httpClient, repo.RepoHost())
	if err != nil {
		return false, err
	}

	url, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName())
	if err != nil {
		return false, err
	}

	req, err := client.NewRequest(context.Background(), http.MethodGet, url.String(), nil)
	if err != nil {
		return false, err
	}

	// The body is not read, so Do discards it rather than Send leaving it open.
	if _, err := client.Do(req, nil); err != nil {
		if statusIs(err, http.StatusNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func hasScript(httpClient *http.Client, repo ghrepo.Interface) (bool, error) {
	client, err := api.NewRESTClient(httpClient, repo.RepoHost())
	if err != nil {
		return false, err
	}

	url, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "contents", repo.RepoName())
	if err != nil {
		return false, err
	}

	req, err := client.NewRequest(context.Background(), http.MethodGet, url.String(), nil)
	if err != nil {
		return false, err
	}

	if _, err := client.Do(req, nil); err != nil {
		if statusIs(err, http.StatusNotFound) {
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
func downloadAsset(httpClient *http.Client, hostname string, assetURL safeurl.SafeURL, destPath string) (downloadErr error) {
	var client *githubrest.Client
	if client, downloadErr = api.NewRESTClient(httpClient, hostname); downloadErr != nil {
		return
	}

	var req *http.Request
	req, downloadErr = client.NewRequest(context.Background(), http.MethodGet, assetURL.String(), nil,
		githubrest.WithHeader("Accept", "application/octet-stream"))
	if downloadErr != nil {
		return
	}

	// Send rather than Do, because the body is streamed to a file.
	var resp *githubrest.Response
	if resp, downloadErr = client.Send(req); downloadErr != nil {
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
	client, err := api.NewRESTClient(httpClient, baseRepo.RepoHost())
	if err != nil {
		return nil, err
	}

	url, err := safeurl.JoinPath("repos", baseRepo.RepoOwner(), baseRepo.RepoName(), "releases", "latest")
	if err != nil {
		return nil, err
	}

	req, err := client.NewRequest(context.Background(), http.MethodGet, url.String(), nil)
	if err != nil {
		return nil, err
	}

	var r release
	if _, err := client.Do(req, &r); err != nil {
		if statusIs(err, http.StatusNotFound) {
			return nil, releaseNotFoundErr
		}
		return nil, err
	}

	return &r, nil
}

// fetchReleaseFromTag finds release by tag name for a repository
func fetchReleaseFromTag(httpClient *http.Client, baseRepo ghrepo.Interface, tagName string) (*release, error) {
	client, err := api.NewRESTClient(httpClient, baseRepo.RepoHost())
	if err != nil {
		return nil, err
	}

	url, err := safeurl.JoinPath("repos", baseRepo.RepoOwner(), baseRepo.RepoName(), "releases", "tags", tagName)
	if err != nil {
		return nil, err
	}

	req, err := client.NewRequest(context.Background(), http.MethodGet, url.String(), nil)
	if err != nil {
		return nil, err
	}

	var r release
	if _, err := client.Do(req, &r); err != nil {
		if statusIs(err, http.StatusNotFound) {
			return nil, releaseNotFoundErr
		}
		return nil, err
	}

	return &r, nil
}

// fetchCommitSHA finds full commit SHA from a target ref in a repo
func fetchCommitSHA(httpClient *http.Client, baseRepo ghrepo.Interface, targetRef string) (string, error) {
	client, err := api.NewRESTClient(httpClient, baseRepo.RepoHost())
	if err != nil {
		return "", err
	}

	url, err := safeurl.JoinPath("repos", baseRepo.RepoOwner(), baseRepo.RepoName(), "commits", targetRef)
	if err != nil {
		return "", err
	}

	req, err := client.NewRequest(context.Background(), http.MethodGet, url.String(), nil,
		githubrest.WithHeader("Accept", "application/vnd.github.v3.sha"))
	if err != nil {
		return "", err
	}

	// Send rather than Do, because the response body is a bare SHA rather than
	// JSON.
	resp, err := client.Send(req)
	if err != nil {
		if statusIs(err, http.StatusUnprocessableEntity) {
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
