package create

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
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
					Remote: &git.Remote{
						Name:     "origin",
						Resolved: "base",
					},
					Repo: ghrepo.New("OWNER", "REPO"),
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

	http.StubRepoInfoResponse("OWNER", "REPO", "master")

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log
	cs.Stub("")                                         // browser

	output, err := runCommand(http, nil, "feature", false, `--web --head=feature`)
	require.NoError(t, err)

	eq(t, output.String(), "")
	eq(t, output.Stderr(), "")

	eq(t, len(cs.Calls), 3)
	browserCall := cs.Calls[2].Args
	eq(t, browserCall[len(browserCall)-1], "https://github.com/OWNER/REPO/compare/master...feature?expand=1")

}

func TestPRCreate_nontty_insufficient_flags(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	output, err := runCommand(http, nil, "feature", false, "")
	if err == nil {
		t.Fatal("expected error")
	}

	assert.Equal(t, "--title or --fill required when not running interactively", err.Error())

	assert.Equal(t, "", output.String())
}

func TestPRCreate_nontty(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.StubRepoInfoResponse("OWNER", "REPO", "master")
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

	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log

	output, err := runCommand(http, nil, "feature", false, `-t "my title" -b "my body" -H feature`)
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

	http.StubRepoInfoResponse("OWNER", "REPO", "master")
	http.StubRepoResponse("OWNER", "REPO")
	http.Register(
		httpmock.GraphQL(`query UserCurrent\b`),
		httpmock.StringResponse(`{"data": {"viewer": {"login": "OWNER"} } }`))
	http.Register(
		httpmock.GraphQL(`query PullRequestForBranch\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "pullRequests": { "nodes" : [
		] } } } }
		`))
	http.Register(
		httpmock.GraphQL(`mutation PullRequestCreate\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "createPullRequest": { "pullRequest": {
			"URL": "https://github.com/OWNER/REPO/pull/12"
		} } } }
		`, func(input map[string]interface{}) {
			assert.Equal(t, "REPOID", input["repositoryId"].(string))
			assert.Equal(t, "my title", input["title"].(string))
			assert.Equal(t, "my body", input["body"].(string))
			assert.Equal(t, "master", input["baseRefName"].(string))
			assert.Equal(t, "feature", input["headRefName"].(string))
		}))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                         // git config --get-regexp (determineTrackingBranch)
	cs.Stub("")                                         // git show-ref --verify   (determineTrackingBranch)
	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log
	cs.Stub("")                                         // git push

	ask, cleanupAsk := prompt.InitAskStubber()
	defer cleanupAsk()
	ask.StubOne(0)

	output, err := runCommand(http, nil, "feature", true, `-t "my title" -b "my body"`)
	require.NoError(t, err)

	assert.Equal(t, "https://github.com/OWNER/REPO/pull/12\n", output.String())
	assert.Equal(t, "\nCreating pull request for feature into master in OWNER/REPO\n\n", output.Stderr())
}

func TestPRCreate_createFork(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.StubRepoInfoResponse("OWNER", "REPO", "master")
	http.StubRepoResponse("OWNER", "REPO")
	http.Register(
		httpmock.GraphQL(`query UserCurrent\b`),
		httpmock.StringResponse(`{"data": {"viewer": {"login": "monalisa"} } }`))
	http.Register(
		httpmock.GraphQL(`query PullRequestForBranch\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "pullRequests": { "nodes" : [
		] } } } }
		`))
	http.Register(
		httpmock.REST("POST", "repos/OWNER/REPO/forks"),
		httpmock.StatusStringResponse(201, `
		{ "node_id": "NODEID",
		  "name": "REPO",
		  "owner": {"login": "monalisa"}
		}
		`))
	http.Register(
		httpmock.GraphQL(`mutation PullRequestCreate\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "createPullRequest": { "pullRequest": {
			"URL": "https://github.com/OWNER/REPO/pull/12"
		} } } }
		`, func(input map[string]interface{}) {
			assert.Equal(t, "REPOID", input["repositoryId"].(string))
			assert.Equal(t, "master", input["baseRefName"].(string))
			assert.Equal(t, "monalisa:feature", input["headRefName"].(string))
		}))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                         // git config --get-regexp (determineTrackingBranch)
	cs.Stub("")                                         // git show-ref --verify   (determineTrackingBranch)
	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log
	cs.Stub("")                                         // git remote add
	cs.Stub("")                                         // git push

	ask, cleanupAsk := prompt.InitAskStubber()
	defer cleanupAsk()
	ask.StubOne(1)

	output, err := runCommand(http, nil, "feature", true, `-t title -b body`)
	require.NoError(t, err)

	assert.Equal(t, []string{"git", "remote", "add", "-f", "fork", "https://github.com/monalisa/REPO.git"}, cs.Calls[4].Args)
	assert.Equal(t, []string{"git", "push", "--set-upstream", "fork", "HEAD:feature"}, cs.Calls[5].Args)

	assert.Equal(t, "https://github.com/OWNER/REPO/pull/12\n", output.String())
}

