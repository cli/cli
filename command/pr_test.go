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

	"github.com/github/gh-cli/utils"
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

func RunCommand(cmd *cobra.Command, args string) (string, error) {
	rootCmd := cmd.Root()
	argv, err := shlex.Split(args)
	if err != nil {
		return "", err
	}
	rootCmd.SetArgs(argv)

	outBuf := bytes.Buffer{}
	cmd.SetOut(&outBuf)

	// Reset flag values so they don't leak between tests
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		switch v := f.Value.(type) {
		case pflag.SliceValue:
			v.Replace([]string{})
		default:
			if v.Type() == "bool" {
				v.Set(f.DefValue)
			}
		}
	})

	_, err = rootCmd.ExecuteC()
	return outBuf.String(), err
}

func TestPRStatus(t *testing.T) {
	initBlankContext("OWNER/REPO", "blueberries")
	http := initFakeHTTP()

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
		if !r.MatchString(output) {
			t.Errorf("output did not match regexp /%s/", r)
		}
	}
}

func TestPRList(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()

	jsonFile, _ := os.Open("../test/fixtures/prList.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	output, err := RunCommand(prListCmd, "pr list")
	if err != nil {
		t.Fatal(err)
	}

	eq(t, output, `32	New feature	feature
29	Fixed bad bug	hubot:bug-fix
28	Improve documentation	docs
`)
}

func TestPRList_filtering(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()

	respBody := bytes.NewBufferString(`{ "data": {} }`)
	http.StubResponse(200, respBody)

	_, err := RunCommand(prListCmd, `pr list -s all -l one,two -l three`)
	if err != nil {
		t.Fatal(err)
	}

	bodyBytes, _ := ioutil.ReadAll(http.Requests[0].Body)
	reqBody := struct {
		Variables struct {
			State  []string
			Labels []string
		}
	}{}
	json.Unmarshal(bodyBytes, &reqBody)

	eq(t, reqBody.Variables.State, []string{"OPEN", "CLOSED", "MERGED"})
	eq(t, reqBody.Variables.Labels, []string{"one", "two", "three"})
}

func TestPRList_filteringAssignee(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()

	respBody := bytes.NewBufferString(`{ "data": {} }`)
	http.StubResponse(200, respBody)

	_, err := RunCommand(prListCmd, `pr list -s merged -l "needs tests" -a hubot -B develop`)
	if err != nil {
		t.Fatal(err)
	}

	bodyBytes, _ := ioutil.ReadAll(http.Requests[0].Body)
	reqBody := struct {
		Variables struct {
			Q string
		}
	}{}
	json.Unmarshal(bodyBytes, &reqBody)

	eq(t, reqBody.Variables.Q, `repo:OWNER/REPO assignee:hubot is:pr sort:created-desc is:merged label:"needs tests" base:"develop"`)
}

func TestPRList_filteringAssigneeLabels(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()

	respBody := bytes.NewBufferString(`{ "data": {} }`)
	http.StubResponse(200, respBody)

	_, err := RunCommand(prListCmd, `pr list -l one,two -a hubot`)
	if err == nil && err.Error() != "multiple labels with --assignee are not supported" {
		t.Fatal(err)
	}
}

func TestPRView_currentBranch(t *testing.T) {
	initBlankContext("OWNER/REPO", "blueberries")
	http := initFakeHTTP()

	jsonFile, _ := os.Open("../test/fixtures/prView.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	var seenCmd *exec.Cmd
	restoreCmd := utils.SetPrepareCmd(func(cmd *exec.Cmd) utils.Runnable {
		switch strings.Join(cmd.Args, " ") {
		case `git config --get-regexp ^branch\.blueberries\.(remote|merge)$`:
			return &outputStub{}
		default:
			seenCmd = cmd
			return &outputStub{}
		}
	})
	defer restoreCmd()

	output, err := RunCommand(prViewCmd, "pr view")
	if err != nil {
		t.Errorf("error running command `pr view`: %v", err)
	}

	if output == "" {
		t.Errorf("command output expected got an empty string")
	}

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	if url != "https://github.com/OWNER/REPO/pull/10" {
		t.Errorf("got: %q", url)
	}
}

func TestPRView_noResultsForBranch(t *testing.T) {
	initBlankContext("OWNER/REPO", "blueberries")
	http := initFakeHTTP()

	jsonFile, _ := os.Open("../test/fixtures/prView_NoActiveBranch.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	var seenCmd *exec.Cmd
	restoreCmd := utils.SetPrepareCmd(func(cmd *exec.Cmd) utils.Runnable {
		switch strings.Join(cmd.Args, " ") {
		case `git config --get-regexp ^branch\.blueberries\.(remote|merge)$`:
			return &outputStub{}
		default:
			seenCmd = cmd
			return &outputStub{}
		}
	})
	defer restoreCmd()

	_, err := RunCommand(prViewCmd, "pr view")
	if err == nil || err.Error() != `no open pull requests found for branch "blueberries"` {
		t.Errorf("error running command `pr view`: %v", err)
	}

	if seenCmd != nil {
		t.Fatalf("unexpected command: %v", seenCmd.Args)
	}
}

func TestPRView_numberArg(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "pullRequest": {
		"url": "https://github.com/OWNER/REPO/pull/23"
	} } } }
	`))

	var seenCmd *exec.Cmd
	restoreCmd := utils.SetPrepareCmd(func(cmd *exec.Cmd) utils.Runnable {
		seenCmd = cmd
		return &outputStub{}
	})
	defer restoreCmd()

	output, err := RunCommand(prViewCmd, "pr view 23")
	if err != nil {
		t.Errorf("error running command `pr view`: %v", err)
	}

	if output == "" {
		t.Errorf("command output expected got an empty string")
	}

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	if url != "https://github.com/OWNER/REPO/pull/23" {
		t.Errorf("got: %q", url)
	}
}
