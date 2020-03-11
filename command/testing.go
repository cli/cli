package command

import (
	"errors"
	"fmt"
	"os/exec"

	"github.com/cli/cli/api"
	"github.com/cli/cli/context"
	"github.com/cli/cli/test"
	"github.com/cli/cli/utils"
)

// TODO this is split between here and test/helpers.go. I did that because otherwise our test
// package would have to import utils (for utils.Runnable) which felt wrong. while utils_test
// currently doesn't import test helpers, i don't see why that ought to be precluded.
// I'm wondering if this is a case for having a global package just for storing Interfaces.
type CmdStubber struct {
	Stubs []*test.OutputStub
	Count int
	Calls []*exec.Cmd
}

func (cs *CmdStubber) Stub(desiredOutput string) {
	// TODO maybe have some kind of command mapping but going simple for now
	cs.Stubs = append(cs.Stubs, &test.OutputStub{[]byte(desiredOutput)})
}

func createStubbedPrepareCmd(cs *CmdStubber) func(*exec.Cmd) utils.Runnable {
	return func(cmd *exec.Cmd) utils.Runnable {
		cs.Calls = append(cs.Calls, cmd)
		call := cs.Count
		cs.Count += 1
		if call >= len(cs.Stubs) {
			panic(fmt.Sprintf("more execs than stubs. most recent call: %v", cmd))
		}
		return cs.Stubs[call]
	}
}

func initBlankContext(repo, branch string) {
	initContext = func() context.Context {
		ctx := context.NewBlank()
		ctx.SetBaseRepo(repo)
		ctx.SetBranch(branch)
		ctx.SetRemotes(map[string]string{
			"origin": "OWNER/REPO",
		})
		return ctx
	}
}

func initFakeHTTP() *api.FakeHTTP {
	http := &api.FakeHTTP{}
	apiClientForContext = func(context.Context) (*api.Client, error) {
		return api.NewClient(api.ReplaceTripper(http)), nil
	}
	return http
}

type errorStub struct {
	message string
}

func (s errorStub) Output() ([]byte, error) {
	return nil, errors.New(s.message)
}

func (s errorStub) Run() error {
	return errors.New(s.message)
}
