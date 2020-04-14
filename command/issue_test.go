package command

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"github.com/cli/cli/internal/run"
	"github.com/cli/cli/test"
	"github.com/google/go-cmp/cmp"
)

func TestIssueStatus(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	jsonFile, _ := os.Open("../test/fixtures/issueStatus.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	output, err := RunCommand(issueStatusCmd, "issue status")
	if err != nil {
		t.Errorf("error running command `issue status`: %v", err)
	}

	expectedIssues := []*regexp.Regexp{
		regexp.MustCompile(`(?m)8.*carrots.*about.*ago`),
		regexp.MustCompile(`(?m)9.*squash.*about.*ago`),
		regexp.MustCompile(`(?m)10.*broccoli.*about.*ago`),
		regexp.MustCompile(`(?m)11.*swiss chard.*about.*ago`),
	}

	for _, r := range expectedIssues {
		if !r.MatchString(output.String()) {
			t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
			return
		}
	}
}

func TestIssueStatus_blankSlate(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": {
		"hasIssuesEnabled": true,
		"assigned": { "nodes": [] },
		"mentioned": { "nodes": [] },
		"authored": { "nodes": [] }
	} } }
	`))

	output, err := RunCommand(issueStatusCmd, "issue status")
	if err != nil {
		t.Errorf("error running command `issue status`: %v", err)
	}

	expectedOutput := `
Relevant issues in OWNER/REPO

Issues assigned to you
  There are no issues assigned to you

Issues mentioning you
  There are no issues mentioning you

Issues opened by you
  There are no issues opened by you

`
	if output.String() != expectedOutput {
		t.Errorf("expected %q, got %q", expectedOutput, output)
	}
}

func TestIssueStatus_disabledIssues(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": {
		"hasIssuesEnabled": false
	} } }
	`))

	_, err := RunCommand(issueStatusCmd, "issue status")
	if err == nil || err.Error() != "the 'OWNER/REPO' repository has disabled issues" {
		t.Errorf("error running command `issue status`: %v", err)
	}
}

func TestIssueList(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	jsonFile, _ := os.Open("../test/fixtures/issueList.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	output, err := RunCommand(issueListCmd, "issue list")
	if err != nil {
		t.Errorf("error running command `issue list`: %v", err)
	}

	eq(t, output.Stderr(), `
Showing 3 of 3 issues in OWNER/REPO

`)

	expectedIssues := []*regexp.Regexp{
		regexp.MustCompile(`(?m)^1\t.*won`),
		regexp.MustCompile(`(?m)^2\t.*too`),
		regexp.MustCompile(`(?m)^4\t.*fore`),
	}

	for _, r := range expectedIssues {
		if !r.MatchString(output.String()) {
			t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, output)
			return
		}
	}
}

func TestIssueList_withFlags(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": {	"repository": {
		"hasIssuesEnabled": true,
		"issues": { "nodes": [] }
	} } }
	`))

	output, err := RunCommand(issueListCmd, "issue list -a probablyCher -l web,bug -s open -A foo")
	if err != nil {
		t.Errorf("error running command `issue list`: %v", err)
	}

	eq(t, output.String(), "")
	eq(t, output.Stderr(), `
No issues match your search in OWNER/REPO

`)

	bodyBytes, _ := ioutil.ReadAll(http.Requests[1].Body)
	reqBody := struct {
		Variables struct {
			Assignee string
			Labels   []string
			States   []string
			Author   string
		}
	}{}
	_ = json.Unmarshal(bodyBytes, &reqBody)

	eq(t, reqBody.Variables.Assignee, "probablyCher")
	eq(t, reqBody.Variables.Labels, []string{"web", "bug"})
	eq(t, reqBody.Variables.States, []string{"OPEN"})
	eq(t, reqBody.Variables.Author, "foo")
}

func TestIssueList_nullAssigneeLabels(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": {	"repository": {
		"hasIssuesEnabled": true,
		"issues": { "nodes": [] }
	} } }
	`))

	_, err := RunCommand(issueListCmd, "issue list")
	if err != nil {
		t.Errorf("error running command `issue list`: %v", err)
	}

	bodyBytes, _ := ioutil.ReadAll(http.Requests[1].Body)
	reqBody := struct {
		Variables map[string]interface{}
	}{}
	_ = json.Unmarshal(bodyBytes, &reqBody)

	_, assigneeDeclared := reqBody.Variables["assignee"]
	_, labelsDeclared := reqBody.Variables["labels"]
	eq(t, assigneeDeclared, false)
	eq(t, labelsDeclared, false)
}

