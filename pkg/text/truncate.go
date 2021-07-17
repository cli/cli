package text

import (
	"github.com/muesli/reflow/ansi"
	"github.com/muesli/reflow/truncate"
)

const (
	ellipsis            = "..."
	minWidthForEllipsis = 5
)

// DisplayWidth calculates what the rendered width of a string may be
func DisplayWidth(s string) int {
	return ansi.PrintableRuneWidth(s)
}

// Truncate shortens a string to fit the maximum display width
func Truncate(maxWidth int, s string) string {
	w := DisplayWidth(s)
	if w <= maxWidth {
		return s
	}

	tail := ""
	if maxWidth >= minWidthForEllipsis {
		tail = ellipsis
	}

	r := truncate.StringWithTail(s, uint(maxWidth), tail)
	if DisplayWidth(r) < maxWidth {
		r += " "
	}

	return r
}
