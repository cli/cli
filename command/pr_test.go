package command

import (
	"bytes"
	"os/exec"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/cli/cli/internal/run"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/prompt"
	"github.com/cli/cli/test"
	"github.com/stretchr/testify/assert"
)

func eq(t *testing.T, got interface{}, expected interface{}) {
	t.Helper()
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("expected: %v, got: %v", expected, got)
	}
}

func TestPRStatus(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "blueberries")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.Register(httpmock.GraphQL(`query PullRequestStatus\b`), httpmock.FileResponse("../test/fixtures/prStatus.json"))

	output, err := RunCommand("pr status")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expectedPrs := []*regexp.Regexp{
		regexp.MustCompile(`#8.*\[strawberries\]`),
		regexp.MustCompile(`#9.*\[apples\]`),
		regexp.MustCompile(`#10.*\[blueberries\]`),
		regexp.MustCompile(`#11.*\[figs\]`),
	}

	for _, r := range expectedPrs {
		if !r.MatchString(output.String()) {
			t.Errorf("output did not match regexp /%s/", r)
		}
	}
}

func TestPRStatus_fork(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "blueberries")
	http := initFakeHTTP()
	http.StubForkedRepoResponse("OWNER/REPO", "PARENT/REPO")
	http.Register(httpmock.GraphQL(`query PullRequestStatus\b`), httpmock.FileResponse("../test/fixtures/prStatusFork.json"))

	defer run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		switch strings.Join(cmd.Args, " ") {
		case `git config --get-regexp ^branch\.blueberries\.(remote|merge)$`:
			return &test.OutputStub{Out: []byte(`branch.blueberries.remote origin
branch.blueberries.merge refs/heads/blueberries`)}
		default:
			panic("not implemented")
		}
	})()

	output, err := RunCommand("pr status")
	if err != nil {
		t.Fatalf("error running command `pr status`: %v", err)
	}

	branchRE := regexp.MustCompile(`#10.*\[OWNER:blueberries\]`)
	if !branchRE.MatchString(output.String()) {
		t.Errorf("did not match current branch:\n%v", output.String())
	}
}

func TestPRStatus_reviewsAndChecks(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "blueberries")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.Register(httpmock.GraphQL(`query PullRequestStatus\b`), httpmock.FileResponse("../test/fixtures/prStatusChecks.json"))

	output, err := RunCommand("pr status")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expected := []string{
		"✓ Checks passing + Changes requested",
		"- Checks pending ✓ Approved",
		"× 1/3 checks failing - Review required",
	}

	for _, line := range expected {
		if !strings.Contains(output.String(), line) {
			t.Errorf("output did not contain %q: %q", line, output.String())
		}
	}
}

func TestPRStatus_currentBranch_showTheMostRecentPR(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "blueberries")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.Register(httpmock.GraphQL(`query PullRequestStatus\b`), httpmock.FileResponse("../test/fixtures/prStatusCurrentBranch.json"))

	output, err := RunCommand("pr status")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expectedLine := regexp.MustCompile(`#10  Blueberries are certainly a good fruit \[blueberries\]`)
	if !expectedLine.MatchString(output.String()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", expectedLine, output)
		return
	}

	unexpectedLines := []*regexp.Regexp{
		regexp.MustCompile(`#9  Blueberries are a good fruit \[blueberries\] - Merged`),
		regexp.MustCompile(`#8  Blueberries are probably a good fruit \[blueberries\] - Closed`),
	}
	for _, r := range unexpectedLines {
		if r.MatchString(output.String()) {
			t.Errorf("output unexpectedly match regexp /%s/\n> output\n%s\n", r, output)
			return
		}
	}
}

