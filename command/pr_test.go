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
	"github.com/cli/cli/test"
	"github.com/stretchr/testify/assert"
)

func eq(t *testing.T, got interface{}, expected interface{}) {
	t.Helper()
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("expected: %v, got: %v", expected, got)
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
Showing 3 of 3 open pull requests in OWNER/REPO

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
