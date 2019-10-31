package command

import (
	"os"
	"regexp"
	"testing"

	"github.com/github/gh-cli/test"
)

func TestPRList(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()

	jsonFile, _ := os.Open("../test/fixtures/prList.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	output, err := test.RunCommand(RootCmd, "pr list")
	if err != nil {
		t.Errorf("error running command `pr list`: %v", err)
	}

	expectedPrs := []*regexp.Regexp{
		regexp.MustCompile(`#8.*\[strawberries\]`),
		regexp.MustCompile(`#9.*\[apples\]`),
		regexp.MustCompile(`#10.*\[blueberries\]`),
		regexp.MustCompile(`#11.*\[figs\]`),
	}

	for _, r := range expectedPrs {
		if !r.MatchString(output) {
			t.Errorf("output did not match regexp /%s/", r)
		}
	}
}

func TestPRView(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()

	jsonFile, _ := os.Open("../test/fixtures/prView.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	teardown, callCount := mockOpenInBrowser()
	defer teardown()

	output, err := test.RunCommand(RootCmd, "pr view")
	if err != nil {
		t.Errorf("error running command `pr view`: %v", err)
	}

	if output == "" {
		t.Errorf("command output expected got an empty string")
	}

	if *callCount != 1 {
		t.Errorf("OpenInBrowser should be called 1 time but was called %d time(s)", *callCount)
	}
}

func TestPRView_NoActiveBranch(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()

	jsonFile, _ := os.Open("../test/fixtures/prView_NoActiveBranch.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	teardown, callCount := mockOpenInBrowser()
	defer teardown()

	output, err := test.RunCommand(RootCmd, "pr view")
	if err == nil || err.Error() != "the 'master' branch has no open pull requests" {
		t.Errorf("error running command `pr view`: %v", err)
	}

	if *callCount > 0 {
		t.Errorf("OpenInBrowser should NOT be called but was called %d time(s)", *callCount)
	}

	// Now run again but provide a PR number
	output, err = test.RunCommand(RootCmd, "pr view 23")
	if err != nil {
		t.Errorf("error running command `pr view`: %v", err)
	}

	if output == "" {
		t.Errorf("command output expected got an empty string")
	}

	if *callCount != 1 {
		t.Errorf("OpenInBrowser should be called once but was called %d time(s)", *callCount)
	}
}
