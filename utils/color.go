package utils

import (
	"io"
	"os"

	"github.com/mattn/go-colorable"
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

// NewColorable returns an output stream that handles ANSI color sequences on Windows
func NewColorable(f *os.File) io.Writer {
	return colorable.NewColorable(f)
}

func makeColorFunc(color string) func(string) string {
	cf := ansi.ColorFunc(color)
	return func(arg string) string {
		if isStdoutTerminal() {
			return cf(arg)
		}
		return arg
	}
}

// Magenta outputs ANSI color if stdout is a tty
var Magenta = makeColorFunc("magenta")

// Cyan outputs ANSI color if stdout is a tty
var Cyan = makeColorFunc("cyan")

// Red outputs ANSI color if stdout is a tty
var Red = makeColorFunc("red")

// Yellow outputs ANSI color if stdout is a tty
var Yellow = makeColorFunc("yellow")

// Blue outputs ANSI color if stdout is a tty
var Blue = makeColorFunc("blue")

// Green outputs ANSI color if stdout is a tty
var Green = makeColorFunc("green")

// Gray outputs ANSI color if stdout is a tty
var Gray = makeColorFunc("black+h")

// Bold outputs ANSI color if stdout is a tty
var Bold = makeColorFunc("default+b")
