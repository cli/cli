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

	"github.com/cli/cli/context"
	"github.com/cli/cli/git"
	"github.com/cli/cli/test"
	"github.com/cli/cli/utils"
)

func TestPrCreateHelperProcess(*testing.T) {
	if test.SkipTestHelperProcess() {
		return
	}

	args := test.GetTestHelperProcessArgs()
	switch args[1] {
	case "log":
		fmt.Println("123,cool,\"\"")
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
	initBlankContext("OWNER/REPO", "feature")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
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
	initBlankContext("OWNER/REPO", "feature")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	ranCommands := [][]string{}
	restoreCmd := utils.SetPrepareCmd(func(cmd *exec.Cmd) utils.Runnable {
		ranCommands = append(ranCommands, cmd.Args)
		return &outputStub{}
	})
	defer restoreCmd()

	output, err := RunCommand(prCreateCmd, `pr create --web`)
	eq(t, err, nil)

	eq(t, output.String(), "")
	eq(t, output.Stderr(), "Opening github.com/OWNER/REPO/compare/master...feature in your browser.\n")

	eq(t, len(ranCommands), 4)
	eq(t, strings.Join(ranCommands[1], " "), "git push --set-upstream origin HEAD:feature")
	eq(t, ranCommands[3][len(ranCommands[3])-1], "https://github.com/OWNER/REPO/compare/master...feature?expand=1")
}

func TestPRCreate_ReportsUncommittedChanges(t *testing.T) {
	initBlankContext("OWNER/REPO", "feature")
	http := initFakeHTTP()

	http.StubRepoResponse("OWNER", "REPO")
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
	eq(t, output.Stderr(), `Warning: 1 uncommitted change

Creating pull request for feature into master in OWNER/REPO

`)
}
func TestPRCreate_cross_repo_same_branch(t *testing.T) {
	ctx := context.NewBlank()
	ctx.SetBranch("default")
	ctx.SetRemotes(map[string]string{
		"origin": "OWNER/REPO",
		"fork":   "MYSELF/REPO",
	})
	initContext = func() context.Context {
		return ctx
	}
	http := initFakeHTTP()
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repo_000": {
									"id": "REPOID0",
									"name": "REPO",
									"owner": {"login": "OWNER"},
									"defaultBranchRef": {
										"name": "default",
										"target": {"oid": "deadbeef"}
									},
									"viewerPermission": "READ"
								},
								"repo_001" : {
									"parent": {
										"id": "REPOID0",
										"name": "REPO",
										"owner": {"login": "OWNER"},
										"defaultBranchRef": {
											"name": "default",
											"target": {"oid": "deadbeef"}
										},
										"viewerPermission": "READ"
									},
									"id": "REPOID1",
									"name": "REPO",
									"owner": {"login": "MYSELF"},
									"defaultBranchRef": {
										"name": "default",
										"target": {"oid": "deadbeef"}
									},
									"viewerPermission": "WRITE"
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

	output, err := RunCommand(prCreateCmd, `pr create -t "cross repo" -b "same branch"`)
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

	eq(t, reqBody.Variables.Input.RepositoryID, "REPOID0")
	eq(t, reqBody.Variables.Input.Title, "cross repo")
	eq(t, reqBody.Variables.Input.Body, "same branch")
	eq(t, reqBody.Variables.Input.BaseRefName, "default")
	eq(t, reqBody.Variables.Input.HeadRefName, "MYSELF:default")

	eq(t, output.String(), "https://github.com/OWNER/REPO/pull/12\n")

	// goal: only care that gql is formatted properly
}

/*
 We aren't testing the survey code paths /at all/.

 so if we want to test those code paths, some cases:

 - user supplies no -t/-b and wants to preview in browser
 - user supplies no -t/-b and wants to submit directly
 - user supplies no -t/-b and wants to edit the title
 - user supplies no -t/-b and wants to edit the body

 for defaults:

 - one commit
 - multiple commits

 checking that defaults are generated appropriately each time.

 it seems that each survey prompt needs to be an injectable hook.
*/

func PRCreate_survey_preview_defaults(t *testing.T) {
	// there are going to be calls to:
	// - git status
	// - git push
	// - git rev-parse
	// - git log
}
