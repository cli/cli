package create

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"reflect"
	"strings"
	"testing"

	"github.com/cli/cli/context"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/cli/cli/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func eq(t *testing.T, got interface{}, expected interface{}) {
	t.Helper()
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("expected: %v, got: %v", expected, got)
	}
}

func runCommand(rt http.RoundTripper, remotes context.Remotes, branch string, isTTY bool, cli string) (*test.CmdOut, error) {
	return runCommandWithRootDirOverridden(rt, remotes, branch, isTTY, cli, "")
}

func runCommandWithRootDirOverridden(rt http.RoundTripper, remotes context.Remotes, branch string, isTTY bool, cli string, rootDir string) (*test.CmdOut, error) {
	io, _, stdout, stderr := iostreams.Test()
	io.SetStdoutTTY(isTTY)
	io.SetStdinTTY(isTTY)
	io.SetStderrTTY(isTTY)

	factory := &cmdutil.Factory{
		IOStreams: io,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: rt}, nil
		},
		Config: func() (config.Config, error) {
			return config.NewBlankConfig(), nil
		},
		Remotes: func() (context.Remotes, error) {
			if remotes != nil {
				return remotes, nil
			}
			return context.Remotes{
				{
					Remote: &git.Remote{Name: "origin"},
					Repo:   ghrepo.New("OWNER", "REPO"),
				},
			}, nil
		},
		Branch: func() (string, error) {
			return branch, nil
		},
	}

	cmd := NewCmdCreate(factory, func(opts *CreateOptions) error {
		opts.RootDirOverride = rootDir
		return createRun(opts)
	})
	cmd.PersistentFlags().StringP("repo", "R", "", "")

	argv, err := shlex.Split(cli)
	if err != nil {
		return nil, err
	}
	cmd.SetArgs(argv)

	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(ioutil.Discard)
	cmd.SetErr(ioutil.Discard)

	_, err = cmd.ExecuteC()
	return &test.CmdOut{
		OutBuf: stdout,
		ErrBuf: stderr,
	}, err
}

func initFakeHTTP() *httpmock.Registry {
	return &httpmock.Registry{}
}

