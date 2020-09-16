package utils

import (
	"fmt"
	"io"
	"os"

	"github.com/cli/cli/pkg/iostreams"
	"github.com/mattn/go-colorable"
	"github.com/mgutz/ansi"
)

var (
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

// NewColorable returns an output stream that handles ANSI color sequences on Windows
func NewColorable(w io.Writer) io.Writer {
	if f, isFile := w.(*os.File); isFile {
		return colorable.NewColorable(f)
	}
	return w
}

func makeColorFunc(color string) func(string) string {
	cf := ansi.ColorFunc(color)
	return func(arg string) string {
		if isColorEnabled() {
			if color == "black+h" && iostreams.Is256ColorSupported() {
				return fmt.Sprintf("\x1b[%d;5;%dm%s\x1b[m", 38, 242, arg)
			}
			return cf(arg)
		}
		return arg
	}
}

func isColorEnabled() bool {
	if iostreams.EnvColorForced() {
		return true
	}

	if iostreams.EnvColorDisabled() {
		return false
	}

	// TODO ignores cmd.OutOrStdout
	return IsTerminal(os.Stdout)
}
