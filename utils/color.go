package utils

import "github.com/mgutz/ansi"

var Black = ansi.ColorFunc("black")
var White = ansi.ColorFunc("white")
var Gray = ansi.ColorFunc("gray")
var Red = ansi.ColorFunc("red")
var Green = ansi.ColorFunc("green")
var Yellow = ansi.ColorFunc("yellow")
var Blue = ansi.ColorFunc("blue")
var Magenta = ansi.ColorFunc("magenta")
var Cyan = ansi.ColorFunc("cyan")

func Bold(arg string) string {
	// This is really annoying.  If you just define Bold as ColorFunc("+b") it will properly bold but
	// will not use the default color, resulting in black and probably unreadable text. This forces
	// the default color before bolding.
	return ansi.Color(ansi.DefaultFG+arg, "+b")
}