func TestPRStatus_currentBranch_defaultBranch(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "blueberries")
	http := initFakeHTTP()
	http.StubRepoResponseWithDefaultBranch("OWNER", "REPO", "blueberries")
	http.Register(httpmock.GraphQL(`query PullRequestStatus\b`), httpmock.FileResponse("../test/fixtures/prStatusCurrentBranch.json"))

	output, err := RunCommand("pr status")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expectedLine := regexp.MustCompile(`#10  Blueberries are certainly a good fruit \[blueberries\]`)
	if !expectedLine.MatchString(output.String()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", expectedLine, output)
		return
	}
}

func TestPRStatus_currentBranch_defaultBranch_repoFlag(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "blueberries")
	http := initFakeHTTP()
	http.Register(httpmock.GraphQL(`query PullRequestStatus\b`), httpmock.FileResponse("../test/fixtures/prStatusCurrentBranchClosedOnDefaultBranch.json"))

	output, err := RunCommand("pr status -R OWNER/REPO")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expectedLine := regexp.MustCompile(`#8  Blueberries are a good fruit \[blueberries\]`)
	if expectedLine.MatchString(output.String()) {
		t.Errorf("output not expected to match regexp /%s/\n> output\n%s\n", expectedLine, output)
		return
	}
}

func TestPRStatus_currentBranch_Closed(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "blueberries")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.Register(httpmock.GraphQL(`query PullRequestStatus\b`), httpmock.FileResponse("../test/fixtures/prStatusCurrentBranchClosed.json"))

	output, err := RunCommand("pr status")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expectedLine := regexp.MustCompile(`#8  Blueberries are a good fruit \[blueberries\] - Closed`)
	if !expectedLine.MatchString(output.String()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", expectedLine, output)
		return
	}
}

func TestPRStatus_currentBranch_Closed_defaultBranch(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "blueberries")
	http := initFakeHTTP()
	http.StubRepoResponseWithDefaultBranch("OWNER", "REPO", "blueberries")
	http.Register(httpmock.GraphQL(`query PullRequestStatus\b`), httpmock.FileResponse("../test/fixtures/prStatusCurrentBranchClosedOnDefaultBranch.json"))

	output, err := RunCommand("pr status")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expectedLine := regexp.MustCompile(`There is no pull request associated with \[blueberries\]`)
	if !expectedLine.MatchString(output.String()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", expectedLine, output)
		return
	}
}

func TestPRStatus_currentBranch_Merged(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "blueberries")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.Register(httpmock.GraphQL(`query PullRequestStatus\b`), httpmock.FileResponse("../test/fixtures/prStatusCurrentBranchMerged.json"))

	output, err := RunCommand("pr status")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expectedLine := regexp.MustCompile(`#8  Blueberries are a good fruit \[blueberries\] - Merged`)
	if !expectedLine.MatchString(output.String()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", expectedLine, output)
		return
	}
}

func TestPRStatus_currentBranch_Merged_defaultBranch(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "blueberries")
	http := initFakeHTTP()
	http.StubRepoResponseWithDefaultBranch("OWNER", "REPO", "blueberries")
	http.Register(httpmock.GraphQL(`query PullRequestStatus\b`), httpmock.FileResponse("../test/fixtures/prStatusCurrentBranchMergedOnDefaultBranch.json"))

	output, err := RunCommand("pr status")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expectedLine := regexp.MustCompile(`There is no pull request associated with \[blueberries\]`)
	if !expectedLine.MatchString(output.String()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", expectedLine, output)
		return
	}
}

func TestPRStatus_blankSlate(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "blueberries")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.Register(httpmock.GraphQL(`query PullRequestStatus\b`), httpmock.StringResponse(`{"data": {}}`))

	output, err := RunCommand("pr status")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expected := `
Relevant pull requests in OWNER/REPO

Current branch
  There is no pull request associated with [blueberries]

Created by you
  You have no open pull requests

Requesting a code review from you
  You have no pull requests to review

`
	if output.String() != expected {
		t.Errorf("expected %q, got %q", expected, output.String())
	}
}