func TestPRCreate_nontty_web(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.StubRepoResponse("OWNER", "REPO")
	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "forks": { "nodes": [
	] } } } }
	`))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                         // git config --get-regexp (determineTrackingBranch)
	cs.Stub("")                                         // git show-ref --verify   (determineTrackingBranch)
	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log
	cs.Stub("")                                         // git push
	cs.Stub("")                                         // browser

	output, err := runCommand(http, nil, "feature", false, `--web`)
	require.NoError(t, err)

	eq(t, output.String(), "")
	eq(t, output.Stderr(), "")

	eq(t, len(cs.Calls), 6)
	eq(t, strings.Join(cs.Calls[4].Args, " "), "git push --set-upstream origin HEAD:feature")
	browserCall := cs.Calls[5].Args
	eq(t, browserCall[len(browserCall)-1], "https://github.com/OWNER/REPO/compare/master...feature?expand=1")

}

func TestPRCreate_nontty_insufficient_flags(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	output, err := runCommand(http, nil, "feature", false, "")
	if err == nil {
		t.Fatal("expected error")
	}

	assert.Equal(t, "--title or --fill required when not attached to a terminal", err.Error())

	assert.Equal(t, "", output.String())
}

func TestPRCreate_nontty(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

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

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                         // git config --get-regexp (determineTrackingBranch)
	cs.Stub("")                                         // git show-ref --verify   (determineTrackingBranch)
	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log
	cs.Stub("")                                         // git push

	output, err := runCommand(http, nil, "feature", false, `-t "my title" -b "my body"`)
	require.NoError(t, err)

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
	_ = json.Unmarshal(bodyBytes, &reqBody)

	assert.Equal(t, "REPOID", reqBody.Variables.Input.RepositoryID)
	assert.Equal(t, "my title", reqBody.Variables.Input.Title)
	assert.Equal(t, "my body", reqBody.Variables.Input.Body)
	assert.Equal(t, "master", reqBody.Variables.Input.BaseRefName)
	assert.Equal(t, "feature", reqBody.Variables.Input.HeadRefName)

	assert.Equal(t, "", output.Stderr())
	assert.Equal(t, "https://github.com/OWNER/REPO/pull/12\n", output.String())
}

func TestPRCreate(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

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

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                         // git config --get-regexp (determineTrackingBranch)
	cs.Stub("")                                         // git show-ref --verify   (determineTrackingBranch)
	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log
	cs.Stub("")                                         // git push

	output, err := runCommand(http, nil, "feature", true, `-t "my title" -b "my body"`)
	require.NoError(t, err)

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
	_ = json.Unmarshal(bodyBytes, &reqBody)

	eq(t, reqBody.Variables.Input.RepositoryID, "REPOID")
	eq(t, reqBody.Variables.Input.Title, "my title")
	eq(t, reqBody.Variables.Input.Body, "my body")
	eq(t, reqBody.Variables.Input.BaseRefName, "master")
	eq(t, reqBody.Variables.Input.HeadRefName, "feature")

	eq(t, output.String(), "https://github.com/OWNER/REPO/pull/12\n")
}
func TestPRCreate_nonLegacyTemplate(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

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

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                         // git config --get-regexp (determineTrackingBranch)
	cs.Stub("")                                         // git show-ref --verify   (determineTrackingBranch)
	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log
	cs.Stub("")                                         // git push

	as, teardown := prompt.InitAskStubber()
	defer teardown()
	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "index",
			Value: 0,
		},
	})
	as.Stub([]*prompt.QuestionStub{
		{
			Name:    "body",
			Default: true,
		},
	})
	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "confirmation",
			Value: 0,
		},
	})

	output, err := runCommandWithRootDirOverridden(http, nil, "feature", true, `-t "my title"`, "./fixtures/repoWithNonLegacyPRTemplates")
	require.NoError(t, err)

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
	_ = json.Unmarshal(bodyBytes, &reqBody)

	eq(t, reqBody.Variables.Input.RepositoryID, "REPOID")
	eq(t, reqBody.Variables.Input.Title, "my title")
	eq(t, reqBody.Variables.Input.Body, "Fixes a bug and Closes an issue")
	eq(t, reqBody.Variables.Input.BaseRefName, "master")
	eq(t, reqBody.Variables.Input.HeadRefName, "feature")

	eq(t, output.String(), "https://github.com/OWNER/REPO/pull/12\n")
}

func TestPRCreate_metadata(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query RepositoryNetwork\b`),
		httpmock.StringResponse(httpmock.RepoNetworkStubResponse("OWNER", "REPO", "master", "WRITE")))
	http.Register(
		httpmock.GraphQL(`query RepositoryFindFork\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "forks": { "nodes": [
		] } } } }
		`))
	http.Register(
		httpmock.GraphQL(`query PullRequestForBranch\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "pullRequests": { "nodes": [
		] } } } }
		`))
	http.Register(
		httpmock.GraphQL(`query RepositoryResolveMetadataIDs\b`),
		httpmock.StringResponse(`
		{ "data": {
			"u000": { "login": "MonaLisa", "id": "MONAID" },
			"u001": { "login": "hubot", "id": "HUBOTID" },
			"repository": {
				"l000": { "name": "bug", "id": "BUGID" },
				"l001": { "name": "TODO", "id": "TODOID" }
			},
			"organization": {
				"t000": { "slug": "core", "id": "COREID" },
				"t001": { "slug": "robots", "id": "ROBOTID" }
			}
		} }
		`))
	http.Register(
		httpmock.GraphQL(`query RepositoryMilestoneList\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "milestones": {
			"nodes": [
				{ "title": "GA", "id": "GAID" },
				{ "title": "Big One.oh", "id": "BIGONEID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	http.Register(
		httpmock.GraphQL(`query RepositoryProjectList\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "projects": {
			"nodes": [
				{ "name": "Cleanup", "id": "CLEANUPID" },
				{ "name": "Roadmap", "id": "ROADMAPID" }
			],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	http.Register(
		httpmock.GraphQL(`query OrganizationProjectList\b`),
		httpmock.StringResponse(`
		{ "data": { "organization": { "projects": {
			"nodes": [],
			"pageInfo": { "hasNextPage": false }
		} } } }
		`))
	http.Register(
		httpmock.GraphQL(`mutation PullRequestCreate\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "createPullRequest": { "pullRequest": {
			"id": "NEWPULLID",
			"URL": "https://github.com/OWNER/REPO/pull/12"
		} } } }
	`, func(inputs map[string]interface{}) {
			eq(t, inputs["title"], "TITLE")
			eq(t, inputs["body"], "BODY")
			if v, ok := inputs["assigneeIds"]; ok {
				t.Errorf("did not expect assigneeIds: %v", v)
			}
			if v, ok := inputs["userIds"]; ok {
				t.Errorf("did not expect userIds: %v", v)
			}
		}))
	http.Register(
		httpmock.GraphQL(`mutation PullRequestCreateMetadata\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "updatePullRequest": {
			"clientMutationId": ""
		} } }
	`, func(inputs map[string]interface{}) {
			eq(t, inputs["pullRequestId"], "NEWPULLID")
			eq(t, inputs["assigneeIds"], []interface{}{"MONAID"})
			eq(t, inputs["labelIds"], []interface{}{"BUGID", "TODOID"})
			eq(t, inputs["projectIds"], []interface{}{"ROADMAPID"})
			eq(t, inputs["milestoneId"], "BIGONEID")
		}))
	http.Register(
		httpmock.GraphQL(`mutation PullRequestCreateRequestReviews\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "requestReviews": {
			"clientMutationId": ""
		} } }
	`, func(inputs map[string]interface{}) {
			eq(t, inputs["pullRequestId"], "NEWPULLID")
			eq(t, inputs["userIds"], []interface{}{"HUBOTID", "MONAID"})
			eq(t, inputs["teamIds"], []interface{}{"COREID", "ROBOTID"})
			eq(t, inputs["union"], true)
		}))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                         // git config --get-regexp (determineTrackingBranch)
	cs.Stub("")                                         // git show-ref --verify   (determineTrackingBranch)
	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log
	cs.Stub("")                                         // git push

	output, err := runCommand(http, nil, "feature", true, `-t TITLE -b BODY -a monalisa -l bug -l todo -p roadmap -m 'big one.oh' -r hubot -r monalisa -r /core -r /robots`)
	eq(t, err, nil)

	eq(t, output.String(), "https://github.com/OWNER/REPO/pull/12\n")
}

func TestPRCreate_withForking(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.StubRepoResponseWithPermission("OWNER", "REPO", "READ")
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": { "forks": { "nodes": [
		] } } } }
	`))
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": { "pullRequests": { "nodes" : [
		] } } } }
	`))
	http.StubResponse(200, bytes.NewBufferString(`
		{ "node_id": "NODEID",
		"name": "REPO",
		"owner": {"login": "myself"},
		"clone_url": "http://example.com",
		"created_at": "2008-02-25T20:21:40Z"
		}
	`))
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "createPullRequest": { "pullRequest": {
			"URL": "https://github.com/OWNER/REPO/pull/12"
		} } } }
	`))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                         // git config --get-regexp (determineTrackingBranch)
	cs.Stub("")                                         // git show-ref --verify   (determineTrackingBranch)
	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log
	cs.Stub("")                                         // git remote add
	cs.Stub("")                                         // git push

	output, err := runCommand(http, nil, "feature", true, `-t title -b body`)
	require.NoError(t, err)

	eq(t, http.Requests[3].URL.Path, "/repos/OWNER/REPO/forks")
	eq(t, output.String(), "https://github.com/OWNER/REPO/pull/12\n")
}

func TestPRCreate_alreadyExists(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

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

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                         // git config --get-regexp (determineTrackingBranch)
	cs.Stub("")                                         // git show-ref --verify   (determineTrackingBranch)
	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log

	_, err := runCommand(http, nil, "feature", true, ``)
	if err == nil {
		t.Fatal("error expected, got nil")
	}
	if err.Error() != "a pull request for branch \"feature\" into branch \"master\" already exists:\nhttps://github.com/OWNER/REPO/pull/123" {
		t.Errorf("got error %q", err)
	}
}

func TestPRCreate_alreadyExistsDifferentBase(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

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
	http.StubResponse(200, bytes.NewBufferString("{}"))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                         // git config --get-regexp (determineTrackingBranch)
	cs.Stub("")                                         // git show-ref --verify   (determineTrackingBranch)
	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log
	cs.Stub("")                                         // git rev-parse

	_, err := runCommand(http, nil, "feature", true, `-BanotherBase -t"cool" -b"nah"`)
	if err != nil {
		t.Errorf("got unexpected error %q", err)
	}
}

func TestPRCreate_web(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.StubRepoResponse("OWNER", "REPO")
	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "forks": { "nodes": [
	] } } } }
	`))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                         // git config --get-regexp (determineTrackingBranch)
	cs.Stub("")                                         // git show-ref --verify   (determineTrackingBranch)
	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log
	cs.Stub("")                                         // git push
	cs.Stub("")                                         // browser

	output, err := runCommand(http, nil, "feature", true, `--web`)
	require.NoError(t, err)

	eq(t, output.String(), "")
	eq(t, output.Stderr(), "Opening github.com/OWNER/REPO/compare/master...feature in your browser.\n")

	eq(t, len(cs.Calls), 6)
	eq(t, strings.Join(cs.Calls[4].Args, " "), "git push --set-upstream origin HEAD:feature")
	browserCall := cs.Calls[5].Args
	eq(t, browserCall[len(browserCall)-1], "https://github.com/OWNER/REPO/compare/master...feature?expand=1")
}

