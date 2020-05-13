package test

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"testing"

	"github.com/cli/cli/internal/run"
)

// OutputStub implements a simple utils.Runnable
type OutputStub struct {
	Out   []byte
	Error error
}

func (s OutputStub) Output() ([]byte, error) {
	if s.Error != nil {
		return s.Out, s.Error
	}
	return s.Out, nil
}

func (s OutputStub) Run() error {
	if s.Error != nil {
		return s.Error
	}
	return nil
}

type CmdStubber struct {
	Stubs []*OutputStub
	Count int
	Calls []*exec.Cmd
}

func InitCmdStubber() (*CmdStubber, func()) {
	cs := CmdStubber{}
	teardown := run.SetPrepareCmd(createStubbedPrepareCmd(&cs))
	return &cs, teardown
}

func (cs *CmdStubber) Stub(desiredOutput string) {
	// TODO maybe have some kind of command mapping but going simple for now
	cs.Stubs = append(cs.Stubs, &OutputStub{[]byte(desiredOutput), nil})
}

func (cs *CmdStubber) StubError(errText string) {
	// TODO support error types beyond CmdError
	stderrBuff := bytes.NewBufferString(errText)
	args := []string{"stub"} // TODO make more real?
	err := errors.New(errText)
	cs.Stubs = append(cs.Stubs, &OutputStub{Error: &run.CmdError{
		Stderr: stderrBuff,
		Args:   args,
		Err:    err,
	}})
}

func createStubbedPrepareCmd(cs *CmdStubber) func(*exec.Cmd) run.Runnable {
	return func(cmd *exec.Cmd) run.Runnable {
		cs.Calls = append(cs.Calls, cmd)
		call := cs.Count
		cs.Count += 1
		if call >= len(cs.Stubs) {
			panic(fmt.Sprintf("more execs than stubs. most recent call: %v", cmd))
		}
		return cs.Stubs[call]
	}
}

func ExpectLines(t *testing.T, output string, lines ...string) {
	var r *regexp.Regexp
	for _, l := range lines {
		r = regexp.MustCompile(l)
		if !r.MatchString(output) {
			t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
			return
		}
	}
}
