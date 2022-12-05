package merge

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewCmdMerge(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "my-body.md")
	err := os.WriteFile(tmpFile, []byte("a body from file"), 0600)
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
				MergeStrategyEmpty:      true,
				Body:                    "",
				BodySet:                 false,
				AuthorEmail:             "",
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
				MergeStrategyEmpty:      true,
				Body:                    "",
				BodySet:                 false,
				AuthorEmail:             "",
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
				MergeStrategyEmpty:      true,
				Body:                    "a body from file",
				BodySet:                 true,
				AuthorEmail:             "",
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
				MergeStrategyEmpty:      true,
				Body:                    "this is on standard input",
				BodySet:                 true,
				AuthorEmail:             "",
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
				MergeStrategyEmpty:      true,
				Body:                    "cool",
				BodySet:                 true,
				AuthorEmail:             "",
			},
		},
		{
			name:  "match-head-commit specified",
			args:  "123 --match-head-commit 555",
			isTTY: true,
			want: MergeOptions{
				SelectorArg:             "123",
				DeleteBranch:            false,
				IsDeleteBranchIndicated: false,
				CanDeleteLocalBranch:    true,
				MergeMethod:             PullRequestMergeMethodMerge,
				MergeStrategyEmpty:      true,
				Body:                    "",
				BodySet:                 false,
				MatchHeadCommit:         "555",
				AuthorEmail:             "",
			},
		},
		{
			name:  "author email",
			args:  "123 --author-email octocat@github.com",
			isTTY: true,
			want: MergeOptions{
				SelectorArg:             "123",
				DeleteBranch:            false,
				IsDeleteBranchIndicated: false,
				CanDeleteLocalBranch:    true,
				MergeMethod:             PullRequestMergeMethodMerge,
				MergeStrategyEmpty:      true,
				Body:                    "",
				BodySet:                 false,
				AuthorEmail:             "octocat@github.com",
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
			ios, stdin, _, _ := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)

			if tt.stdin != "" {
				_, _ = stdin.WriteString(tt.stdin)
			}

			f := &cmdutil.Factory{
				IOStreams: ios,
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
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

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
			assert.Equal(t, tt.want.MergeStrategyEmpty, opts.MergeStrategyEmpty)
			assert.Equal(t, tt.want.Body, opts.Body)
			assert.Equal(t, tt.want.BodySet, opts.BodySet)
			assert.Equal(t, tt.want.MatchHeadCommit, opts.MatchHeadCommit)
			assert.Equal(t, tt.want.AuthorEmail, opts.AuthorEmail)
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

// TODO port to new style tests
func runCommand(rt http.RoundTripper, pm *prompter.PrompterMock, branch string, isTTY bool, cli string) (*test.CmdOut, error) {
	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(isTTY)
	ios.SetStdinTTY(isTTY)
	ios.SetStderrTTY(isTTY)

	factory := &cmdutil.Factory{
		IOStreams: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: rt}, nil
		},
		Branch: func() (string, error) {
			return branch, nil
		},
		Remotes: func() (context.Remotes, error) {
			return []*context.Remote{
				{
					Remote: &git.Remote{
						Name: "origin",
					},
					Repo: ghrepo.New("OWNER", "REPO"),
				},
			}, nil
		},
		GitClient: &git.Client{
			GhPath:  "some/path/gh",
			GitPath: "some/path/git",
		},
		Prompter: pm,
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
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

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
		baseRepo("OWNER", "REPO", "main"),
	)

	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
			assert.NotContains(t, input, "commitHeadline")
		}),
	)

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)
	cs.Register(`git rev-parse --verify refs/heads/`, 0, "")

	output, err := runCommand(http, nil, "main", true, "pr merge 1 --merge")
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
		baseRepo("OWNER", "REPO", "main"),
	)

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)
	cs.Register(`git rev-parse --verify refs/heads/`, 0, "")

	output, err := runCommand(http, nil, "main", true, "pr merge 1 --merge")
	assert.EqualError(t, err, "SilentError")

	assert.Equal(t, "", output.String())
	assert.Equal(t, heredoc.Docf(`
		X Pull request #1 is not mergeable: the base branch policy prohibits the merge.
		To have the pull request merged after all the requirements have been met, add the %[1]s--auto%[1]s flag.
		To use administrator privileges to immediately merge the pull request, add the %[1]s--admin%[1]s flag.
		`, "`"), output.Stderr())
}

