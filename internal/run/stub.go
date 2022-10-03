package run

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type T interface {
	Helper()
	Errorf(string, ...interface{})
}

// Stub installs a catch-all for all external commands invoked from gh. It returns a restore func that, when
// invoked from tests, fails the current test if some stubs that were registered were never matched.
func Stub() (*CommandStubber, func(T)) {
	cs := &CommandStubber{}
	teardown := setPrepareCmd(func(cmd *exec.Cmd) Runnable {
		s := cs.find(cmd.Args)
		if s == nil {
			panic(fmt.Sprintf("no exec stub for `%s`", strings.Join(cmd.Args, " ")))
		}
		for _, c := range s.callbacks {
			c(cmd.Args)
		}
		s.matched = true
		return s
	})

	return cs, func(t T) {
		defer teardown()
		var unmatched []string
		for _, s := range cs.stubs {
			if s.matched {
				continue
			}
			unmatched = append(unmatched, s.pattern.String())
		}
		if len(unmatched) == 0 {
			return
		}
		t.Helper()
		t.Errorf("unmatched stubs (%d): %s", len(unmatched), strings.Join(unmatched, ", "))
	}
}

func setPrepareCmd(fn func(*exec.Cmd) Runnable) func() {
	origPrepare := PrepareCmd
	PrepareCmd = func(cmd *exec.Cmd) Runnable {
		// normalize git executable name for consistency in tests
		if baseName := filepath.Base(cmd.Args[0]); baseName == "git" || baseName == "git.exe" {
			cmd.Args[0] = "git"
		}
		return fn(cmd)
	}
	return func() {
		PrepareCmd = origPrepare
	}
}

// CommandStubber stubs out invocations to external commands.
type CommandStubber struct {
	stubs []*commandStub
}

// Register a stub for an external command. Pattern is a regular expression, output is the standard output
// from a command. Pass callbacks to inspect raw arguments that the command was invoked with.
func (cs *CommandStubber) Register(pattern string, exitStatus int, output string, callbacks ...CommandCallback) {
	if len(pattern) < 1 {
		panic("cannot use empty regexp pattern")
	}
	cs.stubs = append(cs.stubs, &commandStub{
		pattern:    regexp.MustCompile(pattern),
		exitStatus: exitStatus,
		stdout:     output,
		callbacks:  callbacks,
	})
}

func (cs *CommandStubber) find(args []string) *commandStub {
	line := strings.Join(args, " ")
	for _, s := range cs.stubs {
		if !s.matched && s.pattern.MatchString(line) {
			return s
		}
	}
	return nil
}

type CommandCallback func([]string)

type commandStub struct {
	pattern    *regexp.Regexp
	matched    bool
	exitStatus int
	stdout     string
	callbacks  []CommandCallback
}

// Run satisfies Runnable
func (s *commandStub) Run() error {
	if s.exitStatus != 0 {
		return fmt.Errorf("%s exited with status %d", s.pattern, s.exitStatus)
	}
	return nil
}

// Output satisfies Runnable
func (s *commandStub) Output() ([]byte, error) {
	if s.exitStatus != 0 {
		return []byte(nil), fmt.Errorf("%s exited with status %d", s.pattern, s.exitStatus)
	}
	return []byte(s.stdout), nil
}
