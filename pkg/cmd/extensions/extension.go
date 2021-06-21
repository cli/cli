package extensions

import (
	"path/filepath"
	"strings"
)

type Extension struct {
	path string
	url  string
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
