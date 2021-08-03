package merge

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/internal/run"
	"github.com/cli/cli/pkg/cmd/pr/shared"
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
	tmpFile := filepath.Join(t.TempDir(), "my-body.md")
	err := ioutil.WriteFile(tmpFile, []byte("a body from file"), 0600)
	require.NoError(t, err)

	tests := []struct {
		name    string
		args    string
		stdin   string
		isTTY   bool
		want    MergeOptions
		wantErr string
	}{
		{
			name:  "number argument",
			args:  "123",
			isTTY: true,
			want: MergeOptions{
				SelectorArg:             "123",
				DeleteBranch:            false,
				IsDeleteBranchIndicated: false,
				CanDeleteLocalBranch:    true,
				MergeMethod:             PullRequestMergeMethodMerge,
				InteractiveMode:         true,
				Body:                    "",
				BodySet:                 false,
			},
		},
		{
			name:  "delete-branch specified",
			args:  "--delete-branch=false",
			isTTY: true,
			want: MergeOptions{
				SelectorArg:             "",
				DeleteBranch:            false,
				IsDeleteBranchIndicated: true,
				CanDeleteLocalBranch:    true,
				MergeMethod:             PullRequestMergeMethodMerge,
				InteractiveMode:         true,
				Body:                    "",
				BodySet:                 false,
			},
		},
		{
			name:  "body from file",
			args:  fmt.Sprintf("123 --body-file '%s'", tmpFile),
			isTTY: true,
			want: MergeOptions{
				SelectorArg:             "123",
				DeleteBranch:            false,
				IsDeleteBranchIndicated: false,
				CanDeleteLocalBranch:    true,
				MergeMethod:             PullRequestMergeMethodMerge,
				InteractiveMode:         true,
				Body:                    "a body from file",
				BodySet:                 true,
			},
		},
		{
			name:  "body from stdin",
			args:  "123 --body-file -",
			stdin: "this is on standard input",
			isTTY: true,
			want: MergeOptions{
				SelectorArg:             "123",
				DeleteBranch:            false,
				IsDeleteBranchIndicated: false,
				CanDeleteLocalBranch:    true,
				MergeMethod:             PullRequestMergeMethodMerge,
				InteractiveMode:         true,
				Body:                    "this is on standard input",
				BodySet:                 true,
			},
		},
		{
			name:  "body",
			args:  "123 -bcool",
			isTTY: true,
			want: MergeOptions{
				SelectorArg:             "123",
				DeleteBranch:            false,
				IsDeleteBranchIndicated: false,
				CanDeleteLocalBranch:    true,
				MergeMethod:             PullRequestMergeMethodMerge,
				InteractiveMode:         true,
				Body:                    "cool",
				BodySet:                 true,
			},
		},
		{
			name:    "body and body-file flags",
			args:    "123 --body 'test' --body-file 'test-file.txt'",
			isTTY:   true,
			wantErr: "specify only one of `--body` or `--body-file`",
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
			io, stdin, _, _ := iostreams.Test()
			io.SetStdoutTTY(tt.isTTY)
			io.SetStdinTTY(tt.isTTY)
			io.SetStderrTTY(tt.isTTY)

			if tt.stdin != "" {
				_, _ = stdin.WriteString(tt.stdin)
			}

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
			assert.Equal(t, tt.want.CanDeleteLocalBranch, opts.CanDeleteLocalBranch)
			assert.Equal(t, tt.want.MergeMethod, opts.MergeMethod)
			assert.Equal(t, tt.want.InteractiveMode, opts.InteractiveMode)
			assert.Equal(t, tt.want.Body, opts.Body)
			assert.Equal(t, tt.want.BodySet, opts.BodySet)
		})
	}
}

func baseRepo(owner, repo, branch string) ghrepo.Interface {
	return api.InitRepoHostname(&api.Repository{
		Name:             repo,
		Owner:            api.RepositoryOwner{Login: owner},
		DefaultBranchRef: api.BranchRef{Name: branch},
	}, "github.com")
}

