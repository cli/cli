package utils

import (
	"os"

	"github.com/mattn/go-isatty"
	"github.com/mgutz/ansi"
)

func makeColorFunc(color string) func(string) string {
	cf := ansi.ColorFunc(color)
	return func(arg string) string {
		if isatty.IsTerminal(os.Stdout.Fd()) {
			return cf(arg)
		}
		return arg
	}
}

var Magenta = makeColorFunc("magenta")
var Cyan = makeColorFunc("cyan")
var Red = makeColorFunc("red")
var Yellow = makeColorFunc("yellow")
var Blue = makeColorFunc("blue")
var Green = makeColorFunc("green")
var Gray = makeColorFunc("black+h")
var Bold = makeColorFunc("default+b")
