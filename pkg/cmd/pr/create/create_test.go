package create

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/context"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/internal/run"
	prShared "github.com/cli/cli/pkg/cmd/pr/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/cli/cli/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	//nolint:staticcheck // SA1019 TODO: rewrite to use run.Stub
	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log
	cs.Stub("")                                         // browser

	output, err := runCommand(http, nil, "feature", false, `--web --head=feature`)
	require.NoError(t, err)

	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())

	assert.Equal(t, 3, len(cs.Calls))
	browserCall := cs.Calls[2].Args
	assert.Equal(t, "https://github.com/OWNER/REPO/compare/master...feature?expand=1", browserCall[len(browserCall)-1])

}

func TestPRCreate_nontty_insufficient_flags(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	output, err := runCommand(http, nil, "feature", false, "")
	assert.EqualError(t, err, "--title or --fill required when not running interactively")

	assert.Equal(t, "", output.String())
}

func TestPRCreate_recover(t *testing.T) {
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
		httpmock.GraphQL(`query RepositoryResolveMetadataIDs\b`),
		httpmock.StringResponse(`
		{ "data": {
			"u000": { "login": "jillValentine", "id": "JILLID" },
			"repository": {},
			"organization": {}
		} }
		`))
	http.Register(
		httpmock.GraphQL(`mutation PullRequestCreateRequestReviews\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "requestReviews": {
			"clientMutationId": ""
		} } }
	`, func(inputs map[string]interface{}) {
			assert.Equal(t, []interface{}{"JILLID"}, inputs["userIds"])
		}))
	http.Register(
		httpmock.GraphQL(`mutation PullRequestCreate\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "createPullRequest": { "pullRequest": {
			"URL": "https://github.com/OWNER/REPO/pull/12"
		} } } }
		`, func(input map[string]interface{}) {
			assert.Equal(t, "recovered title", input["title"].(string))
			assert.Equal(t, "recovered body", input["body"].(string))
		}))

	//nolint:staticcheck // SA1019 TODO: rewrite to use run.Stub
	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log

	as, teardown := prompt.InitAskStubber()
	defer teardown()
	as.Stub([]*prompt.QuestionStub{
		{
			Name:    "Title",
			Default: true,
		},
	})
	as.Stub([]*prompt.QuestionStub{
		{
			Name:    "Body",
			Default: true,
		},
	})
	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "confirmation",
			Value: 0,
		},
	})

	tmpfile, err := ioutil.TempFile(os.TempDir(), "testrecover*")
	assert.NoError(t, err)

	state := prShared.IssueMetadataState{
		Title:     "recovered title",
		Body:      "recovered body",
		Reviewers: []string{"jillValentine"},
	}

	data, err := json.Marshal(state)
	assert.NoError(t, err)

	_, err = tmpfile.Write(data)
	assert.NoError(t, err)

	args := fmt.Sprintf("--recover '%s' -Hfeature", tmpfile.Name())

	output, err := runCommandWithRootDirOverridden(http, nil, "feature", true, args, "")
	assert.NoError(t, err)

	assert.Equal(t, "https://github.com/OWNER/REPO/pull/12\n", output.String())
}

func TestPRCreate_nontty(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.StubRepoInfoResponse("OWNER", "REPO", "master")
	http.Register(
		httpmock.GraphQL(`query PullRequestForBranch\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": { "pullRequests": { "nodes" : [
			] } } } }`),
	)
	http.Register(
		httpmock.GraphQL(`mutation PullRequestCreate\b`),
		httpmock.GraphQLMutation(`
			{ "data": { "createPullRequest": { "pullRequest": {
				"URL": "https://github.com/OWNER/REPO/pull/12"
			} } } }`,
			func(input map[string]interface{}) {
				assert.Equal(t, "REPOID", input["repositoryId"])
				assert.Equal(t, "my title", input["title"])
				assert.Equal(t, "my body", input["body"])
				assert.Equal(t, "master", input["baseRefName"])
				assert.Equal(t, "feature", input["headRefName"])
			}),
	)

	//nolint:staticcheck // SA1019 TODO: rewrite to use run.Stub
	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log

	output, err := runCommand(http, nil, "feature", false, `-t "my title" -b "my body" -H feature`)
	require.NoError(t, err)

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

	//nolint:staticcheck // SA1019 TODO: rewrite to use run.Stub
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

func TestPRCreate_NoMaintainerModify(t *testing.T) {
	// TODO update this copypasta
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
			assert.Equal(t, false, input["maintainerCanModify"].(bool))
			assert.Equal(t, "REPOID", input["repositoryId"].(string))
			assert.Equal(t, "my title", input["title"].(string))
			assert.Equal(t, "my body", input["body"].(string))
			assert.Equal(t, "master", input["baseRefName"].(string))
			assert.Equal(t, "feature", input["headRefName"].(string))
		}))

	//nolint:staticcheck // SA1019 TODO: rewrite to use run.Stub
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

	output, err := runCommand(http, nil, "feature", true, `-t "my title" -b "my body" --no-maintainer-edit`)
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

	//nolint:staticcheck // SA1019 TODO: rewrite to use run.Stub
	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("") // git config --get-regexp (determineTrackingBranch)
	cs.Stub("") // git show-ref --verify   (determineTrackingBranch)
	cs.Stub("") // git status
	cs.Stub("") // git remote add
	cs.Stub("") // git push

	ask, cleanupAsk := prompt.InitAskStubber()
	defer cleanupAsk()
	ask.StubOne(1)

	output, err := runCommand(http, nil, "feature", true, `-t title -b body`)
	require.NoError(t, err)

	assert.Equal(t, []string{"git", "remote", "add", "-f", "fork", "https://github.com/monalisa/REPO.git"}, cs.Calls[3].Args)
	assert.Equal(t, []string{"git", "push", "--set-upstream", "fork", "HEAD:feature"}, cs.Calls[4].Args)

	assert.Equal(t, "https://github.com/OWNER/REPO/pull/12\n", output.String())
}

