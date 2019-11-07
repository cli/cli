package command

import (
	"os"
	"os/exec"
	"regexp"
	"testing"

	"github.com/github/gh-cli/test"
	"github.com/github/gh-cli/utils"
)

func TestIssueStatus(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()

	jsonFile, _ := os.Open("../test/fixtures/issueStatus.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	output, err := test.RunCommand(RootCmd, "issue status")
	if err != nil {
		t.Errorf("error running command `issue status`: %v", err)
	}

	expectedIssues := []*regexp.Regexp{
		regexp.MustCompile(`#8.*carrots`),
		regexp.MustCompile(`#9.*squash`),
		regexp.MustCompile(`#10.*broccoli`),
		regexp.MustCompile(`#11.*swiss chard`),
	}

	for _, r := range expectedIssues {
		if !r.MatchString(output) {
			t.Errorf("output did not match regexp /%s/", r)
		}
	}
}

func TestIssueView(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()

	jsonFile, _ := os.Open("../test/fixtures/issueView.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	var seenCmd *exec.Cmd
	restoreCmd := utils.SetPrepareCmd(func(cmd *exec.Cmd) utils.Runnable {
		seenCmd = cmd
		return &outputStub{}
	})
	defer restoreCmd()

	output, err := test.RunCommand(RootCmd, "issue view 8")
	if err != nil {
		t.Errorf("error running command `issue view`: %v", err)
	}

	if output == "" {
		t.Errorf("command output expected got an empty string")
	}

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	if url != "https://github.com/OWNER/REPO/issues/8" {
		t.Errorf("got: %q", url)
	}
}
