package merge

import (
	"bytes"
	"errors"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/cli/cli/api"
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

func Test_NewCmdMerge(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		isTTY   bool
		want    MergeOptions
		wantErr string
	}{
		{
			name:  "number argument",
			args:  "123",
			isTTY: true,
			want: MergeOptions{
				SelectorArg:       "123",
				DeleteBranch:      false,
				DeleteLocalBranch: true,
				MergeMethod:       api.PullRequestMergeMethodMerge,
				InteractiveMode:   true,
			},
		},
		{
			name:    "no argument with --repo override",
			args:    "-R owner/repo",
			isTTY:   true,
			wantErr: "argument required when using the --repo flag",
		},
		{
			name:    "insufficient flags in non-interactive mode",
			args:    "123",
			isTTY:   false,
			wantErr: "--merge, --rebase, or --squash required when not running interactively",
		},
		{
			name:    "multiple merge methods",
			args:    "123 --merge --rebase",
			isTTY:   true,
			wantErr: "only one of --merge, --rebase, or --squash can be enabled",
		},
		{
			name:    "multiple merge methods, non-tty",
			args:    "123 --merge --rebase",
			isTTY:   false,
			wantErr: "only one of --merge, --rebase, or --squash can be enabled",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()
			io.SetStdoutTTY(tt.isTTY)
			io.SetStdinTTY(tt.isTTY)
			io.SetStderrTTY(tt.isTTY)

			f := &cmdutil.Factory{
				IOStreams: io,
			}

			var opts *MergeOptions
			cmd := NewCmdMerge(f, func(o *MergeOptions) error {
				opts = o
				return nil
			})
			cmd.PersistentFlags().StringP("repo", "R", "", "")

			argv, err := shlex.Split(tt.args)
			require.NoError(t, err)
			cmd.SetArgs(argv)

			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(ioutil.Discard)
			cmd.SetErr(ioutil.Discard)

			_, err = cmd.ExecuteC()
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.want.SelectorArg, opts.SelectorArg)
			assert.Equal(t, tt.want.DeleteBranch, opts.DeleteBranch)
			assert.Equal(t, tt.want.DeleteLocalBranch, opts.DeleteLocalBranch)
			assert.Equal(t, tt.want.MergeMethod, opts.MergeMethod)
			assert.Equal(t, tt.want.InteractiveMode, opts.InteractiveMode)
		})
	}
}

func runCommand(rt http.RoundTripper, branch string, isTTY bool, cli string) (*test.CmdOut, error) {
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
		BaseRepo: func() (ghrepo.Interface, error) {
			return api.InitRepoHostname(&api.Repository{
				Name:             "REPO",
				Owner:            api.RepositoryOwner{Login: "OWNER"},
				DefaultBranchRef: api.BranchRef{Name: "master"},
			}, "github.com"), nil
		},
		Remotes: func() (context.Remotes, error) {
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

	cmd := NewCmdMerge(factory, nil)
	cmd.PersistentFlags().StringP("repo", "R", "", "")

	cli = strings.TrimPrefix(cli, "pr merge")
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

func TestPrMerge(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)
	http.Register(
		httpmock.GraphQL(`query PullRequestByNumber\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "pullRequest": {
			"id": "THE-ID",
			"number": 1,
			"title": "The title of the PR",
			"state": "OPEN",
			"headRefName": "blueberries",
			"headRepositoryOwner": {"login": "OWNER"}
		} } } }`))
	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
			assert.NotContains(t, input, "commitHeadline")
		}))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("branch.blueberries.remote origin\nbranch.blueberries.merge refs/heads/blueberries") // git config --get-regexp ^branch\.master\.(remote|merge)
	cs.Stub("")                                                                                  // git config --get-regexp ^branch\.blueberries\.(remote|merge)$
	cs.Stub("")                                                                                  // git symbolic-ref --quiet --short HEAD
	cs.Stub("")                                                                                  // git checkout master
	cs.Stub("")

	output, err := runCommand(http, "master", true, "pr merge 1 --merge")
	if err != nil {
		t.Fatalf("error running command `pr merge`: %v", err)
	}

	r := regexp.MustCompile(`Merged pull request #1 \(The title of the PR\)`)

	if !r.MatchString(output.Stderr()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestPrMerge_nontty(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)
	http.Register(
		httpmock.GraphQL(`query PullRequestByNumber\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "pullRequest": {
			"id": "THE-ID",
			"number": 1,
			"title": "The title of the PR",
			"state": "OPEN",
			"headRefName": "blueberries",
			"headRepositoryOwner": {"login": "OWNER"}
		} } } }`))
	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
			assert.NotContains(t, input, "commitHeadline")
		}))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("branch.blueberries.remote origin\nbranch.blueberries.merge refs/heads/blueberries") // git config --get-regexp ^branch\.master\.(remote|merge)
	cs.Stub("")                                                                                  // git config --get-regexp ^branch\.blueberries\.(remote|merge)$
	cs.Stub("")                                                                                  // git symbolic-ref --quiet --short HEAD
	cs.Stub("")                                                                                  // git checkout master
	cs.Stub("")

	output, err := runCommand(http, "master", false, "pr merge 1 --merge")
	if err != nil {
		t.Fatalf("error running command `pr merge`: %v", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
}

func TestPrMerge_withRepoFlag(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)
	http.Register(
		httpmock.GraphQL(`query PullRequestByNumber\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "pullRequest": {
			"id": "THE-ID",
			"number": 1,
			"title": "The title of the PR",
			"state": "OPEN",
			"headRefName": "blueberries",
			"headRepositoryOwner": {"login": "OWNER"}
		} } } }`))
	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
			assert.NotContains(t, input, "commitHeadline")
		}))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	output, err := runCommand(http, "master", true, "pr merge 1 --merge -R OWNER/REPO")
	if err != nil {
		t.Fatalf("error running command `pr merge`: %v", err)
	}

	assert.Equal(t, 0, len(cs.Calls))

	r := regexp.MustCompile(`Merged pull request #1 \(The title of the PR\)`)

	if !r.MatchString(output.Stderr()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestPrMerge_deleteBranch(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)
	http.Register(
		httpmock.GraphQL(`query PullRequestForBranch\b`),
		// FIXME: references fixture from another package
		httpmock.FileResponse("../view/fixtures/prViewPreviewWithMetadataByBranch.json"))
	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "PR_10", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
			assert.NotContains(t, input, "commitHeadline")
		}))
	http.Register(
		httpmock.REST("DELETE", "repos/OWNER/REPO/git/refs/heads/blueberries"),
		httpmock.StringResponse(`{}`))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("") // git config --get-regexp ^branch\.blueberries\.(remote|merge)$
	cs.Stub("") // git checkout master
	cs.Stub("") // git rev-parse --verify blueberries`
	cs.Stub("") // git branch -d
	cs.Stub("") // git push origin --delete blueberries

	output, err := runCommand(http, "blueberries", true, `pr merge --merge --delete-branch`)
	if err != nil {
		t.Fatalf("Got unexpected error running `pr merge` %s", err)
	}

	test.ExpectLines(t, output.Stderr(), `Merged pull request #10 \(Blueberries are a good fruit\)`, `Deleted branch.*blueberries`)
}

