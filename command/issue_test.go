package command

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"github.com/cli/cli/utils"
)

func TestIssueStatus(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	jsonFile, _ := os.Open("../test/fixtures/issueStatus.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	output, err := RunCommand(issueStatusCmd, "issue status")
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
		if !r.MatchString(output.String()) {
			t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
			return
		}
	}
}

func TestIssueStatus_blankSlate(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": {
		"hasIssuesEnabled": true,
		"assigned": { "nodes": [] },
		"mentioned": { "nodes": [] },
		"authored": { "nodes": [] }
	} } }
	`))

	output, err := RunCommand(issueStatusCmd, "issue status")
	if err != nil {
		t.Errorf("error running command `issue status`: %v", err)
	}

	expectedOutput := `
Relevant issues in OWNER/REPO

Issues assigned to you
  There are no issues assigned to you

Issues mentioning you
  There are no issues mentioning you

Issues opened by you
  There are no issues opened by you

`
	if output.String() != expectedOutput {
		t.Errorf("expected %q, got %q", expectedOutput, output)
	}
}

func TestIssueStatus_disabledIssues(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": {
		"hasIssuesEnabled": false
	} } }
	`))

	_, err := RunCommand(issueStatusCmd, "issue status")
	if err == nil || err.Error() != "the 'OWNER/REPO' repository has disabled issues" {
		t.Errorf("error running command `issue status`: %v", err)
	}
}

func TestIssueList(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	jsonFile, _ := os.Open("../test/fixtures/issueList.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	output, err := RunCommand(issueListCmd, "issue list")
	if err != nil {
		t.Errorf("error running command `issue list`: %v", err)
	}

	expectedIssues := []*regexp.Regexp{
		regexp.MustCompile(`(?m)^1\t.*won`),
		regexp.MustCompile(`(?m)^2\t.*too`),
		regexp.MustCompile(`(?m)^4\t.*fore`),
	}

	for _, r := range expectedIssues {
		if !r.MatchString(output.String()) {
			t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
			return
		}
	}
}

func TestIssueList_withFlags(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": {	"repository": {
		"hasIssuesEnabled": true,
		"issues": { "nodes": [] }
	} } }
	`))

	output, err := RunCommand(issueListCmd, "issue list -a probablyCher -l web,bug -s open -A foo")
	if err != nil {
		t.Errorf("error running command `issue list`: %v", err)
	}

	eq(t, output.String(), "")
	eq(t, output.Stderr(), `
Issues for OWNER/REPO

No issues match your search
`)

	bodyBytes, _ := ioutil.ReadAll(http.Requests[1].Body)
	reqBody := struct {
		Variables struct {
			Assignee string
			Labels   []string
			States   []string
			Author   string
		}
	}{}
	json.Unmarshal(bodyBytes, &reqBody)

	eq(t, reqBody.Variables.Assignee, "probablyCher")
	eq(t, reqBody.Variables.Labels, []string{"web", "bug"})
	eq(t, reqBody.Variables.States, []string{"OPEN"})
	eq(t, reqBody.Variables.Author, "foo")
}

func TestIssueList_nullAssigneeLabels(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": {	"repository": {
		"hasIssuesEnabled": true,
		"issues": { "nodes": [] }
	} } }
	`))

	_, err := RunCommand(issueListCmd, "issue list")
	if err != nil {
		t.Errorf("error running command `issue list`: %v", err)
	}

	bodyBytes, _ := ioutil.ReadAll(http.Requests[1].Body)
	reqBody := struct {
		Variables map[string]interface{}
	}{}
	json.Unmarshal(bodyBytes, &reqBody)

	_, assigneeDeclared := reqBody.Variables["assignee"]
	_, labelsDeclared := reqBody.Variables["labels"]
	eq(t, assigneeDeclared, false)
	eq(t, labelsDeclared, false)
}

