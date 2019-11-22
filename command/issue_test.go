package command

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
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
			t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
			return
		}
	}
}

func TestIssueList(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()

	jsonFile, _ := os.Open("../test/fixtures/issueList.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	output, err := test.RunCommand(RootCmd, "issue list")
	if err != nil {
		t.Errorf("error running command `issue list`: %v", err)
	}

	expectedIssues := []*regexp.Regexp{
		regexp.MustCompile(`(?m)^1\t.*won`),
		regexp.MustCompile(`(?m)^2\t.*too`),
		regexp.MustCompile(`(?m)^4\t.*fore`),
	}

	for _, r := range expectedIssues {
		if !r.MatchString(output) {
			t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
			return
		}
	}
}

func TestIssueList_withFlags(t *testing.T) {
	http := initFakeHTTP()

	http.StubResponse(200, bytes.NewBufferString(`{"data": {}}`)) // Since we are testing that the flags are passed, we don't care about the response

	_, err := test.RunCommand(RootCmd, "issue list -a probablyCher -l web,bug -s open")
	if err != nil {
		t.Errorf("error running command `issue list`: %v", err)
	}

	bodyBytes, _ := ioutil.ReadAll(http.Requests[0].Body)
	reqBody := struct {
		Variables struct {
			Assignee string
			Labels   []string
			States   []string
		}
	}{}
	json.Unmarshal(bodyBytes, &reqBody)

	eq(t, reqBody.Variables.Assignee, "probablyCher")
	eq(t, reqBody.Variables.Labels, []string{"web", "bug"})
	eq(t, reqBody.Variables.States, []string{"OPEN"})
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

func TestIssueCreate(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()

	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": {
			"id": "REPOID"
		} } }
	`))
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "createIssue": { "issue": {
			"URL": "https://github.com/OWNER/REPO/issues/12"
		} } } }
	`))

	out := bytes.Buffer{}
	issueCreateCmd.SetOut(&out)

	RootCmd.SetArgs([]string{"issue", "create", "-m", "hello", "-m", "ab", "-m", "cd"})
	_, err := RootCmd.ExecuteC()
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	bodyBytes, _ := ioutil.ReadAll(http.Requests[1].Body)
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
	eq(t, reqBody.Variables.Input.Body, "ab\n\ncd")

	eq(t, out.String(), "https://github.com/OWNER/REPO/issues/12\n")
}
