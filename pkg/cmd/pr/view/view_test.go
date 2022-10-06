package view

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghrepo"
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

func Test_NewCmdView(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		isTTY   bool
		want    ViewOptions
		wantErr string
	}{
		{
			name:  "number argument",
			args:  "123",
			isTTY: true,
			want: ViewOptions{
				SelectorArg: "123",
				BrowserMode: false,
			},
		},
		{
			name:  "no argument",
			args:  "",
			isTTY: true,
			want: ViewOptions{
				SelectorArg: "",
				BrowserMode: false,
			},
		},
		{
			name:  "web mode",
			args:  "123 -w",
			isTTY: true,
			want: ViewOptions{
				SelectorArg: "123",
				BrowserMode: true,
			},
		},
		{
			name:    "no argument with --repo override",
			args:    "-R owner/repo",
			isTTY:   true,
			wantErr: "argument required when using the --repo flag",
		},
		{
			name:  "comments",
			args:  "123 -c",
			isTTY: true,
			want: ViewOptions{
				SelectorArg: "123",
				Comments:    true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)

			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			var opts *ViewOptions
			cmd := NewCmdView(f, func(o *ViewOptions) error {
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
		})
	}
}

func runCommand(rt http.RoundTripper, branch string, isTTY bool, cli string) (*test.CmdOut, error) {
	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(isTTY)
	ios.SetStdinTTY(isTTY)
	ios.SetStderrTTY(isTTY)

	browser := &browser.Stub{}
	factory := &cmdutil.Factory{
		IOStreams: ios,
		Browser:   browser,
	}

	cmd := NewCmdView(factory, nil)

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
		OutBuf:     stdout,
		ErrBuf:     stderr,
		BrowsedURL: browser.BrowsedURL(),
	}, err
}

// hack for compatibility with old JSON fixture files
func prFromFixtures(fixtures map[string]string) (*api.PullRequest, error) {
	var response struct {
		Data struct {
			Repository struct {
				PullRequest *api.PullRequest
			}
		}
	}

	ff, err := os.Open(fixtures["PullRequestByNumber"])
	if err != nil {
		return nil, err
	}
	defer ff.Close()

	dec := json.NewDecoder(ff)
	err = dec.Decode(&response)
	if err != nil {
		return nil, err
	}

	for name := range fixtures {
		switch name {
		case "PullRequestByNumber":
		case "ReviewsForPullRequest", "CommentsForPullRequest":
			ff, err := os.Open(fixtures[name])
			if err != nil {
				return nil, err
			}
			defer ff.Close()
			dec := json.NewDecoder(ff)
			err = dec.Decode(&response)
			if err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("unrecognized fixture type: %q", name)
		}
	}

	return response.Data.Repository.PullRequest, nil
}

