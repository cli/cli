package status

import (
	"bytes"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/test"
	"github.com/google/shlex"
)

func runCommand(rt http.RoundTripper, branch string, isTTY bool, cli string) (*test.CmdOut, error) {
	return runCommandWithDetector(rt, branch, isTTY, cli, &fd.DisabledDetectorMock{})
}

func runCommandWithDetector(rt http.RoundTripper, branch string, isTTY bool, cli string, detector fd.Detector) (*test.CmdOut, error) {
	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(isTTY)
	ios.SetStdinTTY(isTTY)
	ios.SetStderrTTY(isTTY)

	factory := &cmdutil.Factory{
		IOStreams: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: rt}, nil
		},
		Config: func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
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
			if branch == "" {
				return "", git.ErrNotOnAnyBranch
			}
			return branch, nil
		},
		GitClient: &git.Client{GitPath: "some/path/git"},
	}

	withProvidedDetector := func(opts *StatusOptions) error {
		opts.Detector = detector
		return statusRun(opts)
	}

	cmd := NewCmdStatus(factory, withProvidedDetector)
	cmd.PersistentFlags().StringP("repo", "R", "", "")

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

func TestPRStatus(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)
	http.Register(httpmock.GraphQL(`query PullRequestStatus\b`), httpmock.FileResponse("./fixtures/prStatus.json"))

	output, err := runCommand(http, "blueberries", true, "")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expectedPrs := []*regexp.Regexp{
		regexp.MustCompile(`#8.*\[strawberries\]`),
		regexp.MustCompile(`#9.*\[apples\].*✓ Auto-merge enabled`),
		regexp.MustCompile(`#10.*\[blueberries\]`),
		regexp.MustCompile(`#11.*\[figs\]`),
	}

	for _, r := range expectedPrs {
		if !r.MatchString(output.String()) {
			t.Errorf("output did not match regexp /%s/", r)
		}
	}
}

func TestPRStatus_reviewsAndChecks(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)
	// status,conclusion matches the old StatusContextRollup query
	http.Register(httpmock.GraphQL(`status,conclusion`), httpmock.FileResponse("./fixtures/prStatusChecks.json"))

	output, err := runCommand(http, "blueberries", true, "")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expected := []string{
		"✓ Checks passing + Changes requested ! Merge conflict status unknown",
		"- Checks pending ✓ 2 Approved",
		"× 1/3 checks failing - Review required ✓ No merge conflicts",
		"✓ Checks passing × Merge conflicts",
	}

	for _, line := range expected {
		if !strings.Contains(output.String(), line) {
			t.Errorf("output did not contain %q: %q", line, output.String())
		}
	}
}

func TestPRStatus_reviewsAndChecksWithStatesByCount(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)
	// checkRunCount,checkRunCountsByState matches the new StatusContextRollup query
	http.Register(httpmock.GraphQL(`checkRunCount,checkRunCountsByState`), httpmock.FileResponse("./fixtures/prStatusChecksWithStatesByCount.json"))

	output, err := runCommandWithDetector(http, "blueberries", true, "", &fd.EnabledDetectorMock{})
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expected := []string{
		"✓ Checks passing + Changes requested ! Merge conflict status unknown",
		"- Checks pending ✓ 2 Approved",
		"× 1/3 checks failing - Review required ✓ No merge conflicts",
		"✓ Checks passing × Merge conflicts",
	}

	for _, line := range expected {
		if !strings.Contains(output.String(), line) {
			t.Errorf("output did not contain %q: %q", line, output.String())
		}
	}
}

func TestPRStatus_currentBranch_showTheMostRecentPR(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)
	http.Register(httpmock.GraphQL(`query PullRequestStatus\b`), httpmock.FileResponse("./fixtures/prStatusCurrentBranch.json"))

	output, err := runCommand(http, "blueberries", true, "")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expectedLine := regexp.MustCompile(`#10  Blueberries are certainly a good fruit \[blueberries\]`)
	if !expectedLine.MatchString(output.String()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", expectedLine, output)
		return
	}

	unexpectedLines := []*regexp.Regexp{
		regexp.MustCompile(`#9  Blueberries are a good fruit \[blueberries\] - Merged`),
		regexp.MustCompile(`#8  Blueberries are probably a good fruit \[blueberries\] - Closed`),
	}
	for _, r := range unexpectedLines {
		if r.MatchString(output.String()) {
			t.Errorf("output unexpectedly match regexp /%s/\n> output\n%s\n", r, output)
			return
		}
	}
}

func TestPRStatus_currentBranch_defaultBranch(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)
	http.Register(httpmock.GraphQL(`query PullRequestStatus\b`), httpmock.FileResponse("./fixtures/prStatusCurrentBranch.json"))

	output, err := runCommand(http, "blueberries", true, "")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expectedLine := regexp.MustCompile(`#10  Blueberries are certainly a good fruit \[blueberries\]`)
	if !expectedLine.MatchString(output.String()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", expectedLine, output)
		return
	}
}

