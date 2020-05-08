package command

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"testing"
)

func TestPRReview_validation(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	for _, cmd := range []string{
		`pr review 123`,
		`pr review --approve --comment 123`,
		`pr review --approve --comment -b"hey" 123`,
	} {
		http.StubRepoResponse("OWNER", "REPO")
		_, err := RunCommand(cmd)
		eq(t, err.Error(), "did not understand desired review action: need exactly one of --approve, --request-changes, or --comment")
	}
}

func TestPRReview_url_arg(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": { "pullRequest": {
			"id": "foobar123",
			"number": 123,
			"headRefName": "feature",
			"headRepositoryOwner": {
				"login": "hubot"
			},
			"headRepository": {
				"name": "REPO",
				"defaultBranchRef": {
					"name": "master"
				}
			},
			"isCrossRepository": false,
			"maintainerCanModify": false
		} } } } `))
	http.StubResponse(200, bytes.NewBufferString(`{"data": {} }`))

	_, err := RunCommand("pr review --approve https://github.com/OWNER/REPO/pull/123")
	if err != nil {
		t.Fatalf("error running pr review: %s", err)
	}

	bodyBytes, _ := ioutil.ReadAll(http.Requests[2].Body)
	reqBody := struct {
		Variables struct {
			Input struct {
				PullRequestID string
				Event         string
				Body          string
			}
		}
	}{}
	_ = json.Unmarshal(bodyBytes, &reqBody)

	eq(t, reqBody.Variables.Input.PullRequestID, "foobar123")
	eq(t, reqBody.Variables.Input.Event, "APPROVE")
	eq(t, reqBody.Variables.Input.Body, "")
}

func TestPRReview_number_arg(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": { "pullRequest": {
			"id": "foobar123",
			"number": 123,
			"headRefName": "feature",
			"headRepositoryOwner": {
				"login": "hubot"
			},
			"headRepository": {
				"name": "REPO",
				"defaultBranchRef": {
					"name": "master"
				}
			},
			"isCrossRepository": false,
			"maintainerCanModify": false
		} } } } `))
	http.StubResponse(200, bytes.NewBufferString(`{"data": {} }`))

	_, err := RunCommand("pr review --approve 123")
	if err != nil {
		t.Fatalf("error running pr review: %s", err)
	}

	bodyBytes, _ := ioutil.ReadAll(http.Requests[2].Body)
	reqBody := struct {
		Variables struct {
			Input struct {
				PullRequestID string
				Event         string
				Body          string
			}
		}
	}{}
	_ = json.Unmarshal(bodyBytes, &reqBody)

	eq(t, reqBody.Variables.Input.PullRequestID, "foobar123")
	eq(t, reqBody.Variables.Input.Event, "APPROVE")
	eq(t, reqBody.Variables.Input.Body, "")
}

func TestPRReview_no_arg(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "feature")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": { "pullRequests": { "nodes": [
			{ "url": "https://github.com/OWNER/REPO/pull/123",
			  "id": "foobar123",
			  "headRefName": "feature",
				"baseRefName": "master" }
		] } } } }`))
	http.StubResponse(200, bytes.NewBufferString(`{"data": {} }`))

	_, err := RunCommand(`pr review --comment -b "cool story"`)
	if err != nil {
		t.Fatalf("error running pr review: %s", err)
	}

	bodyBytes, _ := ioutil.ReadAll(http.Requests[2].Body)
	reqBody := struct {
		Variables struct {
			Input struct {
				PullRequestID string
				Event         string
				Body          string
			}
		}
	}{}
	_ = json.Unmarshal(bodyBytes, &reqBody)

	eq(t, reqBody.Variables.Input.PullRequestID, "foobar123")
	eq(t, reqBody.Variables.Input.Event, "COMMENT")
	eq(t, reqBody.Variables.Input.Body, "cool story")
}

func TestPRReview_blank_comment(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	_, err := RunCommand(`pr review --comment 123`)
	eq(t, err.Error(), "did not understand desired review action: body cannot be blank for comment review")
}

func TestPRReview_blank_request_changes(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	_, err := RunCommand(`pr review -r 123`)
	eq(t, err.Error(), "did not understand desired review action: body cannot be blank for request-changes review")
}

func TestPRReview(t *testing.T) {
	type c struct {
		Cmd           string
		ExpectedEvent string
		ExpectedBody  string
	}
	cases := []c{
		c{`pr review --request-changes -b"bad"`, "REQUEST_CHANGES", "bad"},
		c{`pr review --approve`, "APPROVE", ""},
		c{`pr review --approve -b"hot damn"`, "APPROVE", "hot damn"},
		c{`pr review --comment --body "i donno"`, "COMMENT", "i donno"},
	}

	for _, kase := range cases {
		initBlankContext("", "OWNER/REPO", "feature")
		http := initFakeHTTP()
		http.StubRepoResponse("OWNER", "REPO")
		http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": { "pullRequests": { "nodes": [
			{ "url": "https://github.com/OWNER/REPO/pull/123",
			  "id": "foobar123",
			  "headRefName": "feature",
				"baseRefName": "master" }
		] } } } }
	`))
		http.StubResponse(200, bytes.NewBufferString(`{"data": {} }`))

		_, err := RunCommand(kase.Cmd)
		if err != nil {
			t.Fatalf("got unexpected error running %s: %s", kase.Cmd, err)
		}

		bodyBytes, _ := ioutil.ReadAll(http.Requests[2].Body)
		reqBody := struct {
			Variables struct {
				Input struct {
					Event string
					Body  string
				}
			}
		}{}
		_ = json.Unmarshal(bodyBytes, &reqBody)

		eq(t, reqBody.Variables.Input.Event, kase.ExpectedEvent)
		eq(t, reqBody.Variables.Input.Body, kase.ExpectedBody)
	}
}
