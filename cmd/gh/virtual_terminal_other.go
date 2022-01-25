//go:build windows
// +build windows

package main

import (
	"github.com/Netflix/go-expect"
	"github.com/cli/cli/v2/internal/conpty"
	"github.com/cli/cli/v2/internal/vt10x"

	"golang.org/x/sys/windows"
)

// newConsole returns a new expect.Console that multiplexes the
// Stdin/Stdout to a VT10X terminal, allowing Console to interact with an
// application sending ANSI escape sequences.
func newConsole(opts ...expect.ConsoleOpt) (*expect.Console, error) {
	pty, err := conpty.Create(80, 24, 0)
	if err != nil {
		return nil, err
	}
	attrList, err := windows.NewProcThreadAttributeList(3)
	if err != nil {
		return nil, err
	}
	err = pty.UpdateProcThreadAttribute(attrList)
	if err != nil {
		return nil, err
	}

	term := vt10x.New(vt10x.WithWriter(pty.OutPipe()))
	c, err := expect.NewConsole(append(opts, expect.WithStdin(pty.InPipe()), expect.WithStdout(term), expect.WithCloser(pty))...)
	if err != nil {
		return nil, err
	}
	return c, nil
}
