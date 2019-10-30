package git

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
)

type outputSpec struct {
	Stdout   string
	ExitCode int
}

var _outputs map[string]outputSpec

func init() {
	_outputs = map[string]outputSpec{
		"no changes": outputSpec{"", 0},
		"one change": outputSpec{` M poem.txt
`, 0},
		"untracked file": outputSpec{` M poem.txt
?? new.txt
`, 0},
		"boom": outputSpec{"", 1},
	}
}

func TestHelperProcess(*testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}
	output := _outputs[args[0]]
	defer os.Exit(output.ExitCode)
	fmt.Println(output.Stdout)
}

func StubGit(desiredOutput string) func(...string) *exec.Cmd {
	return func(args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--", desiredOutput}
		cs = append(cs, args...)
		env := []string{
			"GO_WANT_HELPER_PROCESS=1",
		}

		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(env, os.Environ()...)
		return cmd
	}

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
		GitCommand = StubGit(k)
		ucc, _ := UncommittedChangeCount()

		if ucc != v {
			t.Errorf("got unexpected ucc value: %d for case %s", ucc, k)
		}
	}

	GitCommand = StubGit("boom")
	_, err := UncommittedChangeCount()
	if err.Error() != "failed to run git status: exit status 1" {
		t.Errorf("got unexpected error message: %s", err)
	}
}
