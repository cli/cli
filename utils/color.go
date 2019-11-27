package utils

import (
	"os"

	"github.com/mattn/go-isatty"
	"github.com/mgutz/ansi"
)

var _isStdoutTerminal = false
var checkedTerminal = false

func isStdoutTerminal() bool {
	if !checkedTerminal {
		fd := os.Stdout.Fd()
		_isStdoutTerminal = isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
		checkedTerminal = true
	}
	return _isStdoutTerminal
}

func makeColorFunc(color string) func(string) string {
	return func(arg string) string {
		output := arg
		if isStdoutTerminal() {
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
	if isStdoutTerminal() {
		// This is really annoying.  If you just define Bold as ColorFunc("+b") it will properly bold but
		// will not use the default color, resulting in black and probably unreadable text. This forces
		// the default color before bolding.
		output = ansi.Color(ansi.DefaultFG+arg+ansi.Reset, "+b")
	}
	return output
}
