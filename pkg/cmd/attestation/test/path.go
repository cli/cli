package test

import (
	"path/filepath"
	"runtime"
)

func NormalizeRelativePath(posixPath string) string {
	if runtime.GOOS == "windows" {
		return filepath.FromSlash(posixPath)
	}
	return posixPath
}
