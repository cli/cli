package command

import (
	"bytes"
	"testing"
)

func TestPRDiff_validation(t *testing.T) {
	_, err := RunCommand("pr diff --color=doublerainbow")
	if err == nil {
		t.Fatal("expected error")
	}
	eq(t, err.Error(), `did not understand color: "doublerainbow". Expected one of always, never, or auto`)
}

func TestPRDiff_no_current_pr(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": { "pullRequests": { "nodes": [
			{ "url": "https://github.com/OWNER/REPO/pull/123",
			  "number": 123,
			  "id": "foobar123",
			  "headRefName": "feature",
				"baseRefName": "master" }
		] } } } }`))
	http.StubResponse(200, bytes.NewBufferString(testDiff))
	_, err := RunCommand("pr diff")
	if err == nil {
		t.Fatal("expected error", err)
	}
	eq(t, err.Error(), `could not find pull request: no open pull requests found for branch "master"`)
}

func TestPRDiff_argument_not_found(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.StubResponse(404, bytes.NewBufferString(""))
	_, err := RunCommand("pr diff 123")
	if err == nil {
		t.Fatal("expected error", err)
	}
	eq(t, err.Error(), `could not find pull request diff: pull request not found`)
}

func TestPRDiff(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "feature")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": { "pullRequests": { "nodes": [
			{ "url": "https://github.com/OWNER/REPO/pull/123",
			  "number": 123,
			  "id": "foobar123",
			  "headRefName": "feature",
				"baseRefName": "master" }
		] } } } }`))
	http.StubResponse(200, bytes.NewBufferString(testDiff))
	output, err := RunCommand("pr diff")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	eq(t, output.String(), testDiff)
}

const testDiff = `diff --git a/.github/workflows/releases.yml b/.github/workflows/releases.yml
index 73974448..b7fc0154 100644
--- a/.github/workflows/releases.yml
+++ b/.github/workflows/releases.yml
@@ -44,6 +44,11 @@ jobs:
           token: ${{secrets.SITE_GITHUB_TOKEN}}
       - name: Publish documentation site
         if: "!contains(github.ref, '-')" # skip prereleases
+        env:
+          GIT_COMMITTER_NAME: cli automation
+          GIT_AUTHOR_NAME: cli automation
+          GIT_COMMITTER_EMAIL: noreply@github.com
+          GIT_AUTHOR_EMAIL: noreply@github.com
         run: make site-publish
       - name: Move project cards
         if: "!contains(github.ref, '-')" # skip prereleases
diff --git a/Makefile b/Makefile
index f2b4805c..3d7bd0f9 100644
--- a/Makefile
+++ b/Makefile
@@ -22,8 +22,8 @@ test:
 	go test ./...
 .PHONY: test
 
-site:
-	git clone https://github.com/github/cli.github.com.git "$@"
+site: bin/gh
+	bin/gh repo clone github/cli.github.com "$@"
 
 site-docs: site
 	git -C site pull
`
