package ansi

import (
	"fmt"
	"sort"

	colorable "github.com/mattn/go-colorable"
)

// PrintStyles prints all style combinations to the terminal.
func PrintStyles() {
	// for compatibility with Windows, not needed for *nix
	stdout := colorable.NewColorableStdout()

	bgColors := []string{
		"",
		":black",
		":red",
		":green",
		":yellow",
		":blue",
		":magenta",
		":cyan",
		":white",
	}

	keys := make([]string, 0, len(Colors))
	for k := range Colors {
		keys = append(keys, k)
	}

	sort.Sort(sort.StringSlice(keys))

	for _, fg := range keys {
		for _, bg := range bgColors {
			fmt.Fprintln(stdout, padColor(fg, []string{"" + bg, "+b" + bg, "+bh" + bg, "+u" + bg}))
			fmt.Fprintln(stdout, padColor(fg, []string{"+s" + bg, "+i" + bg}))
			fmt.Fprintln(stdout, padColor(fg, []string{"+uh" + bg, "+B" + bg, "+Bb" + bg /* backgrounds */, "" + bg + "+h"}))
			fmt.Fprintln(stdout, padColor(fg, []string{"+b" + bg + "+h", "+bh" + bg + "+h", "+u" + bg + "+h", "+uh" + bg + "+h"}))
		}
	}
}

func pad(s string, length int) string {
	for len(s) < length {
		s += " "
	}
	return s
}

func padColor(color string, styles []string) string {
	buffer := ""
	for _, style := range styles {
		buffer += Color(pad(color+style, 20), color+style)
	}
	return buffer
}
