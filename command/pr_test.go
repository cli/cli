package command

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/run"
	"github.com/cli/cli/test"
	"github.com/google/go-cmp/cmp"
	"github.com/google/shlex"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func eq(t *testing.T, got interface{}, expected interface{}) {
	t.Helper()
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("expected: %v, got: %v", expected, got)
	}
}

type cmdOut struct {
	outBuf, errBuf *bytes.Buffer
}

func (c cmdOut) String() string {
	return c.outBuf.String()
}

func (c cmdOut) Stderr() string {
	return c.errBuf.String()
}

func RunCommand(cmd *cobra.Command, args string) (*cmdOut, error) {
	rootCmd := cmd.Root()
	argv, err := shlex.Split(args)
	if err != nil {
		return nil, err
	}
	rootCmd.SetArgs(argv)

	outBuf := bytes.Buffer{}
	cmd.SetOut(&outBuf)
	errBuf := bytes.Buffer{}
	cmd.SetErr(&errBuf)

	// Reset flag values so they don't leak between tests
	// FIXME: change how we initialize Cobra commands to render this hack unnecessary
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		switch v := f.Value.(type) {
		case pflag.SliceValue:
			_ = v.Replace([]string{})
		default:
			switch v.Type() {
			case "bool", "string", "int":
				_ = v.Set(f.DefValue)
			}
		}
	})

	_, err = rootCmd.ExecuteC()
	cmd.SetOut(nil)
	cmd.SetErr(nil)

	return &cmdOut{&outBuf, &errBuf}, err
}

func TestPRStatus(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "blueberries")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	jsonFile, _ := os.Open("../test/fixtures/prStatus.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	output, err := RunCommand(prStatusCmd, "pr status")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expectedPrs := []*regexp.Regexp{
		regexp.MustCompile(`#8.*\[strawberries\]`),
		regexp.MustCompile(`#9.*\[apples\]`),
		regexp.MustCompile(`#10.*\[blueberries\]`),
		regexp.MustCompile(`#11.*\[figs\]`),
	}

	for _, r := range expectedPrs {
		if !r.MatchString(output.String()) {
			t.Errorf("output did not match regexp /%s/", r)
		}
	}
}

func TestPRStatus_fork(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "blueberries")
	http := initFakeHTTP()
	http.StubForkedRepoResponse("OWNER/REPO", "PARENT/REPO")

	jsonFile, _ := os.Open("../test/fixtures/prStatusFork.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	defer run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		switch strings.Join(cmd.Args, " ") {
		case `git config --get-regexp ^branch\.blueberries\.(remote|merge)$`:
			return &test.OutputStub{Out: []byte(`branch.blueberries.remote origin
branch.blueberries.merge refs/heads/blueberries`)}
		default:
			panic("not implemented")
		}
	})()

	output, err := RunCommand(prStatusCmd, "pr status")
	if err != nil {
		t.Fatalf("error running command `pr status`: %v", err)
	}

	branchRE := regexp.MustCompile(`#10.*\[OWNER:blueberries\]`)
	if !branchRE.MatchString(output.String()) {
		t.Errorf("did not match current branch:\n%v", output.String())
	}
}

func TestPRStatus_reviewsAndChecks(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "blueberries")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	jsonFile, _ := os.Open("../test/fixtures/prStatusChecks.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	output, err := RunCommand(prStatusCmd, "pr status")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expected := []string{
		"✓ Checks passing + Changes requested",
		"- Checks pending ✓ Approved",
		"× 1/3 checks failing - Review required",
	}

	for _, line := range expected {
		if !strings.Contains(output.String(), line) {
			t.Errorf("output did not contain %q: %q", line, output.String())
		}
	}
}

