package create

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
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

func TestNewCmdCreate(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "my-body.md")
	err := ioutil.WriteFile(tmpFile, []byte("a body from file"), 0600)
	require.NoError(t, err)

	tests := []struct {
		name      string
		tty       bool
		stdin     string
		cli       string
		wantsErr  bool
		wantsOpts CreateOptions
	}{
		{
			name:     "empty non-tty",
			tty:      false,
			cli:      "",
			wantsErr: true,
		},
		{
			name:     "empty tty",
			tty:      true,
			cli:      "",
			wantsErr: false,
			wantsOpts: CreateOptions{
				Title:               "",
				TitleProvided:       false,
				Body:                "",
				BodyProvided:        false,
				Autofill:            false,
				RecoverFile:         "",
				WebMode:             false,
				IsDraft:             false,
				BaseBranch:          "",
				HeadBranch:          "",
				MaintainerCanModify: true,
			},
		},
		{
			name:     "body from stdin",
			tty:      false,
			stdin:    "this is on standard input",
			cli:      "-t mytitle -F -",
			wantsErr: false,
			wantsOpts: CreateOptions{
				Title:               "mytitle",
				TitleProvided:       true,
				Body:                "this is on standard input",
				BodyProvided:        true,
				Autofill:            false,
				RecoverFile:         "",
				WebMode:             false,
				IsDraft:             false,
				BaseBranch:          "",
				HeadBranch:          "",
				MaintainerCanModify: true,
			},
		},
		{
			name:     "body from file",
			tty:      false,
			cli:      fmt.Sprintf("-t mytitle -F '%s'", tmpFile),
			wantsErr: false,
			wantsOpts: CreateOptions{
				Title:               "mytitle",
				TitleProvided:       true,
				Body:                "a body from file",
				BodyProvided:        true,
				Autofill:            false,
				RecoverFile:         "",
				WebMode:             false,
				IsDraft:             false,
				BaseBranch:          "",
				HeadBranch:          "",
				MaintainerCanModify: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, stdin, stdout, stderr := iostreams.Test()
			if tt.stdin != "" {
				_, _ = stdin.WriteString(tt.stdin)
			} else if tt.tty {
				io.SetStdinTTY(true)
				io.SetStdoutTTY(true)
			}

			f := &cmdutil.Factory{
				IOStreams: io,
			}

			var opts *CreateOptions
			cmd := NewCmdCreate(f, func(o *CreateOptions) error {
				opts = o
				return nil
			})

			args, err := shlex.Split(tt.cli)
			require.NoError(t, err)
			cmd.SetArgs(args)
			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				assert.Error(t, err)
				return
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, "", stdout.String())
			assert.Equal(t, "", stderr.String())

			assert.Equal(t, tt.wantsOpts.Body, opts.Body)
			assert.Equal(t, tt.wantsOpts.BodyProvided, opts.BodyProvided)
			assert.Equal(t, tt.wantsOpts.Title, opts.Title)
			assert.Equal(t, tt.wantsOpts.TitleProvided, opts.TitleProvided)
			assert.Equal(t, tt.wantsOpts.Autofill, opts.Autofill)
			assert.Equal(t, tt.wantsOpts.WebMode, opts.WebMode)
			assert.Equal(t, tt.wantsOpts.RecoverFile, opts.RecoverFile)
			assert.Equal(t, tt.wantsOpts.IsDraft, opts.IsDraft)
			assert.Equal(t, tt.wantsOpts.MaintainerCanModify, opts.MaintainerCanModify)
			assert.Equal(t, tt.wantsOpts.BaseBranch, opts.BaseBranch)
			assert.Equal(t, tt.wantsOpts.HeadBranch, opts.HeadBranch)
		})
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

	browser := &cmdutil.TestBrowser{}
	factory := &cmdutil.Factory{
		IOStreams: io,
		Browser:   browser,
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
		OutBuf:     stdout,
		ErrBuf:     stderr,
		BrowsedURL: browser.BrowsedURL(),
	}, err
}

func initFakeHTTP() *httpmock.Registry {
	return &httpmock.Registry{}
}

