package download

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	dirMode  os.FileMode = 0755
	fileMode os.FileMode = 0644
	execMode os.FileMode = 0755
)

func extractZip(zr *zip.Reader, destDir string) error {
	destDirAbs, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}
	pathPrefix := destDirAbs + string(filepath.Separator)
	for _, zf := range zr.File {
		fpath := filepath.Join(destDirAbs, filepath.FromSlash(zf.Name))
		if !strings.HasPrefix(fpath, pathPrefix) {
			continue
		}
		if err := extractZipFile(zf, fpath); err != nil {
			return fmt.Errorf("error extracting %q: %w", zf.Name, err)
		}
	}
	return nil
}

func extractZipFile(zf *zip.File, dest string) error {
	zm := zf.Mode()
	if zm.IsDir() {
		return os.MkdirAll(dest, dirMode)
	}

	f, err := zf.Open()
	if err != nil {
		return err
	}
	defer f.Close()

	if dir := filepath.Dir(dest); dir != "." {
		if err := os.MkdirAll(dir, dirMode); err != nil {
			return err
		}
	}

	df, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, getPerm(zm))
	if err != nil {
		return err
	}
	defer df.Close()

	_, err = io.Copy(df, f)
	return err
}

func getPerm(m os.FileMode) os.FileMode {
	if m&0111 == 0 {
		return fileMode
	}
	return execMode
}
