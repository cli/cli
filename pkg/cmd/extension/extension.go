package extension

import (
	"path/filepath"
	"strings"
	"sync"
)

const manifestName = "manifest.yml"

type ExtensionKind int

const (
	GitKind ExtensionKind = iota
	BinaryKind
	LocalKind
)

type Extension struct {
	path string
	// isLocal bool
	kind ExtensionKind

	mu sync.RWMutex

	// These fields get resolved dynamically:

	url            string
	isPinned       *bool
	currentVersion string
	latestVersion  string
}

func (e *Extension) Name() string {
	return strings.TrimPrefix(filepath.Base(e.path), "gh-")
}

func (e *Extension) Path() string {
	return e.path
}

func (e *Extension) IsLocal() bool {
	return e.kind == LocalKind
}

func (e *Extension) URL() string {
	e.mu.RLock()
	if e.url != "" {
		defer e.mu.RUnlock()
		return e.url
	}
	e.mu.RUnlock()
	e.mu.Lock()
	defer e.mu.Unlock()
	// TODO: lookup extension URL
	return ""
}

func (e *Extension) CurrentVersion() string {
	e.mu.RLock()
	if e.currentVersion != "" {
		defer e.mu.RUnlock()
		return e.currentVersion
	}
	e.mu.RUnlock()
	e.mu.Lock()
	defer e.mu.Unlock()
	// TODO: lookup the current version for the extension
	return ""
}

func (e *Extension) IsPinned() bool {
	e.mu.RLock()
	if e.isPinned != nil {
		defer e.mu.RUnlock()
		return *e.isPinned
	}
	e.mu.RUnlock()
	e.mu.Lock()
	defer e.mu.Unlock()
	// TODO: lookup the pinned property of the extension
	return false
}

func (e *Extension) UpdateAvailable() bool {
	if e.IsLocal() ||
		e.CurrentVersion() == "" ||
		e.latestVersion == "" ||
		e.CurrentVersion() == e.latestVersion {
		return false
	}
	return true
}

func (e *Extension) IsBinary() bool {
	return e.kind == BinaryKind
}
