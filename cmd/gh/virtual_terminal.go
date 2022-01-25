//go:build !windows
// +build !windows

package main

import (
	"github.com/Netflix/go-expect"
	"github.com/cli/cli/v2/internal/vt10x"
	"github.com/creack/pty"
)

// newConsole returns a new expect.Console that multiplexes the
// Stdin/Stdout to a VT10X terminal, allowing Console to interact with an
// application sending ANSI escape sequences.
func newConsole(opts ...expect.ConsoleOpt) (*expect.Console, error) {
	ptm, pts, err := pty.Open()
	if err != nil {
		return nil, err
	}
	term := vt10x.New(vt10x.WithWriter(pts))
	c, err := expect.NewConsole(append(opts, expect.WithStdin(ptm), expect.WithStdout(term), expect.WithCloser(pts, ptm))...)
	if err != nil {
		return nil, err
	}
	return c, nil
}