func TestIssueList_disabledIssues(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": {	"repository": {
		"hasIssuesEnabled": false
	} } }
	`))

	_, err := RunCommand(issueListCmd, "issue list")
	if err == nil || err.Error() != "the 'OWNER/REPO' repository has disabled issues" {
		t.Errorf("error running command `issue list`: %v", err)
	}
}

func TestIssueView_web(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "hasIssuesEnabled": true, "issue": {
		"number": 123,
		"url": "https://github.com/OWNER/REPO/issues/123"
	} } } }
	`))

	var seenCmd *exec.Cmd
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		seenCmd = cmd
		return &test.OutputStub{}
	})
	defer restoreCmd()

	output, err := RunCommand(issueViewCmd, "issue view -w 123")
	if err != nil {
		t.Errorf("error running command `issue view`: %v", err)
	}

	eq(t, output.String(), "")
	eq(t, output.Stderr(), "Opening https://github.com/OWNER/REPO/issues/123 in your browser.\n")

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	eq(t, url, "https://github.com/OWNER/REPO/issues/123")
}

func TestIssueView_web_numberArgWithHash(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "hasIssuesEnabled": true, "issue": {
		"number": 123,
		"url": "https://github.com/OWNER/REPO/issues/123"
	} } } }
	`))

	var seenCmd *exec.Cmd
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		seenCmd = cmd
		return &test.OutputStub{}
	})
	defer restoreCmd()

	output, err := RunCommand(issueViewCmd, "issue view -w \"#123\"")
	if err != nil {
		t.Errorf("error running command `issue view`: %v", err)
	}

	eq(t, output.String(), "")
	eq(t, output.Stderr(), "Opening https://github.com/OWNER/REPO/issues/123 in your browser.\n")

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	eq(t, url, "https://github.com/OWNER/REPO/issues/123")
}

func TestIssueView_Preview(t *testing.T) {
	tests := map[string]struct {
		ownerRepo       string
		command         string
		fixture         string
		expectedOutputs []string
	}{
		"Open issue without metadata": {
			ownerRepo: "master",
			command:   "issue view 123",
			fixture:   "../test/fixtures/issueView_preview.json",
			expectedOutputs: []string{
				`ix of coins`,
				`Open • marseilles opened about 292 years ago • 9 comments`,
				`bold story`,
				`View this issue on GitHub: https://github.com/OWNER/REPO/issues/123`,
			},
		},
		"Open issue with metadata": {
			ownerRepo: "master",
			command:   "issue view 123",
			fixture:   "../test/fixtures/issueView_previewWithMetadata.json",
			expectedOutputs: []string{
				`ix of coins`,
				`Open • marseilles opened about 292 years ago • 9 comments`,
				`Assignees: marseilles, monaco\n`,
				`Labels: one, two, three, four, five\n`,
				`Projects: Project 1 \(column A\), Project 2 \(column B\), Project 3 \(column C\)\n`,
				`Milestone: uluru\n`,
				`bold story`,
				`View this issue on GitHub: https://github.com/OWNER/REPO/issues/123`,
			},
		},
		"Open issue with empty body": {
			ownerRepo: "master",
			command:   "issue view 123",
			fixture:   "../test/fixtures/issueView_previewWithEmptyBody.json",
			expectedOutputs: []string{
				`ix of coins`,
				`Open • marseilles opened about 292 years ago • 9 comments`,
				`View this issue on GitHub: https://github.com/OWNER/REPO/issues/123`,
			},
		},
		"Closed issue": {
			ownerRepo: "master",
			command:   "issue view 123",
			fixture:   "../test/fixtures/issueView_previewClosedState.json",
			expectedOutputs: []string{
				`ix of coins`,
				`Closed • marseilles opened about 292 years ago • 9 comments`,
				`bold story`,
				`View this issue on GitHub: https://github.com/OWNER/REPO/issues/123`,
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

			output, err := RunCommand(issueViewCmd, tc.command)
			if err != nil {
				t.Errorf("error running command `%v`: %v", tc.command, err)
			}

			eq(t, output.Stderr(), "")

			test.ExpectLines(t, output.String(), tc.expectedOutputs...)
		})
	}
}