func TestIssueList_disabledIssues(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": {	"repository": {
		"hasIssuesEnabled": false
	} } }
	`))

	_, err := RunCommand(issueListCmd, "issue list")
	if err == nil || err.Error() != "the 'OWNER/REPO' repository has disabled issues" {
		t.Errorf("error running command `issue list`: %v", err)
	}
}

func TestIssueView(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "hasIssuesEnabled": true, "issue": {
		"number": 123,
		"url": "https://github.com/OWNER/REPO/issues/123"
	} } } }
	`))

	var seenCmd *exec.Cmd
	restoreCmd := utils.SetPrepareCmd(func(cmd *exec.Cmd) utils.Runnable {
		seenCmd = cmd
		return &outputStub{}
	})
	defer restoreCmd()

	output, err := RunCommand(issueViewCmd, "issue view 123")
	if err != nil {
		t.Errorf("error running command `issue view`: %v", err)
	}

	eq(t, output.String(), "")
	eq(t, output.Stderr(), "Opening https://github.com/OWNER/REPO/issues/123 in your browser.\n")

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	eq(t, url, "https://github.com/OWNER/REPO/issues/123")
}

func TestIssueView_numberArgWithHash(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "hasIssuesEnabled": true, "issue": {
		"number": 123,
		"url": "https://github.com/OWNER/REPO/issues/123"
	} } } }
	`))

	var seenCmd *exec.Cmd
	restoreCmd := utils.SetPrepareCmd(func(cmd *exec.Cmd) utils.Runnable {
		seenCmd = cmd
		return &outputStub{}
	})
	defer restoreCmd()

	output, err := RunCommand(issueViewCmd, "issue view \"#123\"")
	if err != nil {
		t.Errorf("error running command `issue view`: %v", err)
	}

	eq(t, output.String(), "")
	eq(t, output.Stderr(), "Opening https://github.com/OWNER/REPO/issues/123 in your browser.\n")

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	eq(t, url, "https://github.com/OWNER/REPO/issues/123")
}

func TestIssueView_preview(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	jsonFile, _ := os.Open("../test/fixtures/issueView_preview.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	output, err := RunCommand(issueViewCmd, "issue view -p 123")
	if err != nil {
		t.Errorf("error running command `issue view`: %v", err)
	}

	eq(t, output.Stderr(), "")

	expectedLines := []*regexp.Regexp{
		regexp.MustCompile(`ix of coins`),
		regexp.MustCompile(`OPEN • marseilles opened about 292 years ago • 9 comments • tarot`),
		regexp.MustCompile(`bold story`),
		regexp.MustCompile(`View this issue on GitHub: https://github.com/OWNER/REPO/issues/123`),
	}
	for _, r := range expectedLines {
		if !r.MatchString(output.String()) {
			t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
			return
		}
	}
}

func TestIssueView_previewClosedState(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	jsonFile, _ := os.Open("../test/fixtures/issueView_previewClosedState.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	output, err := RunCommand(issueViewCmd, "issue view -p 123")
	if err != nil {
		t.Errorf("error running command `issue view`: %v", err)
	}

	eq(t, output.Stderr(), "")

	expectedLines := []*regexp.Regexp{
		regexp.MustCompile(`ix of coins`),
		regexp.MustCompile(`CLOSED • marseilles opened about 292 years ago • 9 comments • tarot`),
		regexp.MustCompile(`bold story`),
		regexp.MustCompile(`View this issue on GitHub: https://github.com/OWNER/REPO/issues/123`),
	}
	for _, r := range expectedLines {
		if !r.MatchString(output.String()) {
			t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
			return
		}
	}
}

func TestIssueView_previewWithEmptyBody(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	jsonFile, _ := os.Open("../test/fixtures/issueView_previewWithEmptyBody.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	output, err := RunCommand(issueViewCmd, "issue view -p 123")
	if err != nil {
		t.Errorf("error running command `issue view`: %v", err)
	}

	eq(t, output.Stderr(), "")

	expectedLines := []*regexp.Regexp{
		regexp.MustCompile(`ix of coins`),
		regexp.MustCompile(`OPEN • marseilles opened about 292 years ago • 9 comments • tarot`),
		regexp.MustCompile(`View this issue on GitHub: https://github.com/OWNER/REPO/issues/123`),
	}
	for _, r := range expectedLines {
		if !r.MatchString(output.String()) {
			t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
			return
		}
	}
}

func TestIssueView_notFound(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()

	http.StubResponse(200, bytes.NewBufferString(`
	{ "errors": [
		{ "message": "Could not resolve to an Issue with the number of 9999." }
	] }
	`))

	var seenCmd *exec.Cmd
	restoreCmd := utils.SetPrepareCmd(func(cmd *exec.Cmd) utils.Runnable {
		seenCmd = cmd
		return &outputStub{}
	})
	defer restoreCmd()

	_, err := RunCommand(issueViewCmd, "issue view 9999")
	if err == nil || err.Error() != "graphql error: 'Could not resolve to an Issue with the number of 9999.'" {
		t.Errorf("error running command `issue view`: %v", err)
	}

	if seenCmd != nil {
		t.Fatal("did not expect any command to run")
	}
}

func TestIssueView_disabledIssues(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": {
			"id": "REPOID",
			"hasIssuesEnabled": false
		} } }
	`))

	_, err := RunCommand(issueViewCmd, `issue view 6666`)
	if err == nil || err.Error() != "the 'OWNER/REPO' repository has disabled issues" {
		t.Errorf("error running command `issue view`: %v", err)
	}
}

func TestIssueView_urlArg(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "hasIssuesEnabled": true, "issue": {
		"number": 123,
		"url": "https://github.com/OWNER/REPO/issues/123"
	} } } }
	`))

	var seenCmd *exec.Cmd
	restoreCmd := utils.SetPrepareCmd(func(cmd *exec.Cmd) utils.Runnable {
		seenCmd = cmd
		return &outputStub{}
	})
	defer restoreCmd()

	output, err := RunCommand(issueViewCmd, "issue view https://github.com/OWNER/REPO/issues/123")
	if err != nil {
		t.Errorf("error running command `issue view`: %v", err)
	}

	eq(t, output.String(), "")

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	eq(t, url, "https://github.com/OWNER/REPO/issues/123")
}