func TestPRStatus_detachedHead(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.Register(httpmock.GraphQL(`query PullRequestStatus\b`), httpmock.StringResponse(`{"data": {}}`))

	output, err := RunCommand("pr status")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expected := `
Relevant pull requests in OWNER/REPO

Current branch
  There is no current branch

Created by you
  You have no open pull requests

Requesting a code review from you
  You have no pull requests to review

`
	if output.String() != expected {
		t.Errorf("expected %q, got %q", expected, output.String())
	}
}

func TestPRList(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	defer stubTerminal(true)()
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.Register(httpmock.GraphQL(`query PullRequestList\b`), httpmock.FileResponse("../test/fixtures/prList.json"))

	output, err := RunCommand("pr list")
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, `
Showing 3 of 3 pull requests in OWNER/REPO

`, output.Stderr())

	lines := strings.Split(output.String(), "\n")
	res := []*regexp.Regexp{
		regexp.MustCompile(`#32.*New feature.*feature`),
		regexp.MustCompile(`#29.*Fixed bad bug.*hubot:bug-fix`),
		regexp.MustCompile(`#28.*Improve documentation.*docs`),
	}

	for i, r := range res {
		if !r.MatchString(lines[i]) {
			t.Errorf("%s did not match %s", lines[i], r)
		}
	}
}

func TestPRList_nontty(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	defer stubTerminal(false)()
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.Register(httpmock.GraphQL(`query PullRequestList\b`), httpmock.FileResponse("../test/fixtures/prList.json"))

	output, err := RunCommand("pr list")
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "", output.Stderr())

	assert.Equal(t, `32	New feature	feature	DRAFT
29	Fixed bad bug	hubot:bug-fix	OPEN
28	Improve documentation	docs	MERGED
`, output.String())
}

func TestPRList_filtering(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	defer stubTerminal(true)()
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.Register(
		httpmock.GraphQL(`query PullRequestList\b`),
		httpmock.GraphQLQuery(`{}`, func(_ string, params map[string]interface{}) {
			assert.Equal(t, []interface{}{"OPEN", "CLOSED", "MERGED"}, params["state"].([]interface{}))
			assert.Equal(t, []interface{}{"one", "two", "three"}, params["labels"].([]interface{}))
		}))

	output, err := RunCommand(`pr list -s all -l one,two -l three`)
	if err != nil {
		t.Fatal(err)
	}

	eq(t, output.String(), "")
	eq(t, output.Stderr(), `
No pull requests match your search in OWNER/REPO

`)
}

func TestPRList_filteringRemoveDuplicate(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	defer stubTerminal(true)()
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.Register(
		httpmock.GraphQL(`query PullRequestList\b`),
		httpmock.FileResponse("../test/fixtures/prListWithDuplicates.json"))

	output, err := RunCommand("pr list -l one,two")
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(output.String(), "\n")

	res := []*regexp.Regexp{
		regexp.MustCompile(`#32.*New feature.*feature`),
		regexp.MustCompile(`#29.*Fixed bad bug.*hubot:bug-fix`),
		regexp.MustCompile(`#28.*Improve documentation.*docs`),
	}

	for i, r := range res {
		if !r.MatchString(lines[i]) {
			t.Errorf("%s did not match %s", lines[i], r)
		}
	}
}

