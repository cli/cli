package extension

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
)

func hasScript(httpClient *http.Client, repo ghrepo.Interface) (hs bool, err error) {
	path := fmt.Sprintf("repos/%s/%s/contents/%s",
		repo.RepoOwner(), repo.RepoName(), repo.RepoName())
	url := ghinstance.RESTPrefix(repo.RepoHost()) + path
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return
	}

	if resp.StatusCode > 299 {
		err = api.HandleHTTPError(resp)
		return
	}

	hs = true
	return
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
func downloadAsset(httpClient *http.Client, asset releaseAsset, destPath string) error {
	req, err := http.NewRequest("GET", asset.APIURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "application/octet-stream")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		return api.HandleHTTPError(resp)
	}

	f, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0755)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

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

	if resp.StatusCode > 299 {
		return nil, api.HandleHTTPError(resp)
	}

	b, err := ioutil.ReadAll(resp.Body)
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