func TestPRStatus_currentBranch_showTheMostRecentPR(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "blueberries")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	jsonFile, _ := os.Open("../test/fixtures/prStatusCurrentBranch.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	output, err := RunCommand(prStatusCmd, "pr status")
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

func TestPRStatus_currentBranch_Closed(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "blueberries")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	jsonFile, _ := os.Open("../test/fixtures/prStatusCurrentBranchClosed.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	output, err := RunCommand(prStatusCmd, "pr status")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expectedLine := regexp.MustCompile(`#8  Blueberries are a good fruit \[blueberries\] - Closed`)
	if !expectedLine.MatchString(output.String()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", expectedLine, output)
		return
	}
}

func TestPRStatus_currentBranch_Merged(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "blueberries")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	jsonFile, _ := os.Open("../test/fixtures/prStatusCurrentBranchMerged.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	output, err := RunCommand(prStatusCmd, "pr status")
	if err != nil {
		t.Errorf("error running command `pr status`: %v", err)
	}

	expectedLine := regexp.MustCompile(`#8  Blueberries are a good fruit \[blueberries\] - Merged`)
	if !expectedLine.MatchString(output.String()) {
		t.Errorf("output did not match regexp /%s/\n> output\n%s\n", expectedLine, output)
		return
	}
}

func TestPRStatus_blankSlate(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "blueberries")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": {} }
	`))

	output, err := RunCommand(prStatusCmd, "pr status")
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

func TestPRList(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	jsonFile, _ := os.Open("../test/fixtures/prList.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	output, err := RunCommand(prListCmd, "pr list")
	if err != nil {
		t.Fatal(err)
	}

	eq(t, output.Stderr(), `
Showing 3 of 3 pull requests in OWNER/REPO

`)
	eq(t, output.String(), `32	New feature	feature
29	Fixed bad bug	hubot:bug-fix
28	Improve documentation	docs
`)
}

func TestPRList_filtering(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	respBody := bytes.NewBufferString(`{ "data": {} }`)
	http.StubResponse(200, respBody)

	output, err := RunCommand(prListCmd, `pr list -s all -l one,two -l three`)
	if err != nil {
		t.Fatal(err)
	}

	eq(t, output.String(), "")
	eq(t, output.Stderr(), `
No pull requests match your search in OWNER/REPO

`)

	bodyBytes, _ := ioutil.ReadAll(http.Requests[1].Body)
	reqBody := struct {
		Variables struct {
			State  []string
			Labels []string
		}
	}{}
	_ = json.Unmarshal(bodyBytes, &reqBody)

	eq(t, reqBody.Variables.State, []string{"OPEN", "CLOSED", "MERGED"})
	eq(t, reqBody.Variables.Labels, []string{"one", "two", "three"})
}

func TestPRList_filteringRemoveDuplicate(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	jsonFile, _ := os.Open("../test/fixtures/prListWithDuplicates.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	output, err := RunCommand(prListCmd, "pr list -l one,two")
	if err != nil {
		t.Fatal(err)
	}

	eq(t, output.String(), `32	New feature	feature
29	Fixed bad bug	hubot:bug-fix
28	Improve documentation	docs
`)
}

func TestPRList_filteringClosed(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	respBody := bytes.NewBufferString(`{ "data": {} }`)
	http.StubResponse(200, respBody)

	_, err := RunCommand(prListCmd, `pr list -s closed`)
	if err != nil {
		t.Fatal(err)
	}

	bodyBytes, _ := ioutil.ReadAll(http.Requests[1].Body)
	reqBody := struct {
		Variables struct {
			State []string
		}
	}{}
	_ = json.Unmarshal(bodyBytes, &reqBody)

	eq(t, reqBody.Variables.State, []string{"CLOSED", "MERGED"})
}

func TestPRList_filteringAssignee(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	respBody := bytes.NewBufferString(`{ "data": {} }`)
	http.StubResponse(200, respBody)

	_, err := RunCommand(prListCmd, `pr list -s merged -l "needs tests" -a hubot -B develop`)
	if err != nil {
		t.Fatal(err)
	}

	bodyBytes, _ := ioutil.ReadAll(http.Requests[1].Body)
	reqBody := struct {
		Variables struct {
			Q string
		}
	}{}
	_ = json.Unmarshal(bodyBytes, &reqBody)

	eq(t, reqBody.Variables.Q, `repo:OWNER/REPO assignee:hubot is:pr sort:created-desc is:merged label:"needs tests" base:"develop"`)
}

func TestPRList_filteringAssigneeLabels(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	respBody := bytes.NewBufferString(`{ "data": {} }`)
	http.StubResponse(200, respBody)

	_, err := RunCommand(prListCmd, `pr list -l one,two -a hubot`)
	if err == nil && err.Error() != "multiple labels with --assignee are not supported" {
		t.Fatal(err)
	}
}

func TestPRView_Preview(t *testing.T) {
	tests := map[string]struct {
		ownerRepo       string
		args            string
		fixture         string
		expectedOutputs []string
	}{
		"Open PR without metadata": {
			ownerRepo: "master",
			args:      "pr view 12",
			fixture:   "../test/fixtures/prViewPreview.json",
			expectedOutputs: []string{
				`Blueberries are from a fork`,
				`Open • nobody wants to merge 12 commits into master from blueberries`,
				`blueberries taste good`,
				`View this pull request on GitHub: https://github.com/OWNER/REPO/pull/12`,
			},
		},
		"Open PR with metadata by number": {
			ownerRepo: "master",
			args:      "pr view 12",
			fixture:   "../test/fixtures/prViewPreviewWithMetadataByNumber.json",
			expectedOutputs: []string{
				`Blueberries are from a fork`,
				`Open • nobody wants to merge 12 commits into master from blueberries`,
				`Assignees: marseilles, monaco\n`,
				`Labels: one, two, three, four, five\n`,
				`Projects: Project 1 \(column A\), Project 2 \(column B\), Project 3 \(column C\)\n`,
				`Milestone: uluru\n`,
				`blueberries taste good`,
				`View this pull request on GitHub: https://github.com/OWNER/REPO/pull/12\n`,
			},
		},
		"Open PR with metadata by branch": {
			ownerRepo: "master",
			args:      "pr view blueberries",
			fixture:   "../test/fixtures/prViewPreviewWithMetadataByBranch.json",
			expectedOutputs: []string{
				`Blueberries are a good fruit`,
				`Open • nobody wants to merge 8 commits into master from blueberries`,
				`Assignees: marseilles, monaco\n`,
				`Labels: one, two, three, four, five\n`,
				`Projects: Project 1 \(column A\), Project 2 \(column B\), Project 3 \(column C\)\n`,
				`Milestone: uluru\n`,
				`blueberries taste good`,
				`View this pull request on GitHub: https://github.com/OWNER/REPO/pull/10\n`,
			},
		},
		"Open PR for the current branch": {
			ownerRepo: "blueberries",
			args:      "pr view",
			fixture:   "../test/fixtures/prView.json",
			expectedOutputs: []string{
				`Blueberries are a good fruit`,
				`Open • nobody wants to merge 8 commits into master from blueberries`,
				`blueberries taste good`,
				`View this pull request on GitHub: https://github.com/OWNER/REPO/pull/10`,
			},
		},
		"Open PR wth empty body for the current branch": {
			ownerRepo: "blueberries",
			args:      "pr view",
			fixture:   "../test/fixtures/prView_EmptyBody.json",
			expectedOutputs: []string{
				`Blueberries are a good fruit`,
				`Open • nobody wants to merge 8 commits into master from blueberries`,
				`View this pull request on GitHub: https://github.com/OWNER/REPO/pull/10`,
			},
		},
		"Closed PR": {
			ownerRepo: "master",
			args:      "pr view 12",
			fixture:   "../test/fixtures/prViewPreviewClosedState.json",
			expectedOutputs: []string{
				`Blueberries are from a fork`,
				`Closed • nobody wants to merge 12 commits into master from blueberries`,
				`blueberries taste good`,
				`View this pull request on GitHub: https://github.com/OWNER/REPO/pull/12`,
			},
		},
		"Merged PR": {
			ownerRepo: "master",
			args:      "pr view 12",
			fixture:   "../test/fixtures/prViewPreviewMergedState.json",
			expectedOutputs: []string{
				`Blueberries are from a fork`,
				`Merged • nobody wants to merge 12 commits into master from blueberries`,
				`blueberries taste good`,
				`View this pull request on GitHub: https://github.com/OWNER/REPO/pull/12`,
			},
		},
		"Draft PR": {
			ownerRepo: "master",
			args:      "pr view 12",
			fixture:   "../test/fixtures/prViewPreviewDraftState.json",
			expectedOutputs: []string{
				`Blueberries are from a fork`,
				`Draft • nobody wants to merge 12 commits into master from blueberries`,
				`blueberries taste good`,
				`View this pull request on GitHub: https://github.com/OWNER/REPO/pull/12`,
			},
		},
		"Draft PR by branch": {
			ownerRepo: "master",
			args:      "pr view blueberries",
			fixture:   "../test/fixtures/prViewPreviewDraftStatebyBranch.json",
			expectedOutputs: []string{
				`Blueberries are a good fruit`,
				`Draft • nobody wants to merge 8 commits into master from blueberries`,
				`blueberries taste good`,
				`View this pull request on GitHub: https://github.com/OWNER/REPO/pull/10`,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			initBlankContext("", "OWNER/REPO", tc.ownerRepo)
			http := initFakeHTTP()
			http.StubRepoResponse("OWNER", "REPO")

			jsonFile, _ := os.Open(tc.fixture)
			defer jsonFile.Close()
			http.StubResponse(200, jsonFile)

			output, err := RunCommand(prViewCmd, tc.args)
			if err != nil {
				t.Errorf("error running command `%v`: %v", tc.args, err)
			}

			eq(t, output.Stderr(), "")

			test.ExpectLines(t, output.String(), tc.expectedOutputs...)
		})
	}
}

