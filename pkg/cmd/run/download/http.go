package download

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/internal/safepaths"
	"github.com/cli/cli/v2/internal/safeurl"
	ghzip "github.com/cli/cli/v2/internal/zip"
	"github.com/cli/cli/v2/pkg/cmd/run/shared"
)

type apiPlatform struct {
	ctx    context.Context
	client *githubrest.Client
	repo   ghrepo.Interface
}

func (p *apiPlatform) List(runID string) ([]shared.Artifact, error) {
	return shared.ListArtifacts(p.ctx, p.client, p.repo, runID)
}

func (p *apiPlatform) Download(url safeurl.SafeURL, dir safepaths.Absolute) error {
	return downloadArtifact(p.ctx, p.client, url, dir)
}

func downloadArtifact(ctx context.Context, client *githubrest.Client, url safeurl.SafeURL, destDir safepaths.Absolute) error {
	// The server rejects an Accept of application/zip, so the request asks for
	// nothing in particular and takes what it is given.
	req, err := client.NewRequest(ctx, http.MethodGet, url.String(), nil)
	if err != nil {
		return err
	}

	// Send rather than Do, because the archive is streamed to a temporary file
	// so it can be read back as a zip, which needs a ReaderAt.
	resp, err := client.Send(req)
	if err != nil {
		if resp != nil {
			resp.Body.Close()
		}
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