func TestPRStatus_currentBranch_defaultBranch_repoFlag(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)
	http.Register(httpmock.GraphQL(`query PullRequestStatus\b`), httpmock.FileResponse("./fixtures/prStatusCurrentBranchClosedOnDefaultBranch.json"))

	output, err := runCommand(http, "blueberries", true, "-R OWNER/REPO")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expectedLine := regexp.MustCompile(`#8  Blueberries are a good fruit \[blueberries\]`)
	if expectedLine.MatchString(output.String()) {
		t.Errorf("output not expected to match regexp /%s/\n> output\n%s\n", expectedLine, output)
		return
	}
}

func TestPRStatus_currentBranch_Closed(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)
	http.Register(httpmock.GraphQL(`query PullRequestStatus\b`), httpmock.FileResponse("./fixtures/prStatusCurrentBranchClosed.json"))

	output, err := runCommand(http, "blueberries", true, "")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expectedLine := regexp.MustCompile(`#8  Blueberries are a good fruit \[blueberries\] - Closed`)
	if !expectedLine.MatchString(output.String()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", expectedLine, output)
		return
	}
}

func TestPRStatus_currentBranch_Closed_defaultBranch(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)
	http.Register(httpmock.GraphQL(`query PullRequestStatus\b`), httpmock.FileResponse("./fixtures/prStatusCurrentBranchClosedOnDefaultBranch.json"))

	output, err := runCommand(http, "blueberries", true, "")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expectedLine := regexp.MustCompile(`There is no pull request associated with \[blueberries\]`)
	if !expectedLine.MatchString(output.String()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", expectedLine, output)
		return
	}
}

func TestPRStatus_currentBranch_Merged(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)
	http.Register(httpmock.GraphQL(`query PullRequestStatus\b`), httpmock.FileResponse("./fixtures/prStatusCurrentBranchMerged.json"))

	output, err := runCommand(http, "blueberries", true, "")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expectedLine := regexp.MustCompile(`#8  Blueberries are a good fruit \[blueberries\] - Merged`)
	if !expectedLine.MatchString(output.String()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", expectedLine, output)
		return
	}
}

func TestPRStatus_currentBranch_Merged_defaultBranch(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)
	http.Register(httpmock.GraphQL(`query PullRequestStatus\b`), httpmock.FileResponse("./fixtures/prStatusCurrentBranchMergedOnDefaultBranch.json"))

	output, err := runCommand(http, "blueberries", true, "")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expectedLine := regexp.MustCompile(`There is no pull request associated with \[blueberries\]`)
	if !expectedLine.MatchString(output.String()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", expectedLine, output)
		return
	}
}

func TestPRStatus_blankSlate(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)
	http.Register(httpmock.GraphQL(`query PullRequestStatus\b`), httpmock.StringResponse(`{"data": {}}`))

	output, err := runCommand(http, "blueberries", true, "")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expected := `
Relevant pull requests in OWNER/REPO

Current branch
  There is no pull request associated with [blueberries]

Created by you
  You have no open pull requests

Requesting a code review from you
  You have no pull requests to review

`
	if output.String() != expected {
		t.Errorf("expected %q, got %q", expected, output.String())
	}
}

func TestPRStatus_blankSlateRepoOverride(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)
	http.Register(httpmock.GraphQL(`query PullRequestStatus\b`), httpmock.StringResponse(`{"data": {}}`))

	output, err := runCommand(http, "blueberries", true, "--repo OWNER/REPO")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expected := `
Relevant pull requests in OWNER/REPO

Created by you
  You have no open pull requests

Requesting a code review from you
  You have no pull requests to review

`
	if output.String() != expected {
		t.Errorf("expected %q, got %q", expected, output.String())
	}
}

func TestPRStatus_detachedHead(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)
	http.Register(httpmock.GraphQL(`query PullRequestStatus\b`), httpmock.StringResponse(`{"data": {}}`))

	output, err := runCommand(http, "", true, "")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expected := `
Relevant pull requests in OWNER/REPO

Current branch
  There is no current branch

Created by you
  You have no open pull requests

Requesting a code review from you
  You have no pull requests to review

`
	if output.String() != expected {
		t.Errorf("expected %q, got %q", expected, output.String())
	}
}

func Test_prSelectorForCurrentBranch(t *testing.T) {
	rs, cleanup := run.Stub()
	defer cleanup(t)

	rs.Register(`git config --get-regexp \^branch\\.`, 0, heredoc.Doc(`
		branch.Frederick888/main.remote git@github.com:Frederick888/playground.git
		branch.Frederick888/main.merge refs/heads/main
	`))
	rs.Register(`git rev-parse --verify --quiet --abbrev-ref Frederick888/main@\{push\}`, 1, "")

	repo := ghrepo.NewWithHost("octocat", "playground", "github.com")
	rem := context.Remotes{
		&context.Remote{
			Remote: &git.Remote{Name: "origin"},
			Repo:   repo,
		},
	}
	gitClient := &git.Client{GitPath: "some/path/git"}
	prNum, headRef, err := prSelectorForCurrentBranch(gitClient, repo, "Frederick888/main", rem)
	if err != nil {
		t.Fatalf("prSelectorForCurrentBranch error: %v", err)
	}
	if prNum != 0 {
		t.Errorf("expected prNum to be 0, got %q", prNum)
	}
	if headRef != "Frederick888:main" {
		t.Errorf("expected headRef to be \"Frederick888:main\", got %q", headRef)
	}
}
