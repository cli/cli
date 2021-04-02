package download

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/internal/ghrepo"
)

type Artifact struct {
	Name        string `json:"name"`
	Size        uint32 `json:"size_in_bytes"`
	DownloadURL string `json:"archive_download_url"`
	Expired     bool   `json:"expired"`
}

type apiPlatform struct {
	client *http.Client
	repo   ghrepo.Interface
}

func (p *apiPlatform) List(runID string) ([]Artifact, error) {
	return listArtifacts(p.client, p.repo, runID)
}

func (p *apiPlatform) Download(url string, dir string) error {
	return downloadArtifact(p.client, url, dir)
}

func listArtifacts(httpClient *http.Client, repo ghrepo.Interface, runID string) ([]Artifact, error) {
	perPage := 100
	path := fmt.Sprintf("repos/%s/%s/actions/artifacts?per_page=%d", repo.RepoOwner(), repo.RepoName(), perPage)
	if runID != "" {
		path = fmt.Sprintf("repos/%s/%s/actions/runs/%s/artifacts?per_page=%d", repo.RepoOwner(), repo.RepoName(), runID, perPage)
	}

	req, err := http.NewRequest("GET", ghinstance.RESTPrefix(repo.RepoHost())+path, nil)
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

	var response struct {
		TotalCount uint16 `json:"total_count"`
		Artifacts  []Artifact
	}

	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&response); err != nil {
		return response.Artifacts, fmt.Errorf("error reading JSON: %w", err)
	}

	return response.Artifacts, nil
}

func downloadArtifact(httpClient *http.Client, url, destDir string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	// The API rejects requesting the "application/octet-stream" type :(
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		return api.HandleHTTPError(resp)
	}

	tmpfile, err := ioutil.TempFile("", "gh-artifact.*.zip")
	if err != nil {
		return fmt.Errorf("error initializing temporary file: %w", err)
	}
	defer tmpfile.Close()

	_, err = io.Copy(tmpfile, resp.Body)
	if err != nil {
		return fmt.Errorf("error writing zip archive: %w", err)
	}

	tmpfile.Close()
	if err := extractZip(tmpfile, destDir); err != nil {
		return fmt.Errorf("error extracting zip archive: %w", err)
	}
	if err := os.Remove(tmpfile.Name()); err != nil {
		return err
	}

	return nil
}
