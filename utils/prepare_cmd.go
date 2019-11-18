package utils

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Runnable is typically an exec.Cmd or its stub in tests
type Runnable interface {
	Output() ([]byte, error)
	Run() error
}

// PrepareCmd extends exec.Cmd with extra error reporting features and provides a
// hook to stub command execution in tests
var PrepareCmd = func(cmd *exec.Cmd) Runnable {
	return &cmdWithStderr{cmd}
}

// SetPrepareCmd overrides PrepareCmd and returns a func to revert it back
func SetPrepareCmd(fn func(*exec.Cmd) Runnable) func() {
	origPrepare := PrepareCmd
	PrepareCmd = fn
	return func() {
		PrepareCmd = origPrepare
	}
}

// cmdWithStderr augments exec.Cmd by adding stderr to the error message
type cmdWithStderr struct {
	*exec.Cmd
}

func (c cmdWithStderr) Output() ([]byte, error) {
	if os.Getenv("DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "%v\n", c.Cmd.Args)
	}
	if c.Cmd.Stderr != nil {
		return c.Cmd.Output()
	}
	errStream := &bytes.Buffer{}
	c.Cmd.Stderr = errStream
	out, err := c.Cmd.Output()
	if err != nil {
		err = &CmdError{errStream, c.Cmd.Args, err}
	}
	return out, err
}

func (c cmdWithStderr) Run() error {
	if os.Getenv("DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "%v\n", c.Cmd.Args)
	}
	if c.Cmd.Stderr != nil {
		return c.Cmd.Run()
	}
	errStream := &bytes.Buffer{}
	c.Cmd.Stderr = errStream
	err := c.Cmd.Run()
	if err != nil {
		err = &CmdError{errStream, c.Cmd.Args, err}
	}
	return err
}

// CmdError provides more visibility into why an exec.Cmd had failed
type CmdError struct {
	Stderr *bytes.Buffer
	Args   []string
	Err    error
}

func (e CmdError) Error() string {
	msg := e.Stderr.String()
	if msg != "" && !strings.HasSuffix(msg, "\n") {
		msg += "\n"
	}
	return fmt.Sprintf("%s%s: %s", msg, e.Args[0], e.Err)
}