func TestPRList_filteringClosed(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	defer stubTerminal(true)()
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.Register(
		httpmock.GraphQL(`query PullRequestList\b`),
		httpmock.GraphQLQuery(`{}`, func(_ string, params map[string]interface{}) {
			assert.Equal(t, []interface{}{"CLOSED", "MERGED"}, params["state"].([]interface{}))
		}))

	_, err := RunCommand(`pr list -s closed`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPRList_filteringAssignee(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	defer stubTerminal(true)()
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.Register(
		httpmock.GraphQL(`query PullRequestList\b`),
		httpmock.GraphQLQuery(`{}`, func(_ string, params map[string]interface{}) {
			assert.Equal(t, `repo:OWNER/REPO assignee:hubot is:pr sort:created-desc is:merged label:"needs tests" base:"develop"`, params["q"].(string))
		}))

	_, err := RunCommand(`pr list -s merged -l "needs tests" -a hubot -B develop`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPRList_filteringAssigneeLabels(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	defer stubTerminal(true)()
	initFakeHTTP()

	_, err := RunCommand(`pr list -l one,two -a hubot`)
	if err == nil && err.Error() != "multiple labels with --assignee are not supported" {
		t.Fatal(err)
	}
}

func TestPRList_withInvalidLimitFlag(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	defer stubTerminal(true)()
	initFakeHTTP()

	_, err := RunCommand(`pr list --limit=0`)
	if err == nil && err.Error() != "invalid limit: 0" {
		t.Errorf("error running command `issue list`: %v", err)
	}
}

func TestPRList_web(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	defer stubTerminal(true)()
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	var seenCmd *exec.Cmd
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		seenCmd = cmd
		return &test.OutputStub{}
	})
	defer restoreCmd()

	output, err := RunCommand("pr list --web -a peter -l bug -l docs -L 10 -s merged -B trunk")
	if err != nil {
		t.Errorf("error running command `pr list` with `--web` flag: %v", err)
	}

	expectedURL := "https://github.com/OWNER/REPO/pulls?q=is%3Apr+is%3Amerged+assignee%3Apeter+label%3Abug+label%3Adocs+base%3Atrunk"

	eq(t, output.String(), "")
	eq(t, output.Stderr(), "Opening github.com/OWNER/REPO/pulls in your browser.\n")

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	eq(t, url, expectedURL)
}

func TestReplaceExcessiveWhitespace(t *testing.T) {
	eq(t, replaceExcessiveWhitespace("hello\ngoodbye"), "hello goodbye")
	eq(t, replaceExcessiveWhitespace("  hello goodbye  "), "hello goodbye")
	eq(t, replaceExcessiveWhitespace("hello   goodbye"), "hello goodbye")
	eq(t, replaceExcessiveWhitespace("   hello \n goodbye   "), "hello goodbye")
}

func TestPrClose(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": {
			"pullRequest": { "number": 96, "title": "The title of the PR" }
		} } }
	`))

	http.StubResponse(200, bytes.NewBufferString(`{"id": "THE-ID"}`))

	output, err := RunCommand("pr close 96")
	if err != nil {
		t.Fatalf("error running command `pr close`: %v", err)
	}

	r := regexp.MustCompile(`Closed pull request #96 \(The title of the PR\)`)

	if !r.MatchString(output.Stderr()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestPrClose_alreadyClosed(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": {
			"pullRequest": { "number": 101, "title": "The title of the PR", "closed": true }
		} } }
	`))

	http.StubResponse(200, bytes.NewBufferString(`{"id": "THE-ID"}`))

	output, err := RunCommand("pr close 101")
	if err != nil {
		t.Fatalf("error running command `pr close`: %v", err)
	}

	r := regexp.MustCompile(`Pull request #101 \(The title of the PR\) is already closed`)

	if !r.MatchString(output.Stderr()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestPRReopen(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": {
		"pullRequest": { "number": 666, "title": "The title of the PR", "closed": true}
	} } }
	`))

	http.StubResponse(200, bytes.NewBufferString(`{"id": "THE-ID"}`))

	output, err := RunCommand("pr reopen 666")
	if err != nil {
		t.Fatalf("error running command `pr reopen`: %v", err)
	}

	r := regexp.MustCompile(`Reopened pull request #666 \(The title of the PR\)`)

	if !r.MatchString(output.Stderr()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestPRReopen_alreadyOpen(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": {
		"pullRequest": { "number": 666,  "title": "The title of the PR", "closed": false}
	} } }
	`))

	http.StubResponse(200, bytes.NewBufferString(`{"id": "THE-ID"}`))

	output, err := RunCommand("pr reopen 666")
	if err != nil {
		t.Fatalf("error running command `pr reopen`: %v", err)
	}

	r := regexp.MustCompile(`Pull request #666 \(The title of the PR\) is already open`)

	if !r.MatchString(output.Stderr()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestPRReopen_alreadyMerged(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": {
		"pullRequest": { "number": 666, "title": "The title of the PR", "closed": true, "state": "MERGED"}
	} } }
	`))

	http.StubResponse(200, bytes.NewBufferString(`{"id": "THE-ID"}`))

	output, err := RunCommand("pr reopen 666")
	if err == nil {
		t.Fatalf("expected an error running command `pr reopen`: %v", err)
	}

	r := regexp.MustCompile(`Pull request #666 \(The title of the PR\) can't be reopened because it was already merged`)

	if !r.MatchString(err.Error()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestPrMerge(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	defer stubTerminal(true)()
	http := initFakeHTTP()
	defer http.Verify(t)
	http.StubRepoResponse("OWNER", "REPO")
	http.Register(
		httpmock.GraphQL(`query PullRequestByNumber\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "pullRequest": {
			"id": "THE-ID",
			"number": 1,
			"title": "The title of the PR",
			"state": "OPEN",
			"headRefName": "blueberries",
			"headRepositoryOwner": {"login": "OWNER"}
		} } } }`))
	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
		}))
	http.Register(
		httpmock.REST("DELETE", "repos/OWNER/REPO/git/refs/heads/blueberries"),
		httpmock.StringResponse(`{}`))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("branch.blueberries.remote origin\nbranch.blueberries.merge refs/heads/blueberries") // git config --get-regexp ^branch\.master\.(remote|merge)
	cs.Stub("")                                                                                  // git config --get-regexp ^branch\.blueberries\.(remote|merge)$
	cs.Stub("")                                                                                  // git symbolic-ref --quiet --short HEAD
	cs.Stub("")                                                                                  // git checkout master
	cs.Stub("")

	output, err := RunCommand("pr merge 1 --merge")
	if err != nil {
		t.Fatalf("error running command `pr merge`: %v", err)
	}

	r := regexp.MustCompile(`Merged pull request #1 \(The title of the PR\)`)

	if !r.MatchString(output.Stderr()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestPrMerge_nontty(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	defer stubTerminal(false)()
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.Register(
		httpmock.GraphQL(`query PullRequestByNumber\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": {
			"pullRequest": { "number": 1, "title": "The title of the PR", "state": "OPEN", "id": "THE-ID"}
		} } }`))
	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
		}))
	http.Register(
		httpmock.GraphQL(`query RepositoryInfo\b`),
		httpmock.StringResponse(`{
			"data": {
				"repository": {
					"defaultBranchRef": {"name": "master"}
				}
			}
		}`))
	http.Register(
		httpmock.REST("DELETE", "repos/OWNER/REPO/git/refs/heads/blueberries"),
		httpmock.StringResponse(`{}`))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("branch.blueberries.remote origin\nbranch.blueberries.merge refs/heads/blueberries") // git config --get-regexp ^branch\.master\.(remote|merge)
	cs.Stub("")                                                                                  // git config --get-regexp ^branch\.blueberries\.(remote|merge)$
	cs.Stub("")                                                                                  // git symbolic-ref --quiet --short HEAD
	cs.Stub("")                                                                                  // git checkout master
	cs.Stub("")

	output, err := RunCommand("pr merge 1 --merge")
	if err != nil {
		t.Fatalf("error running command `pr merge`: %v", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
}

func TestPrMerge_nontty_insufficient_flags(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	defer stubTerminal(false)()
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.Register(
		httpmock.GraphQL(`query PullRequestByNumber\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": {
			"pullRequest": { "number": 1, "title": "The title of the PR", "state": "OPEN", "id": "THE-ID"}
		} } }`))

	output, err := RunCommand("pr merge 1")
	if err == nil {
		t.Fatal("expected error")
	}

	assert.Equal(t, "--merge, --rebase, or --squash required when not attached to a tty", err.Error())
	assert.Equal(t, "", output.String())
}

func TestPrMerge_withRepoFlag(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	defer stubTerminal(true)()
	http := initFakeHTTP()
	defer http.Verify(t)
	http.Register(
		httpmock.GraphQL(`query PullRequestByNumber\b`),
		httpmock.GraphQLQuery(`
		{ "data": { "repository": {
			"pullRequest": { "number": 1, "title": "The title of the PR", "state": "OPEN", "id": "THE-ID"}
		} } }`, func(_ string, params map[string]interface{}) {
			assert.Equal(t, "stinky", params["owner"].(string))
			assert.Equal(t, "boi", params["repo"].(string))
		}))
	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
		}))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	eq(t, len(cs.Calls), 0)

	output, err := RunCommand("pr merge 1 --merge -R stinky/boi")
	if err != nil {
		t.Fatalf("error running command `pr merge`: %v", err)
	}

	r := regexp.MustCompile(`Merged pull request #1 \(The title of the PR\)`)

	if !r.MatchString(output.Stderr()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestPrMerge_deleteBranch(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "blueberries")
	defer stubTerminal(true)()
	http := initFakeHTTP()
	defer http.Verify(t)
	http.StubRepoResponse("OWNER", "REPO")
	http.Register(
		httpmock.GraphQL(`query PullRequestForBranch\b`),
		httpmock.FileResponse("../test/fixtures/prViewPreviewWithMetadataByBranch.json"))
	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "PR_10", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
		}))
	http.Register(
		httpmock.REST("DELETE", "repos/OWNER/REPO/git/refs/heads/blueberries"),
		httpmock.StringResponse(`{}`))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("") // git config --get-regexp ^branch\.blueberries\.(remote|merge)$
	cs.Stub("") // git checkout master
	cs.Stub("") // git rev-parse --verify blueberries`
	cs.Stub("") // git branch -d
	cs.Stub("") // git push origin --delete blueberries

	output, err := RunCommand(`pr merge --merge --delete-branch`)
	if err != nil {
		t.Fatalf("Got unexpected error running `pr merge` %s", err)
	}

	test.ExpectLines(t, output.Stderr(), `Merged pull request #10 \(Blueberries are a good fruit\)`, `Deleted branch.*blueberries`)
}

