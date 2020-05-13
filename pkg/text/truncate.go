package text

import (
	"golang.org/x/text/width"
)

// DisplayWidth calculates what the rendered width of a string may be
func DisplayWidth(s string) int {
	w := 0
	for _, r := range s {
		w += runeDisplayWidth(r)
	}
	return w
}

const (
	ellipsisWidth       = 3
	minWidthForEllipsis = 5
)

// Truncate shortens a string to fit the maximum display width
func Truncate(max int, s string) string {
	w := DisplayWidth(s)
	if w <= max {
		return s
	}

	useEllipsis := false
	if max >= minWidthForEllipsis {
		useEllipsis = true
		max -= ellipsisWidth
	}

	cw := 0
	ri := 0
	for _, r := range s {
		rw := runeDisplayWidth(r)
		if cw+rw > max {
			break
		}
		cw += rw
		ri++
	}

	res := string([]rune(s)[:ri])
	if useEllipsis {
		res += "..."
	}
	if cw < max {
		// compensate if truncating a wide character left an odd space
		res += " "
	}
	return res
}

func runeDisplayWidth(r rune) int {
	switch width.LookupRune(r).Kind() {
	case width.EastAsianWide, width.EastAsianAmbiguous, width.EastAsianFullwidth:
		return 2
	default:
		return 1
	}
}
