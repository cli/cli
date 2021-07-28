package close

import (
	"bytes"
	"io/ioutil"
	"net/http"
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
	"github.com/cli/cli/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

// repo: either "baseOwner/baseRepo" or "baseOwner/baseRepo:defaultBranch"
// prHead: "headOwner/headRepo:headBranch"
func stubPR(repo, prHead string) (ghrepo.Interface, *api.PullRequest) {
	defaultBranch := ""
	if idx := strings.IndexRune(repo, ':'); idx >= 0 {
		defaultBranch = repo[idx+1:]
		repo = repo[:idx]
	}
	baseRepo, err := ghrepo.FromFullName(repo)
	if err != nil {
		panic(err)
	}
	if defaultBranch != "" {
		baseRepo = api.InitRepoHostname(&api.Repository{
			Name:             baseRepo.RepoName(),
			Owner:            api.RepositoryOwner{Login: baseRepo.RepoOwner()},
			DefaultBranchRef: api.BranchRef{Name: defaultBranch},
		}, baseRepo.RepoHost())
	}

	idx := strings.IndexRune(prHead, ':')
	headRefName := prHead[idx+1:]
	headRepo, err := ghrepo.FromFullName(prHead[:idx])
	if err != nil {
		panic(err)
	}

	return baseRepo, &api.PullRequest{
		ID:                  "THE-ID",
		Number:              96,
		State:               "OPEN",
		HeadRefName:         headRefName,
		HeadRepositoryOwner: api.Owner{Login: headRepo.RepoOwner()},
		HeadRepository:      &api.PRRepository{Name: headRepo.RepoName()},
		IsCrossRepository:   !ghrepo.IsSame(baseRepo, headRepo),
	}
}

func runCommand(rt http.RoundTripper, isTTY bool, cli string) (*test.CmdOut, error) {
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
			return "trunk", nil
		},
	}

	cmd := NewCmdClose(factory, nil)

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

func TestPrClose(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	baseRepo, pr := stubPR("OWNER/REPO", "OWNER/REPO:feature")
	pr.Title = "The title of the PR"
	shared.RunCommandFinder("96", pr, baseRepo)

	http.Register(
		httpmock.GraphQL(`mutation PullRequestClose\b`),
		httpmock.GraphQLMutation(`{"id": "THE-ID"}`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, inputs["pullRequestId"], "THE-ID")
			}),
	)

	output, err := runCommand(http, true, "96")
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, "✓ Closed pull request #96 (The title of the PR)\n", output.Stderr())
}

func TestPrClose_alreadyClosed(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	baseRepo, pr := stubPR("OWNER/REPO", "OWNER/REPO:feature")
	pr.State = "CLOSED"
	pr.Title = "The title of the PR"
	shared.RunCommandFinder("96", pr, baseRepo)

	output, err := runCommand(http, true, "96")
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, "! Pull request #96 (The title of the PR) is already closed\n", output.Stderr())
}

func TestPrClose_deleteBranch_sameRepo(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	baseRepo, pr := stubPR("OWNER/REPO", "OWNER/REPO:blueberries")
	pr.Title = "The title of the PR"
	shared.RunCommandFinder("96", pr, baseRepo)

	http.Register(
		httpmock.GraphQL(`mutation PullRequestClose\b`),
		httpmock.GraphQLMutation(`{"id": "THE-ID"}`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, inputs["pullRequestId"], "THE-ID")
			}),
	)
	http.Register(
		httpmock.REST("DELETE", "repos/OWNER/REPO/git/refs/heads/blueberries"),
		httpmock.StringResponse(`{}`))

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git rev-parse --verify refs/heads/blueberries`, 0, "")
	cs.Register(`git branch -D blueberries`, 0, "")

	output, err := runCommand(http, true, `96 --delete-branch`)
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, heredoc.Doc(`
		✓ Closed pull request #96 (The title of the PR)
		✓ Deleted branch blueberries
	`), output.Stderr())
}

func TestPrClose_deleteBranch_crossRepo(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	baseRepo, pr := stubPR("OWNER/REPO", "hubot/REPO:blueberries")
	pr.Title = "The title of the PR"
	shared.RunCommandFinder("96", pr, baseRepo)

	http.Register(
		httpmock.GraphQL(`mutation PullRequestClose\b`),
		httpmock.GraphQLMutation(`{"id": "THE-ID"}`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, inputs["pullRequestId"], "THE-ID")
			}),
	)

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git rev-parse --verify refs/heads/blueberries`, 0, "")
	cs.Register(`git branch -D blueberries`, 0, "")

	output, err := runCommand(http, true, `96 --delete-branch`)
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, heredoc.Doc(`
		✓ Closed pull request #96 (The title of the PR)
		! Skipped deleting the remote branch of a pull request from fork
		✓ Deleted branch blueberries
	`), output.Stderr())
}

func TestPrClose_deleteBranch_sameBranch(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	baseRepo, pr := stubPR("OWNER/REPO:main", "OWNER/REPO:trunk")
	pr.Title = "The title of the PR"
	shared.RunCommandFinder("96", pr, baseRepo)

	http.Register(
		httpmock.GraphQL(`mutation PullRequestClose\b`),
		httpmock.GraphQLMutation(`{"id": "THE-ID"}`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, inputs["pullRequestId"], "THE-ID")
			}),
	)
	http.Register(
		httpmock.REST("DELETE", "repos/OWNER/REPO/git/refs/heads/trunk"),
		httpmock.StringResponse(`{}`))

	cs, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	cs.Register(`git checkout main`, 0, "")
	cs.Register(`git rev-parse --verify refs/heads/trunk`, 0, "")
	cs.Register(`git branch -D trunk`, 0, "")

	output, err := runCommand(http, true, `96 --delete-branch`)
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, heredoc.Doc(`
		✓ Closed pull request #96 (The title of the PR)
		✓ Deleted branch trunk and switched to branch main
	`), output.Stderr())
}