func TestPrMerge_dirty(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	shared.RunCommandFinder(
		"1",
		&api.PullRequest{
			ID:               "THE-ID",
			Number:           123,
			State:            "OPEN",
			Title:            "The title of the PR",
			MergeStateStatus: "DIRTY",
			BaseRefName:      "trunk",
			HeadRefName:      "feature",
		},
		baseRepo("OWNER", "REPO", "main"),
	)

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)
	cs.Register(`git rev-parse --verify refs/heads/`, 0, "")

	output, err := runCommand(http, nil, "main", true, "pr merge 1 --merge")
	assert.EqualError(t, err, "SilentError")

	assert.Equal(t, "", output.String())
	assert.Equal(t, heredoc.Docf(`
		X Pull request #123 is not mergeable: the merge commit cannot be cleanly created.
		To have the pull request merged after all the requirements have been met, add the %[1]s--auto%[1]s flag.
		Run the following to resolve the merge conflicts locally:
		  gh pr checkout 123 && git fetch origin trunk && git merge origin/trunk
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
		baseRepo("OWNER", "REPO", "main"),
	)

	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
			assert.NotContains(t, input, "commitHeadline")
		}))

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git rev-parse --verify refs/heads/`, 0, "")

	output, err := runCommand(http, nil, "main", false, "pr merge 1 --merge")
	if err != nil {
		t.Fatalf("error running command `pr merge`: %v", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
}

func TestPrMerge_editMessage_nontty(t *testing.T) {
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
		baseRepo("OWNER", "REPO", "main"),
	)

	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
			assert.Equal(t, "mytitle", input["commitHeadline"].(string))
			assert.Equal(t, "mybody", input["commitBody"].(string))
		}))

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git rev-parse --verify refs/heads/`, 0, "")

	output, err := runCommand(http, nil, "main", false, "pr merge 1 --merge -t mytitle -b mybody")
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
		baseRepo("OWNER", "REPO", "main"),
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

	output, err := runCommand(http, nil, "main", true, "pr merge 1 --merge -R OWNER/REPO")
	if err != nil {
		t.Fatalf("error running command `pr merge`: %v", err)
	}

	r := regexp.MustCompile(`Merged pull request #1 \(The title of the PR\)`)

	if !r.MatchString(output.Stderr()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestPrMerge_withMatchCommitHeadFlag(t *testing.T) {
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
		baseRepo("OWNER", "REPO", "main"),
	)

	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, 3, len(input))
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
			assert.Equal(t, "285ed5ab740f53ff6b0b4b629c59a9df23b9c6db", input["expectedHeadOid"].(string))
		}))

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)
	cs.Register(`git rev-parse --verify refs/heads/`, 0, "")

	output, err := runCommand(http, nil, "main", true, "pr merge 1 --merge --match-head-commit 285ed5ab740f53ff6b0b4b629c59a9df23b9c6db")
	if err != nil {
		t.Fatalf("error running command `pr merge`: %v", err)
	}

	r := regexp.MustCompile(`Merged pull request #1 \(The title of the PR\)`)

	if !r.MatchString(output.Stderr()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestPrMerge_withAuthorFlag(t *testing.T) {
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
		baseRepo("OWNER", "REPO", "main"),
	)

	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
			assert.Equal(t, "octocat@github.com", input["authorEmail"].(string))
			assert.NotContains(t, input, "commitHeadline")
		}),
	)

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)
	cs.Register(`git rev-parse --verify refs/heads/`, 0, "")

	output, err := runCommand(http, nil, "main", true, "pr merge 1 --merge --author-email octocat@github.com")
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
			BaseRefName:      "main",
			MergeStateStatus: "CLEAN",
		},
		baseRepo("OWNER", "REPO", "main"),
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

	cs.Register(`git rev-parse --verify refs/heads/main`, 0, "")
	cs.Register(`git checkout main`, 0, "")
	cs.Register(`git rev-parse --verify refs/heads/blueberries`, 0, "")
	cs.Register(`git branch -D blueberries`, 0, "")
	cs.Register(`git pull --ff-only`, 0, "")

	output, err := runCommand(http, nil, "blueberries", true, `pr merge --merge --delete-branch`)
	if err != nil {
		t.Fatalf("Got unexpected error running `pr merge` %s", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, heredoc.Doc(`
		✓ Merged pull request #10 (Blueberries are a good fruit)
		✓ Deleted branch blueberries and switched to branch main
	`), output.Stderr())
}