func stubCommit(pr *api.PullRequest, oid string) {
	pr.Commits.Nodes = append(pr.Commits.Nodes, api.PullRequestCommit{
		Commit: api.PullRequestCommitCommit{OID: oid},
	})
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

	shared.RunCommandFinder(
		"1",
		&api.PullRequest{
			ID:               "THE-ID",
			Number:           1,
			State:            "OPEN",
			Title:            "The title of the PR",
			MergeStateStatus: "CLEAN",
		},
		baseRepo("OWNER", "REPO", "master"),
	)

	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
			assert.NotContains(t, input, "commitHeadline")
		}))

	_, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	output, err := runCommand(http, "master", true, "pr merge 1 --merge")
	if err != nil {
		t.Fatalf("error running command `pr merge`: %v", err)
	}

	r := regexp.MustCompile(`Merged pull request #1 \(The title of the PR\)`)

	if !r.MatchString(output.Stderr()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestPrMerge_blocked(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	shared.RunCommandFinder(
		"1",
		&api.PullRequest{
			ID:               "THE-ID",
			Number:           1,
			State:            "OPEN",
			Title:            "The title of the PR",
			MergeStateStatus: "BLOCKED",
		},
		baseRepo("OWNER", "REPO", "master"),
	)

	_, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	output, err := runCommand(http, "master", true, "pr merge 1 --merge")
	assert.EqualError(t, err, "SilentError")

	assert.Equal(t, "", output.String())
	assert.Equal(t, heredoc.Docf(`
		X Pull request #1 is not mergeable: the base branch policy prohibits the merge.
		To have the pull request merged after all the requirements have been met, add the %[1]s--auto%[1]s flag.
		To use administrator privileges to immediately merge the pull request, add the %[1]s--admin%[1]s flag.
		`, "`"), output.Stderr())
}

func TestPrMerge_nontty(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	shared.RunCommandFinder(
		"1",
		&api.PullRequest{
			ID:               "THE-ID",
			Number:           1,
			State:            "OPEN",
			Title:            "The title of the PR",
			MergeStateStatus: "CLEAN",
		},
		baseRepo("OWNER", "REPO", "master"),
	)

	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
			assert.NotContains(t, input, "commitHeadline")
		}))

	_, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

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

	shared.RunCommandFinder(
		"1",
		&api.PullRequest{
			ID:               "THE-ID",
			Number:           1,
			State:            "OPEN",
			Title:            "The title of the PR",
			MergeStateStatus: "CLEAN",
		},
		baseRepo("OWNER", "REPO", "master"),
	)

	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
			assert.NotContains(t, input, "commitHeadline")
		}))

	_, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	output, err := runCommand(http, "master", true, "pr merge 1 --merge -R OWNER/REPO")
	if err != nil {
		t.Fatalf("error running command `pr merge`: %v", err)
	}

	r := regexp.MustCompile(`Merged pull request #1 \(The title of the PR\)`)

	if !r.MatchString(output.Stderr()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestPrMerge_deleteBranch(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	shared.RunCommandFinder(
		"",
		&api.PullRequest{
			ID:               "PR_10",
			Number:           10,
			State:            "OPEN",
			Title:            "Blueberries are a good fruit",
			HeadRefName:      "blueberries",
			MergeStateStatus: "CLEAN",
		},
		baseRepo("OWNER", "REPO", "master"),
	)

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

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git checkout master`, 0, "")
	cs.Register(`git rev-parse --verify refs/heads/blueberries`, 0, "")
	cs.Register(`git branch -D blueberries`, 0, "")

	output, err := runCommand(http, "blueberries", true, `pr merge --merge --delete-branch`)
	if err != nil {
		t.Fatalf("Got unexpected error running `pr merge` %s", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, heredoc.Doc(`
		✓ Merged pull request #10 (Blueberries are a good fruit)
		✓ Deleted branch blueberries and switched to branch master
	`), output.Stderr())
}

func TestPrMerge_deleteNonCurrentBranch(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	shared.RunCommandFinder(
		"blueberries",
		&api.PullRequest{
			ID:               "PR_10",
			Number:           10,
			State:            "OPEN",
			Title:            "Blueberries are a good fruit",
			HeadRefName:      "blueberries",
			MergeStateStatus: "CLEAN",
		},
		baseRepo("OWNER", "REPO", "master"),
	)

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

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git rev-parse --verify refs/heads/blueberries`, 0, "")
	cs.Register(`git branch -D blueberries`, 0, "")

	output, err := runCommand(http, "master", true, `pr merge --merge --delete-branch blueberries`)
	if err != nil {
		t.Fatalf("Got unexpected error running `pr merge` %s", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, heredoc.Doc(`
		✓ Merged pull request #10 (Blueberries are a good fruit)
		✓ Deleted branch blueberries
	`), output.Stderr())
}

func Test_nonDivergingPullRequest(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	pr := &api.PullRequest{
		ID:               "PR_10",
		Number:           10,
		Title:            "Blueberries are a good fruit",
		State:            "OPEN",
		MergeStateStatus: "CLEAN",
	}
	stubCommit(pr, "COMMITSHA1")

	prFinder := shared.RunCommandFinder("", pr, baseRepo("OWNER", "REPO", "master"))
	prFinder.ExpectFields([]string{"id", "number", "state", "title", "lastCommit", "mergeStateStatus", "headRepositoryOwner", "headRefName"})

	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "PR_10", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
			assert.NotContains(t, input, "commitHeadline")
		}))

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git .+ show .+ HEAD`, 0, "COMMITSHA1,title")

	output, err := runCommand(http, "blueberries", true, "pr merge --merge")
	if err != nil {
		t.Fatalf("error running command `pr merge`: %v", err)
	}

	assert.Equal(t, heredoc.Doc(`
		✓ Merged pull request #10 (Blueberries are a good fruit)
	`), output.Stderr())
}

func Test_divergingPullRequestWarning(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	pr := &api.PullRequest{
		ID:               "PR_10",
		Number:           10,
		Title:            "Blueberries are a good fruit",
		State:            "OPEN",
		MergeStateStatus: "CLEAN",
	}
	stubCommit(pr, "COMMITSHA1")

	prFinder := shared.RunCommandFinder("", pr, baseRepo("OWNER", "REPO", "master"))
	prFinder.ExpectFields([]string{"id", "number", "state", "title", "lastCommit", "mergeStateStatus", "headRepositoryOwner", "headRefName"})

	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "PR_10", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
			assert.NotContains(t, input, "commitHeadline")
		}))

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git .+ show .+ HEAD`, 0, "COMMITSHA2,title")

	output, err := runCommand(http, "blueberries", true, "pr merge --merge")
	if err != nil {
		t.Fatalf("error running command `pr merge`: %v", err)
	}

	assert.Equal(t, heredoc.Doc(`
		! Pull request #10 (Blueberries are a good fruit) has diverged from local branch
		✓ Merged pull request #10 (Blueberries are a good fruit)
	`), output.Stderr())
}