func TestPRCreate_ReportsUncommittedChanges(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

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

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                         // git config --get-regexp (determineTrackingBranch)
	cs.Stub("")                                         // git show-ref --verify   (determineTrackingBranch)
	cs.Stub(" M git/git.go")                            // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log
	cs.Stub("")                                         // git push

	output, err := runCommand(http, nil, "feature", true, `-t "my title" -b "my body"`)
	eq(t, err, nil)

	eq(t, output.String(), "https://github.com/OWNER/REPO/pull/12\n")
	test.ExpectLines(t, output.Stderr(), `Warning: 1 uncommitted change`, `Creating pull request for.*feature.*into.*master.*in OWNER/REPO`)
}

func TestPRCreate_cross_repo_same_branch(t *testing.T) {
	remotes := context.Remotes{
		{
			Remote: &git.Remote{Name: "origin"},
			Repo:   ghrepo.New("OWNER", "REPO"),
		},
		{
			Remote: &git.Remote{Name: "fork"},
			Repo:   ghrepo.New("MYSELF", "REPO"),
		},
	}

	http := initFakeHTTP()
	defer http.Verify(t)

	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repo_000": {
									"id": "REPOID0",
									"name": "REPO",
									"owner": {"login": "OWNER"},
									"defaultBranchRef": {
										"name": "default"
									},
									"viewerPermission": "READ"
								},
								"repo_001" : {
									"parent": {
										"id": "REPOID0",
										"name": "REPO",
										"owner": {"login": "OWNER"},
										"defaultBranchRef": {
											"name": "default"
										},
										"viewerPermission": "READ"
									},
									"id": "REPOID1",
									"name": "REPO",
									"owner": {"login": "MYSELF"},
									"defaultBranchRef": {
										"name": "default"
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

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                         // git config --get-regexp (determineTrackingBranch)
	cs.Stub("")                                         // git show-ref --verify   (determineTrackingBranch)
	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log
	cs.Stub("")                                         // git push

	output, err := runCommand(http, remotes, "default", true, `-t "cross repo" -b "same branch"`)
	require.NoError(t, err)

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
	_ = json.Unmarshal(bodyBytes, &reqBody)

	eq(t, reqBody.Variables.Input.RepositoryID, "REPOID0")
	eq(t, reqBody.Variables.Input.Title, "cross repo")
	eq(t, reqBody.Variables.Input.Body, "same branch")
	eq(t, reqBody.Variables.Input.BaseRefName, "default")
	eq(t, reqBody.Variables.Input.HeadRefName, "MYSELF:default")

	eq(t, output.String(), "https://github.com/OWNER/REPO/pull/12\n")

	// goal: only care that gql is formatted properly
}

func TestPRCreate_survey_defaults_multicommit(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

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

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                         // git config --get-regexp (determineTrackingBranch)
	cs.Stub("")                                         // git show-ref --verify   (determineTrackingBranch)
	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log
	cs.Stub("")                                         // git rev-parse
	cs.Stub("")                                         // git push

	as, surveyTeardown := prompt.InitAskStubber()
	defer surveyTeardown()

	as.Stub([]*prompt.QuestionStub{
		{
			Name:    "title",
			Default: true,
		},
		{
			Name:    "body",
			Default: true,
		},
	})
	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "confirmation",
			Value: 0,
		},
	})

	output, err := runCommand(http, nil, "cool_bug-fixes", true, ``)
	require.NoError(t, err)

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
	_ = json.Unmarshal(bodyBytes, &reqBody)

	expectedBody := "- commit 1\n- commit 0\n"

	eq(t, reqBody.Variables.Input.RepositoryID, "REPOID")
	eq(t, reqBody.Variables.Input.Title, "cool bug fixes")
	eq(t, reqBody.Variables.Input.Body, expectedBody)
	eq(t, reqBody.Variables.Input.BaseRefName, "master")
	eq(t, reqBody.Variables.Input.HeadRefName, "cool_bug-fixes")

	eq(t, output.String(), "https://github.com/OWNER/REPO/pull/12\n")
}