func TestIssueView_web_notFound(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()

	http.StubResponse(200, bytes.NewBufferString(`
	{ "errors": [
		{ "message": "Could not resolve to an Issue with the number of 9999." }
	] }
	`))

	var seenCmd *exec.Cmd
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		seenCmd = cmd
		return &test.OutputStub{}
	})
	defer restoreCmd()

	_, err := RunCommand(issueViewCmd, "issue view -w 9999")
	if err == nil || err.Error() != "graphql error: 'Could not resolve to an Issue with the number of 9999.'" {
		t.Errorf("error running command `issue view`: %v", err)
	}

	if seenCmd != nil {
		t.Fatal("did not expect any command to run")
	}
}

func TestIssueView_disabledIssues(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": {
			"id": "REPOID",
			"hasIssuesEnabled": false
		} } }
	`))

	_, err := RunCommand(issueViewCmd, `issue view 6666`)
	if err == nil || err.Error() != "the 'OWNER/REPO' repository has disabled issues" {
		t.Errorf("error running command `issue view`: %v", err)
	}
}

func TestIssueView_web_urlArg(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": { "hasIssuesEnabled": true, "issue": {
		"number": 123,
		"url": "https://github.com/OWNER/REPO/issues/123"
	} } } }
	`))

	var seenCmd *exec.Cmd
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		seenCmd = cmd
		return &test.OutputStub{}
	})
	defer restoreCmd()

	output, err := RunCommand(issueViewCmd, "issue view -w https://github.com/OWNER/REPO/issues/123")
	if err != nil {
		t.Errorf("error running command `issue view`: %v", err)
	}

	eq(t, output.String(), "")

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	eq(t, url, "https://github.com/OWNER/REPO/issues/123")
}

func TestIssueCreate(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": {
			"id": "REPOID",
			"hasIssuesEnabled": true
		} } }
	`))
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "createIssue": { "issue": {
			"URL": "https://github.com/OWNER/REPO/issues/12"
		} } } }
	`))

	output, err := RunCommand(issueCreateCmd, `issue create -t hello -b "cash rules everything around me"`)
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	bodyBytes, _ := ioutil.ReadAll(http.Requests[2].Body)
	reqBody := struct {
		Variables struct {
			Input struct {
				RepositoryID string
				Title        string
				Body         string
			}
		}
	}{}
	_ = json.Unmarshal(bodyBytes, &reqBody)

	eq(t, reqBody.Variables.Input.RepositoryID, "REPOID")
	eq(t, reqBody.Variables.Input.Title, "hello")
	eq(t, reqBody.Variables.Input.Body, "cash rules everything around me")

	eq(t, output.String(), "https://github.com/OWNER/REPO/issues/12\n")
}

