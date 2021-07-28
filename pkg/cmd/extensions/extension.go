package extensions

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type Extension struct {
	path            string
	url             string
	updateAvailable bool
}

func (e *Extension) Name() string {
	return strings.TrimPrefix(filepath.Base(e.path), "gh-")
}

func (e *Extension) Path() string {
	return e.path
}

func (e *Extension) URL() string {
	return e.url
}

func (e *Extension) IsLocal() bool {
	dir := filepath.Dir(e.path)
	fileInfo, err := os.Lstat(dir)
	if err != nil {
		return false
	}
	// Check if extension is a symlink
	if fileInfo.Mode()&os.ModeSymlink != 0 {
		return true
	}
	// Check if extension does not have a git directory
	if _, err = os.Stat(filepath.Join(dir, ".git")); errors.Is(err, os.ErrNotExist) {
		return true
	}
	return false
}

func (e *Extension) UpdateAvailable() bool {
	return e.updateAvailable
}