func TestPrMerge_deleteBranch_nonDefault(t *testing.T) {
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
			BaseRefName:      "fruit",
		},
		baseRepo("OWNER", "REPO", "main"),
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

	cs.Register(`git rev-parse --verify refs/heads/fruit`, 0, "")
	cs.Register(`git checkout fruit`, 0, "")
	cs.Register(`git rev-parse --verify refs/heads/blueberries`, 0, "")
	cs.Register(`git branch -D blueberries`, 0, "")
	cs.Register(`git pull --ff-only`, 0, "")

	output, err := runCommand(http, nil, "blueberries", true, `pr merge --merge --delete-branch`)
	if err != nil {
		t.Fatalf("Got unexpected error running `pr merge` %s", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, heredoc.Doc(`
		✓ Merged pull request #10 (Blueberries are a good fruit)
		✓ Deleted branch blueberries and switched to branch fruit
	`), output.Stderr())
}

func TestPrMerge_deleteBranch_checkoutNewBranch(t *testing.T) {
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
			BaseRefName:      "fruit",
		},
		baseRepo("OWNER", "REPO", "main"),
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

	cs.Register(`git rev-parse --verify refs/heads/fruit`, 1, "")
	cs.Register(`git checkout -b fruit --track origin/fruit`, 0, "")
	cs.Register(`git rev-parse --verify refs/heads/blueberries`, 0, "")
	cs.Register(`git branch -D blueberries`, 0, "")
	cs.Register(`git pull --ff-only`, 0, "")

	output, err := runCommand(http, nil, "blueberries", true, `pr merge --merge --delete-branch`)
	if err != nil {
		t.Fatalf("Got unexpected error running `pr merge` %s", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, heredoc.Doc(`
		✓ Merged pull request #10 (Blueberries are a good fruit)
		✓ Deleted branch blueberries and switched to branch fruit
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
		baseRepo("OWNER", "REPO", "main"),
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

	output, err := runCommand(http, nil, "main", true, `pr merge --merge --delete-branch blueberries`)
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
		BaseRefName:      "main",
	}
	stubCommit(pr, "COMMITSHA1")

	shared.RunCommandFinder("", pr, baseRepo("OWNER", "REPO", "main"))

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
	cs.Register(`git rev-parse --verify refs/heads/`, 0, "")

	output, err := runCommand(http, nil, "blueberries", true, "pr merge --merge")
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
		BaseRefName:      "main",
	}
	stubCommit(pr, "COMMITSHA1")

	shared.RunCommandFinder("", pr, baseRepo("OWNER", "REPO", "main"))

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
	cs.Register(`git rev-parse --verify refs/heads/`, 0, "")

	output, err := runCommand(http, nil, "blueberries", true, "pr merge --merge")
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
		baseRepo("OWNER", "REPO", "main"),
	)

	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "PR_10", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
			assert.NotContains(t, input, "commitHeadline")
		}))

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git rev-parse --verify refs/heads/`, 0, "")

	output, err := runCommand(http, nil, "blueberries", true, "pr merge --merge")
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
		baseRepo("OWNER", "REPO", "main"),
	)

	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "REBASE", input["mergeMethod"].(string))
			assert.NotContains(t, input, "commitHeadline")
		}))

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git rev-parse --verify refs/heads/`, 0, "")

	output, err := runCommand(http, nil, "main", true, "pr merge 2 --rebase")
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
		baseRepo("OWNER", "REPO", "main"),
	)

	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "SQUASH", input["mergeMethod"].(string))
			assert.NotContains(t, input, "commitHeadline")
		}))

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git rev-parse --verify refs/heads/`, 0, "")

	output, err := runCommand(http, nil, "main", true, "pr merge 3 --squash")
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
			BaseRefName:      "main",
			MergeStateStatus: "CLEAN",
		},
		baseRepo("OWNER", "REPO", "main"),
	)

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git rev-parse --verify refs/heads/main`, 0, "")
	cs.Register(`git checkout main`, 0, "")
	cs.Register(`git rev-parse --verify refs/heads/blueberries`, 0, "")
	cs.Register(`git branch -D blueberries`, 0, "")
	cs.Register(`git pull --ff-only`, 0, "")

	pm := &prompter.PrompterMock{
		ConfirmFunc: func(p string, d bool) (bool, error) {
			if p == "Pull request #4 was already merged. Delete the branch locally?" {
				return true, nil
			} else {
				return false, prompter.NoSuchPromptErr(p)
			}
		},
	}

	output, err := runCommand(http, pm, "blueberries", true, "pr merge 4")
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, "✓ Deleted branch blueberries and switched to branch main\n", output.Stderr())
}

func TestPrMerge_alreadyMerged_withMergeStrategy(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	shared.RunCommandFinder(
		"4",
		&api.PullRequest{
			ID:                  "THE-ID",
			Number:              4,
			State:               "MERGED",
			HeadRepositoryOwner: api.Owner{Login: "OWNER"},
			MergeStateStatus:    "CLEAN",
		},
		baseRepo("OWNER", "REPO", "main"),
	)

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git rev-parse --verify refs/heads/`, 0, "")

	output, err := runCommand(http, nil, "blueberries", false, "pr merge 4 --merge")
	if err != nil {
		t.Fatalf("Got unexpected error running `pr merge` %s", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, "! Pull request #4 was already merged\n", output.Stderr())
}

