package command

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/github/gh-cli/api"
)

func TestPRList(t *testing.T) {
	teardown := mockGraphQLResponse("fixtures/pr.json")
	defer teardown()

	output, err := runCommand("pr list")
	if err != nil {
		t.Errorf("error running command `pr list`: %v", err)
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

func mockGraphQLResponse(fixturePath string) (teardown func()) {
	api.OverriddenQueryFunction = func(query string, variables map[string]string, v interface{}) error {
		contents, err := ioutil.ReadFile(fixturePath)
		if err != nil {
			return err
		}
		json.Unmarshal(contents, &v)
		if err != nil {
			return err
		}

		return nil
	}

	return func() {
		api.OverriddenQueryFunction = nil
	}
}

func runCommand(s string) (string, error) {
	var err error
	output := captureOutput(func() {
		RootCmd.SetArgs(strings.Split(s, " "))
		_, err = RootCmd.ExecuteC()
	})
	if err != nil {
		return "", err
	}
	return output, nil
}

func captureOutput(f func()) string {
	originalStdout := os.Stdout
	defer func() {
		os.Stdout = originalStdout
	}()

	r, w, err := os.Pipe()
	if err != nil {
		panic("failed to pipe stdout")
	}
	os.Stdout = w

	f()

	w.Close()
	out, err := ioutil.ReadAll(r)
	if err != nil {
		panic("failed to read captured input from stdout")
	}

	return string(out)
}
