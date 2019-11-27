package utils

import (
	"io"
	"os"

	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
	"github.com/mgutz/ansi"
)

// NewColorable returns an output stream that handles ANSI color sequences on Windows
func NewColorable(f *os.File) io.Writer {
	return colorable.NewColorable(f)
}

func makeColorFunc(color string) func(string) string {
	return func(arg string) string {
		output := arg
		if isatty.IsTerminal(os.Stdout.Fd()) {
			output = ansi.Color(color+arg+ansi.Reset, "")
		}

		return output
	}
}

var Black = makeColorFunc(ansi.Black)
var White = makeColorFunc(ansi.White)
var Magenta = makeColorFunc(ansi.Magenta)
var Cyan = makeColorFunc(ansi.Cyan)
var Red = makeColorFunc(ansi.Red)
var Yellow = makeColorFunc(ansi.Yellow)
var Blue = makeColorFunc(ansi.Blue)
var Green = makeColorFunc(ansi.Green)
var Gray = makeColorFunc(ansi.LightBlack)

func Bold(arg string) string {
	output := arg
	if isatty.IsTerminal(os.Stdout.Fd()) {
		// This is really annoying.  If you just define Bold as ColorFunc("+b") it will properly bold but
		// will not use the default color, resulting in black and probably unreadable text. This forces
		// the default color before bolding.
		output = ansi.Color(ansi.DefaultFG+arg+ansi.Reset, "+b")
	}
	return output
}