func Test_pullRequestWithoutCommits(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	shared.RunCommandFinder(
		"",
		&api.PullRequest{
			ID:               "PR_10",
			Number:           10,
			Title:            "Blueberries are a good fruit",
			State:            "OPEN",
			MergeStateStatus: "CLEAN",
		},
		baseRepo("OWNER", "REPO", "master"),
	)

	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "PR_10", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
			assert.NotContains(t, input, "commitHeadline")
		}))

	_, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	output, err := runCommand(http, "blueberries", true, "pr merge --merge")
	if err != nil {
		t.Fatalf("error running command `pr merge`: %v", err)
	}

	assert.Equal(t, heredoc.Doc(`
		✓ Merged pull request #10 (Blueberries are a good fruit)
	`), output.Stderr())
}

func TestPrMerge_rebase(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	shared.RunCommandFinder(
		"2",
		&api.PullRequest{
			ID:               "THE-ID",
			Number:           2,
			Title:            "The title of the PR",
			State:            "OPEN",
			MergeStateStatus: "CLEAN",
		},
		baseRepo("OWNER", "REPO", "master"),
	)

	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "REBASE", input["mergeMethod"].(string))
			assert.NotContains(t, input, "commitHeadline")
		}))

	_, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

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

	shared.RunCommandFinder(
		"3",
		&api.PullRequest{
			ID:               "THE-ID",
			Number:           3,
			Title:            "The title of the PR",
			State:            "OPEN",
			MergeStateStatus: "CLEAN",
		},
		baseRepo("OWNER", "REPO", "master"),
	)

	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "SQUASH", input["mergeMethod"].(string))
			assert.NotContains(t, input, "commitHeadline")
		}))

	_, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	output, err := runCommand(http, "master", true, "pr merge 3 --squash")
	if err != nil {
		t.Fatalf("error running command `pr merge`: %v", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, heredoc.Doc(`
		✓ Squashed and merged pull request #3 (The title of the PR)
	`), output.Stderr())
}