func TestPrMerge_deleteNonCurrentBranch(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "another-branch")
	defer stubTerminal(true)()
	http := initFakeHTTP()
	defer http.Verify(t)
	http.StubRepoResponse("OWNER", "REPO")
	http.Register(
		httpmock.GraphQL(`query PullRequestForBranch\b`),
		httpmock.FileResponse("../test/fixtures/prViewPreviewWithMetadataByBranch.json"))
	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "PR_10", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
		}))
	http.Register(
		httpmock.REST("DELETE", "repos/OWNER/REPO/git/refs/heads/blueberries"),
		httpmock.StringResponse(`{}`))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()
	// We don't expect the default branch to be checked out, just that blueberries is deleted
	cs.Stub("") // git rev-parse --verify blueberries
	cs.Stub("") // git branch -d blueberries
	cs.Stub("") // git push origin --delete blueberries

	output, err := RunCommand(`pr merge --merge --delete-branch blueberries`)
	if err != nil {
		t.Fatalf("Got unexpected error running `pr merge` %s", err)
	}

	test.ExpectLines(t, output.Stderr(), `Merged pull request #10 \(Blueberries are a good fruit\)`, `Deleted branch.*blueberries`)
}

func TestPrMerge_noPrNumberGiven(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "blueberries")
	defer stubTerminal(true)()
	http := initFakeHTTP()
	defer http.Verify(t)
	http.StubRepoResponse("OWNER", "REPO")
	http.Register(
		httpmock.GraphQL(`query PullRequestForBranch\b`),
		httpmock.FileResponse("../test/fixtures/prViewPreviewWithMetadataByBranch.json"))
	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "PR_10", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
		}))
	http.Register(
		httpmock.REST("DELETE", "repos/OWNER/REPO/git/refs/heads/blueberries"),
		httpmock.StringResponse(`{}`))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("branch.blueberries.remote origin\nbranch.blueberries.merge refs/heads/blueberries") // git config --get-regexp ^branch\.master\.(remote|merge)
	cs.Stub("")                                                                                  // git config --get-regexp ^branch\.blueberries\.(remote|merge)$
	cs.Stub("")                                                                                  // git symbolic-ref --quiet --short HEAD
	cs.Stub("")                                                                                  // git checkout master
	cs.Stub("")                                                                                  // git branch -d

	output, err := RunCommand("pr merge --merge")
	if err != nil {
		t.Fatalf("error running command `pr merge`: %v", err)
	}

	r := regexp.MustCompile(`Merged pull request #10 \(Blueberries are a good fruit\)`)

	if !r.MatchString(output.Stderr()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestPrMerge_rebase(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	defer stubTerminal(true)()
	http := initFakeHTTP()
	defer http.Verify(t)
	http.StubRepoResponse("OWNER", "REPO")
	http.Register(
		httpmock.GraphQL(`query PullRequestByNumber\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "pullRequest": {
			"id": "THE-ID",
			"number": 2,
			"title": "The title of the PR",
			"state": "OPEN",
			"headRefName": "blueberries",
			"headRepositoryOwner": {"login": "OWNER"}
		} } } }`))
	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "REBASE", input["mergeMethod"].(string))
		}))
	http.Register(
		httpmock.REST("DELETE", "repos/OWNER/REPO/git/refs/heads/blueberries"),
		httpmock.StringResponse(`{}`))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("") // git config --get-regexp ^branch\.blueberries\.(remote|merge)$
	cs.Stub("") // git symbolic-ref --quiet --short HEAD
	cs.Stub("") // git checkout master
	cs.Stub("") // git branch -d

	output, err := RunCommand("pr merge 2 --rebase")
	if err != nil {
		t.Fatalf("error running command `pr merge`: %v", err)
	}

	r := regexp.MustCompile(`Rebased and merged pull request #2 \(The title of the PR\)`)

	if !r.MatchString(output.Stderr()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestPrMerge_squash(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	defer stubTerminal(true)()
	http := initFakeHTTP()
	defer http.Verify(t)
	http.StubRepoResponse("OWNER", "REPO")
	http.Register(
		httpmock.GraphQL(`query PullRequestByNumber\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "pullRequest": {
			"id": "THE-ID",
			"number": 3,
			"title": "The title of the PR",
			"state": "OPEN",
			"headRefName": "blueberries",
			"headRepositoryOwner": {"login": "OWNER"}
		} } } }`))
	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "SQUASH", input["mergeMethod"].(string))
		}))
	http.Register(
		httpmock.REST("DELETE", "repos/OWNER/REPO/git/refs/heads/blueberries"),
		httpmock.StringResponse(`{}`))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("") // git config --get-regexp ^branch\.blueberries\.(remote|merge)$
	cs.Stub("") // git symbolic-ref --quiet --short HEAD
	cs.Stub("") // git checkout master
	cs.Stub("") // git branch -d

	output, err := RunCommand("pr merge 3 --squash")
	if err != nil {
		t.Fatalf("error running command `pr merge`: %v", err)
	}

	test.ExpectLines(t, output.Stderr(), "Squashed and merged pull request #3", `Deleted branch.*blueberries`)
}