func TestPRView_Preview_nontty(t *testing.T) {
	tests := map[string]struct {
		branch          string
		args            string
		fixtures        map[string]string
		expectedOutputs []string
	}{
		"Open PR without metadata": {
			branch: "master",
			args:   "12",
			fixtures: map[string]string{
				"PullRequestByNumber": "./fixtures/prViewPreview.json",
			},
			expectedOutputs: []string{
				`title:\tBlueberries are from a fork\n`,
				`state:\tOPEN\n`,
				`author:\tnobody\n`,
				`labels:\t\n`,
				`assignees:\t\n`,
				`reviewers:\t\n`,
				`projects:\t\n`,
				`milestone:\t\n`,
				`url:\thttps://github.com/OWNER/REPO/pull/12\n`,
				`additions:\t100\n`,
				`deletions:\t10\n`,
				`number:\t12\n`,
				`blueberries taste good`,
			},
		},
		"Open PR with metadata by number": {
			branch: "master",
			args:   "12",
			fixtures: map[string]string{
				"PullRequestByNumber": "./fixtures/prViewPreviewWithMetadataByNumber.json",
			},
			expectedOutputs: []string{
				`title:\tBlueberries are from a fork\n`,
				`reviewers:\t1 \(Requested\)\n`,
				`assignees:\tmarseilles, monaco\n`,
				`labels:\tone, two, three, four, five\n`,
				`projects:\tProject 1 \(column A\), Project 2 \(column B\), Project 3 \(column C\), Project 4 \(Awaiting triage\)\n`,
				`milestone:\tuluru\n`,
				`\*\*blueberries taste good\*\*`,
			},
		},
		"Open PR with reviewers by number": {
			branch: "master",
			args:   "12",
			fixtures: map[string]string{
				"PullRequestByNumber":   "./fixtures/prViewPreviewWithReviewersByNumber.json",
				"ReviewsForPullRequest": "./fixtures/prViewPreviewManyReviews.json",
			},
			expectedOutputs: []string{
				`title:\tBlueberries are from a fork\n`,
				`state:\tOPEN\n`,
				`author:\tnobody\n`,
				`labels:\t\n`,
				`assignees:\t\n`,
				`projects:\t\n`,
				`milestone:\t\n`,
				`additions:\t100\n`,
				`deletions:\t10\n`,
				`reviewers:\tDEF \(Commented\), def \(Changes requested\), ghost \(Approved\), hubot \(Commented\), xyz \(Approved\), 123 \(Requested\), abc \(Requested\), my-org\/team-1 \(Requested\)\n`,
				`\*\*blueberries taste good\*\*`,
			},
		},
		"Closed PR": {
			branch: "master",
			args:   "12",
			fixtures: map[string]string{
				"PullRequestByNumber": "./fixtures/prViewPreviewClosedState.json",
			},
			expectedOutputs: []string{
				`state:\tCLOSED\n`,
				`author:\tnobody\n`,
				`labels:\t\n`,
				`assignees:\t\n`,
				`reviewers:\t\n`,
				`projects:\t\n`,
				`milestone:\t\n`,
				`additions:\t100\n`,
				`deletions:\t10\n`,
				`\*\*blueberries taste good\*\*`,
			},
		},
		"Merged PR": {
			branch: "master",
			args:   "12",
			fixtures: map[string]string{
				"PullRequestByNumber": "./fixtures/prViewPreviewMergedState.json",
			},
			expectedOutputs: []string{
				`state:\tMERGED\n`,
				`author:\tnobody\n`,
				`labels:\t\n`,
				`assignees:\t\n`,
				`reviewers:\t\n`,
				`projects:\t\n`,
				`milestone:\t\n`,
				`additions:\t100\n`,
				`deletions:\t10\n`,
				`\*\*blueberries taste good\*\*`,
			},
		},
		"Draft PR": {
			branch: "master",
			args:   "12",
			fixtures: map[string]string{
				"PullRequestByNumber": "./fixtures/prViewPreviewDraftState.json",
			},
			expectedOutputs: []string{
				`title:\tBlueberries are from a fork\n`,
				`state:\tDRAFT\n`,
				`author:\tnobody\n`,
				`labels:`,
				`assignees:`,
				`reviewers:`,
				`projects:`,
				`milestone:`,
				`additions:\t100\n`,
				`deletions:\t10\n`,
				`\*\*blueberries taste good\*\*`,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			http := &httpmock.Registry{}
			defer http.Verify(t)

			pr, err := prFromFixtures(tc.fixtures)
			require.NoError(t, err)
			shared.RunCommandFinder("12", pr, ghrepo.New("OWNER", "REPO"))

			output, err := runCommand(http, tc.branch, false, tc.args)
			if err != nil {
				t.Errorf("error running command `%v`: %v", tc.args, err)
			}

			assert.Equal(t, "", output.Stderr())

			//nolint:staticcheck // prefer exact matchers over ExpectLines
			test.ExpectLines(t, output.String(), tc.expectedOutputs...)
		})
	}
}

