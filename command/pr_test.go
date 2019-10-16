package command

import (
	"regexp"
	"testing"

	"github.com/github/gh-cli/utils"

	"github.com/github/gh-cli/test"
)

func FTestPRList(t *testing.T) {
	teardown := test.MockGraphQLResponse("test/fixtures/pr.json")
	defer teardown()

	gitRepo := test.UseTempGitRepo()
	defer gitRepo.TearDown()

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
	teardown := test.MockGraphQLResponse("test/fixtures/prView.json")
	defer teardown()

	gitRepo := test.UseTempGitRepo()
	defer gitRepo.TearDown()

	openInBrowserCalls := 0
	utils.OpenInBrowser = func(_ string) error {
		openInBrowserCalls++
		return nil
	}

	output, err := test.RunCommand(RootCmd, "pr view")
	if err != nil {
		t.Errorf("error running command `pr view`: %v", err)
	}

	if output == "" {
		t.Errorf("command output expected got an empty string")
	}

	if openInBrowserCalls != 1 {
		t.Errorf("OpenInBrowser should be called 1 time but was called %d time(s)", openInBrowserCalls)
	}
}