func TestPrMerge_alreadyMerged(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	shared.RunCommandFinder(
		"4",
		&api.PullRequest{
			ID:               "THE-ID",
			Number:           4,
			State:            "MERGED",
			HeadRefName:      "blueberries",
			BaseRefName:      "master",
			MergeStateStatus: "CLEAN",
		},
		baseRepo("OWNER", "REPO", "master"),
	)

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git checkout master`, 0, "")
	cs.Register(`git rev-parse --verify refs/heads/blueberries`, 0, "")
	cs.Register(`git branch -D blueberries`, 0, "")

	as, surveyTeardown := prompt.InitAskStubber()
	defer surveyTeardown()
	as.StubOne(true)

	output, err := runCommand(http, "blueberries", true, "pr merge 4")
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, "✓ Deleted branch blueberries and switched to branch master\n", output.Stderr())
}

func TestPrMerge_alreadyMerged_nonInteractive(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	shared.RunCommandFinder(
		"4",
		&api.PullRequest{
			ID:                  "THE-ID",
			Number:              4,
			State:               "MERGED",
			HeadRepositoryOwner: api.Owner{Login: "monalisa"},
			MergeStateStatus:    "CLEAN",
		},
		baseRepo("OWNER", "REPO", "master"),
	)

	_, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	output, err := runCommand(http, "blueberries", true, "pr merge 4 --merge")
	if err != nil {
		t.Fatalf("Got unexpected error running `pr merge` %s", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, "! Pull request #4 was already merged\n", output.Stderr())
}

func TestPRMerge_interactive(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	shared.RunCommandFinder(
		"",
		&api.PullRequest{
			ID:               "THE-ID",
			Number:           3,
			Title:            "It was the best of times",
			HeadRefName:      "blueberries",
			MergeStateStatus: "CLEAN",
		},
		baseRepo("OWNER", "REPO", "master"),
	)

	http.Register(
		httpmock.GraphQL(`query RepositoryInfo\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": {
			"mergeCommitAllowed": true,
			"rebaseMergeAllowed": true,
			"squashMergeAllowed": true
		} } }`))
	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
			assert.NotContains(t, input, "commitHeadline")
		}))

	_, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	as, surveyTeardown := prompt.InitAskStubber()
	defer surveyTeardown()

	as.StubOne(0)        // Merge method survey
	as.StubOne(false)    // Delete branch survey
	as.StubOne("Submit") // Confirm submit survey

	output, err := runCommand(http, "blueberries", true, "")
	if err != nil {
		t.Fatalf("Got unexpected error running `pr merge` %s", err)
	}

	//nolint:staticcheck // prefer exact matchers over ExpectLines
	test.ExpectLines(t, output.Stderr(), "Merged pull request #3")
}

func TestPRMerge_interactiveWithDeleteBranch(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	shared.RunCommandFinder(
		"",
		&api.PullRequest{
			ID:               "THE-ID",
			Number:           3,
			Title:            "It was the best of times",
			HeadRefName:      "blueberries",
			MergeStateStatus: "CLEAN",
		},
		baseRepo("OWNER", "REPO", "master"),
	)

	http.Register(
		httpmock.GraphQL(`query RepositoryInfo\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": {
			"mergeCommitAllowed": true,
			"rebaseMergeAllowed": true,
			"squashMergeAllowed": true
		} } }`))
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

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git checkout master`, 0, "")
	cs.Register(`git rev-parse --verify refs/heads/blueberries`, 0, "")
	cs.Register(`git branch -D blueberries`, 0, "")

	as, surveyTeardown := prompt.InitAskStubber()
	defer surveyTeardown()

	as.StubOne(0)        // Merge method survey
	as.StubOne("Submit") // Confirm submit survey

	output, err := runCommand(http, "blueberries", true, "-d")
	if err != nil {
		t.Fatalf("Got unexpected error running `pr merge` %s", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, heredoc.Doc(`
		✓ Merged pull request #3 (It was the best of times)
		✓ Deleted branch blueberries and switched to branch master
	`), output.Stderr())
}

