package git

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
)

func StubbedGit(args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestHelperProcess", "--", "git"}
	cs = append(cs, args...)
	env := []string{
		"GO_WANT_HELPER_PROCESS=1",
	}

	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = append(env, os.Environ()...)
	fmt.Printf("%+v", cmd.Env)
	return cmd
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
	cmd, args := args[0], args[1:]
	switch cmd {
	case "git":
		fmt.Println("fart")
		fmt.Println("town")
	}

}

func Test_UncommittedChangeCount(t *testing.T) {
	origGitCommand := GitCommand
	defer func() {
		GitCommand = origGitCommand
	}()

	GitCommand = StubbedGit

	ucc, err := UncommittedChangeCount()
	if err != nil {
		t.Fatalf("failed to run UncommittedChangeCount: %s", err)
	}

	if ucc != 10 {
		t.Errorf("got unexpected ucc value: %d", ucc)
	}

}