func TestPrMerge_alreadyMerged_withMergeStrategy_TTY(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	shared.RunCommandFinder(
		"4",
		&api.PullRequest{
			ID:                  "THE-ID",
			Number:              4,
			State:               "MERGED",
			HeadRepositoryOwner: api.Owner{Login: "OWNER"},
			MergeStateStatus:    "CLEAN",
		},
		baseRepo("OWNER", "REPO", "main"),
	)

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git rev-parse --verify refs/heads/`, 0, "")
	cs.Register(`git branch -D `, 0, "")

	pm := &prompter.PrompterMock{
		ConfirmFunc: func(p string, d bool) (bool, error) {
			if p == "Pull request #4 was already merged. Delete the branch locally?" {
				return true, nil
			} else {
				return false, prompter.NoSuchPromptErr(p)
			}
		},
	}

	output, err := runCommand(http, pm, "blueberries", true, "pr merge 4 --merge")
	if err != nil {
		t.Fatalf("Got unexpected error running `pr merge` %s", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, "✓ Deleted branch \n", output.Stderr())
}

func TestPrMerge_alreadyMerged_withMergeStrategy_crossRepo(t *testing.T) {
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
		baseRepo("OWNER", "REPO", "main"),
	)

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git rev-parse --verify refs/heads/`, 0, "")

	output, err := runCommand(http, nil, "blueberries", true, "pr merge 4 --merge")
	if err != nil {
		t.Fatalf("Got unexpected error running `pr merge` %s", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
}
func TestPRMergeTTY(t *testing.T) {
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
		baseRepo("OWNER", "REPO", "main"),
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

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git rev-parse --verify refs/heads/blueberries`, 0, "")

	pm := &prompter.PrompterMock{
		ConfirmFunc: func(p string, d bool) (bool, error) {
			if p == "Delete the branch locally and on GitHub?" {
				return d, nil
			} else {
				return false, prompter.NoSuchPromptErr(p)
			}
		},
		SelectFunc: func(p, d string, opts []string) (int, error) {
			switch p {
			case "What's next?":
				return prompter.IndexFor(opts, "Submit")
			case "What merge method would you like to use?":
				return 0, nil
			default:
				return -1, prompter.NoSuchPromptErr(p)
			}
		},
	}

	output, err := runCommand(http, pm, "blueberries", true, "")
	if err != nil {
		t.Fatalf("Got unexpected error running `pr merge` %s", err)
	}

	assert.Equal(t, "✓ Merged pull request #3 (It was the best of times)\n", output.Stderr())
}

func TestPRMergeTTY_withDeleteBranch(t *testing.T) {
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
			BaseRefName:      "main",
		},
		baseRepo("OWNER", "REPO", "main"),
	)

	http.Register(
		httpmock.GraphQL(`query RepositoryInfo\b`),
		httpmock.StringResponse(`
		{ "data": { "repository": {
			"mergeCommitAllowed": true,
			"rebaseMergeAllowed": true,
			"squashMergeAllowed": true,
			"mergeQueue": {
				"mergeMethod": ""
			}
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

	cs.Register(`git rev-parse --verify refs/heads/main`, 0, "")
	cs.Register(`git checkout main`, 0, "")
	cs.Register(`git rev-parse --verify refs/heads/blueberries`, 0, "")
	cs.Register(`git branch -D blueberries`, 0, "")
	cs.Register(`git pull --ff-only`, 0, "")

	pm := &prompter.PrompterMock{
		SelectFunc: func(p, d string, opts []string) (int, error) {
			switch p {
			case "What's next?":
				return prompter.IndexFor(opts, "Submit")
			case "What merge method would you like to use?":
				return 0, nil
			default:
				return -1, prompter.NoSuchPromptErr(p)
			}
		},
	}

	output, err := runCommand(http, pm, "blueberries", true, "-d")
	if err != nil {
		t.Fatalf("Got unexpected error running `pr merge` %s", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, heredoc.Doc(`
		✓ Merged pull request #3 (It was the best of times)
		✓ Deleted branch blueberries and switched to branch main
	`), output.Stderr())
}

func TestPRMergeTTY_squashEditCommitMsgAndSubject(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdinTTY(true)
	ios.SetStdoutTTY(true)
	ios.SetStderrTTY(true)

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
			"viewerMergeHeadlineText": "default headline text",
			"viewerMergeBodyText": "default body text"
		} } }`))
	tr.Register(
		httpmock.GraphQL(`query PullRequestMergeText\b`),
		httpmock.StringResponse(`
		{ "data": { "node": {
			"viewerMergeHeadlineText": "default headline text",
			"viewerMergeBodyText": "default body text"
		} } }`))
	tr.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "SQUASH", input["mergeMethod"].(string))
			assert.Equal(t, "DEFAULT HEADLINE TEXT", input["commitHeadline"].(string))
			assert.Equal(t, "DEFAULT BODY TEXT", input["commitBody"].(string))
		}))

	_, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	selectCount := -1
	answers := []string{"Edit commit message", "Edit commit subject", "Submit"}

	pm := &prompter.PrompterMock{
		ConfirmFunc: func(p string, d bool) (bool, error) {
			if p == "Delete the branch on GitHub?" {
				return d, nil
			} else {
				return false, prompter.NoSuchPromptErr(p)
			}
		},
		SelectFunc: func(p, d string, opts []string) (int, error) {
			switch p {
			case "What's next?":
				selectCount++
				return prompter.IndexFor(opts, answers[selectCount])
			case "What merge method would you like to use?":
				return prompter.IndexFor(opts, "Squash and merge")
			default:
				return -1, prompter.NoSuchPromptErr(p)
			}
		},
	}

	err := mergeRun(&MergeOptions{
		IO:     ios,
		Editor: testEditor{},
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: tr}, nil
		},
		Prompter:           pm,
		SelectorArg:        "https://github.com/OWNER/REPO/pull/123",
		MergeStrategyEmpty: true,
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

func TestPRMergeEmptyStrategyNonTTY(t *testing.T) {
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
			BaseRefName:      "main",
		},
		baseRepo("OWNER", "REPO", "main"),
	)

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)
	cs.Register(`git rev-parse --verify refs/heads/`, 0, "")

	output, err := runCommand(http, nil, "blueberries", false, "pr merge 1")
	assert.EqualError(t, err, "--merge, --rebase, or --squash required when not running interactively")
	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
}

func TestPRTTY_cancelled(t *testing.T) {
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

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git rev-parse --verify refs/heads/`, 0, "")

	pm := &prompter.PrompterMock{
		ConfirmFunc: func(p string, d bool) (bool, error) {
			if p == "Delete the branch locally and on GitHub?" {
				return d, nil
			} else {
				return false, prompter.NoSuchPromptErr(p)
			}
		},
		SelectFunc: func(p, d string, opts []string) (int, error) {
			switch p {
			case "What's next?":
				return prompter.IndexFor(opts, "Cancel")
			case "What merge method would you like to use?":
				return 0, nil
			default:
				return -1, prompter.NoSuchPromptErr(p)
			}
		},
	}

	output, err := runCommand(http, pm, "blueberries", true, "")
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

	pm := &prompter.PrompterMock{
		SelectFunc: func(p, d string, opts []string) (int, error) {
			if p == "What merge method would you like to use?" {
				return prompter.IndexFor(opts, "Rebase and merge")
			} else {
				return -1, prompter.NoSuchPromptErr(p)
			}
		},
	}

	method, err := mergeMethodSurvey(pm, repo)
	assert.Nil(t, err)
	assert.Equal(t, PullRequestMergeMethodRebase, method)
}

func TestMergeRun_autoMerge(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(true)
	ios.SetStderrTTY(true)

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
		IO: ios,
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
	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(true)
	ios.SetStderrTTY(true)

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
		IO: ios,
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
	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(true)
	ios.SetStderrTTY(true)

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
		IO: ios,
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

func TestPrInMergeQueue(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	shared.RunCommandFinder(
		"1",
		&api.PullRequest{
			ID:                  "THE-ID",
			Number:              1,
			State:               "OPEN",
			Title:               "The title of the PR",
			MergeStateStatus:    "CLEAN",
			IsInMergeQueue:      true,
			IsMergeQueueEnabled: true,
		},
		baseRepo("OWNER", "REPO", "main"),
	)

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)
	cs.Register(`git rev-parse --verify refs/heads/`, 0, "")

	output, err := runCommand(http, nil, "blueberries", true, "pr merge 1")
	if err != nil {
		t.Fatalf("error running command `pr merge`: %v", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, "! Pull request #1 is already queued to merge\n", output.Stderr())
}

func TestPrAddToMergeQueueWithMergeMethod(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	shared.RunCommandFinder(
		"1",
		&api.PullRequest{
			ID:                  "THE-ID",
			Number:              1,
			State:               "OPEN",
			Title:               "The title of the PR",
			MergeStateStatus:    "CLEAN",
			IsInMergeQueue:      false,
			IsMergeQueueEnabled: true,
			BaseRefName:         "main",
		},
		baseRepo("OWNER", "REPO", "main"),
	)
	http.Register(
		httpmock.GraphQL(`mutation PullRequestAutoMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
		}),
	)

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)
	cs.Register(`git rev-parse --verify refs/heads/`, 0, "")

	output, err := runCommand(http, nil, "blueberries", true, "pr merge 1 --merge")
	if err != nil {
		t.Fatalf("error running command `pr merge`: %v", err)
	}
	assert.Equal(t, "", output.String())
	assert.Equal(t, "! The merge strategy for main is set by the merge queue\n✓ Pull request #1 will be added to the merge queue for main when ready\n", output.Stderr())
}

func TestPrAddToMergeQueueClean(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	shared.RunCommandFinder(
		"1",
		&api.PullRequest{
			ID:                  "THE-ID",
			Number:              1,
			State:               "OPEN",
			Title:               "The title of the PR",
			MergeStateStatus:    "CLEAN",
			IsInMergeQueue:      false,
			IsMergeQueueEnabled: true,
			BaseRefName:         "main",
		},
		baseRepo("OWNER", "REPO", "main"),
	)

	http.Register(
		httpmock.GraphQL(`mutation PullRequestAutoMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
		}),
	)

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)
	cs.Register(`git rev-parse --verify refs/heads/`, 0, "")

	output, err := runCommand(http, nil, "blueberries", true, "pr merge 1")
	if err != nil {
		t.Fatalf("error running command `pr merge`: %v", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, "✓ Pull request #1 will be added to the merge queue for main when ready\n", output.Stderr())
}

func TestPrAddToMergeQueueBlocked(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	shared.RunCommandFinder(
		"1",
		&api.PullRequest{
			ID:                  "THE-ID",
			Number:              1,
			State:               "OPEN",
			Title:               "The title of the PR",
			MergeStateStatus:    "BLOCKED",
			IsInMergeQueue:      false,
			IsMergeQueueEnabled: true,
			BaseRefName:         "main",
		},
		baseRepo("OWNER", "REPO", "main"),
	)

	http.Register(
		httpmock.GraphQL(`mutation PullRequestAutoMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
		}),
	)

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)
	cs.Register(`git rev-parse --verify refs/heads/`, 0, "")

	output, err := runCommand(http, nil, "blueberries", true, "pr merge 1")
	if err != nil {
		t.Fatalf("error running command `pr merge`: %v", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, "✓ Pull request #1 will be added to the merge queue for main when ready\n", output.Stderr())
}

func TestPrAddToMergeQueueAdmin(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	shared.RunCommandFinder(
		"1",
		&api.PullRequest{
			ID:                  "THE-ID",
			Number:              1,
			State:               "OPEN",
			Title:               "The title of the PR",
			MergeStateStatus:    "CLEAN",
			IsInMergeQueue:      false,
			IsMergeQueueEnabled: true,
		},
		baseRepo("OWNER", "REPO", "main"),
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
		}),
	)

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)
	cs.Register(`git rev-parse --verify refs/heads/`, 0, "")

	pm := &prompter.PrompterMock{
		ConfirmFunc: func(p string, d bool) (bool, error) {
			if p == "Delete the branch locally and on GitHub?" {
				return d, nil
			} else {
				return false, prompter.NoSuchPromptErr(p)
			}
		},
		SelectFunc: func(p, d string, opts []string) (int, error) {
			switch p {
			case "What's next?":
				return 0, nil
			case "What merge method would you like to use?":
				return 0, nil
			default:
				return -1, prompter.NoSuchPromptErr(p)
			}
		},
	}

	output, err := runCommand(http, pm, "blueberries", true, "pr merge 1 --admin")
	if err != nil {
		t.Fatalf("error running command `pr merge`: %v", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, "✓ Merged pull request #1 (The title of the PR)\n", output.Stderr())
}

func TestPrAddToMergeQueueAdminWithMergeStrategy(t *testing.T) {
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
			IsInMergeQueue:   false,
		},
		baseRepo("OWNER", "REPO", "main"),
	)

	http.Register(
		httpmock.GraphQL(`mutation PullRequestMerge\b`),
		httpmock.GraphQLMutation(`{}`, func(input map[string]interface{}) {
			assert.Equal(t, "THE-ID", input["pullRequestId"].(string))
			assert.Equal(t, "MERGE", input["mergeMethod"].(string))
			assert.NotContains(t, input, "commitHeadline")
		}),
	)

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)
	cs.Register(`git rev-parse --verify refs/heads/`, 0, "")

	output, err := runCommand(http, nil, "blueberries", true, "pr merge 1 --admin --merge")
	if err != nil {
		t.Fatalf("error running command `pr merge`: %v", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, "✓ Merged pull request #1 (The title of the PR)\n", output.Stderr())
}

type testEditor struct{}

func (e testEditor) Edit(filename, text string) (string, error) {
	return strings.ToUpper(text), nil
}