func TestPRCreate_nonLegacyTemplate(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.StubRepoInfoResponse("OWNER", "REPO", "master")
	http.Register(
		httpmock.GraphQL(`query PullRequestForBranch\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "pullRequests": { "nodes" : [
		] } } } }
		`))
	http.Register(
		httpmock.GraphQL(`mutation PullRequestCreate\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "createPullRequest": { "pullRequest": {
			"URL": "https://github.com/OWNER/REPO/pull/12"
		} } } }
		`, func(input map[string]interface{}) {
			assert.Equal(t, "my title", input["title"].(string))
			assert.Equal(t, "- commit 1\n- commit 0\n\nFixes a bug and Closes an issue", input["body"].(string))
		}))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log

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

	output, err := runCommandWithRootDirOverridden(http, nil, "feature", true, `-t "my title" -H feature`, "./fixtures/repoWithNonLegacyPRTemplates")
	require.NoError(t, err)

	eq(t, output.String(), "https://github.com/OWNER/REPO/pull/12\n")
}

func TestPRCreate_metadata(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.StubRepoInfoResponse("OWNER", "REPO", "master")
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

	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log

	output, err := runCommand(http, nil, "feature", true, `-t TITLE -b BODY -H feature -a monalisa -l bug -l todo -p roadmap -m 'big one.oh' -r hubot -r monalisa -r /core -r /robots`)
	eq(t, err, nil)

	eq(t, output.String(), "https://github.com/OWNER/REPO/pull/12\n")
}

func TestPRCreate_alreadyExists(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.StubRepoInfoResponse("OWNER", "REPO", "master")
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

	_, err := runCommand(http, nil, "feature", true, `-H feature`)
	if err == nil {
		t.Fatal("error expected, got nil")
	}
	if err.Error() != "a pull request for branch \"feature\" into branch \"master\" already exists:\nhttps://github.com/OWNER/REPO/pull/123" {
		t.Errorf("got error %q", err)
	}
}

func TestPRCreate_web(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.StubRepoInfoResponse("OWNER", "REPO", "master")
	http.StubRepoResponse("OWNER", "REPO")
	http.Register(
		httpmock.GraphQL(`query UserCurrent\b`),
		httpmock.StringResponse(`{"data": {"viewer": {"login": "OWNER"} } }`))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                         // git config --get-regexp (determineTrackingBranch)
	cs.Stub("")                                         // git show-ref --verify   (determineTrackingBranch)
	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log
	cs.Stub("")                                         // git push
	cs.Stub("")                                         // browser

	ask, cleanupAsk := prompt.InitAskStubber()
	defer cleanupAsk()
	ask.StubOne(0)

	output, err := runCommand(http, nil, "feature", true, `--web`)
	require.NoError(t, err)

	eq(t, output.String(), "")
	eq(t, output.Stderr(), "Opening github.com/OWNER/REPO/compare/master...feature in your browser.\n")

	eq(t, len(cs.Calls), 6)
	eq(t, strings.Join(cs.Calls[4].Args, " "), "git push --set-upstream origin HEAD:feature")
	browserCall := cs.Calls[5].Args
	eq(t, browserCall[len(browserCall)-1], "https://github.com/OWNER/REPO/compare/master...feature?expand=1")
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
