package command

import (
	"regexp"
	"testing"

	"github.com/github/gh-cli/context"
	"github.com/github/gh-cli/test"
	"github.com/github/gh-cli/utils"
)

func TestPRList(t *testing.T) {
	ctx := context.InitBlankContext()
	ctx.SetBaseRepo("github/FAKE-GITHUB-REPO-NAME")
	ctx.SetBranch("master")

	teardown := test.MockGraphQLResponse("test/fixtures/prList.json")
	defer teardown()

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
	teardown := test.MockGraphQLResponse("test/fixtures/prView_NoActiveBranch.json")
	defer teardown()

	gitRepo := test.UseTempGitRepo()
	defer gitRepo.TearDown()

	teardown, callCount := mockOpenInBrowser()
	defer teardown()

	output, err := test.RunCommand(RootCmd, "pr view")
	if err != nil {
		t.Errorf("error running command `pr view`: %v", err)
	}

	if output == "" {
		t.Errorf("command output expected got an empty string")
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

func mockOpenInBrowser() (func(), *int) {
	callCount := 0
	originalOpenInBrowser := utils.OpenInBrowser
	teardown := func() {
		utils.OpenInBrowser = originalOpenInBrowser
	}

	utils.OpenInBrowser = func(_ string) error {
		callCount++
		return nil
	}

	return teardown, &callCount
}
