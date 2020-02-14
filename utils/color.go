package utils

import (
	"fmt"
	"io"
	"os"

	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
	"github.com/mgutz/ansi"
	"github.com/spf13/cobra"
)

var _isStdoutTerminal = false
var checkedTerminal = false

type ColorStrategy int

const (
	NeverColor ColorStrategy = iota + 1
	AlwaysColor
	AutoColor
)

func parseColorStrategy(strategy string) (ColorStrategy, error) {
	switch strategy {
	case "never":
		return NeverColor, nil
	case "always":
		return AlwaysColor, nil
	case "auto":
		return AutoColor, nil
	}
	return AutoColor, fmt.Errorf("Couldnt parse color strategy \"%s\"", strategy)
}

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

func makeColorFunc(color string, colorStrategy ColorStrategy) func(string) string {
	cf := ansi.ColorFunc(color)
	return func(arg string) string {
		switch colorStrategy {
		case AlwaysColor:
			return cf(arg)
		case NeverColor:
			return arg
		case AutoColor:
			if isStdoutTerminal() {
				return cf(arg)
			} else {
				return arg
			}
		default: // Unreachable
			return arg
		}
	}
}

type Palette struct {
	Magenta func(string) string
	Cyan    func(string) string
	Red     func(string) string
	Yellow  func(string) string
	Blue    func(string) string
	Green   func(string) string
	Gray    func(string) string
	Bold    func(string) string
}

func NewPalette(cmd *cobra.Command) (*Palette, error) {
	noColorFlag, err := cmd.Flags().GetBool("no-color")
	if err != nil {
		return nil, fmt.Errorf("could not parse no-color: %w", err)
	}

	colorStrategyFlag, _ := cmd.Flags().GetString("color")
	colorStrategy, err := parseColorStrategy(colorStrategyFlag)
	if err != nil {
		return nil, err
	}

	if noColorFlag {
		colorStrategy = NeverColor
	}

	return &Palette{
		Magenta: makeColorFunc("magenta", colorStrategy),
		Cyan:    makeColorFunc("cyan", colorStrategy),
		Red:     makeColorFunc("red", colorStrategy),
		Yellow:  makeColorFunc("yellow", colorStrategy),
		Blue:    makeColorFunc("blue", colorStrategy),
		Green:   makeColorFunc("green", colorStrategy),
		Gray:    makeColorFunc("black+h", colorStrategy),
		Bold:    makeColorFunc("default+b", colorStrategy),
	}, nil
}
