package zip

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"

	"github.com/cli/cli/v2/internal/safepaths"
)

const (
	dirMode  os.FileMode = 0755
	fileMode os.FileMode = 0644
	execMode os.FileMode = 0755
)

// ExtractZip extracts the contents of a zip archive to destDir. Each entry is
// bounded to its declared uncompressed size to guard against decompression
// bombs, but no cumulative total-size limit is imposed, since this is used to
// extract arbitrarily large artifacts (e.g. `gh run download`). Callers that
// extract a fixed-size payload and want a total cap should use
// ExtractZipWithLimit.
// Files that would result in path traversal are silently skipped.
// Files that would produce any other error cause the extraction to be aborted,
// and the error is returned.
func ExtractZip(zr *zip.Reader, destDir safepaths.Absolute) error {
	return ExtractZipWithLimit(zr, destDir, 0)
}

// ExtractZipWithLimit behaves like ExtractZip but aborts if the cumulative
// uncompressed size of the extracted entries would exceed maxTotalSize bytes,
// which guards against decompression bombs. A maxTotalSize <= 0 disables the
// total limit (the per-entry declared-size bound still applies).
func ExtractZipWithLimit(zr *zip.Reader, destDir safepaths.Absolute, maxTotalSize int64) error {
	var totalWritten int64
	for _, zf := range zr.File {
		fpath, err := destDir.Join(zf.Name)
		if err != nil {
			var pathTraversalError safepaths.PathTraversalError
			if errors.As(err, &pathTraversalError) {
				continue
			}
			return err
		}

		// Bound this entry to whatever remains of the total budget so that a
		// single oversized entry cannot exhaust disk before the total is checked.
		remaining := int64(-1) // unlimited
		if maxTotalSize > 0 {
			remaining = maxTotalSize - totalWritten
		}

		written, err := extractZipFile(zf, fpath, remaining)
		if err != nil {
			return fmt.Errorf("error extracting %q: %w", zf.Name, err)
		}
		totalWritten += written
	}
	return nil
}

// extractZipFile writes a single zip entry and returns the number of bytes
// written. The copy is bounded by the entry's declared uncompressed size and,
// when remaining >= 0, by the remaining total-archive budget; exceeding either
// aborts extraction (a decompression-bomb guard).
func extractZipFile(zf *zip.File, dest safepaths.Absolute, remaining int64) (written int64, extractErr error) {
	zm := zf.Mode()
	if zm.IsDir() {
		extractErr = os.MkdirAll(dest.String(), dirMode)
		return
	}

	var f io.ReadCloser
	f, extractErr = zf.Open()
	if extractErr != nil {
		return
	}
	defer f.Close()

	if extractErr = os.MkdirAll(filepath.Dir(dest.String()), dirMode); extractErr != nil {
		return
	}

	var df *os.File
	if df, extractErr = os.OpenFile(dest.String(), os.O_WRONLY|os.O_CREATE|os.O_EXCL, getPerm(zm)); extractErr != nil {
		return
	}

	defer func() {
		if err := df.Close(); extractErr == nil && err != nil {
			extractErr = err
		}
	}()

	// Bound extraction to the entry's declared uncompressed size and, if set, the
	// remaining total budget. Counting the bytes actually written (rather than
	// trusting the header) defends against a decompressed stream that overruns
	// either bound. We read one extra byte so an overrun is detectable.
	if zf.UncompressedSize64 >= math.MaxInt64 {
		extractErr = fmt.Errorf("zip entry %q declares an implausible uncompressed size", zf.Name)
		return
	}
	declaredSize := int64(zf.UncompressedSize64)

	limit := declaredSize
	if remaining >= 0 && remaining < limit {
		limit = remaining
	}

	written, extractErr = io.Copy(df, io.LimitReader(f, limit+1))
	if extractErr != nil {
		return
	}
	if written > declaredSize {
		extractErr = fmt.Errorf("zip entry %q exceeds its declared uncompressed size of %d bytes", zf.Name, declaredSize)
		return
	}
	if remaining >= 0 && written > remaining {
		extractErr = fmt.Errorf("zip entry %q would exceed the maximum allowed uncompressed archive size", zf.Name)
	}
	return
}

func getPerm(m os.FileMode) os.FileMode {
	if m&0111 == 0 {
		return fileMode
	}
	return execMode
}