func TestPRMerge_interactiveSquashEditCommitMsg(t *testing.T) {
	io, _, stdout, stderr := iostreams.Test()
	io.SetStdoutTTY(true)
	io.SetStderrTTY(true)

	tr := initFakeHTTP()
	defer tr.Verify(t)
	tr.Register(
		httpmock.GraphQL(`query RepositoryInfo\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": {
			"mergeCommitAllowed": true,
			"rebaseMergeAllowed": true,
			"squashMergeAllowed": true
		} } }`))
	tr.Register(
		httpmock.GraphQL(`query PullRequestMergeText\b`),
		httpmock.StringResponse(`
		{ "data": { "node": {
			"viewerMergeBodyText": "default body text"
		} } }`))
	tr.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "SQUASH", input["mergeMethod"].(string))
			assert.Equal(t, "DEFAULT BODY TEXT", input["commitBody"].(string))
		}))

	_, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	as, surveyTeardown := prompt.InitAskStubber()
	defer surveyTeardown()

	as.StubOne(2)                     // Merge method survey
	as.StubOne(false)                 // Delete branch survey
	as.StubOne("Edit commit message") // Confirm submit survey
	as.StubOne("Submit")              // Confirm submit survey

	err := mergeRun(&MergeOptions{
		IO:     io,
		Editor: testEditor{},
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: tr}, nil
		},
		SelectorArg:     "https://github.com/OWNER/REPO/pull/123",
		InteractiveMode: true,
		Finder: shared.NewMockFinder(
			"https://github.com/OWNER/REPO/pull/123",
			&api.PullRequest{ID: "THE-ID", Number: 123, Title: "title", MergeStateStatus: "CLEAN"},
			ghrepo.New("OWNER", "REPO"),
		),
	})
	assert.NoError(t, err)

	assert.Equal(t, "", stdout.String())
	assert.Equal(t, "✓ Squashed and merged pull request #123 (title)\n", stderr.String())
}

func TestPRMerge_interactiveCancelled(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	shared.RunCommandFinder(
		"",
		&api.PullRequest{ID: "THE-ID", Number: 123, MergeStateStatus: "CLEAN"},
		ghrepo.New("OWNER", "REPO"),
	)

	http.Register(
		httpmock.GraphQL(`query RepositoryInfo\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": {
			"mergeCommitAllowed": true,
			"rebaseMergeAllowed": true,
			"squashMergeAllowed": true
		} } }`))

	_, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	as, surveyTeardown := prompt.InitAskStubber()
	defer surveyTeardown()

	as.StubOne(0)        // Merge method survey
	as.StubOne(true)     // Delete branch survey
	as.StubOne("Cancel") // Confirm submit survey

	output, err := runCommand(http, "blueberries", true, "")
	if !errors.Is(err, cmdutil.CancelError) {
		t.Fatalf("got error %v", err)
	}

	assert.Equal(t, "Cancelled.\n", output.Stderr())
}