func TestPRCreate_nontty_web(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.StubRepoInfoResponse("OWNER", "REPO", "master")

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git status --porcelain`, 0, "")
	cs.Register(`git( .+)? log( .+)? origin/master\.\.\.feature`, 0, "")

	output, err := runCommand(http, nil, "feature", false, `--web --head=feature`)
	require.NoError(t, err)

	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
	assert.Equal(t, "https://github.com/OWNER/REPO/compare/master...feature?expand=1", output.BrowsedURL)
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

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git status --porcelain`, 0, "")
	cs.Register(`git( .+)? log( .+)? origin/master\.\.\.feature`, 0, "")

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

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git status --porcelain`, 0, "")

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

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git status --porcelain`, 0, "")
	cs.Register(`git config --get-regexp.+branch\\\.feature\\\.`, 0, "")
	cs.Register(`git show-ref --verify -- HEAD refs/remotes/origin/feature`, 0, "")
	cs.Register(`git push --set-upstream origin HEAD:feature`, 0, "")

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

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git config --get-regexp.+branch\\\.feature\\\.`, 0, "")
	cs.Register(`git status --porcelain`, 0, "")
	cs.Register(`git show-ref --verify -- HEAD refs/remotes/origin/feature`, 0, "")
	cs.Register(`git push --set-upstream origin HEAD:feature`, 0, "")

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

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git config --get-regexp.+branch\\\.feature\\\.`, 0, "")
	cs.Register(`git status --porcelain`, 0, "")
	cs.Register(`git show-ref --verify -- HEAD refs/remotes/origin/feature`, 0, "")
	cs.Register(`git remote add -f fork https://github.com/monalisa/REPO.git`, 0, "")
	cs.Register(`git push --set-upstream fork HEAD:feature`, 0, "")

	ask, cleanupAsk := prompt.InitAskStubber()
	defer cleanupAsk()
	ask.StubOne(1)

	output, err := runCommand(http, nil, "feature", true, `-t title -b body`)
	require.NoError(t, err)

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

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git( .+)? log( .+)? origin/master\.\.\.feature`, 0, "1234567890,commit 0\n2345678901,commit 1")
	cs.Register(`git status --porcelain`, 0, "")

	as, teardown := prompt.InitAskStubber()
	defer teardown()
	as.StubOne(0) // template
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

	_, err := runCommand(http, nil, "feature", true, `-t title -b body -H feature`)
	assert.EqualError(t, err, "a pull request for branch \"feature\" into branch \"master\" already exists:\nhttps://github.com/OWNER/REPO/pull/123")
}

func TestPRCreate_web(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.StubRepoInfoResponse("OWNER", "REPO", "master")
	http.StubRepoResponse("OWNER", "REPO")
	http.Register(
		httpmock.GraphQL(`query UserCurrent\b`),
		httpmock.StringResponse(`{"data": {"viewer": {"login": "OWNER"} } }`))

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git config --get-regexp.+branch\\\.feature\\\.`, 0, "")
	cs.Register(`git status --porcelain`, 0, "")
	cs.Register(`git show-ref --verify -- HEAD refs/remotes/origin/feature`, 0, "")
	cs.Register(`git( .+)? log( .+)? origin/master\.\.\.feature`, 0, "")
	cs.Register(`git push --set-upstream origin HEAD:feature`, 0, "")

	ask, cleanupAsk := prompt.InitAskStubber()
	defer cleanupAsk()
	ask.StubOne(0)

	output, err := runCommand(http, nil, "feature", true, `--web`)
	require.NoError(t, err)

	assert.Equal(t, "", output.String())
	assert.Equal(t, "Opening github.com/OWNER/REPO/compare/master...feature in your browser.\n", output.Stderr())
	assert.Equal(t, "https://github.com/OWNER/REPO/compare/master...feature?expand=1", output.BrowsedURL)
}