func TestPRCreate_survey_defaults_monocommit(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.Register(httpmock.GraphQL(`query RepositoryNetwork\b`), httpmock.StringResponse(httpmock.RepoNetworkStubResponse("OWNER", "REPO", "master", "WRITE")))
	http.Register(httpmock.GraphQL(`query RepositoryFindFork\b`), httpmock.StringResponse(`
		{ "data": { "repository": { "forks": { "nodes": [
		] } } } }
	`))
	http.Register(httpmock.GraphQL(`query PullRequestForBranch\b`), httpmock.StringResponse(`
		{ "data": { "repository": { "pullRequests": { "nodes" : [
		] } } } }
	`))
	http.Register(httpmock.GraphQL(`mutation PullRequestCreate\b`), httpmock.GraphQLMutation(`
		{ "data": { "createPullRequest": { "pullRequest": {
			"URL": "https://github.com/OWNER/REPO/pull/12"
		} } } }
	`, func(inputs map[string]interface{}) {
		eq(t, inputs["repositoryId"], "REPOID")
		eq(t, inputs["title"], "the sky above the port")
		eq(t, inputs["body"], "was the color of a television, turned to a dead channel")
		eq(t, inputs["baseRefName"], "master")
		eq(t, inputs["headRefName"], "feature")
	}))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                                        // git config --get-regexp (determineTrackingBranch)
	cs.Stub("")                                                        // git show-ref --verify   (determineTrackingBranch)
	cs.Stub("")                                                        // git status
	cs.Stub("1234567890,the sky above the port")                       // git log
	cs.Stub("was the color of a television, turned to a dead channel") // git show
	cs.Stub("")                                                        // git rev-parse
	cs.Stub("")                                                        // git push

	as, surveyTeardown := prompt.InitAskStubber()
	defer surveyTeardown()

	as.Stub([]*prompt.QuestionStub{
		{
			Name:    "title",
			Default: true,
		},
		{
			Name:    "body",
			Default: true,
		},
	})
	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "confirmation",
			Value: 0,
		},
	})

	output, err := runCommand(http, nil, "feature", true, ``)
	eq(t, err, nil)
	eq(t, output.String(), "https://github.com/OWNER/REPO/pull/12\n")
}