func TestPRView_Preview(t *testing.T) {
	tests := map[string]struct {
		branch          string
		args            string
		fixtures        map[string]string
		expectedOutputs []string
	}{
		"Open PR without metadata": {
			branch: "master",
			args:   "12",
			fixtures: map[string]string{
				"PullRequestByNumber": "./fixtures/prViewPreview.json",
			},
			expectedOutputs: []string{
				`Blueberries are from a fork #12`,
				`Open.*nobody wants to merge 12 commits into master from blueberries . about X years ago`,
				`.+100.-10`,
				`blueberries taste good`,
				`View this pull request on GitHub: https://github.com/OWNER/REPO/pull/12`,
			},
		},
		"Open PR with metadata by number": {
			branch: "master",
			args:   "12",
			fixtures: map[string]string{
				"PullRequestByNumber": "./fixtures/prViewPreviewWithMetadataByNumber.json",
			},
			expectedOutputs: []string{
				`Blueberries are from a fork #12`,
				`Open.*nobody wants to merge 12 commits into master from blueberries . about X years ago`,
				`.+100.-10`,
				`Reviewers:.*1 \(.*Requested.*\)\n`,
				`Assignees:.*marseilles, monaco\n`,
				`Labels:.*one, two, three, four, five\n`,
				`Projects:.*Project 1 \(column A\), Project 2 \(column B\), Project 3 \(column C\), Project 4 \(Awaiting triage\)\n`,
				`Milestone:.*uluru\n`,
				`blueberries taste good`,
				`View this pull request on GitHub: https://github.com/OWNER/REPO/pull/12`,
			},
		},
		"Open PR with reviewers by number": {
			branch: "master",
			args:   "12",
			fixtures: map[string]string{
				"PullRequestByNumber":   "./fixtures/prViewPreviewWithReviewersByNumber.json",
				"ReviewsForPullRequest": "./fixtures/prViewPreviewManyReviews.json",
			},
			expectedOutputs: []string{
				`Blueberries are from a fork #12`,
				`Reviewers: DEF \(Commented\), def \(Changes requested\), ghost \(Approved\), hubot \(Commented\), xyz \(Approved\), 123 \(Requested\), abc \(Requested\), my-org\/team-1 \(Requested\)`,
				`blueberries taste good`,
				`View this pull request on GitHub: https://github.com/OWNER/REPO/pull/12`,
			},
		},
		"Closed PR": {
			branch: "master",
			args:   "12",
			fixtures: map[string]string{
				"PullRequestByNumber": "./fixtures/prViewPreviewClosedState.json",
			},
			expectedOutputs: []string{
				`Blueberries are from a fork #12`,
				`Closed.*nobody wants to merge 12 commits into master from blueberries . about X years ago`,
				`.+100.-10`,
				`blueberries taste good`,
				`View this pull request on GitHub: https://github.com/OWNER/REPO/pull/12`,
			},
		},
		"Merged PR": {
			branch: "master",
			args:   "12",
			fixtures: map[string]string{
				"PullRequestByNumber": "./fixtures/prViewPreviewMergedState.json",
			},
			expectedOutputs: []string{
				`Blueberries are from a fork #12`,
				`Merged.*nobody wants to merge 12 commits into master from blueberries . about X years ago`,
				`.+100.-10`,
				`blueberries taste good`,
				`View this pull request on GitHub: https://github.com/OWNER/REPO/pull/12`,
			},
		},
		"Draft PR": {
			branch: "master",
			args:   "12",
			fixtures: map[string]string{
				"PullRequestByNumber": "./fixtures/prViewPreviewDraftState.json",
			},
			expectedOutputs: []string{
				`Blueberries are from a fork #12`,
				`Draft.*nobody wants to merge 12 commits into master from blueberries . about X years ago`,
				`.+100.-10`,
				`blueberries taste good`,
				`View this pull request on GitHub: https://github.com/OWNER/REPO/pull/12`,
			},
		},
		"Open PR with all checks passing": {
			branch: "master",
			args:   "12",
			fixtures: map[string]string{
				"PullRequestByNumber": "./fixtures/prViewPreviewWithAllChecksPassing.json",
			},
			expectedOutputs: []string{
				`Blueberries are from a fork #12`,
				`Open.*nobody wants to merge 12 commits into master from blueberries . about X years ago`,
				`.+100.-10 • ✓ Checks passing`,
				`blueberries taste good`,
				`View this pull request on GitHub: https://github.com/OWNER/REPO/pull/12`,
			},
		},
		"Open PR with all checks failing": {
			branch: "master",
			args:   "12",
			fixtures: map[string]string{
				"PullRequestByNumber": "./fixtures/prViewPreviewWithAllChecksFailing.json",
			},
			expectedOutputs: []string{
				`Blueberries are from a fork #12`,
				`Open.*nobody wants to merge 12 commits into master from blueberries . about X years ago`,
				`.+100.-10 • × All checks failing`,
				`blueberries taste good`,
				`View this pull request on GitHub: https://github.com/OWNER/REPO/pull/12`,
			},
		},
		"Open PR with some checks failing": {
			branch: "master",
			args:   "12",
			fixtures: map[string]string{
				"PullRequestByNumber": "./fixtures/prViewPreviewWithSomeChecksFailing.json",
			},
			expectedOutputs: []string{
				`Blueberries are from a fork #12`,
				`Open.*nobody wants to merge 12 commits into master from blueberries . about X years ago`,
				`.+100.-10 • × 1/2 checks failing`,
				`blueberries taste good`,
				`View this pull request on GitHub: https://github.com/OWNER/REPO/pull/12`,
			},
		},
		"Open PR with some checks pending": {
			branch: "master",
			args:   "12",
			fixtures: map[string]string{
				"PullRequestByNumber": "./fixtures/prViewPreviewWithSomeChecksPending.json",
			},
			expectedOutputs: []string{
				`Blueberries are from a fork #12`,
				`Open.*nobody wants to merge 12 commits into master from blueberries . about X years ago`,
				`.+100.-10 • - Checks pending`,
				`blueberries taste good`,
				`View this pull request on GitHub: https://github.com/OWNER/REPO/pull/12`,
			},
		},
		"Open PR with no checks": {
			branch: "master",
			args:   "12",
			fixtures: map[string]string{
				"PullRequestByNumber": "./fixtures/prViewPreviewWithNoChecks.json",
			},
			expectedOutputs: []string{
				`Blueberries are from a fork #12`,
				`Open.*nobody wants to merge 12 commits into master from blueberries . about X years ago`,
				`.+100.-10 • No checks`,
				`blueberries taste good`,
				`View this pull request on GitHub: https://github.com/OWNER/REPO/pull/12`,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			http := &httpmock.Registry{}
			defer http.Verify(t)

			pr, err := prFromFixtures(tc.fixtures)
			require.NoError(t, err)
			shared.RunCommandFinder("12", pr, ghrepo.New("OWNER", "REPO"))

			output, err := runCommand(http, tc.branch, true, tc.args)
			if err != nil {
				t.Errorf("error running command `%v`: %v", tc.args, err)
			}

			assert.Equal(t, "", output.Stderr())

			out := output.String()
			timeRE := regexp.MustCompile(`\d+ years`)
			out = timeRE.ReplaceAllString(out, "X years")

			//nolint:staticcheck // prefer exact matchers over ExpectLines
			test.ExpectLines(t, out, tc.expectedOutputs...)
		})
	}
}

