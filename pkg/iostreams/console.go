//go:build !windows
// +build !windows

package iostreams

import "os"

func hasAlternateScreenBuffer(hasTrueColor bool) bool {
	// on non-Windows, we just assume that alternate screen buffer is supported in most cases
	return os.Getenv("TERM") != "dumb"
}
