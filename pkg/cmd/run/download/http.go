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
	"github.com/cli/cli/v2/internal/safeurl"
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

func (p *apiPlatform) Download(url safeurl.SafeURL, dir safepaths.Absolute) error {
	return downloadArtifact(p.client, p.repo.RepoHost(), url, dir)
}

func downloadArtifact(httpClient *http.Client, hostname string, url safeurl.SafeURL, destDir safepaths.Absolute) error {
	// The server rejects this :(
	//api.WithHeader("Accept", "application/zip")
	//
	// The artifact download URL is supplied by the API, so it is absolute and is requested as
	// given rather than being resolved against the host's REST endpoint.
	// TODO(api-client-rollout)
	// This line of code is part of a mechanical roll out of the api client.
	// As a follow up, consider whether the api client can be injected to this call site, rather than constructed
	resp, err := api.NewClientFromHTTP(httpClient).Request(hostname, http.MethodGet, url.String(), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

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
