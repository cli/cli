package utils

import (
	"io"
	"os"

	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
	"github.com/mgutz/ansi"
)

var (
	_isColorEnabled                                    bool = true
	_isStdoutTerminal, checkedTerminal, checkedNoColor bool

	// Outputs ANSI color if stdout is a tty
	Magenta = makeColorFunc("magenta")
	Cyan    = makeColorFunc("cyan")
	Red     = makeColorFunc("red")
	Yellow  = makeColorFunc("yellow")
	Blue    = makeColorFunc("blue")
	Green   = makeColorFunc("green")
	Gray    = makeColorFunc("black+h")
	Bold    = makeColorFunc("default+b")
)

func isStdoutTerminal() bool {
	if !checkedTerminal {
		_isStdoutTerminal = IsTerminal(os.Stdout)
		checkedTerminal = true
	}
	return _isStdoutTerminal
}

// IsTerminal reports whether the file descriptor is connected to a terminal
func IsTerminal(f *os.File) bool {
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

// NewColorable returns an output stream that handles ANSI color sequences on Windows
func NewColorable(f *os.File) io.Writer {
	return colorable.NewColorable(f)
}

func makeColorFunc(color string) func(string) string {
	cf := ansi.ColorFunc(color)
	return func(arg string) string {
		if isColorEnabled() && isStdoutTerminal() {
			return cf(arg)
		}
		return arg
	}
}

func isColorEnabled() bool {
	if !isStdoutTerminal() {
		return false
	}

	if !checkedNoColor {
		_isColorEnabled = os.Getenv("NO_COLOR") == ""
		checkedNoColor = true
	}
	return _isColorEnabled
}
