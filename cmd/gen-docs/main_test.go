package main

import (
	"io/ioutil"
	"strings"
	"testing"
)

func Test_run(t *testing.T) {
	dir := t.TempDir()
	args := []string{"--man-page", "--website", "--doc-path", dir}
	err := run(args)
	if err != nil {
		t.Fatalf("got error: %v", err)
	}

	manPage, err := ioutil.ReadFile(dir + "/gh-issue-create.1")
	if err != nil {
		t.Fatalf("error reading `gh-issue-create.1`: %v", err)
	}
	if !strings.Contains(string(manPage), `\fBgh issue create`) {
		t.Fatal("man page corrupted")
	}

	markdownPage, err := ioutil.ReadFile(dir + "/gh_issue_create.md")
	if err != nil {
		t.Fatalf("error reading `gh_issue_create.md`: %v", err)
	}
	if !strings.Contains(string(markdownPage), `## gh issue create`) {
		t.Fatal("markdown page corrupted")
	}
}
