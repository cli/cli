package extension

import (
	"path/filepath"
	"strings"
)

const manifestName = "manifest.yml"

type Extension struct {
	path            string
	url             string
	isLocal         bool
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
	return e.isLocal
}

func (e *Extension) UpdateAvailable() bool {
	return e.updateAvailable
}