func TestPRCreate_pushedToNonBaseRepo(t *testing.T) {
	remotes := context.Remotes{
		{
			Remote: &git.Remote{
				Name:     "upstream",
				Resolved: "base",
			},
			Repo: ghrepo.New("OWNER", "REPO"),
		},
		{
			Remote: &git.Remote{
				Name:     "origin",
				Resolved: "base",
			},
			Repo: ghrepo.New("monalisa", "REPO"),
		},
	}

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
			assert.Equal(t, "REPOID", input["repositoryId"].(string))
			assert.Equal(t, "master", input["baseRefName"].(string))
			assert.Equal(t, "monalisa:feature", input["headRefName"].(string))
		}))

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register("git status", 0, "")
	cs.Register(`git config --get-regexp \^branch\\\.feature\\\.`, 1, "") // determineTrackingBranch
	cs.Register("git show-ref --verify", 0, heredoc.Doc(`
		deadbeef HEAD
		deadb00f refs/remotes/upstream/feature
		deadbeef refs/remotes/origin/feature
	`)) // determineTrackingBranch

	_, cleanupAsk := prompt.InitAskStubber()
	defer cleanupAsk()

	output, err := runCommand(http, remotes, "feature", true, `-t title -b body`)
	require.NoError(t, err)

	assert.Equal(t, "\nCreating pull request for monalisa:feature into master in OWNER/REPO\n\n", output.Stderr())
	assert.Equal(t, "https://github.com/OWNER/REPO/pull/12\n", output.String())
}

func TestPRCreate_pushedToDifferentBranchName(t *testing.T) {
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
			assert.Equal(t, "REPOID", input["repositoryId"].(string))
			assert.Equal(t, "master", input["baseRefName"].(string))
			assert.Equal(t, "my-feat2", input["headRefName"].(string))
		}))

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register("git status", 0, "")
	cs.Register(`git config --get-regexp \^branch\\\.feature\\\.`, 0, heredoc.Doc(`
		branch.feature.remote origin
		branch.feature.merge refs/heads/my-feat2
	`)) // determineTrackingBranch
	cs.Register("git show-ref --verify", 0, heredoc.Doc(`
		deadbeef HEAD
		deadbeef refs/remotes/origin/my-feat2
	`)) // determineTrackingBranch

	_, cleanupAsk := prompt.InitAskStubber()
	defer cleanupAsk()

	output, err := runCommand(http, nil, "feature", true, `-t title -b body`)
	require.NoError(t, err)

	assert.Equal(t, "\nCreating pull request for my-feat2 into master in OWNER/REPO\n\n", output.Stderr())
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

	//nolint:staticcheck // SA1019 TODO: rewrite to use run.Stub
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
	}) // template
	as.Stub([]*prompt.QuestionStub{
		{
			Name:    "Body",
			Default: true,
		},
	}) // body
	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "confirmation",
			Value: 0,
		},
	}) // confirm

	output, err := runCommandWithRootDirOverridden(http, nil, "feature", true, `-t "my title" -H feature`, "./fixtures/repoWithNonLegacyPRTemplates")
	require.NoError(t, err)

	assert.Equal(t, "https://github.com/OWNER/REPO/pull/12\n", output.String())
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
			assert.Equal(t, "TITLE", inputs["title"])
			assert.Equal(t, "BODY", inputs["body"])
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
			assert.Equal(t, "NEWPULLID", inputs["pullRequestId"])
			assert.Equal(t, []interface{}{"MONAID"}, inputs["assigneeIds"])
			assert.Equal(t, []interface{}{"BUGID", "TODOID"}, inputs["labelIds"])
			assert.Equal(t, []interface{}{"ROADMAPID"}, inputs["projectIds"])
			assert.Equal(t, "BIGONEID", inputs["milestoneId"])
		}))
	http.Register(
		httpmock.GraphQL(`mutation PullRequestCreateRequestReviews\b`),
		httpmock.GraphQLMutation(`
		{ "data": { "requestReviews": {
			"clientMutationId": ""
		} } }
	`, func(inputs map[string]interface{}) {
			assert.Equal(t, "NEWPULLID", inputs["pullRequestId"])
			assert.Equal(t, []interface{}{"HUBOTID", "MONAID"}, inputs["userIds"])
			assert.Equal(t, []interface{}{"COREID", "ROBOTID"}, inputs["teamIds"])
			assert.Equal(t, true, inputs["union"])
		}))

	//nolint:staticcheck // SA1019 TODO: rewrite to use run.Stub
	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("")                                         // git status
	cs.Stub("1234567890,commit 0\n2345678901,commit 1") // git log

	output, err := runCommand(http, nil, "feature", true, `-t TITLE -b BODY -H feature -a monalisa -l bug -l todo -p roadmap -m 'big one.oh' -r hubot -r monalisa -r /core -r /robots`)
	assert.NoError(t, err)

	assert.Equal(t, "https://github.com/OWNER/REPO/pull/12\n", output.String())
}

