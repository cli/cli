package utils

import (
	"os"
)

func (t *ttyTablePrinter) availableWidth() int {
	if os.Getenv("WT_SESSION") == "" {
		// If a line takes up exactly the width of the powershell.exe terminal, it will still wrap.
		// Windows Terminal does not seem affected.
		return t.maxWidth - 1
	}
	return t.maxWidth
}
