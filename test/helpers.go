package test

import (
	"fmt"
	"os/exec"

	"github.com/cli/cli/internal/run"
)

// OutputStub implements a simple utils.Runnable
type OutputStub struct {
	Out []byte
}

func (s OutputStub) Output() ([]byte, error) {
	return s.Out, nil
}

func (s OutputStub) Run() error {
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
	cs.Stubs = append(cs.Stubs, &OutputStub{[]byte(desiredOutput)})
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
