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

func (p *apiPlatform) Download(url string, name string, dir safepaths.Absolute) error {
	return downloadArtifact(p.client, url, name, dir)
}

func downloadArtifact(httpClient *http.Client, url string, name string, destDir safepaths.Absolute) error {
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

	// Check if the artifact is a zip file by checking Content-Type
	contentType := resp.Header.Get("Content-Type")
	isZip := contentType == "application/zip" || contentType == "application/x-zip-compressed"

	if !isZip {
		// Non-zipped artifact: write directly to destination
		destPath, err := destDir.Join(name)
		if err != nil {
			return fmt.Errorf("error creating destination path: %w", err)
		}
		outFile, err := os.Create(destPath.String())
		if err != nil {
			return fmt.Errorf("error creating output file: %w", err)
		}
		defer outFile.Close()

		_, err = io.Copy(outFile, resp.Body)
		if err != nil {
			return fmt.Errorf("error writing artifact: %w", err)
		}
		return nil
	}

	// Zipped artifact: extract to destination
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

	return nil
}