func TestIssueCreate_disabledIssues(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": {
			"id": "REPOID",
			"hasIssuesEnabled": false
		} } }
	`))

	_, err := RunCommand(issueCreateCmd, `issue create -t heres -b johnny`)
	if err == nil || err.Error() != "the 'OWNER/REPO' repository has disabled issues" {
		t.Errorf("error running command `issue create`: %v", err)
	}
}

func TestIssueCreate_web(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	var seenCmd *exec.Cmd
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		seenCmd = cmd
		return &test.OutputStub{}
	})
	defer restoreCmd()

	output, err := RunCommand(issueCreateCmd, `issue create --web`)
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	eq(t, url, "https://github.com/OWNER/REPO/issues/new")
	eq(t, output.String(), "Opening github.com/OWNER/REPO/issues/new in your browser.\n")
	eq(t, output.Stderr(), "")
}

func TestIssueCreate_webTitleBody(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "master")
	http := initFakeHTTP()
	http.StubRepoResponse("OWNER", "REPO")

	var seenCmd *exec.Cmd
	restoreCmd := run.SetPrepareCmd(func(cmd *exec.Cmd) run.Runnable {
		seenCmd = cmd
		return &test.OutputStub{}
	})
	defer restoreCmd()

	output, err := RunCommand(issueCreateCmd, `issue create -w -t mytitle -b mybody`)
	if err != nil {
		t.Errorf("error running command `issue create`: %v", err)
	}

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := strings.ReplaceAll(seenCmd.Args[len(seenCmd.Args)-1], "^", "")
	eq(t, url, "https://github.com/OWNER/REPO/issues/new?title=mytitle&body=mybody")
	eq(t, output.String(), "Opening github.com/OWNER/REPO/issues/new in your browser.\n")
}

func Test_listHeader(t *testing.T) {
	type args struct {
		repoName        string
		itemName        string
		matchCount      int
		totalMatchCount int
		hasFilters      bool
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "no results",
			args: args{
				repoName:        "REPO",
				itemName:        "table",
				matchCount:      0,
				totalMatchCount: 0,
				hasFilters:      false,
			},
			want: "There are no open tables in REPO",
		},
		{
			name: "no matches after filters",
			args: args{
				repoName:        "REPO",
				itemName:        "Luftballon",
				matchCount:      0,
				totalMatchCount: 0,
				hasFilters:      true,
			},
			want: "No Luftballons match your search in REPO",
		},
		{
			name: "one result",
			args: args{
				repoName:        "REPO",
				itemName:        "genie",
				matchCount:      1,
				totalMatchCount: 23,
				hasFilters:      false,
			},
			want: "Showing 1 of 23 genies in REPO",
		},
		{
			name: "one result after filters",
			args: args{
				repoName:        "REPO",
				itemName:        "tiny cup",
				matchCount:      1,
				totalMatchCount: 23,
				hasFilters:      true,
			},
			want: "Showing 1 of 23 tiny cups in REPO that match your search",
		},
		{
			name: "one result in total",
			args: args{
				repoName:        "REPO",
				itemName:        "chip",
				matchCount:      1,
				totalMatchCount: 1,
				hasFilters:      false,
			},
			want: "Showing 1 of 1 chip in REPO",
		},
		{
			name: "one result in total after filters",
			args: args{
				repoName:        "REPO",
				itemName:        "spicy noodle",
				matchCount:      1,
				totalMatchCount: 1,
				hasFilters:      true,
			},
			want: "Showing 1 of 1 spicy noodle in REPO that matches your search",
		},
		{
			name: "multiple results",
			args: args{
				repoName:        "REPO",
				itemName:        "plant",
				matchCount:      4,
				totalMatchCount: 23,
				hasFilters:      false,
			},
			want: "Showing 4 of 23 plants in REPO",
		},
		{
			name: "multiple results after filters",
			args: args{
				repoName:        "REPO",
				itemName:        "boomerang",
				matchCount:      4,
				totalMatchCount: 23,
				hasFilters:      true,
			},
			want: "Showing 4 of 23 boomerangs in REPO that match your search",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := listHeader(tt.args.repoName, tt.args.itemName, tt.args.matchCount, tt.args.totalMatchCount, tt.args.hasFilters); got != tt.want {
				t.Errorf("listHeader() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIssueStateTitleWithColor(t *testing.T) {
	tests := map[string]struct {
		state string
		want  string
	}{
		"Open state":   {state: "OPEN", want: "Open"},
		"Closed state": {state: "CLOSED", want: "Closed"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := issueStateTitleWithColor(tc.state)
			diff := cmp.Diff(tc.want, got)
			if diff != "" {
				t.Fatalf(diff)
			}
		})
	}
}
