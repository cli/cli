package extension

import (
	"path/filepath"
	"strings"
)

const manifestName = "manifest.yml"

type ExtensionKind int

const (
	GitKind ExtensionKind = iota
	BinaryKind
)

type Extension struct {
	path           string
	url            string
	isLocal        bool
	isPinned       bool
	currentVersion string
	latestVersion  string
	kind           ExtensionKind
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

func (e *Extension) CurrentVersion() string {
	return e.currentVersion
}

func (e *Extension) IsPinned() bool {
	return e.isPinned
}

func (e *Extension) UpdateAvailable() bool {
	if e.isLocal ||
		e.currentVersion == "" ||
		e.latestVersion == "" ||
		e.currentVersion == e.latestVersion {
		return false
	}
	return true
}

func (e *Extension) IsBinary() bool {
	return e.kind == BinaryKind
}
