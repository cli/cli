package command

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/cli/cli/context"
)

func TestPRCreate(t *testing.T) {
	initBlankContext("OWNER/REPO", "feature")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "forks": { "nodes": [
	] } } } }
	`))
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": { "pullRequests": { "nodes" : [
		] } } } }
	`))
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "createPullRequest": { "pullRequest": {
			"URL": "https://github.com/OWNER/REPO/pull/12"
		} } } }
	`))

	cs, cmdTeardown := initCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log
	cs.Stub("")                                         // git push

	output, err := RunCommand(prCreateCmd, `pr create -t "my title" -b "my body"`)
	eq(t, err, nil)

	bodyBytes, _ := ioutil.ReadAll(http.Requests[3].Body)
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

func TestPRCreate_alreadyExists(t *testing.T) {
	initBlankContext("OWNER/REPO", "feature")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "forks": { "nodes": [
	] } } } }
	`))
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": { "pullRequests": { "nodes": [
			{ "url": "https://github.com/OWNER/REPO/pull/123",
			  "headRefName": "feature",
				"baseRefName": "master" }
		] } } } }
	`))

	cs, cmdTeardown := initCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log

	_, err := RunCommand(prCreateCmd, `pr create`)
	if err == nil {
		t.Fatal("error expected, got nil")
	}
	if err.Error() != "a pull request for branch \"feature\" into branch \"master\" already exists:\nhttps://github.com/OWNER/REPO/pull/123" {
		t.Errorf("got error %q", err)
	}
}

func TestPRCreate_alreadyExistsDifferentBase(t *testing.T) {
	initBlankContext("OWNER/REPO", "feature")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": { "pullRequests": { "nodes": [
			{ "url": "https://github.com/OWNER/REPO/pull/123",
			  "headRefName": "feature",
				"baseRefName": "master" }
		] } } } }
	`))
	http.StubResponse(200, bytes.NewBufferString("{}"))

	cs, cmdTeardown := initCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log
	cs.Stub("")                                         // git rev-parse

	_, err := RunCommand(prCreateCmd, `pr create -BanotherBase -t"cool" -b"nah"`)
	if err != nil {
		t.Errorf("got unexpected error %q", err)
	}
}

func TestPRCreate_web(t *testing.T) {
	initBlankContext("OWNER/REPO", "feature")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "forks": { "nodes": [
	] } } } }
	`))

	cs, cmdTeardown := initCmdStubber()
	defer cmdTeardown()

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
	{ "data": { "repository": { "forks": { "nodes": [
	] } } } }
	`))
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": { "pullRequests": { "nodes" : [
		] } } } }
	`))
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "createPullRequest": { "pullRequest": {
			"URL": "https://github.com/OWNER/REPO/pull/12"
		} } } }
	`))

	cs, cmdTeardown := initCmdStubber()
	defer cmdTeardown()

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
		{ "data": { "repository": { "pullRequests": { "nodes" : [
		] } } } }
	`))
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "createPullRequest": { "pullRequest": {
			"URL": "https://github.com/OWNER/REPO/pull/12"
		} } } }
	`))

	cs, cmdTeardown := initCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log
	cs.Stub("")                                         // git push

	output, err := RunCommand(prCreateCmd, `pr create -t "cross repo" -b "same branch"`)
	eq(t, err, nil)

	bodyBytes, _ := ioutil.ReadAll(http.Requests[2].Body)
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

func TestPRCreate_survey_defaults_multicommit(t *testing.T) {
	initBlankContext("OWNER/REPO", "cool_bug-fixes")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "forks": { "nodes": [
	] } } } }
	`))
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": { "pullRequests": { "nodes" : [
		] } } } }
	`))
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "createPullRequest": { "pullRequest": {
			"URL": "https://github.com/OWNER/REPO/pull/12"
		} } } }
	`))

	cs, cmdTeardown := initCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log
	cs.Stub("")                                         // git rev-parse
	cs.Stub("")                                         // git push

	as, surveyTeardown := initAskStubber()
	defer surveyTeardown()

	as.Stub([]*QuestionStub{
		&QuestionStub{
			Name:    "title",
			Default: true,
		},
		&QuestionStub{
			Name:    "body",
			Default: true,
		},
	})
	as.Stub([]*QuestionStub{
		&QuestionStub{
			Name:  "confirmation",
			Value: 1,
		},
	})

	output, err := RunCommand(prCreateCmd, `pr create`)
	eq(t, err, nil)

	bodyBytes, _ := ioutil.ReadAll(http.Requests[3].Body)
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

	expectedBody := "- commit 0\n- commit 1\n"

	eq(t, reqBody.Variables.Input.RepositoryID, "REPOID")
	eq(t, reqBody.Variables.Input.Title, "cool bug fixes")
	eq(t, reqBody.Variables.Input.Body, expectedBody)
	eq(t, reqBody.Variables.Input.BaseRefName, "master")
	eq(t, reqBody.Variables.Input.HeadRefName, "cool_bug-fixes")

	eq(t, output.String(), "https://github.com/OWNER/REPO/pull/12\n")
}

func TestPRCreate_survey_defaults_monocommit(t *testing.T) {
	initBlankContext("OWNER/REPO", "feature")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "forks": { "nodes": [
	] } } } }
	`))
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": { "pullRequests": { "nodes" : [
		] } } } }
	`))
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "createPullRequest": { "pullRequest": {
			"URL": "https://github.com/OWNER/REPO/pull/12"
		} } } }
	`))

	cs, cmdTeardown := initCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                                        // git status
	cs.Stub("1234567890,the sky above the port")                       // git log
	cs.Stub("was the color of a television, turned to a dead channel") // git show
	cs.Stub("")                                                        // git rev-parse
	cs.Stub("")                                                        // git push

	as, surveyTeardown := initAskStubber()
	defer surveyTeardown()

	as.Stub([]*QuestionStub{
		&QuestionStub{
			Name:    "title",
			Default: true,
		},
		&QuestionStub{
			Name:    "body",
			Default: true,
		},
	})
	as.Stub([]*QuestionStub{
		&QuestionStub{
			Name:  "confirmation",
			Value: 1,
		},
	})

	output, err := RunCommand(prCreateCmd, `pr create`)
	eq(t, err, nil)

	bodyBytes, _ := ioutil.ReadAll(http.Requests[3].Body)
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

	expectedBody := "was the color of a television, turned to a dead channel"

	eq(t, reqBody.Variables.Input.RepositoryID, "REPOID")
	eq(t, reqBody.Variables.Input.Title, "the sky above the port")
	eq(t, reqBody.Variables.Input.Body, expectedBody)
	eq(t, reqBody.Variables.Input.BaseRefName, "master")
	eq(t, reqBody.Variables.Input.HeadRefName, "feature")

	eq(t, output.String(), "https://github.com/OWNER/REPO/pull/12\n")
}

