package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/github/gh-cli/context"
	"github.com/github/gh-cli/git"
	"github.com/github/gh-cli/test"
	"github.com/github/gh-cli/utils"
)

func TestPrCreateHelperProcess(*testing.T) {
	if test.SkipTestHelperProcess() {
		return
	}

	args := test.GetTestHelperProcessArgs()
	switch args[1] {
	case "status":
		switch args[0] {
		case "clean":
		case "dirty":
			fmt.Println(" M git/git.go")
		default:
			fmt.Fprintf(os.Stderr, "unknown scenario: %q", args[0])
			os.Exit(1)
		}
	case "push":
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %q", args[1])
		os.Exit(1)
	}
	os.Exit(0)
}

func TestPRCreate(t *testing.T) {
	ctx := context.NewBlank()
	ctx.SetBranch("feature")
	ctx.SetRemotes(map[string]string{
		"origin": "OWNER/REPO",
	})
	initContext = func() context.Context {
		return ctx
	}
	http := initFakeHTTP()

	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": {
			"id": "REPOID"
		} } }
	`))
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "createPullRequest": { "pullRequest": {
			"URL": "https://github.com/OWNER/REPO/pull/12"
		} } } }
	`))

	origGitCommand := git.GitCommand
	git.GitCommand = test.StubExecCommand("TestPrCreateHelperProcess", "clean")
	defer func() {
		git.GitCommand = origGitCommand
	}()

	output, err := RunCommand(prCreateCmd, `pr create -t "my title" -b "my body"`)
	eq(t, err, nil)

	bodyBytes, _ := ioutil.ReadAll(http.Requests[1].Body)
	reqBody := struct {
		Variables struct {
			Input struct {
				RepositoryID string
				Title        string
				Body         string
				BaseRefName  string
				HeadRefName  string
			}
		}
	}{}
	json.Unmarshal(bodyBytes, &reqBody)

	eq(t, reqBody.Variables.Input.RepositoryID, "REPOID")
	eq(t, reqBody.Variables.Input.Title, "my title")
	eq(t, reqBody.Variables.Input.Body, "my body")
	eq(t, reqBody.Variables.Input.BaseRefName, "master")
	eq(t, reqBody.Variables.Input.HeadRefName, "feature")

	eq(t, output.String(), "https://github.com/OWNER/REPO/pull/12\n")
}

func TestPRCreate_web(t *testing.T) {
	ctx := context.NewBlank()
	ctx.SetBranch("feature")
	ctx.SetRemotes(map[string]string{
		"origin": "OWNER/REPO",
	})
	initContext = func() context.Context {
		return ctx
	}
	initFakeHTTP()

	ranCommands := [][]string{}
	restoreCmd := utils.SetPrepareCmd(func(cmd *exec.Cmd) utils.Runnable {
		ranCommands = append(ranCommands, cmd.Args)
		return &outputStub{}
	})
	defer restoreCmd()

	output, err := RunCommand(prCreateCmd, `pr create --web`)
	eq(t, err, nil)

	eq(t, output.String(), "")
	eq(t, output.Stderr(), "Opening https://github.com/OWNER/REPO/pull/feature in your browser.\n")

	eq(t, len(ranCommands), 3)
	eq(t, strings.Join(ranCommands[1], " "), "git push --set-upstream origin HEAD:feature")
	eq(t, ranCommands[2][len(ranCommands[2])-1], "https://github.com/OWNER/REPO/pull/feature")
}

func TestPRCreate_ReportsUncommittedChanges(t *testing.T) {
	ctx := context.NewBlank()
	ctx.SetBranch("feature")
	ctx.SetRemotes(map[string]string{
		"origin": "OWNER/REPO",
	})
	initContext = func() context.Context {
		return ctx
	}
	http := initFakeHTTP()

	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": {
			"id": "REPOID"
		} } }
	`))
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "createPullRequest": { "pullRequest": {
			"URL": "https://github.com/OWNER/REPO/pull/12"
		} } } }
	`))

	origGitCommand := git.GitCommand
	git.GitCommand = test.StubExecCommand("TestPrCreateHelperProcess", "dirty")
	defer func() {
		git.GitCommand = origGitCommand
	}()

	output, err := RunCommand(prCreateCmd, `pr create -t "my title" -b "my body"`)
	eq(t, err, nil)

	eq(t, output.String(), "https://github.com/OWNER/REPO/pull/12\n")
	eq(t, output.Stderr(), "Warning: 1 uncommitted change\n")
}
