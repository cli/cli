package download

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safepaths"
	ghzip "github.com/cli/cli/v2/internal/zip"
	"github.com/cli/cli/v2/pkg/cmd/run/shared"
)

type apiPlatform struct {
	client *http.Client
	repo   ghrepo.Interface
}

func (p *apiPlatform) List(runID string) ([]shared.Artifact, error) {
	return shared.ListArtifacts(p.client, p.repo, runID)
}

func (p *apiPlatform) Download(url string, dir safepaths.Absolute) error {
	return downloadArtifact(p.client, url, dir, "")
}

func (p *apiPlatform) DownloadWithName(url string, name string, dir safepaths.Absolute) error {
	return downloadArtifact(p.client, url, dir, name)
}

func downloadArtifact(httpClient *http.Client, url string, destDir safepaths.Absolute, name string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		return api.HandleHTTPError(resp)
	}

	// Non-zipped artifacts (uploaded with archive: false) are returned
	// with Content-Type application/octet-stream. Only attempt zip
	// extraction when the response is actually a zip archive.
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "application/zip") {
		tmpfile, err := os.CreateTemp("", "gh-artifact.*.zip")
		if err != nil {
			return fmt.Errorf("error initializing temporary file: %w", err)
		}
		defer func() {
			_ = tmpfile.Close()
			_ = os.Remove(tmpfile.Name())
		}()

		size, err := io.Copy(tmpfile, resp.Body)
		if err != nil {
			return fmt.Errorf("error writing zip archive: %w", err)
		}

		zipfile, err := zip.NewReader(tmpfile, size)
		if err != nil {
			return fmt.Errorf("error extracting zip archive: %w", err)
		}
		if err := ghzip.ExtractZip(zipfile, destDir); err != nil {
			return fmt.Errorf("error extracting zip archive: %w", err)
		}
	} else {
		// Non-zipped artifact: save the raw file directly.
		fileName := name
		if fileName == "" {
			fileName = "artifact"
		}
		filePath, err := destDir.Join(fileName)
		if err != nil {
			return fmt.Errorf("error creating file path: %w", err)
		}
		f, err := os.Create(filePath.String())
		if err != nil {
			return fmt.Errorf("error creating file: %w", err)
		}
		defer f.Close()
		if _, err := io.Copy(f, resp.Body); err != nil {
			return fmt.Errorf("error writing file: %w", err)
		}
	}

	return nil
}