func TestPRCreate_survey_defaults_monocommit_template(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.Register(httpmock.GraphQL(`query RepositoryNetwork\b`), httpmock.StringResponse(httpmock.RepoNetworkStubResponse("OWNER", "REPO", "master", "WRITE")))
	http.Register(httpmock.GraphQL(`query RepositoryFindFork\b`), httpmock.StringResponse(`
		{ "data": { "repository": { "forks": { "nodes": [
		] } } } }
	`))
	http.Register(httpmock.GraphQL(`query PullRequestForBranch\b`), httpmock.StringResponse(`
		{ "data": { "repository": { "pullRequests": { "nodes" : [
		] } } } }
	`))
	http.Register(httpmock.GraphQL(`mutation PullRequestCreate\b`), httpmock.GraphQLMutation(`
		{ "data": { "createPullRequest": { "pullRequest": {
			"URL": "https://github.com/OWNER/REPO/pull/12"
		} } } }
	`, func(inputs map[string]interface{}) {
		eq(t, inputs["repositoryId"], "REPOID")
		eq(t, inputs["title"], "the sky above the port")
		eq(t, inputs["body"], "was the color of a television\n\n... turned to a dead channel")
		eq(t, inputs["baseRefName"], "master")
		eq(t, inputs["headRefName"], "feature")
	}))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	tmpdir, err := ioutil.TempDir("", "gh-cli")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	templateFp := path.Join(tmpdir, ".github/PULL_REQUEST_TEMPLATE.md")
	_ = os.MkdirAll(path.Dir(templateFp), 0700)
	ioutil.WriteFile(templateFp, []byte("... turned to a dead channel"), 0700)

	cs.Stub("")                                  // git config --get-regexp (determineTrackingBranch)
	cs.Stub("")                                  // git show-ref --verify   (determineTrackingBranch)
	cs.Stub("")                                  // git status
	cs.Stub("1234567890,the sky above the port") // git log
	cs.Stub("was the color of a television")     // git show
	cs.Stub(tmpdir)                              // git rev-parse
	cs.Stub("")                                  // git push

	as, surveyTeardown := prompt.InitAskStubber()
	defer surveyTeardown()

	as.Stub([]*prompt.QuestionStub{
		{
			Name:    "title",
			Default: true,
		},
		{
			Name:    "body",
			Default: true,
		},
	})
	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "confirmation",
			Value: 0,
		},
	})

	output, err := runCommand(http, nil, "feature", true, ``)
	eq(t, err, nil)
	eq(t, output.String(), "https://github.com/OWNER/REPO/pull/12\n")
}