func TestIssueCreate(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": {
			"id": "REPOID",
			"hasIssuesEnabled": true
		} } }
	`))
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "createIssue": { "issue": {
			"URL": "https://github.com/OWNER/REPO/issues/12"
		} } } }
	`))

	output, err := RunCommand(issueCreateCmd, `issue create -t hello -b "cash rules everything around me"`)
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	bodyBytes, _ := ioutil.ReadAll(http.Requests[2].Body)
	reqBody := struct {
		Variables struct {
			Input struct {
				RepositoryID string
				Title        string
				Body         string
			}
		}
	}{}
	json.Unmarshal(bodyBytes, &reqBody)

	eq(t, reqBody.Variables.Input.RepositoryID, "REPOID")
	eq(t, reqBody.Variables.Input.Title, "hello")
	eq(t, reqBody.Variables.Input.Body, "cash rules everything around me")

	eq(t, output.String(), "https://github.com/OWNER/REPO/issues/12\n")
}

func TestIssueCreate_disabledIssues(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": {
			"id": "REPOID",
			"hasIssuesEnabled": false
		} } }
	`))

	_, err := RunCommand(issueCreateCmd, `issue create -t heres -b johnny`)
	if err == nil || err.Error() != "the 'OWNER/REPO' repository has disabled issues" {
		t.Errorf("error running command `issue create`: %v", err)
	}
}

func TestIssueCreate_web(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	var seenCmd *exec.Cmd
	restoreCmd := utils.SetPrepareCmd(func(cmd *exec.Cmd) utils.Runnable {
		seenCmd = cmd
		return &outputStub{}
	})
	defer restoreCmd()

	output, err := RunCommand(issueCreateCmd, `issue create --web`)
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	eq(t, url, "https://github.com/OWNER/REPO/issues/new")
	eq(t, output.String(), "Opening github.com/OWNER/REPO/issues/new in your browser.\n")
	eq(t, output.Stderr(), "")
}

func TestIssueCreate_webTitleBody(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	var seenCmd *exec.Cmd
	restoreCmd := utils.SetPrepareCmd(func(cmd *exec.Cmd) utils.Runnable {
		seenCmd = cmd
		return &outputStub{}
	})
	defer restoreCmd()

	output, err := RunCommand(issueCreateCmd, `issue create -w -t mytitle -b mybody`)
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := strings.ReplaceAll(seenCmd.Args[len(seenCmd.Args)-1], "^", "")
	eq(t, url, "https://github.com/OWNER/REPO/issues/new?title=mytitle&body=mybody")
	eq(t, output.String(), "Opening github.com/OWNER/REPO/issues/new in your browser.\n")
}
