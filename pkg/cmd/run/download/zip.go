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
	for _, zf := range zr.File {
		fpath := filepath.Join(destDir, filepath.FromSlash(zf.Name))
		if !filepathDescendsFrom(fpath, destDir) {
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

func filepathDescendsFrom(p, dir string) bool {
	p = filepath.Clean(p)
	dir = filepath.Clean(dir)
	if dir == "." && !filepath.IsAbs(p) {
		return !strings.HasPrefix(p, ".."+string(filepath.Separator))
	}
	if !strings.HasSuffix(dir, string(filepath.Separator)) {
		dir += string(filepath.Separator)
	}
	return strings.HasPrefix(p, dir)
}
