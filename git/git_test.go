package git

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
)

var _cases map[string]string

func init() {
	_cases = map[string]string{
		"foobar": `fart
		bar
		town`,
	}
}

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
	c, args := args[0], args[1:]
	fmt.Println(_cases[c])
}

func StubGit(c string) func(...string) *exec.Cmd {
	return func(args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--", c}
		cs = append(cs, args...)
		env := []string{
			"GO_WANT_HELPER_PROCESS=1",
		}

		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(env, os.Environ()...)
		fmt.Printf("%+v", cmd.Env)
		return cmd
	}

}

func Test_UncommittedChangeCount(t *testing.T) {
	origGitCommand := GitCommand
	defer func() {
		GitCommand = origGitCommand
	}()

	GitCommand = StubGit("foobar")

	ucc, err := UncommittedChangeCount()
	if err != nil {
		t.Fatalf("failed to run UncommittedChangeCount: %s", err)
	}

	if ucc != 10 {
		t.Errorf("got unexpected ucc value: %d", ucc)
	}

}
