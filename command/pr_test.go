package command

import (
	"regexp"
	"testing"

	"github.com/github/gh-cli/test"
)

func TestPRList(t *testing.T) {
	teardown := test.MockGraphQLResponse("fixtures/pr.json")
	defer teardown()

	gitRepo := test.SetupTempGitRepo()
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
