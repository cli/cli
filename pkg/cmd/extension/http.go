package extension

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safeurl"
	"github.com/hashicorp/go-version"
)

func repoExists(httpClient *http.Client, repo ghrepo.Interface) (bool, error) {
	url, err := safeurl.JoinPathWithHostPrefix(ghinstance.RESTPrefix(repo.RepoHost()), "repos", repo.RepoOwner(), repo.RepoName())
	if err != nil {
		return false, err
	}
	req, err := http.NewRequest("GET", url.String(), nil)
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
	url, err := safeurl.JoinPathWithHostPrefix(ghinstance.RESTPrefix(repo.RepoHost()), "repos", repo.RepoOwner(), repo.RepoName(), "contents", repo.RepoName())
	if err != nil {
		return false, err
	}
	req, err := http.NewRequest("GET", url.String(), nil)
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
	Tag          string    `json:"tag_name"`
	IsPrerelease bool      `json:"prerelease"`
	IsDraft      bool      `json:"draft"`
	PublishedAt  time.Time `json:"published_at"`
	Assets       []releaseAsset
}

// downloadAsset downloads a single asset to the given file path.
func downloadAsset(httpClient *http.Client, assetURL safeurl.SafeURL, destPath string) (downloadErr error) {
	var req *http.Request
	if req, downloadErr = http.NewRequest("GET", assetURL.String(), nil); downloadErr != nil {
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
var noPrereleasesFoundErr = errors.New("no pre-releases found")

// fetchLatestRelease finds the latest published release for a repository.
func fetchLatestRelease(httpClient *http.Client, baseRepo ghrepo.Interface) (*release, error) {
	url, err := safeurl.JoinPathWithHostPrefix(ghinstance.RESTPrefix(baseRepo.RepoHost()), "repos", baseRepo.RepoOwner(), baseRepo.RepoName(), "releases", "latest")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("GET", url.String(), nil)
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

// fetchLatestPrerelease finds the highest-versioned pre-release for a
// repository. It only considers releases marked as pre-releases, selecting the
// one with the highest version. If the repository has no pre-releases it
// returns noPrereleasesFoundErr.
//
// When a stable (non-pre-release) release beats the chosen pre-release, either
// by a higher version or by a more recent publish date, it is returned as
// newerStable so the caller can warn the user that a newer stable release is
// available.
//
// Note that if the latest pre-release is not on the first page of 100, it is
// possible that this will not find it; for performance reasons in busy
// repositories it is not safe or efficient to iterate over every page of
// releases. In those cases, the user should specify a tag with --pin.
func fetchLatestPrerelease(httpClient *http.Client, baseRepo ghrepo.Interface) (prerelease *release, newerStable *release, err error) {
	path := fmt.Sprintf("repos/%s/%s/releases?per_page=100", baseRepo.RepoOwner(), baseRepo.RepoName())
	url := ghinstance.RESTPrefix(baseRepo.RepoHost()) + path
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, nil, releaseNotFoundErr
	}
	if resp.StatusCode > 299 {
		return nil, nil, api.HandleHTTPError(resp)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	var releases []release
	if err := json.Unmarshal(b, &releases); err != nil {
		return nil, nil, err
	}

	var bestPre *release
	var bestPreVersion *version.Version
	var bestStable *release
	var bestStableVersion *version.Version
	for i := range releases {
		r := &releases[i]
		if r.IsDraft {
			continue
		}
		// Tags that are not valid semver cannot be ordered against other
		// releases, so they are skipped. This means a repository whose newest
		// pre-release uses an unparseable tag (e.g. v1.0.0.beta.2) may resolve
		// to an older pre-release; users can reach such a release with --pin.
		v, verr := version.NewVersion(r.Tag)
		if verr != nil {
			continue
		}
		if r.IsPrerelease {
			if bestPre == nil || v.GreaterThan(bestPreVersion) {
				bestPre = r
				bestPreVersion = v
			}
			continue
		}
		if bestStable == nil || v.GreaterThan(bestStableVersion) {
			bestStable = r
			bestStableVersion = v
		}
	}

	if bestPre == nil {
		return nil, nil, noPrereleasesFoundErr
	}

	if bestStable != nil {
		if bestStableVersion.GreaterThan(bestPreVersion) || bestStable.PublishedAt.After(bestPre.PublishedAt) {
			newerStable = bestStable
		}
	}

	return bestPre, newerStable, nil
}

// fetchReleaseFromTag finds release by tag name for a repository
func fetchReleaseFromTag(httpClient *http.Client, baseRepo ghrepo.Interface, tagName string) (*release, error) {
	url, err := safeurl.JoinPathWithHostPrefix(ghinstance.RESTPrefix(baseRepo.RepoHost()), "repos", baseRepo.RepoOwner(), baseRepo.RepoName(), "releases", "tags", tagName)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("GET", url.String(), nil)
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
	url, err := safeurl.JoinPathWithHostPrefix(ghinstance.RESTPrefix(baseRepo.RepoHost()), "repos", baseRepo.RepoOwner(), baseRepo.RepoName(), "commits", targetRef)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest("GET", url.String(), nil)
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
