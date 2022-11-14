//go:build windows
// +build windows

package iostreams

func hasAlternateScreenBuffer(hasTrueColor bool) bool {
	// on Windows we just assume that alternate screen buffer is supported if we
	// enabled virtual terminal processing, which in turn enables truecolor
	return hasTrueColor
}
