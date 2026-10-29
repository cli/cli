package git

import (
	"bytes"
	"errors"
	"io"
	"os/exec"
	"slices"

	"github.com/cli/cli/v2/internal/run"
)

type Command struct {
	*exec.Cmd
}

func (gc *Command) Run() error {
	stderr := &bytes.Buffer{}
	if gc.Cmd.Stderr == nil {
		gc.Cmd.Stderr = stderr
	}
	// This is a hack in order to not break the hundreds of
	// existing tests that rely on `run.PrepareCmd` to be invoked.
	err := run.PrepareCmd(gc.Cmd).Run()
	if err != nil {
		ge := GitError{err: err, Stderr: stderr.String()}
		if exitError, ok := errors.AsType[*exec.ExitError](err); ok {
			ge.ExitCode = exitError.ExitCode()
		}
		return &ge
	}
	return nil
}

func (gc *Command) Output() ([]byte, error) {
	gc.Stdout = nil
	gc.Stderr = nil
	// This is a hack in order to not break the hundreds of
	// existing tests that rely on `run.PrepareCmd` to be invoked.
	out, err := run.PrepareCmd(gc.Cmd).Output()
	if err != nil {
		ge := GitError{err: err}

		// In real implementation, this should be an exec.ExitError, as below,
		// but the tests use a different type because exec.ExitError are difficult
		// to create. We want to get the exit code and stderr, but stderr
		// is not a method and so tests can't access it.
		// THIS MEANS THAT TESTS WILL NOT CORRECTLY HAVE STDERR SET,
		// but at least tests can get the exit code.
		if exitErrorWithExitCode, ok := errors.AsType[errWithExitCode](err); ok {
			ge.ExitCode = exitErrorWithExitCode.ExitCode()
		}

		if exitError, ok := errors.AsType[*exec.ExitError](err); ok {
			ge.Stderr = string(exitError.Stderr)
		}
		err = &ge
	}
	return out, err
}

func (gc *Command) setRepoDir(repoDir string) {
	if len(gc.Args) >= 3 && gc.Args[1] == "-C" {
		gc.Args[2] = repoDir
		return
	}
	gc.Args = slices.Insert(gc.Args, 1, "-C", repoDir)
}

// Allow individual commands to be modified from the default client options.
type CommandModifier func(*Command)

func WithStderr(stderr io.Writer) CommandModifier {
	return func(gc *Command) {
		gc.Stderr = stderr
	}
}

func WithStdout(stdout io.Writer) CommandModifier {
	return func(gc *Command) {
		gc.Stdout = stdout
	}
}

func WithStdin(stdin io.Reader) CommandModifier {
	return func(gc *Command) {
		gc.Stdin = stdin
	}
}

func WithRepoDir(repoDir string) CommandModifier {
	return func(gc *Command) {
		gc.setRepoDir(repoDir)
	}
}
