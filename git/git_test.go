package git

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/github/gh-cli/test"
)

func TestGitStatusHelperProcess(*testing.T) {
	if test.SkipTestHelperProcess() {
		return
	}

	args := test.GetTestHelperProcessArgs()
	switch args[0] {
	case "no changes":
	case "one change":
		fmt.Println(" M poem.txt")
	case "untracked file":
		fmt.Println(" M poem.txt")
		fmt.Println("?? new.txt")
	case "boom":
		os.Exit(1)
	}
	os.Exit(0)
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
	if !strings.HasSuffix(err.Error(), "git.test: exit status 1") {
		t.Errorf("got unexpected error message: %s", err)
	}
}
