package utils

import (
	"fmt"
	"io"
	"os"

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
			return cf(arg)
		}
		return arg
	}
}

func isColorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}

	// TODO ignores cmd.OutOrStdout
	return IsTerminal(os.Stdout)
}

func RGB(r, g, b int, x string) string {
	return fmt.Sprintf("\033[38;2;%d;%d;%dm%s\033[0m", r, g, b, x)
}