func TestPrMerge_deleteNonCurrentBranch(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)
	http.Register(
		httpmock.GraphQL(`query PullRequestForBranch\b`),
		// FIXME: references fixture from another package
		httpmock.FileResponse("../view/fixtures/prViewPreviewWithMetadataByBranch.json"))
	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "PR_10", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
			assert.NotContains(t, input, "commitHeadline")
		}))
	http.Register(
		httpmock.REST("DELETE", "repos/OWNER/REPO/git/refs/heads/blueberries"),
		httpmock.StringResponse(`{}`))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	// We don't expect the default branch to be checked out, just that blueberries is deleted
	cs.Stub("") // git rev-parse --verify blueberries
	cs.Stub("") // git branch -d blueberries
	cs.Stub("") // git push origin --delete blueberries

	output, err := runCommand(http, "master", true, `pr merge --merge --delete-branch blueberries`)
	if err != nil {
		t.Fatalf("Got unexpected error running `pr merge` %s", err)
	}

	test.ExpectLines(t, output.Stderr(), `Merged pull request #10 \(Blueberries are a good fruit\)`, `Deleted branch.*blueberries`)
}

func TestPrMerge_noPrNumberGiven(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)
	http.Register(
		httpmock.GraphQL(`query PullRequestForBranch\b`),
		// FIXME: references fixture from another package
		httpmock.FileResponse("../view/fixtures/prViewPreviewWithMetadataByBranch.json"))
	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "PR_10", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
			assert.NotContains(t, input, "commitHeadline")
		}))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("branch.blueberries.remote origin\nbranch.blueberries.merge refs/heads/blueberries") // git config --get-regexp ^branch\.master\.(remote|merge)
	cs.Stub("")                                                                                  // git config --get-regexp ^branch\.blueberries\.(remote|merge)$
	cs.Stub("")                                                                                  // git symbolic-ref --quiet --short HEAD
	cs.Stub("")                                                                                  // git checkout master
	cs.Stub("")                                                                                  // git branch -d

	output, err := runCommand(http, "blueberries", true, "pr merge --merge")
	if err != nil {
		t.Fatalf("error running command `pr merge`: %v", err)
	}

	r := regexp.MustCompile(`Merged pull request #10 \(Blueberries are a good fruit\)`)

	if !r.MatchString(output.Stderr()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestPrMerge_rebase(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)
	http.Register(
		httpmock.GraphQL(`query PullRequestByNumber\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "pullRequest": {
			"id": "THE-ID",
			"number": 2,
			"title": "The title of the PR",
			"state": "OPEN",
			"headRefName": "blueberries",
			"headRepositoryOwner": {"login": "OWNER"}
		} } } }`))
	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "REBASE", input["mergeMethod"].(string))
			assert.NotContains(t, input, "commitHeadline")
		}))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("") // git config --get-regexp ^branch\.blueberries\.(remote|merge)$
	cs.Stub("") // git symbolic-ref --quiet --short HEAD
	cs.Stub("") // git checkout master
	cs.Stub("") // git branch -d

	output, err := runCommand(http, "master", true, "pr merge 2 --rebase")
	if err != nil {
		t.Fatalf("error running command `pr merge`: %v", err)
	}

	r := regexp.MustCompile(`Rebased and merged pull request #2 \(The title of the PR\)`)

	if !r.MatchString(output.Stderr()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestPrMerge_squash(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)
	http.Register(
		httpmock.GraphQL(`query PullRequestByNumber\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "pullRequest": {
			"id": "THE-ID",
			"number": 3,
			"title": "The title of the PR",
			"state": "OPEN",
			"headRefName": "blueberries",
			"headRepositoryOwner": {"login": "OWNER"}
		} } } }`))
	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "SQUASH", input["mergeMethod"].(string))
			assert.Equal(t, "The title of the PR (#3)", input["commitHeadline"].(string))
		}))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("") // git config --get-regexp ^branch\.blueberries\.(remote|merge)$
	cs.Stub("") // git symbolic-ref --quiet --short HEAD
	cs.Stub("") // git checkout master
	cs.Stub("") // git branch -d

	output, err := runCommand(http, "master", true, "pr merge 3 --squash")
	if err != nil {
		t.Fatalf("error running command `pr merge`: %v", err)
	}

	test.ExpectLines(t, output.Stderr(), "Squashed and merged pull request #3")
}