func TestPRView_web_currentBranch(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "blueberries")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	jsonFile, _ := os.Open("../test/fixtures/prView.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	var seenCmd *exec.Cmd
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		switch strings.Join(cmd.Args, " ") {
		case `git config --get-regexp ^branch\.blueberries\.(remote|merge)$`:
			return &test.OutputStub{}
		default:
			seenCmd = cmd
			return &test.OutputStub{}
		}
	})
	defer restoreCmd()

	output, err := RunCommand(prViewCmd, "pr view -w")
	if err != nil {
		t.Errorf("error running command `pr view`: %v", err)
	}

	eq(t, output.String(), "")
	eq(t, output.Stderr(), "Opening https://github.com/OWNER/REPO/pull/10 in your browser.\n")

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	if url != "https://github.com/OWNER/REPO/pull/10" {
		t.Errorf("got: %q", url)
	}
}

func TestPRView_web_noResultsForBranch(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "blueberries")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	jsonFile, _ := os.Open("../test/fixtures/prView_NoActiveBranch.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	var seenCmd *exec.Cmd
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		switch strings.Join(cmd.Args, " ") {
		case `git config --get-regexp ^branch\.blueberries\.(remote|merge)$`:
			return &test.OutputStub{}
		default:
			seenCmd = cmd
			return &test.OutputStub{}
		}
	})
	defer restoreCmd()

	_, err := RunCommand(prViewCmd, "pr view -w")
	if err == nil || err.Error() != `no open pull requests found for branch "blueberries"` {
		t.Errorf("error running command `pr view`: %v", err)
	}

	if seenCmd != nil {
		t.Fatalf("unexpected command: %v", seenCmd.Args)
	}
}

