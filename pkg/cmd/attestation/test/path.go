package test

import (
	"runtime"
	"strings"
)

func NormalizeRelativePath(posixPath string) string {
	if runtime.GOOS == "windows" {
		return strings.ReplaceAll(posixPath, "/", "\\")
	}
	return posixPath
}