func TestPrMerge_alreadyMerged(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	defer stubTerminal(true)()
	http := initFakeHTTP()
	defer http.Verify(t)
	http.StubRepoResponse("OWNER", "REPO")
	http.Register(
		httpmock.GraphQL(`query PullRequestByNumber\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": {
			"pullRequest": { "number": 4, "title": "The title of the PR", "state": "MERGED"}
		} } }`))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("") // git config --get-regexp ^branch\.blueberries\.(remote|merge)$
	cs.Stub("") // git symbolic-ref --quiet --short HEAD
	cs.Stub("") // git checkout master
	cs.Stub("") // git branch -d

	output, err := RunCommand("pr merge 4")
	if err == nil {
		t.Fatalf("expected an error running command `pr merge`: %v", err)
	}

	r := regexp.MustCompile(`Pull request #4 \(The title of the PR\) was already merged`)

	if !r.MatchString(err.Error()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestPRMerge_interactive(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "blueberries")
	defer stubTerminal(true)()
	http := initFakeHTTP()
	defer http.Verify(t)
	http.StubRepoResponse("OWNER", "REPO")
	http.Register(
		httpmock.GraphQL(`query PullRequestForBranch\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "pullRequests": { "nodes": [{
			"headRefName": "blueberries",
			"headRepositoryOwner": {"login": "OWNER"},
			"id": "THE-ID",
			"number": 3
		}] } } } }`))
	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
		}))
	http.Register(
		httpmock.REST("DELETE", "repos/OWNER/REPO/git/refs/heads/blueberries"),
		httpmock.StringResponse(`{}`))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("") // git config --get-regexp ^branch\.blueberries\.(remote|merge)$
	cs.Stub("") // git symbolic-ref --quiet --short HEAD
	cs.Stub("") // git checkout master
	cs.Stub("") // git push origin --delete blueberries
	cs.Stub("") // git branch -d

	as, surveyTeardown := prompt.InitAskStubber()
	defer surveyTeardown()

	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "mergeMethod",
			Value: 0,
		},
		{
			Name:  "deleteBranch",
			Value: true,
		},
	})

	output, err := RunCommand(`pr merge`)
	if err != nil {
		t.Fatalf("Got unexpected error running `pr merge` %s", err)
	}

	test.ExpectLines(t, output.Stderr(), "Merged pull request #3", `Deleted branch.*blueberries`)
}