func TestPRView_web_numberArg(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "pullRequest": {
		"url": "https://github.com/OWNER/REPO/pull/23"
	} } } }
	`))

	var seenCmd *exec.Cmd
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		seenCmd = cmd
		return &test.OutputStub{}
	})
	defer restoreCmd()

	output, err := RunCommand(prViewCmd, "pr view -w 23")
	if err != nil {
		t.Errorf("error running command `pr view`: %v", err)
	}

	eq(t, output.String(), "")

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	eq(t, url, "https://github.com/OWNER/REPO/pull/23")
}

func TestPRView_web_numberArgWithHash(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "pullRequest": {
		"url": "https://github.com/OWNER/REPO/pull/23"
	} } } }
	`))

	var seenCmd *exec.Cmd
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		seenCmd = cmd
		return &test.OutputStub{}
	})
	defer restoreCmd()

	output, err := RunCommand(prViewCmd, "pr view -w \"#23\"")
	if err != nil {
		t.Errorf("error running command `pr view`: %v", err)
	}

	eq(t, output.String(), "")

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	eq(t, url, "https://github.com/OWNER/REPO/pull/23")
}

func TestPRView_web_urlArg(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "pullRequest": {
		"url": "https://github.com/OWNER/REPO/pull/23"
	} } } }
	`))

	var seenCmd *exec.Cmd
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		seenCmd = cmd
		return &test.OutputStub{}
	})
	defer restoreCmd()

	output, err := RunCommand(prViewCmd, "pr view -w https://github.com/OWNER/REPO/pull/23/files")
	if err != nil {
		t.Errorf("error running command `pr view`: %v", err)
	}

	eq(t, output.String(), "")

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	eq(t, url, "https://github.com/OWNER/REPO/pull/23")
}

func TestPRView_web_branchArg(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "pullRequests": { "nodes": [
		{ "headRefName": "blueberries",
		  "isCrossRepository": false,
		  "url": "https://github.com/OWNER/REPO/pull/23" }
	] } } } }
	`))

	var seenCmd *exec.Cmd
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		seenCmd = cmd
		return &test.OutputStub{}
	})
	defer restoreCmd()

	output, err := RunCommand(prViewCmd, "pr view -w blueberries")
	if err != nil {
		t.Errorf("error running command `pr view`: %v", err)
	}

	eq(t, output.String(), "")

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	eq(t, url, "https://github.com/OWNER/REPO/pull/23")
}