func TestPRCreate_webProject(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.StubRepoInfoResponse("OWNER", "REPO", "master")
	http.StubRepoResponse("OWNER", "REPO")
	http.Register(
		httpmock.GraphQL(`query UserCurrent\b`),
		httpmock.StringResponse(`{"data": {"viewer": {"login": "OWNER"} } }`))
	http.Register(
		httpmock.GraphQL(`query RepositoryProjectList\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": { "projects": {
				"nodes": [
					{ "name": "Cleanup", "id": "CLEANUPID", "resourcePath": "/OWNER/REPO/projects/1" }
				],
				"pageInfo": { "hasNextPage": false }
			} } } }
			`))
	http.Register(
		httpmock.GraphQL(`query OrganizationProjectList\b`),
		httpmock.StringResponse(`
			{ "data": { "organization": { "projects": {
				"nodes": [
					{ "name": "Triage", "id": "TRIAGEID", "resourcePath": "/orgs/ORG/projects/1"  }
				],
				"pageInfo": { "hasNextPage": false }
			} } } }
			`))

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git config --get-regexp.+branch\\\.feature\\\.`, 0, "")
	cs.Register(`git status --porcelain`, 0, "")
	cs.Register(`git show-ref --verify -- HEAD refs/remotes/origin/feature`, 0, "")
	cs.Register(`git( .+)? log( .+)? origin/master\.\.\.feature`, 0, "")
	cs.Register(`git push --set-upstream origin HEAD:feature`, 0, "")

	ask, cleanupAsk := prompt.InitAskStubber()
	defer cleanupAsk()
	ask.StubOne(0)

	output, err := runCommand(http, nil, "feature", true, `--web -p Triage`)
	require.NoError(t, err)

	assert.Equal(t, "", output.String())
	assert.Equal(t, "Opening github.com/OWNER/REPO/compare/master...feature in your browser.\n", output.Stderr())
	assert.Equal(t, "https://github.com/OWNER/REPO/compare/master...feature?expand=1&projects=ORG%2F1", output.BrowsedURL)
}

func Test_determineTrackingBranch_empty(t *testing.T) {
	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git config --get-regexp.+branch\\\.feature\\\.`, 0, "")
	cs.Register(`git show-ref --verify -- HEAD`, 0, "abc HEAD")

	remotes := context.Remotes{}

	ref := determineTrackingBranch(remotes, "feature")
	if ref != nil {
		t.Errorf("expected nil result, got %v", ref)
	}
}

func Test_determineTrackingBranch_noMatch(t *testing.T) {
	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git config --get-regexp.+branch\\\.feature\\\.`, 0, "")
	cs.Register("git show-ref --verify -- HEAD refs/remotes/origin/feature refs/remotes/upstream/feature", 0, "abc HEAD\nbca refs/remotes/origin/feature")

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

	ref := determineTrackingBranch(remotes, "feature")
	if ref != nil {
		t.Errorf("expected nil result, got %v", ref)
	}
}

func Test_determineTrackingBranch_hasMatch(t *testing.T) {
	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git config --get-regexp.+branch\\\.feature\\\.`, 0, "")
	cs.Register(`git show-ref --verify -- HEAD refs/remotes/origin/feature refs/remotes/upstream/feature$`, 0, heredoc.Doc(`
		deadbeef HEAD
		deadb00f refs/remotes/origin/feature
		deadbeef refs/remotes/upstream/feature
	`))

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

	ref := determineTrackingBranch(remotes, "feature")
	if ref == nil {
		t.Fatal("expected result, got nil")
	}

	assert.Equal(t, "upstream", ref.RemoteName)
	assert.Equal(t, "feature", ref.BranchName)
}

func Test_determineTrackingBranch_respectTrackingConfig(t *testing.T) {
	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git config --get-regexp.+branch\\\.feature\\\.`, 0, heredoc.Doc(`
		branch.feature.remote origin
		branch.feature.merge refs/heads/great-feat
	`))
	cs.Register(`git show-ref --verify -- HEAD refs/remotes/origin/great-feat refs/remotes/origin/feature$`, 0, heredoc.Doc(`
		deadbeef HEAD
		deadb00f refs/remotes/origin/feature
	`))

	remotes := context.Remotes{
		&context.Remote{
			Remote: &git.Remote{Name: "origin"},
			Repo:   ghrepo.New("hubot", "Spoon-Knife"),
		},
	}

	ref := determineTrackingBranch(remotes, "feature")
	if ref != nil {
		t.Errorf("expected nil result, got %v", ref)
	}
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
				BaseRepo:        api.InitRepoHostname(&api.Repository{Name: "REPO", Owner: api.RepositoryOwner{Login: "OWNER"}}, "github.com"),
				BaseBranch:      "main",
				HeadBranchLabel: "feature",
			},
			want:    "https://github.com/OWNER/REPO/compare/main...feature?expand=1",
			wantErr: false,
		},
		{
			name: "with labels",
			ctx: CreateContext{
				BaseRepo:        api.InitRepoHostname(&api.Repository{Name: "REPO", Owner: api.RepositoryOwner{Login: "OWNER"}}, "github.com"),
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
				BaseRepo:        api.InitRepoHostname(&api.Repository{Name: "REPO", Owner: api.RepositoryOwner{Login: "OWNER"}}, "github.com"),
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
