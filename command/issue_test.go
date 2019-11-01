package command

import (
	"os"
	"regexp"
	"testing"

	"github.com/github/gh-cli/test"
)

func TestIssueList(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()

	jsonFile, _ := os.Open("../test/fixtures/issueList.json")
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

	teardown, callCount := mockOpenInBrowser()
	defer teardown()

	output, err := test.RunCommand(RootCmd, "issue view 8")
	if err != nil {
		t.Errorf("error running command `issue view`: %v", err)
	}

	if output == "" {
		t.Errorf("command output expected got an empty string")
	}

	if *callCount != 1 {
		t.Errorf("OpenInBrowser should be called 1 time but was called %d time(s)", *callCount)
	}
}
