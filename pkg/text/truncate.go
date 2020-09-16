package text

import (
	runewidth "github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"
)

const (
	ellipsisWidth       = 3
	minWidthForEllipsis = 5
)

// DisplayWidth calculates what the rendered width of a string may be
func DisplayWidth(s string) int {
	g := uniseg.NewGraphemes(s)
	w := 0
	for g.Next() {
		w += graphemeWidth(g)
	}
	return w
}

// Truncate shortens a string to fit the maximum display width
func Truncate(maxWidth int, s string) string {
	w := DisplayWidth(s)
	if w <= maxWidth {
		return s
	}

	useEllipsis := false
	if maxWidth >= minWidthForEllipsis {
		useEllipsis = true
		maxWidth -= ellipsisWidth
	}

	g := uniseg.NewGraphemes(s)
	r := ""
	rWidth := 0
	for {
		g.Next()
		gWidth := graphemeWidth(g)

		if rWidth+gWidth <= maxWidth {
			r += g.Str()
			rWidth += gWidth
			continue
		} else {
			break
		}
	}

	if useEllipsis {
		r += "..."
	}

	if rWidth < maxWidth {
		r += " "
	}

	return r
}

// graphemeWidth calculates what the rendered width of a grapheme may be
func graphemeWidth(g *uniseg.Graphemes) int {
	// If grapheme spans one rune use that rune width
	// If grapheme spans multiple runes use the first non-zero rune width
	w := 0
	for _, r := range g.Runes() {
		w = runewidth.RuneWidth(r)
		if w > 0 {
			break
		}
	}
	return w
}
