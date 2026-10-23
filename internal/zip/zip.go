package zip

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/cli/cli/v2/internal/safepaths"
)

const (
	dirMode  os.FileMode = 0755
	fileMode os.FileMode = 0644
	execMode os.FileMode = 0755
)

// ExtractZip extracts the contents of a zip archive to destDir.
// Entries that would escape destDir lexically are skipped.
func ExtractZip(zr *zip.Reader, destDir *safepaths.Root) error {
	for _, zf := range zr.File {
		if err := extractZipFile(zf, destDir); err != nil {
			if _, ok := errors.AsType[safepaths.PathTraversalError](err); ok {
				continue
			}
			return fmt.Errorf("error extracting %q: %w", zf.Name, err)
		}
	}
	return nil
}

func extractZipFile(zf *zip.File, dest *safepaths.Root) (extractErr error) {
	zm := zf.Mode()
	if zm.IsDir() {
		extractErr = dest.MkdirAll(zf.Name, dirMode)
		return
	}

	var f io.ReadCloser
	f, extractErr = zf.Open()
	if extractErr != nil {
		return
	}
	defer f.Close()

	var df *os.File
	if df, extractErr = dest.Create(zf.Name, getPerm(zm), dirMode, false); extractErr != nil {
		return
	}

	defer func() {
		if err := df.Close(); extractErr == nil && err != nil {
			extractErr = err
		}
	}()

	_, extractErr = io.Copy(df, f)
	return
}

func getPerm(m os.FileMode) os.FileMode {
	if m&0111 == 0 {
		return fileMode
	}
	return execMode
}
