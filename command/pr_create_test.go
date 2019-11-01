package command

import (
	//"regexp"
	"testing"

	//"github.com/github/gh-cli/context"
	"github.com/github/gh-cli/test"
	//"github.com/github/gh-cli/utils"
)

func TestPrCreateHelperProcess(*testing.T) {
	if test.SkipTestHelperProcess() {
		return
	}

	statusOutputs := map[string]string{
		"clean": "",
		"dirty": " M git/git.go",
	}

	args := GetTestHelperProcessArgs()
	switch args[1] {
	case "status":
		fmt.Println(statusOutputs(args[0]))
	case "push":
		fmt.Println()
	}

	defer os.Exit(0)
}

func TestReportsUncommittedChanges(t *testing.T) {
	repoIdFix, _ := os.Open("test/fixtures/repoId.json")
	defer repoIdFix.Close()
	createPrFix, _ := os.Open("test/fixtures/createPr.json")
	defer createPrFix.Close()

	http := initFakeHTTP()
	http.StubResponse(200, repoIdFix)
	http.StubResponse(200, createPrFix)

	origGitCommand := git.GitCommand
	defer func() {
		git.GitCommand = origGitCommand
	}()

	git.GitCommand = test.StubExecCommand("TestPrCreateHelperProcess", "dirty")

	// init blank command and just run prCreate? Or try to use RunCommand?

	// output, err := test.RunCommand(RootCmd, "pr create -tfoo -bbar")
	// if err != nil {
	// 	t.Errorf("error running command `pr create`: %v", err)
	// }

	// if len(output) == 0 {
	// 	panic("lol")
	// }
}