func Test_mergeMethodSurvey(t *testing.T) {
	repo := &api.Repository{
		MergeCommitAllowed: false,
		RebaseMergeAllowed: true,
		SquashMergeAllowed: true,
	}
	as, surveyTeardown := prompt.InitAskStubber()
	defer surveyTeardown()
	as.StubOne(0) // Select first option which is rebase merge
	method, err := mergeMethodSurvey(repo)
	assert.Nil(t, err)
	assert.Equal(t, PullRequestMergeMethodRebase, method)
}

func TestMergeRun_autoMerge(t *testing.T) {
	io, _, stdout, stderr := iostreams.Test()
	io.SetStdoutTTY(true)
	io.SetStderrTTY(true)

	tr := initFakeHTTP()
	defer tr.Verify(t)
	tr.Register(
		httpmock.GraphQL(`mutation PullRequestAutoMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "SQUASH", input["mergeMethod"].(string))
		}))

	_, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	err := mergeRun(&MergeOptions{
		IO: io,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: tr}, nil
		},
		SelectorArg:     "https://github.com/OWNER/REPO/pull/123",
		AutoMergeEnable: true,
		MergeMethod:     PullRequestMergeMethodSquash,
		Finder: shared.NewMockFinder(
			"https://github.com/OWNER/REPO/pull/123",
			&api.PullRequest{ID: "THE-ID", Number: 123, MergeStateStatus: "BLOCKED"},
			ghrepo.New("OWNER", "REPO"),
		),
	})
	assert.NoError(t, err)

	assert.Equal(t, "", stdout.String())
	assert.Equal(t, "✓ Pull request #123 will be automatically merged via squash when all requirements are met\n", stderr.String())
}

func TestMergeRun_autoMerge_directMerge(t *testing.T) {
	io, _, stdout, stderr := iostreams.Test()
	io.SetStdoutTTY(true)
	io.SetStderrTTY(true)

	tr := initFakeHTTP()
	defer tr.Verify(t)
	tr.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
			assert.NotContains(t, input, "commitHeadline")
		}))

	_, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	err := mergeRun(&MergeOptions{
		IO: io,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: tr}, nil
		},
		SelectorArg:     "https://github.com/OWNER/REPO/pull/123",
		AutoMergeEnable: true,
		MergeMethod:     PullRequestMergeMethodMerge,
		Finder: shared.NewMockFinder(
			"https://github.com/OWNER/REPO/pull/123",
			&api.PullRequest{ID: "THE-ID", Number: 123, MergeStateStatus: "CLEAN"},
			ghrepo.New("OWNER", "REPO"),
		),
	})
	assert.NoError(t, err)

	assert.Equal(t, "", stdout.String())
	assert.Equal(t, "✓ Merged pull request #123 ()\n", stderr.String())
}

func TestMergeRun_disableAutoMerge(t *testing.T) {
	io, _, stdout, stderr := iostreams.Test()
	io.SetStdoutTTY(true)
	io.SetStderrTTY(true)

	tr := initFakeHTTP()
	defer tr.Verify(t)
	tr.Register(
		httpmock.GraphQL(`mutation PullRequestAutoMergeDisable\b`),
		httpmock.GraphQLQuery(`{}`, func(s string, m map[string]interface{}) {
			assert.Equal(t, map[string]interface{}{"prID": "THE-ID"}, m)
		}))

	_, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	err := mergeRun(&MergeOptions{
		IO: io,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: tr}, nil
		},
		SelectorArg:      "https://github.com/OWNER/REPO/pull/123",
		AutoMergeDisable: true,
		Finder: shared.NewMockFinder(
			"https://github.com/OWNER/REPO/pull/123",
			&api.PullRequest{ID: "THE-ID", Number: 123},
			ghrepo.New("OWNER", "REPO"),
		),
	})
	assert.NoError(t, err)

	assert.Equal(t, "", stdout.String())
	assert.Equal(t, "✓ Auto-merge disabled for pull request #123\n", stderr.String())
}

type testEditor struct{}

func (e testEditor) Edit(filename, text string) (string, error) {
	return strings.ToUpper(text), nil
}
