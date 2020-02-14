package utils

import (
	"io"
	"os"
	"fmt"

	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
	"github.com/mgutz/ansi"
	"github.com/spf13/cobra"
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

func makeColorFunc(color string, forceColor bool) func(string) string {
	cf := ansi.ColorFunc(color)
	return func(arg string) string {
		if isStdoutTerminal() || forceColor {
			return cf(arg)
		}
		return arg
	}
}

type Palette struct {
	Magenta func(string) string
	Cyan func(string) string
	Red func(string) string
	Yellow func(string) string
	Blue func(string) string
	Green func(string) string
	Gray func(string) string
	Bold func(string) string
}

func NewPalette(cmd *cobra.Command) (*Palette, error) {
	forceColor, err := cmd.Flags().GetBool("force-color")
	if err != nil {
		return nil, fmt.Errorf("could not parse force-color: %w", err)
	}

	return &Palette{
		Magenta: makeColorFunc("magenta", forceColor),
		Cyan: makeColorFunc("cyan", forceColor),
		Red: makeColorFunc("red", forceColor),
		Yellow: makeColorFunc("yellow", forceColor),
		Blue: makeColorFunc("blue", forceColor),
		Green: makeColorFunc("green", forceColor),
		Gray: makeColorFunc("black+h", forceColor),
		Bold: makeColorFunc("default+b", forceColor),
	}, nil
}