func TestPrMerge_multipleMergeMethods(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	defer stubTerminal(true)()

	_, err := RunCommand("pr merge 1 --merge --squash")
	if err == nil {
		t.Fatal("expected error running `pr merge` with multiple merge methods")
	}
}

func TestPrMerge_multipleMergeMethods_nontty(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	defer stubTerminal(false)()

	_, err := RunCommand("pr merge 1 --merge --squash")
	if err == nil {
		t.Fatal("expected error running `pr merge` with multiple merge methods")
	}
}

func TestPRReady(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": {
		"pullRequest": { "number": 444, "closed": false, "isDraft": true}
	} } }
	`))
	http.StubResponse(200, bytes.NewBufferString(`{"id": "THE-ID"}`))

	output, err := RunCommand("pr ready 444")
	if err != nil {
		t.Fatalf("error running command `pr ready`: %v", err)
	}

	r := regexp.MustCompile(`Pull request #444 is marked as "ready for review"`)

	if !r.MatchString(output.Stderr()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestPRReady_alreadyReady(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": {
		"pullRequest": { "number": 445, "closed": false, "isDraft": false}
	} } }
	`))
	http.StubResponse(200, bytes.NewBufferString(`{"id": "THE-ID"}`))

	output, err := RunCommand("pr ready 445")
	if err != nil {
		t.Fatalf("error running command `pr ready`: %v", err)
	}

	r := regexp.MustCompile(`Pull request #445 is already "ready for review"`)

	if !r.MatchString(output.Stderr()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestPRReady_closed(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": {
		"pullRequest": { "number": 446, "closed": true, "isDraft": true}
	} } }
	`))
	http.StubResponse(200, bytes.NewBufferString(`{"id": "THE-ID"}`))

	_, err := RunCommand("pr ready 446")
	if err == nil {
		t.Fatalf("expected an error running command `pr ready` on a review that is closed!: %v", err)
	}

	r := regexp.MustCompile(`Pull request #446 is closed. Only draft pull requests can be marked as "ready for review"`)

	if !r.MatchString(err.Error()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, err.Error())
	}
}
