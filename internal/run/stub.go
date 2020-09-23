package run

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

type T interface {
	Helper()
	Errorf(string, ...interface{})
}

func Stub() (*CommandStubber, func(T)) {
	cs := &CommandStubber{}
	teardown := SetPrepareCmd(func(cmd *exec.Cmd) Runnable {
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
		t.Errorf("umatched stubs (%d): %s", len(unmatched), strings.Join(unmatched, ", "))
	}
}

type CommandStubber struct {
	stubs []*commandStub
}

func (cs *CommandStubber) Register(p string, exitStatus int, output string, callbacks ...CommandCallback) {
	cs.stubs = append(cs.stubs, &commandStub{
		pattern:    regexp.MustCompile(p),
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

func (s *commandStub) Run() error {
	if s.exitStatus != 0 {
		return fmt.Errorf("%s exited with status %d", s.pattern, s.exitStatus)
	}
	return nil
}

func (s *commandStub) Output() ([]byte, error) {
	if s.exitStatus != 0 {
		return []byte(nil), fmt.Errorf("%s exited with status %d", s.pattern, s.exitStatus)
	}
	return []byte(s.stdout), nil
}
