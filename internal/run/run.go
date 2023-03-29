package run

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cli/cli/v2/utils"
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

// cmdWithStderr augments exec.Cmd by adding stderr to the error message
type cmdWithStderr struct {
	*exec.Cmd
}

func (c cmdWithStderr) Output() ([]byte, error) {
	if isVerbose, _ := utils.IsDebugEnabled(); isVerbose {
		_ = printArgs(os.Stderr, c.Cmd.Args)
	}
	out, err := c.Cmd.Output()
	if c.Cmd.Stderr != nil || err == nil {
		return out, err
	}
	cmdErr := &CmdError{
		Args: c.Cmd.Args,
		Err:  err,
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		cmdErr.Stderr = bytes.NewBuffer(exitError.Stderr)
	}
	return out, cmdErr
}

func (c cmdWithStderr) Run() error {
	if isVerbose, _ := utils.IsDebugEnabled(); isVerbose {
		_ = printArgs(os.Stderr, c.Cmd.Args)
	}
	if c.Cmd.Stderr != nil {
		return c.Cmd.Run()
	}
	errStream := &bytes.Buffer{}
	c.Cmd.Stderr = errStream
	err := c.Cmd.Run()
	if err != nil {
		err = &CmdError{
			Args:   c.Cmd.Args,
			Err:    err,
			Stderr: errStream,
		}
	}
	return err
}

// CmdError provides more visibility into why an exec.Cmd had failed
type CmdError struct {
	Args   []string
	Err    error
	Stderr *bytes.Buffer
}

func (e CmdError) Error() string {
	msg := e.Stderr.String()
	if msg != "" && !strings.HasSuffix(msg, "\n") {
		msg += "\n"
	}
	return fmt.Sprintf("%s%s: %s", msg, e.Args[0], e.Err)
}

func (e CmdError) Unwrap() error {
	return e.Err
}

func printArgs(w io.Writer, args []string) error {
	if len(args) > 0 {
		// print commands, but omit the full path to an executable
		args = append([]string{filepath.Base(args[0])}, args[1:]...)
	}
	_, err := fmt.Fprintf(w, "%v\n", args)
	return err
}
