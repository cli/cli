package download

import (
	"archive/zip"
	"context"
	"crypto/md5" //nolint:gosec // MD5 is used only for server-supplied integrity check, not for security
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"

	"golang.org/x/sync/errgroup"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/safepaths"
	ghzip "github.com/cli/cli/v2/internal/zip"
	"github.com/cli/cli/v2/pkg/cmd/run/shared"
)

// maxPreallocBytes caps the size we will pre-allocate for a chunked download
// to guard against a server supplying a fraudulent Content-Length.
const maxPreallocBytes = int64(2 * 1024 * 1024 * 1024) // 2 GiB

// chunkThreshold is the minimum Content-Length above which a parallel chunked
// download is attempted. Declared as a var so tests can override it.
var chunkThreshold = int64(32 * 1024 * 1024) // 32 MiB

// chunkSize is the byte length of each Range GET request.
var chunkSize = int64(16 * 1024 * 1024) // 16 MiB

// chunkConcurrency is the maximum number of in-flight Range GET goroutines.
// Derived from runtime.NumCPU()*2 (keeping goroutines runnable while siblings
// block on I/O), capped at 10 to avoid overwhelming intermediate proxies.
var chunkConcurrency = min(runtime.NumCPU()*2, 10)

type apiPlatform struct {
	client       *http.Client
	repo         ghrepo.Interface
	singleStream bool
}

func (p *apiPlatform) List(runID string) ([]shared.Artifact, error) {
	return shared.ListArtifacts(p.client, p.repo, runID)
}

func (p *apiPlatform) Download(url string, dir safepaths.Absolute) error {
	return downloadArtifact(p.client, url, dir, p.singleStream)
}

// DownloadByID downloads the artifact with the given ID without requiring a
// prior listing call. It constructs the REST download URL from the repo and
// artifact ID and then delegates to the same chunked/single-stream path as
// Download.
func (p *apiPlatform) DownloadByID(id int64, dir safepaths.Absolute) error {
	url := fmt.Sprintf("%srepos/%s/%s/actions/artifacts/%d/zip",
		ghinstance.RESTPrefix(p.repo.RepoHost()),
		p.repo.RepoOwner(), p.repo.RepoName(), id)
	return downloadArtifact(p.client, url, dir, p.singleStream)
}

func downloadArtifact(httpClient *http.Client, url string, destDir safepaths.Absolute, singleStream bool) error {
	if !singleStream {
		err := downloadArtifactChunked(httpClient, url, destDir)
		if err == nil {
			return nil
		}
		if !errors.Is(err, errNoRangeSupport) {
			return err
		}
	}
	return downloadArtifactSingleStream(httpClient, url, destDir)
}

// errNoRangeSupport is returned by downloadArtifactChunked when the server
// does not advertise Accept-Ranges: bytes or the artifact is below the
// chunking threshold. It is never surfaced to the end user.
var errNoRangeSupport = errors.New("server does not support range requests or artifact is below chunking threshold")

func downloadArtifactChunked(httpClient *http.Client, url string, destDir safepaths.Absolute) error {
	ctx := context.Background()

	headReq, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return errNoRangeSupport
	}
	headResp, err := httpClient.Do(headReq)
	if err != nil {
		return errNoRangeSupport
	}
	_, _ = io.Copy(io.Discard, headResp.Body)
	_ = headResp.Body.Close()

	if headResp.Header.Get("Accept-Ranges") != "bytes" {
		return errNoRangeSupport
	}

	contentLength := headResp.ContentLength
	if contentLength <= 0 || contentLength < chunkThreshold {
		return errNoRangeSupport
	}

	// Reject unreasonably large Content-Length values before pre-allocating.
	if contentLength > maxPreallocBytes {
		return errNoRangeSupport
	}

	expectedMD5 := headResp.Header.Get("Content-MD5")

	tmpfile, err := os.CreateTemp("", "gh-artifact.*.zip")
	if err != nil {
		return fmt.Errorf("error initializing temporary file: %w", err)
	}
	defer func() {
		_ = tmpfile.Close()
		_ = os.Remove(tmpfile.Name())
	}()

	// Pre-allocate to the exact final size so concurrent WriteAt calls land in
	// valid, non-overlapping regions.
	if err := tmpfile.Truncate(contentLength); err != nil {
		return fmt.Errorf("error pre-sizing temporary file: %w", err)
	}

	numChunks := (contentLength + chunkSize - 1) / chunkSize
	g, gCtx := errgroup.WithContext(ctx)
	sem := make(chan struct{}, chunkConcurrency)

	for i := range numChunks {
		start := i * chunkSize
		end := start + chunkSize - 1
		if end >= contentLength {
			end = contentLength - 1
		}
		g.Go(func() error {
			sem <- struct{}{}
			defer func() { <-sem }()
			return downloadChunk(gCtx, httpClient, url, tmpfile, start, end)
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	// Verify whole-file MD5 when the server supplied one (Content-MD5 header,
	// base64-encoded, per RFC 1864).
	if expectedMD5 != "" {
		if err := verifyMD5(tmpfile, contentLength, expectedMD5); err != nil {
			return err
		}
	}

	zipfile, err := zip.NewReader(tmpfile, contentLength)
	if err != nil {
		return fmt.Errorf("error extracting zip archive: %w", err)
	}
	return ghzip.ExtractZip(zipfile, destDir)
}

func downloadChunk(ctx context.Context, httpClient *http.Client, url string, out *os.File, start, end int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("error building range request: %w", err)
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error fetching bytes=%d-%d: %w", start, end, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("range request for bytes=%d-%d returned unexpected status %s", start, end, resp.Status)
	}

	if err := validateContentRange(resp.Header.Get("Content-Range"), start, end); err != nil {
		return err
	}

	if _, err = io.Copy(io.NewOffsetWriter(out, start), resp.Body); err != nil {
		return fmt.Errorf("error writing bytes=%d-%d: %w", start, end, err)
	}
	return nil
}

// validateContentRange checks that the Content-Range response header matches
// the byte range we requested, guarding against a server returning unexpected
// ranges without signalling an error.
func validateContentRange(contentRange string, start, end int64) error {
	if contentRange == "" {
		return nil // server omitted header; treat as valid
	}
	expected := fmt.Sprintf("bytes %d-%d/", start, end)
	if len(contentRange) < len(expected) || contentRange[:len(expected)] != expected {
		return fmt.Errorf("Content-Range mismatch: requested bytes=%d-%d but got %q", start, end, contentRange)
	}
	return nil
}

func verifyMD5(f *os.File, size int64, expectedBase64 string) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("error seeking temp file for MD5 check: %w", err)
	}
	h := md5.New() //nolint:gosec
	if _, err := io.Copy(h, io.LimitReader(f, size)); err != nil {
		return fmt.Errorf("error computing MD5: %w", err)
	}
	actual := base64.StdEncoding.EncodeToString(h.Sum(nil))
	if actual != expectedBase64 {
		return fmt.Errorf("artifact integrity check failed: Content-MD5 mismatch (expected %s, got %s)", expectedBase64, actual)
	}
	return nil
}

func downloadArtifactSingleStream(httpClient *http.Client, url string, destDir safepaths.Absolute) error {
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