func TestPRCreate_alreadyExists(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.StubRepoInfoResponse("OWNER", "REPO", "master")
	http.Register(
		httpmock.GraphQL(`query PullRequestForBranch\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": { "pullRequests": { "nodes": [
				{ "url": "https://github.com/OWNER/REPO/pull/123",
				  "headRefName": "feature",
					"baseRefName": "master" }
			] } } } }`),
	)

	//nolint:staticcheck // SA1019 TODO: rewrite to use run.Stub
	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("") // git config --get-regexp (determineTrackingBranch)
	cs.Stub("") // git show-ref --verify   (determineTrackingBranch)
	cs.Stub("") // git status

	_, err := runCommand(http, nil, "feature", true, `-ttitle -bbody -H feature`)
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

	//nolint:staticcheck // SA1019 TODO: rewrite to use run.Stub
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

	assert.Equal(t, "", output.String())
	assert.Equal(t, "Opening github.com/OWNER/REPO/compare/master...feature in your browser.\n", output.Stderr())

	assert.Equal(t, 6, len(cs.Calls))
	assert.Equal(t, "git push --set-upstream origin HEAD:feature", strings.Join(cs.Calls[4].Args, " "))
	browserCall := cs.Calls[5].Args
	assert.Equal(t, "https://github.com/OWNER/REPO/compare/master...feature?expand=1", browserCall[len(browserCall)-1])
}

func Test_determineTrackingBranch_empty(t *testing.T) {
	//nolint:staticcheck // SA1019 TODO: rewrite to use run.Stub
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
	//nolint:staticcheck // SA1019 TODO: rewrite to use run.Stub
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
	//nolint:staticcheck // SA1019 TODO: rewrite to use run.Stub
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

	assert.Equal(t, []string{"git", "show-ref", "--verify", "--", "HEAD", "refs/remotes/origin/feature", "refs/remotes/upstream/feature"}, cs.Calls[1].Args)

	assert.Equal(t, "upstream", ref.RemoteName)
	assert.Equal(t, "feature", ref.BranchName)
}

func Test_determineTrackingBranch_respectTrackingConfig(t *testing.T) {
	//nolint:staticcheck // SA1019 TODO: rewrite to use run.Stub
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

	assert.Equal(t, []string{"git", "show-ref", "--verify", "--", "HEAD", "refs/remotes/origin/great-feat", "refs/remotes/origin/feature"}, cs.Calls[1].Args)
}

func Test_generateCompareURL(t *testing.T) {
	tests := []struct {
		name    string
		ctx     CreateContext
		state   prShared.IssueMetadataState
		want    string
		wantErr bool
	}{
		{
			name: "basic",
			ctx: CreateContext{
				BaseRepo:        ghrepo.New("OWNER", "REPO"),
				BaseBranch:      "main",
				HeadBranchLabel: "feature",
			},
			want:    "https://github.com/OWNER/REPO/compare/main...feature?expand=1",
			wantErr: false,
		},
		{
			name: "with labels",
			ctx: CreateContext{
				BaseRepo:        ghrepo.New("OWNER", "REPO"),
				BaseBranch:      "a",
				HeadBranchLabel: "b",
			},
			state: prShared.IssueMetadataState{
				Labels: []string{"one", "two three"},
			},
			want:    "https://github.com/OWNER/REPO/compare/a...b?expand=1&labels=one%2Ctwo+three",
			wantErr: false,
		},
		{
			name: "complex branch names",
			ctx: CreateContext{
				BaseRepo:        ghrepo.New("OWNER", "REPO"),
				BaseBranch:      "main/trunk",
				HeadBranchLabel: "owner:feature",
			},
			want:    "https://github.com/OWNER/REPO/compare/main%2Ftrunk...owner%3Afeature?expand=1",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := generateCompareURL(tt.ctx, tt.state)
			if (err != nil) != tt.wantErr {
				t.Errorf("generateCompareURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("generateCompareURL() = %v, want %v", got, tt.want)
			}
		})
	}
}
