package iostreams

import (
	"os"
)

func Is256ColorSupported() bool {
	// Windows advertises neither TERM nor COLORTERM, but we assume that 256 colors are supported.
	return true
}

func IsTrueColorSupported() bool {
	// Windows Terminal supports true color.
	return os.Getenv("WT_SESSION") != ""
}
