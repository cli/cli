package git

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
)

var _outputs map[string]string

func init() {
	_outputs = map[string]string{
		"no changes": "",
		"one change": ` M poem.txt
`,
		"untracked file": ` M poem.txt
?? new.txt
`,
	}
}

func TestHelperProcess(*testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)
	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}
	fmt.Println(_outputs[args[0]])
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

	// TODO handle git status exiting poorly

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
}
