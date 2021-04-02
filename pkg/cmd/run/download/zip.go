package download

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func extractZip(zipfile *os.File, destDir string) error {
	zr, err := zip.OpenReader(zipfile.Name())
	if err != nil {
		return err
	}
	defer zr.Close()

	for _, zf := range zr.File {
		fpath := filepath.Join(destDir, zf.Name)
		if err := extractZipFile(zf, fpath); err != nil {
			return fmt.Errorf("error extracting %q: %w", zf.Name, err)
		}
	}

	return nil
}

func extractZipFile(zf *zip.File, dest string) error {
	zm := zf.Mode()
	if zm.IsDir() {
		return os.MkdirAll(dest, 0755)
	}

	f, err := zf.Open()
	if err != nil {
		return err
	}
	defer f.Close()

	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}

	var mode os.FileMode = 0644
	if isBinary(zm) {
		mode = 0755
	}
	df, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer df.Close()

	_, err = io.Copy(df, f)
	return err
}

func isBinary(m os.FileMode) bool {
	return m&0111 != 0
}
