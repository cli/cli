package command

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/cli/cli/context"
	"github.com/cli/cli/utils"
)

func TestPRCreate(t *testing.T) {
	initBlankContext("OWNER/REPO", "feature")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "createPullRequest": { "pullRequest": {
			"URL": "https://github.com/OWNER/REPO/pull/12"
		} } } }
	`))

	cs := CmdStubber{}
	teardown := utils.SetPrepareCmd(createStubbedPrepareCmd(&cs))
	defer teardown()

	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log
	cs.Stub("")                                         // git push

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

	cs := CmdStubber{}
	teardown := utils.SetPrepareCmd(createStubbedPrepareCmd(&cs))
	defer teardown()

	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log
	cs.Stub("")                                         // git push
	cs.Stub("")                                         // browser

	output, err := RunCommand(prCreateCmd, `pr create --web`)
	eq(t, err, nil)

	eq(t, output.String(), "")
	eq(t, output.Stderr(), "Opening github.com/OWNER/REPO/compare/master...feature in your browser.\n")

	eq(t, len(cs.Calls), 4)
	eq(t, strings.Join(cs.Calls[2].Args, " "), "git push --set-upstream origin HEAD:feature")
	browserCall := cs.Calls[3].Args
	eq(t, browserCall[len(browserCall)-1], "https://github.com/OWNER/REPO/compare/master...feature?expand=1")
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

	cs := CmdStubber{}
	teardown := utils.SetPrepareCmd(createStubbedPrepareCmd(&cs))
	defer teardown()

	cs.Stub(" M git/git.go")                            // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log
	cs.Stub("")                                         // git push

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

	cs := CmdStubber{}
	teardown := utils.SetPrepareCmd(createStubbedPrepareCmd(&cs))
	defer teardown()

	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log
	cs.Stub("")                                         // git push

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
