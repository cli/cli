package command

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"reflect"
	"regexp"
	"testing"

	"github.com/github/gh-cli/test"
)

func eq(t *testing.T, got interface{}, expected interface{}) {
	t.Helper()
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("expected: %v, got: %v", expected, got)
	}
}

func TestPRStatus(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()

	jsonFile, _ := os.Open("../test/fixtures/prStatus.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	output, err := test.RunCommand(RootCmd, "pr status")
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

	out := bytes.Buffer{}
	prListCmd.SetOut(&out)

	RootCmd.SetArgs([]string{"pr", "list"})
	_, err := RootCmd.ExecuteC()
	if err != nil {
		t.Fatal(err)
	}

	eq(t, out.String(), `32	New feature	feature
29	Fixed bad bug	bug-fix
28	Improve documentation	docs
`)
}

func TestPRList_filtering(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()

	respBody := bytes.NewBufferString(`{ "data": {} }`)
	http.StubResponse(200, respBody)

	prListCmd.SetOut(ioutil.Discard)

	RootCmd.SetArgs([]string{"pr", "list", "-s", "all", "-l", "one", "-l", "two"})
	_, err := RootCmd.ExecuteC()
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
	eq(t, reqBody.Variables.Labels, []string{"one", "two"})
}

func TestPRView(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()

	jsonFile, _ := os.Open("../test/fixtures/prView.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	teardown, callCount := mockOpenInBrowser()
	defer teardown()

	output, err := test.RunCommand(RootCmd, "pr view")
	if err != nil {
		t.Errorf("error running command `pr view`: %v", err)
	}

	if output == "" {
		t.Errorf("command output expected got an empty string")
	}

	if *callCount != 1 {
		t.Errorf("OpenInBrowser should be called 1 time but was called %d time(s)", *callCount)
	}
}

func TestPRView_NoActiveBranch(t *testing.T) {
	initBlankContext("OWNER/REPO", "master")
	http := initFakeHTTP()

	jsonFile, _ := os.Open("../test/fixtures/prView_NoActiveBranch.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	teardown, callCount := mockOpenInBrowser()
	defer teardown()

	output, err := test.RunCommand(RootCmd, "pr view")
	if err == nil || err.Error() != "the 'master' branch has no open pull requests" {
		t.Errorf("error running command `pr view`: %v", err)
	}

	if *callCount > 0 {
		t.Errorf("OpenInBrowser should NOT be called but was called %d time(s)", *callCount)
	}

	// Now run again but provide a PR number
	output, err = test.RunCommand(RootCmd, "pr view 23")
	if err != nil {
		t.Errorf("error running command `pr view`: %v", err)
	}

	if output == "" {
		t.Errorf("command output expected got an empty string")
	}

	if *callCount != 1 {
		t.Errorf("OpenInBrowser should be called once but was called %d time(s)", *callCount)
	}
}