func TestPrMerge_alreadyMerged(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)
	http.Register(
		httpmock.GraphQL(`query PullRequestByNumber\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": {
			"pullRequest": { "number": 4, "title": "The title of the PR", "state": "MERGED"}
		} } }`))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("") // git config --get-regexp ^branch\.blueberries\.(remote|merge)$
	cs.Stub("") // git symbolic-ref --quiet --short HEAD
	cs.Stub("") // git checkout master
	cs.Stub("") // git branch -d

	output, err := runCommand(http, "master", true, "pr merge 4")
	if err == nil {
		t.Fatalf("expected an error running command `pr merge`: %v", err)
	}

	r := regexp.MustCompile(`Pull request #4 \(The title of the PR\) was already merged`)

	if !r.MatchString(err.Error()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestPRMerge_interactive(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)
	http.Register(
		httpmock.GraphQL(`query PullRequestForBranch\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "pullRequests": { "nodes": [{
			"headRefName": "blueberries",
			"headRepositoryOwner": {"login": "OWNER"},
			"id": "THE-ID",
			"number": 3
		}] } } } }`))
	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
			assert.NotContains(t, input, "commitHeadline")
		}))
	http.Register(
		httpmock.REST("DELETE", "repos/OWNER/REPO/git/refs/heads/blueberries"),
		httpmock.StringResponse(`{}`))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("") // git config --get-regexp ^branch\.blueberries\.(remote|merge)$
	cs.Stub("") // git symbolic-ref --quiet --short HEAD
	cs.Stub("") // git checkout master
	cs.Stub("") // git push origin --delete blueberries
	cs.Stub("") // git branch -d

	as, surveyTeardown := prompt.InitAskStubber()
	defer surveyTeardown()

	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "mergeMethod",
			Value: 0,
		},
		{
			Name:  "deleteBranch",
			Value: true,
		},
		{
			Name:  "isConfirmed",
			Value: true,
		},
	})

	output, err := runCommand(http, "blueberries", true, "")
	if err != nil {
		t.Fatalf("Got unexpected error running `pr merge` %s", err)
	}

	test.ExpectLines(t, output.Stderr(), "Merged pull request #3")
}

func TestPRMerge_interactiveCancelled(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)
	http.Register(
		httpmock.GraphQL(`query PullRequestForBranch\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": { "pullRequests": { "nodes": [{
			"headRefName": "blueberries",
			"headRepositoryOwner": {"login": "OWNER"},
			"id": "THE-ID",
			"number": 3
		}] } } } }`))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("") // git config --get-regexp ^branch\.blueberries\.(remote|merge)$
	cs.Stub("") // git symbolic-ref --quiet --short HEAD
	cs.Stub("") // git checkout master
	cs.Stub("") // git push origin --delete blueberries
	cs.Stub("") // git branch -d

	as, surveyTeardown := prompt.InitAskStubber()
	defer surveyTeardown()

	as.Stub([]*prompt.QuestionStub{
		{
			Name:  "mergeMethod",
			Value: 0,
		},
		{
			Name:  "deleteBranch",
			Value: true,
		},
		{
			Name:  "isConfirmed",
			Value: false,
		},
	})

	output, err := runCommand(http, "blueberries", true, "")
	if !errors.Is(err, cmdutil.SilentError) {
		t.Fatalf("got error %v", err)
	}

	assert.Equal(t, "Cancelled.\n", output.Stderr())
}
