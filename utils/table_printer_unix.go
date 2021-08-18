//+build !windows

package utils

func (t *ttyTablePrinter) availableWidth() int {
	return t.maxWidth
}
