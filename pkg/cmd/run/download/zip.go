package download

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/cli/cli/v2/internal/safepaths"
)

const (
	dirMode  os.FileMode = 0755
	fileMode os.FileMode = 0644
	execMode os.FileMode = 0755
)

func extractZip(zr *zip.Reader, destDir safepaths.Absolute) error {
	for _, zf := range zr.File {
		fpath, err := destDir.Join(zf.Name)
		if err != nil {
			var pathTraversalError safepaths.PathTraversalError
			if errors.As(err, &pathTraversalError) {
				continue
			}
			return err
		}

		if err := extractZipFile(zf, fpath); err != nil {
			return fmt.Errorf("error extracting %q: %w", zf.Name, err)
		}
	}
	return nil
}

func extractZipFile(zf *zip.File, dest safepaths.Absolute) (extractErr error) {
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

	_, extractErr = io.Copy(df, f)
	return
}

func getPerm(m os.FileMode) os.FileMode {
	if m&0111 == 0 {
		return fileMode
	}
	return execMode
}
