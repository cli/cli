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
	"github.com/cli/cli/v2/pkg/iostreams"
)

type apiPlatform struct {
	client *http.Client
	repo   ghrepo.Interface
}

func (p *apiPlatform) List(runID string) ([]shared.Artifact, error) {
	return shared.ListArtifacts(p.client, p.repo, runID)
}

func (p *apiPlatform) Download(url string, dir safepaths.Absolute) error {
	return downloadArtifact(p.client, url, dir, "", 0, nil)
}

func (p *apiPlatform) DownloadWithProgress(url string, dir safepaths.Absolute, name string, size uint64, ioStreams *iostreams.IOStreams) error {
	return downloadArtifact(p.client, url, dir, name, int64(size), ioStreams)
}

func downloadArtifact(httpClient *http.Client, url string, destDir safepaths.Absolute, name string, size int64, ioStreams *iostreams.IOStreams) error {
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
	
	// Use progress reader if we have size information and IO streams
	if size > 0 && ioStreams != nil && name != "" {
		reader = NewProgressReader(resp.Body, size, name, ioStreams)
	}

	copySize, err := io.Copy(tmpfile, reader)
	if err != nil {
		return fmt.Errorf("error writing zip archive: %w", err)
	}

	zipfile, err := zip.NewReader(tmpfile, copySize)
	if err != nil {
		return fmt.Errorf("error extracting zip archive: %w", err)
	}
	if err := extractZip(zipfile, destDir); err != nil {
		return fmt.Errorf("error extracting zip archive: %w", err)
	}

	return nil
}
