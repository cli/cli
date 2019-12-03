package test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
)

func GetTestHelperProcessArgs() []string {
	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}
	return args
}

func SkipTestHelperProcess() bool {
	return os.Getenv("GO_WANT_HELPER_PROCESS") != "1"
}

func StubExecCommand(testHelper string, desiredOutput string) func(...string) *exec.Cmd {
	return func(args ...string) *exec.Cmd {
		cs := []string{
			fmt.Sprintf("-test.run=%s", testHelper),
			"--", desiredOutput}
		cs = append(cs, args...)
		env := []string{
			"GO_WANT_HELPER_PROCESS=1",
		}

		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(env, os.Environ()...)
		return cmd
	}
}

type TempGitRepo struct {
	Remote   string
	TearDown func()
}

func UseTempGitRepo() *TempGitRepo {
	pwd, _ := os.Getwd()
	oldEnv := make(map[string]string)
	overrideEnv := func(name, value string) {
		oldEnv[name] = os.Getenv(name)
		os.Setenv(name, value)
	}

	remotePath := filepath.Join(pwd, "..", "test", "fixtures", "test.git")
	home, err := ioutil.TempDir("", "test-repo")
	if err != nil {
		panic(err)
	}

	overrideEnv("HOME", home)
	overrideEnv("XDG_CONFIG_HOME", "")
	overrideEnv("XDG_CONFIG_DIRS", "")

	targetPath := filepath.Join(home, "test.git")
	cmd := exec.Command("git", "clone", remotePath, targetPath)
	if output, err := cmd.Output(); err != nil {
		panic(fmt.Errorf("error running %s\n%s\n%s", cmd, err, output))
	}

	if err = os.Chdir(targetPath); err != nil {
		panic(err)
	}

	// Our libs expect the origin to be a github url
	cmd = exec.Command("git", "remote", "set-url", "origin", "https://github.com/github/FAKE-GITHUB-REPO-NAME")
	if output, err := cmd.Output(); err != nil {
		panic(fmt.Errorf("error running %s\n%s\n%s", cmd, err, output))
	}

	tearDown := func() {
		if err := os.Chdir(pwd); err != nil {
			panic(err)
		}
		for name, value := range oldEnv {
			os.Setenv(name, value)
		}
		if err = os.RemoveAll(home); err != nil {
			panic(err)
		}
	}

	return &TempGitRepo{Remote: remotePath, TearDown: tearDown}
}
