package test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/github/gh-cli/api"
	"github.com/github/gh-cli/github"
	"github.com/spf13/cobra"
)

type TempGitRepo struct {
	Remote   string
	TearDown func()
}

func UseTempGitRepo() *TempGitRepo {
	github.CreateTestConfigs("mario", "i-love-peach")

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

func MockGraphQLResponse(fixtureName string) (teardown func()) {
	pwd, _ := os.Getwd()
	fixturePath := filepath.Join(pwd, "..", "test", "fixtures", fixtureName)

	api.OverriddenQueryFunction = func(query string, variables map[string]string, v interface{}) error {
		contents, err := ioutil.ReadFile(fixturePath)
		if err != nil {
			return err
		}

		json.Unmarshal(contents, &v)
		if err != nil {
			return err
		}

		return nil
	}

	return func() {
		api.OverriddenQueryFunction = nil
	}
}

func RunCommand(root *cobra.Command, s string) (string, error) {
	var err error
	output := captureOutput(func() {
		root.SetArgs(strings.Split(s, " "))
		_, err = root.ExecuteC()
	})
	if err != nil {
		return "", err
	}
	return output, nil
}

func captureOutput(f func()) string {
	originalStdout := os.Stdout
	defer func() {
		os.Stdout = originalStdout
	}()

	r, w, err := os.Pipe()
	if err != nil {
		panic("failed to pipe stdout")
	}
	os.Stdout = w

	f()

	w.Close()
	out, err := ioutil.ReadAll(r)
	if err != nil {
		panic("failed to read captured input from stdout")
	}

	return string(out)
}
