// +build !windows

// The method in the file has no effect
// Only for compatibility with non-Windows systems

package color

func winSet(_ ...Color) (n int, err error) {
	return
}

func winReset() (n int, err error) {
	return
}

func winPrint(_ string, _ ...Color)   {}
func winPrintln(_ string, _ ...Color) {}
func renderColorCodeOnCmd(_ func())   {}

// IsTerminal check currently is terminal
func IsTerminal(_ int) bool {
	return true
}