func TestPRCreate_survey_autofill_nontty(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

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

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                                        // git config --get-regexp (determineTrackingBranch)
	cs.Stub("")                                                        // git show-ref --verify   (determineTrackingBranch)
	cs.Stub("")                                                        // git status
	cs.Stub("1234567890,the sky above the port")                       // git log
	cs.Stub("was the color of a television, turned to a dead channel") // git show
	cs.Stub("")                                                        // git rev-parse
	cs.Stub("")                                                        // git push
	cs.Stub("")                                                        // browser open

	output, err := runCommand(http, nil, "feature", false, `-f`)
	require.NoError(t, err)

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
	_ = json.Unmarshal(bodyBytes, &reqBody)

	expectedBody := "was the color of a television, turned to a dead channel"

	assert.Equal(t, "REPOID", reqBody.Variables.Input.RepositoryID)
	assert.Equal(t, "the sky above the port", reqBody.Variables.Input.Title)
	assert.Equal(t, expectedBody, reqBody.Variables.Input.Body)
	assert.Equal(t, "master", reqBody.Variables.Input.BaseRefName)
	assert.Equal(t, "feature", reqBody.Variables.Input.HeadRefName)

	assert.Equal(t, "https://github.com/OWNER/REPO/pull/12\n", output.String())

	assert.Equal(t, "", output.Stderr())
}

func TestPRCreate_survey_autofill(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

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

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                                        // git config --get-regexp (determineTrackingBranch)
	cs.Stub("")                                                        // git show-ref --verify   (determineTrackingBranch)
	cs.Stub("")                                                        // git status
	cs.Stub("1234567890,the sky above the port")                       // git log
	cs.Stub("was the color of a television, turned to a dead channel") // git show
	cs.Stub("")                                                        // git rev-parse
	cs.Stub("")                                                        // git push
	cs.Stub("")                                                        // browser open

	output, err := runCommand(http, nil, "feature", true, `-f`)
	require.NoError(t, err)

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
	_ = json.Unmarshal(bodyBytes, &reqBody)

	expectedBody := "was the color of a television, turned to a dead channel"

	eq(t, reqBody.Variables.Input.RepositoryID, "REPOID")
	eq(t, reqBody.Variables.Input.Title, "the sky above the port")
	eq(t, reqBody.Variables.Input.Body, expectedBody)
	eq(t, reqBody.Variables.Input.BaseRefName, "master")
	eq(t, reqBody.Variables.Input.HeadRefName, "feature")

	eq(t, output.String(), "https://github.com/OWNER/REPO/pull/12\n")
}

func TestPRCreate_defaults_error_autofill(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.StubRepoResponse("OWNER", "REPO")

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("") // git config --get-regexp (determineTrackingBranch)
	cs.Stub("") // git show-ref --verify   (determineTrackingBranch)
	cs.Stub("") // git status
	cs.Stub("") // git log

	_, err := runCommand(http, nil, "feature", true, "-f")

	eq(t, err.Error(), "could not compute title or body defaults: could not find any commits between origin/master and feature")
}

func TestPRCreate_defaults_error_web(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.StubRepoResponse("OWNER", "REPO")

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("") // git config --get-regexp (determineTrackingBranch)
	cs.Stub("") // git show-ref --verify   (determineTrackingBranch)
	cs.Stub("") // git status
	cs.Stub("") // git log

	_, err := runCommand(http, nil, "feature", true, "-w")

	eq(t, err.Error(), "could not compute title or body defaults: could not find any commits between origin/master and feature")
}