func TestPRCreate_survey_autofill(t *testing.T) {
	initBlankContext("OWNER/REPO", "feature")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "forks": { "nodes": [
	] } } } }
	`))
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": { "pullRequests": { "nodes" : [
		] } } } }
	`))
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "createPullRequest": { "pullRequest": {
			"URL": "https://github.com/OWNER/REPO/pull/12"
		} } } }
	`))

	cs, cmdTeardown := initCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                                        // git status
	cs.Stub("1234567890,the sky above the port")                       // git log
	cs.Stub("was the color of a television, turned to a dead channel") // git show
	cs.Stub("")                                                        // git rev-parse
	cs.Stub("")                                                        // git push
	cs.Stub("")                                                        // browser open

	output, err := RunCommand(prCreateCmd, `pr create -f`)
	eq(t, err, nil)

	bodyBytes, _ := ioutil.ReadAll(http.Requests[3].Body)
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

	expectedBody := "was the color of a television, turned to a dead channel"

	eq(t, reqBody.Variables.Input.RepositoryID, "REPOID")
	eq(t, reqBody.Variables.Input.Title, "the sky above the port")
	eq(t, reqBody.Variables.Input.Body, expectedBody)
	eq(t, reqBody.Variables.Input.BaseRefName, "master")
	eq(t, reqBody.Variables.Input.HeadRefName, "feature")

	eq(t, output.String(), "https://github.com/OWNER/REPO/pull/12\n")
}

func TestPRCreate_defaults_error_autofill(t *testing.T) {
	initBlankContext("OWNER/REPO", "feature")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	cs, cmdTeardown := initCmdStubber()
	defer cmdTeardown()

	cs.Stub("") // git status
	cs.Stub("") // git log

	_, err := RunCommand(prCreateCmd, "pr create -f")

	eq(t, err.Error(), "could not compute title or body defaults: could not find any commits between master and feature")
}

func TestPRCreate_defaults_error_web(t *testing.T) {
	initBlankContext("OWNER/REPO", "feature")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	cs, cmdTeardown := initCmdStubber()
	defer cmdTeardown()

	cs.Stub("") // git status
	cs.Stub("") // git log

	_, err := RunCommand(prCreateCmd, "pr create -w")

	eq(t, err.Error(), "could not compute title or body defaults: could not find any commits between master and feature")
}

func TestPRCreate_defaults_error_interactive(t *testing.T) {
	initBlankContext("OWNER/REPO", "feature")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "forks": { "nodes": [
	] } } } }
	`))
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "createPullRequest": { "pullRequest": {
			"URL": "https://github.com/OWNER/REPO/pull/12"
		} } } }
	`))

	cs, cmdTeardown := initCmdStubber()
	defer cmdTeardown()

	cs.Stub("") // git status
	cs.Stub("") // git log
	cs.Stub("") // git rev-parse
	cs.Stub("") // git push
	cs.Stub("") // browser open

	as, surveyTeardown := initAskStubber()
	defer surveyTeardown()

	as.Stub([]*QuestionStub{
		&QuestionStub{
			Name:    "title",
			Default: true,
		},
		&QuestionStub{
			Name:  "body",
			Value: "social distancing",
		},
	})
	as.Stub([]*QuestionStub{
		&QuestionStub{
			Name:  "confirmation",
			Value: 0,
		},
	})

	output, err := RunCommand(prCreateCmd, `pr create`)
	eq(t, err, nil)

	stderr := string(output.Stderr())
	eq(t, strings.Contains(stderr, "warning: could not compute title or body defaults: could not find any commits"), true)
}
