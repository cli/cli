package upload

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/internal/ghrepo"
)

type Release struct {
	UploadURL string `json:"upload_url"`
	Assets    []ReleaseAsset
}

type ReleaseAsset struct {
	Name  string
	State string
	URL   string
}

func fetchRelease(httpClient *http.Client, baseRepo ghrepo.Interface, tagName string) (*Release, error) {
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

func uploadAsset(httpClient *http.Client, uploadURL string, asset AssetForUpload) (*ReleaseAsset, error) {
	u, err := url.Parse(uploadURL)
	if err != nil {
		return nil, err
	}
	params := u.Query()
	params.Set("name", asset.Name)
	params.Set("label", asset.Label)
	u.RawQuery = params.Encode()

	req, err := http.NewRequest("POST", u.String(), asset.Data)
	if err != nil {
		return nil, err
	}
	req.ContentLength = asset.Size
	req.Header.Set("Content-Type", asset.MIMEType)

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

	var newAsset ReleaseAsset
	err = json.Unmarshal(b, &newAsset)
	if err != nil {
		return nil, err
	}

	return &newAsset, nil
}

func deleteAsset(httpClient *http.Client, assetURL string) error {
	req, err := http.NewRequest("DELETE", assetURL, nil)
	if err != nil {
		return err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !success {
		return api.HandleHTTPError(resp)
	}

	return nil
}