func TestPRView_web_branchWithOwnerArg(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "pullRequests": { "nodes": [
		{ "headRefName": "blueberries",
		  "isCrossRepository": true,
		  "headRepositoryOwner": { "login": "hubot" },
		  "url": "https://github.com/hubot/REPO/pull/23" }
	] } } } }
	`))

	var seenCmd *exec.Cmd
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		seenCmd = cmd
		return &test.OutputStub{}
	})
	defer restoreCmd()

	output, err := RunCommand(prViewCmd, "pr view -w hubot:blueberries")
	if err != nil {
		t.Errorf("error running command `pr view`: %v", err)
	}

	eq(t, output.String(), "")

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	eq(t, url, "https://github.com/hubot/REPO/pull/23")
}

func TestReplaceExcessiveWhitespace(t *testing.T) {
	eq(t, replaceExcessiveWhitespace("hello\ngoodbye"), "hello goodbye")
	eq(t, replaceExcessiveWhitespace("  hello goodbye  "), "hello goodbye")
	eq(t, replaceExcessiveWhitespace("hello   goodbye"), "hello goodbye")
	eq(t, replaceExcessiveWhitespace("   hello \n goodbye   "), "hello goodbye")
}

func TestPrStateTitleWithColor(t *testing.T) {
	tests := map[string]struct {
		pr   api.PullRequest
		want string
	}{
		"Format OPEN state":              {pr: api.PullRequest{State: "OPEN", IsDraft: false}, want: "Open"},
		"Format OPEN state for Draft PR": {pr: api.PullRequest{State: "OPEN", IsDraft: true}, want: "Draft"},
		"Format CLOSED state":            {pr: api.PullRequest{State: "CLOSED", IsDraft: false}, want: "Closed"},
		"Format MERGED state":            {pr: api.PullRequest{State: "MERGED", IsDraft: false}, want: "Merged"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := prStateTitleWithColor(tc.pr)
			diff := cmp.Diff(tc.want, got)
			if diff != "" {
				t.Fatalf(diff)
			}
		})
	}
}
