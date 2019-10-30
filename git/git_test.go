package git

import (
	"fmt"
	"github.com/github/gh-cli/test"
	"os"
	"testing"
)

func TestGitStatusHelperProcess(*testing.T) {
	if test.SkipTestHelperProcess() {
		return
	}
	outputs := map[string]test.ExecStub{
		"no changes": test.ExecStub{"", 0},
		"one change": test.ExecStub{` M poem.txt
`, 0},
		"untracked file": test.ExecStub{` M poem.txt
?? new.txt
`, 0},
		"boom": test.ExecStub{"", 1},
	}
	output := test.GetExecStub(outputs)
	defer os.Exit(output.ExitCode)
	fmt.Println(output.Stdout)
}

func Test_UncommittedChangeCount(t *testing.T) {
	origGitCommand := GitCommand
	defer func() {
		GitCommand = origGitCommand
	}()

	cases := map[string]int{
		"no changes":     0,
		"one change":     1,
		"untracked file": 2,
	}

	for k, v := range cases {
		GitCommand = test.StubExecCommand("TestGitStatusHelperProcess", k)
		ucc, _ := UncommittedChangeCount()

		if ucc != v {
			t.Errorf("got unexpected ucc value: %d for case %s", ucc, k)
		}
	}

	GitCommand = test.StubExecCommand("TestGitStatusHelperProcess", "boom")
	_, err := UncommittedChangeCount()
	if err.Error() != "failed to run git status: exit status 1" {
		t.Errorf("got unexpected error message: %s", err)
	}
}
