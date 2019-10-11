package command

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/github/gh-cli/api"
)

func TestPR(t *testing.T) {
	teardown := mockGraphQL("pr.json")
	defer teardown()

	output := captureOutput(func() {
		err := ExecutePr()
		if err != nil {
			t.Errorf("got error %v", err)
		}
	})

	if !strings.Contains(output, "#9 Pull in the GraphQL API functionality [graphql]") {
		t.Errorf("expected to list PR #8 but did not")
	}
}

func mockGraphQL(fixtureName string) (teardown func()) {
	api.OverriddenQueryFunction = func(query string, variables map[string]string, v interface{}) error {
		contents, err := ioutil.ReadFile("fixtures/" + fixtureName)
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
