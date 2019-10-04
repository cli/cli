package ui

import (
	"fmt"
	"io"
	"os"

	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
)

type UI interface {
	Print(a ...interface{}) (n int, err error)
	Printf(format string, a ...interface{}) (n int, err error)
	Println(a ...interface{}) (n int, err error)
	Errorf(format string, a ...interface{}) (n int, err error)
	Errorln(a ...interface{}) (n int, err error)
}

var (
	Stdout     = colorable.NewColorableStdout()
	Stderr     = colorable.NewColorableStderr()
	Default UI = Console{Stdout: Stdout, Stderr: Stderr}
)

func Print(a ...interface{}) (n int) {
	n, err := Default.Print(a...)
	if err != nil {
		// If something as basic as printing to stdout fails, just panic and exit
		os.Exit(1)
	}
	return
}

func Printf(format string, a ...interface{}) (n int) {
	n, err := Default.Printf(format, a...)
	if err != nil {
		// If something as basic as printing to stdout fails, just panic and exit
		os.Exit(1)
	}
	return
}

func Println(a ...interface{}) (n int) {
	n, err := Default.Println(a...)
	if err != nil {
		// If something as basic as printing to stdout fails, just panic and exit
		os.Exit(1)
	}
	return
}

func Errorf(format string, a ...interface{}) (n int) {
	n, err := Default.Errorf(format, a...)
	if err != nil {
		// If something as basic as printing to stderr fails, just panic and exit
		os.Exit(1)
	}
	return
}

func Errorln(a ...interface{}) (n int) {
	n, err := Default.Errorln(a...)
	if err != nil {
		// If something as basic as printing to stderr fails, just panic and exit
		os.Exit(1)
	}
	return
}

func IsTerminal(f *os.File) bool {
	return isatty.IsTerminal(f.Fd())
}

type Console struct {
	Stdout io.Writer
	Stderr io.Writer
}

func (c Console) Print(a ...interface{}) (n int, err error) {
	return fmt.Fprint(c.Stdout, a...)
}

func (c Console) Printf(format string, a ...interface{}) (n int, err error) {
	return fmt.Fprintf(c.Stdout, format, a...)
}

func (c Console) Println(a ...interface{}) (n int, err error) {
	return fmt.Fprintln(c.Stdout, a...)
}

func (c Console) Errorf(format string, a ...interface{}) (n int, err error) {
	return fmt.Fprintf(c.Stderr, format, a...)
}

func (c Console) Errorln(a ...interface{}) (n int, err error) {
	return fmt.Fprintln(c.Stderr, a...)
}