func TestPRView_web_currentBranch(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.RunCommandFinder("", &api.PullRequest{URL: "https://github.com/OWNER/REPO/pull/10"}, ghrepo.New("OWNER", "REPO"))

	_, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	output, err := runCommand(http, "blueberries", true, "-w")
	if err != nil {
		t.Errorf("error running command `pr view`: %v", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, "Opening github.com/OWNER/REPO/pull/10 in your browser.\n", output.Stderr())
	assert.Equal(t, "https://github.com/OWNER/REPO/pull/10", output.BrowsedURL)
}

func TestPRView_web_noResultsForBranch(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.RunCommandFinder("", nil, nil)

	_, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	_, err := runCommand(http, "blueberries", true, "-w")
	if err == nil || err.Error() != `no pull requests found` {
		t.Errorf("error running command `pr view`: %v", err)
	}
}

func TestPRView_tty_Comments(t *testing.T) {
	tests := map[string]struct {
		branch          string
		cli             string
		fixtures        map[string]string
		expectedOutputs []string
		wantsErr        bool
	}{
		"without comments flag": {
			branch: "master",
			cli:    "123",
			fixtures: map[string]string{
				"PullRequestByNumber":   "./fixtures/prViewPreviewSingleComment.json",
				"ReviewsForPullRequest": "./fixtures/prViewPreviewReviews.json",
			},
			expectedOutputs: []string{
				`some title #12`,
				`1 \x{1f615} • 2 \x{1f440} • 3 \x{2764}\x{fe0f}`,
				`some body`,
				`———————— Not showing 9 comments ————————`,
				`marseilles \(Collaborator\) • Jan  9, 2020 • Newest comment`,
				`4 \x{1f389} • 5 \x{1f604} • 6 \x{1f680}`,
				`Comment 5`,
				`Use --comments to view the full conversation`,
				`View this pull request on GitHub: https://github.com/OWNER/REPO/pull/12`,
			},
		},
		"with comments flag": {
			branch: "master",
			cli:    "123 --comments",
			fixtures: map[string]string{
				"PullRequestByNumber":    "./fixtures/prViewPreviewSingleComment.json",
				"ReviewsForPullRequest":  "./fixtures/prViewPreviewReviews.json",
				"CommentsForPullRequest": "./fixtures/prViewPreviewFullComments.json",
			},
			expectedOutputs: []string{
				`some title #12`,
				`some body`,
				`monalisa • Jan  1, 2020 • Edited`,
				`1 \x{1f615} • 2 \x{1f440} • 3 \x{2764}\x{fe0f} • 4 \x{1f389} • 5 \x{1f604} • 6 \x{1f680} • 7 \x{1f44e} • 8 \x{1f44d}`,
				`Comment 1`,
				`sam commented • Jan  2, 2020`,
				`1 \x{1f44e} • 1 \x{1f44d}`,
				`Review 1`,
				`View the full review: https://github.com/OWNER/REPO/pull/12#pullrequestreview-1`,
				`johnnytest \(Contributor\) • Jan  3, 2020`,
				`Comment 2`,
				`matt requested changes \(Owner\) • Jan  4, 2020`,
				`1 \x{1f615} • 1 \x{1f440}`,
				`Review 2`,
				`View the full review: https://github.com/OWNER/REPO/pull/12#pullrequestreview-2`,
				`elvisp \(Member\) • Jan  5, 2020`,
				`Comment 3`,
				`leah approved \(Member\) • Jan  6, 2020 • Edited`,
				`Review 3`,
				`View the full review: https://github.com/OWNER/REPO/pull/12#pullrequestreview-3`,
				`loislane \(Owner\) • Jan  7, 2020`,
				`Comment 4`,
				`louise dismissed • Jan  8, 2020`,
				`Review 4`,
				`View the full review: https://github.com/OWNER/REPO/pull/12#pullrequestreview-4`,
				`sam-spam • This comment has been marked as spam`,
				`marseilles \(Collaborator\) • Jan  9, 2020 • Newest comment`,
				`Comment 5`,
				`View this pull request on GitHub: https://github.com/OWNER/REPO/pull/12`,
			},
		},
		"with invalid comments flag": {
			branch:   "master",
			cli:      "123 --comments 3",
			wantsErr: true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			http := &httpmock.Registry{}
			defer http.Verify(t)

			if len(tt.fixtures) > 0 {
				pr, err := prFromFixtures(tt.fixtures)
				require.NoError(t, err)
				shared.RunCommandFinder("123", pr, ghrepo.New("OWNER", "REPO"))
			} else {
				shared.RunCommandFinder("123", nil, nil)
			}

			output, err := runCommand(http, tt.branch, true, tt.cli)
			if tt.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, "", output.Stderr())
			//nolint:staticcheck // prefer exact matchers over ExpectLines
			test.ExpectLines(t, output.String(), tt.expectedOutputs...)
		})
	}
}