func TestPRCreate_defaults_error_interactive(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

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

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("") // git config --get-regexp (determineTrackingBranch)
	cs.Stub("") // git show-ref --verify   (determineTrackingBranch)
	cs.Stub("") // git status
	cs.Stub("") // git log
	cs.Stub("") // git rev-parse
	cs.Stub("") // git push
	cs.Stub("") // browser open

	as, surveyTeardown := prompt.InitAskStubber()
	defer surveyTeardown()

	as.Stub([]*prompt.QuestionStub{
		{
			Name:    "title",
			Default: true,
		},
		{
			Name:  "body",
			Value: "social distancing",
		},
	})
	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "confirmation",
			Value: 1,
		},
	})

	output, err := runCommand(http, nil, "feature", true, ``)
	require.NoError(t, err)

	stderr := string(output.Stderr())
	eq(t, strings.Contains(stderr, "warning: could not compute title or body defaults: could not find any commits"), true)
}

func Test_determineTrackingBranch_empty(t *testing.T) {
	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	remotes := context.Remotes{}

	cs.Stub("")              // git config --get-regexp (ReadBranchConfig)
	cs.Stub("deadbeef HEAD") // git show-ref --verify   (ShowRefs)

	ref := determineTrackingBranch(remotes, "feature")
	if ref != nil {
		t.Errorf("expected nil result, got %v", ref)
	}
}

func Test_determineTrackingBranch_noMatch(t *testing.T) {
	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	remotes := context.Remotes{
		&context.Remote{
			Remote: &git.Remote{Name: "origin"},
			Repo:   ghrepo.New("hubot", "Spoon-Knife"),
		},
		&context.Remote{
			Remote: &git.Remote{Name: "upstream"},
			Repo:   ghrepo.New("octocat", "Spoon-Knife"),
		},
	}

	cs.Stub("") // git config --get-regexp (ReadBranchConfig)
	cs.Stub(`deadbeef HEAD
deadb00f refs/remotes/origin/feature`) // git show-ref --verify (ShowRefs)

	ref := determineTrackingBranch(remotes, "feature")
	if ref != nil {
		t.Errorf("expected nil result, got %v", ref)
	}
}

func Test_determineTrackingBranch_hasMatch(t *testing.T) {
	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	remotes := context.Remotes{
		&context.Remote{
			Remote: &git.Remote{Name: "origin"},
			Repo:   ghrepo.New("hubot", "Spoon-Knife"),
		},
		&context.Remote{
			Remote: &git.Remote{Name: "upstream"},
			Repo:   ghrepo.New("octocat", "Spoon-Knife"),
		},
	}

	cs.Stub("") // git config --get-regexp (ReadBranchConfig)
	cs.Stub(`deadbeef HEAD
deadb00f refs/remotes/origin/feature
deadbeef refs/remotes/upstream/feature`) // git show-ref --verify (ShowRefs)

	ref := determineTrackingBranch(remotes, "feature")
	if ref == nil {
		t.Fatal("expected result, got nil")
	}

	eq(t, cs.Calls[1].Args, []string{"git", "show-ref", "--verify", "--", "HEAD", "refs/remotes/origin/feature", "refs/remotes/upstream/feature"})

	eq(t, ref.RemoteName, "upstream")
	eq(t, ref.BranchName, "feature")
}

func Test_determineTrackingBranch_respectTrackingConfig(t *testing.T) {
	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	remotes := context.Remotes{
		&context.Remote{
			Remote: &git.Remote{Name: "origin"},
			Repo:   ghrepo.New("hubot", "Spoon-Knife"),
		},
	}

	cs.Stub(`branch.feature.remote origin
branch.feature.merge refs/heads/great-feat`) // git config --get-regexp (ReadBranchConfig)
	cs.Stub(`deadbeef HEAD
deadb00f refs/remotes/origin/feature`) // git show-ref --verify (ShowRefs)

	ref := determineTrackingBranch(remotes, "feature")
	if ref != nil {
		t.Errorf("expected nil result, got %v", ref)
	}

	eq(t, cs.Calls[1].Args, []string{"git", "show-ref", "--verify", "--", "HEAD", "refs/remotes/origin/great-feat", "refs/remotes/origin/feature"})
}
