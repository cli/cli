package download

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safepaths"
	"github.com/cli/cli/v2/pkg/cmd/run/shared"
	"github.com/schollz/progressbar/v3"
)

type apiPlatform struct {
	client *http.Client
	repo   ghrepo.Interface
}

func (p *apiPlatform) List(runID string) ([]shared.Artifact, error) {
	return shared.ListArtifacts(p.client, p.repo, runID)
}

func (p *apiPlatform) Download(url string, dir safepaths.Absolute) error {
	return downloadArtifact(p.client, url, dir)
}

func downloadArtifact(httpClient *http.Client, url string, destDir safepaths.Absolute) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	// The server rejects this :(
	//req.Header.Set("Accept", "application/zip")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		return api.HandleHTTPError(resp)
	}

	tmpfile, err := os.CreateTemp("", "gh-artifact.*.zip")
	if err != nil {
		return fmt.Errorf("error initializing temporary file: %w", err)
	}
	defer func() {
		_ = tmpfile.Close()
		_ = os.Remove(tmpfile.Name())
	}()

	var reader io.Reader = resp.Body
	if resp.ContentLength > 0 {
		bar := progressbar.DefaultBytes(
			resp.ContentLength,
			"Downloading artifact",
		)
		reader = io.TeeReader(resp.Body, bar)
	}

	size, err := io.Copy(tmpfile, reader)
	if err != nil {
		return fmt.Errorf("error writing zip archive: %w", err)
	}

	zipfile, err := zip.NewReader(tmpfile, size)
	if err != nil {
		return fmt.Errorf("error extracting zip archive: %w", err)
	}
	if err := extractZip(zipfile, destDir); err != nil {
		return fmt.Errorf("error extracting zip archive: %w", err)
	}

	return nil
}
