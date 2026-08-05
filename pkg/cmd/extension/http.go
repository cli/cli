package extension

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/internal/safeurl"
)

func repoExists(ctx context.Context, client *githubrest.Client, repo ghrepo.Interface) (bool, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName())
	if err != nil {
		return false, err
	}

	req, err := client.NewRequest(ctx, http.MethodGet, path.String(), nil)
	if err != nil {
		return false, err
	}

	// Send is used rather than Do because this check is about the status code:
	// it returns a non-nil response even for the *ErrorResponse a 404 produces,
	// and the caller must close the body.
	resp, err := client.Send(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if resp == nil {
		return false, err
	}

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, githubrest.NewErrorResponse(resp.Response)
	}
}

func hasScript(ctx context.Context, client *githubrest.Client, repo ghrepo.Interface) (bool, error) {
	path, err := safeurl.JoinPath("repos", repo.RepoOwner(), repo.RepoName(), "contents", repo.RepoName())
	if err != nil {
		return false, err
	}

	// The response body is not decoded, because a script is considered present for any
	// successful response regardless of the content type reported.
	req, err := client.NewRequest(ctx, http.MethodGet, path.String(), nil)
	if err != nil {
		return false, err
	}
	if _, err := client.Do(req, nil); err != nil {
		var httpErr *githubrest.ErrorResponse
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
// The asset URL is a GitHub REST API URL, so it goes through client, whose host
// is credentialed. Send is used rather than Do so a non-2xx status is caught
// before the destination file is created, leaving no empty file behind on
// failure, and because the binary body is streamed straight to disk.
func downloadAsset(ctx context.Context, client *githubrest.Client, assetURL safeurl.SafeURL, destPath string) (downloadErr error) {
	req, err := client.NewRequest(ctx, http.MethodGet, assetURL.String(), nil, githubrest.WithHeader("Accept", "application/octet-stream"))
	if err != nil {
		return err
	}

	resp, err := client.Send(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return err
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
func fetchLatestRelease(ctx context.Context, client *githubrest.Client, baseRepo ghrepo.Interface) (*release, error) {
	path, err := safeurl.JoinPath("repos", baseRepo.RepoOwner(), baseRepo.RepoName(), "releases", "latest")
	if err != nil {
		return nil, err
	}

	var data json.RawMessage
	req, err := client.NewRequest(ctx, http.MethodGet, path.String(), nil)
	if err != nil {
		return nil, err
	}
	if _, err := client.Do(req, &data); err != nil {
		var httpErr *githubrest.ErrorResponse
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
func fetchReleaseFromTag(ctx context.Context, client *githubrest.Client, baseRepo ghrepo.Interface, tagName string) (*release, error) {
	path, err := safeurl.JoinPath("repos", baseRepo.RepoOwner(), baseRepo.RepoName(), "releases", "tags", tagName)
	if err != nil {
		return nil, err
	}

	var data json.RawMessage
	req, err := client.NewRequest(ctx, http.MethodGet, path.String(), nil)
	if err != nil {
		return nil, err
	}
	if _, err := client.Do(req, &data); err != nil {
		var httpErr *githubrest.ErrorResponse
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
func fetchCommitSHA(ctx context.Context, client *githubrest.Client, baseRepo ghrepo.Interface, targetRef string) (string, error) {
	path, err := safeurl.JoinPath("repos", baseRepo.RepoOwner(), baseRepo.RepoName(), "commits", targetRef)
	if err != nil {
		return "", err
	}

	// The v3.sha media type makes the endpoint return the bare SHA as the body,
	// so it is streamed into a buffer rather than decoded as JSON.
	req, err := client.NewRequest(ctx, http.MethodGet, path.String(), nil, githubrest.WithHeader("Accept", "application/vnd.github.v3.sha"))
	if err != nil {
		return "", err
	}

	var body bytes.Buffer
	resp, err := client.Do(req, &body)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnprocessableEntity {
			return "", commitNotFoundErr
		}
		return "", err
	}

	return body.String(), nil
}
