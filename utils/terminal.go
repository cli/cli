package utils

import (
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
	"golang.org/x/crypto/ssh/terminal"
)

func isStdoutTerminal() bool {
	if !checkedTerminal {
		_isStdoutTerminal = IsTerminal(os.Stdout)
		checkedTerminal = true
	}
	return _isStdoutTerminal
}

// TODO I don't like this use of interface{} but we need to accept both io.Writer and io.Reader
// interfaces.

var IsTerminal = func(w interface{}) bool {
	if f, isFile := w.(*os.File); isFile {
		return isatty.IsTerminal(f.Fd()) || IsCygwinTerminal(f)
	}

	return false
}

func IsCygwinTerminal(w interface{}) bool {
	if f, isFile := w.(*os.File); isFile {
		return isatty.IsCygwinTerminal(f.Fd())
	}

	return false
}

var TerminalSize = func(w interface{}) (int, int, error) {
	if f, isFile := w.(*os.File); isFile {
		return terminal.GetSize(int(f.Fd()))
	}

	return 0, 0, fmt.Errorf("%v is not a file", w)
}