func TestPRView_nontty_Comments(t *testing.T) {
	tests := map[string]struct {
		branch          string
		cli             string
		fixtures        map[string]string
		expectedOutputs []string
		wantsErr        bool
	}{
		"without comments flag": {
			branch: "master",
			cli:    "123",
			fixtures: map[string]string{
				"PullRequestByNumber":   "./fixtures/prViewPreviewSingleComment.json",
				"ReviewsForPullRequest": "./fixtures/prViewPreviewReviews.json",
			},
			expectedOutputs: []string{
				`title:\tsome title`,
				`state:\tOPEN`,
				`author:\tnobody`,
				`url:\thttps://github.com/OWNER/REPO/pull/12`,
				`some body`,
			},
		},
		"with comments flag": {
			branch: "master",
			cli:    "123 --comments",
			fixtures: map[string]string{
				"PullRequestByNumber":    "./fixtures/prViewPreviewSingleComment.json",
				"ReviewsForPullRequest":  "./fixtures/prViewPreviewReviews.json",
				"CommentsForPullRequest": "./fixtures/prViewPreviewFullComments.json",
			},
			expectedOutputs: []string{
				`author:\tmonalisa`,
				`association:\tnone`,
				`edited:\ttrue`,
				`status:\tnone`,
				`Comment 1`,
				`author:\tsam`,
				`association:\tnone`,
				`edited:\tfalse`,
				`status:\tcommented`,
				`Review 1`,
				`author:\tjohnnytest`,
				`association:\tcontributor`,
				`edited:\tfalse`,
				`status:\tnone`,
				`Comment 2`,
				`author:\tmatt`,
				`association:\towner`,
				`edited:\tfalse`,
				`status:\tchanges requested`,
				`Review 2`,
				`author:\telvisp`,
				`association:\tmember`,
				`edited:\tfalse`,
				`status:\tnone`,
				`Comment 3`,
				`author:\tleah`,
				`association:\tmember`,
				`edited:\ttrue`,
				`status:\tapproved`,
				`Review 3`,
				`author:\tloislane`,
				`association:\towner`,
				`edited:\tfalse`,
				`status:\tnone`,
				`Comment 4`,
				`author:\tlouise`,
				`association:\tnone`,
				`edited:\tfalse`,
				`status:\tdismissed`,
				`Review 4`,
				`author:\tmarseilles`,
				`association:\tcollaborator`,
				`edited:\tfalse`,
				`status:\tnone`,
				`Comment 5`,
			},
		},
		"with invalid comments flag": {
			branch:   "master",
			cli:      "123 --comments 3",
			wantsErr: true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			http := &httpmock.Registry{}
			defer http.Verify(t)

			if len(tt.fixtures) > 0 {
				pr, err := prFromFixtures(tt.fixtures)
				require.NoError(t, err)
				shared.RunCommandFinder("123", pr, ghrepo.New("OWNER", "REPO"))
			} else {
				shared.RunCommandFinder("123", nil, nil)
			}

			output, err := runCommand(http, tt.branch, false, tt.cli)
			if tt.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, "", output.Stderr())
			//nolint:staticcheck // prefer exact matchers over ExpectLines
			test.ExpectLines(t, output.String(), tt.expectedOutputs...)
		})
	}
}
