package issue

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"testing"

	"github.com/alecthomas/assert"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/google/shlex"
)

func TestIssueList_parsing(t *testing.T) {
	tests := []struct {
		name     string
		cli      string
		wants    ListOptions
		wantsErr bool
	}{
		{
			name: "no flags",
			cli:  "",
			wants: ListOptions{
				Assignee:   "",
				Author:     "",
				Labels:     []string{},
				Limit:      30,
				State:      "open",
				HasFilters: false,
			},
			wantsErr: false,
		},
		{
			name: "closed state",
			cli:  "--state closed",
			wants: ListOptions{
				Assignee:   "",
				Author:     "",
				Labels:     []string{},
				Limit:      30,
				State:      "closed",
				HasFilters: true,
			},
			wantsErr: false,
		},
		{
			name: "new limit",
			cli:  "--limit 10",
			wants: ListOptions{
				Assignee:   "",
				Author:     "",
				Labels:     []string{},
				Limit:      10,
				State:      "open",
				HasFilters: false,
			},
			wantsErr: false,
		},
		{
			name: "set assignee",
			cli:  "--assignee mislav",
			wants: ListOptions{
				Assignee:   "mislav",
				Author:     "",
				Labels:     []string{},
				Limit:      30,
				State:      "open",
				HasFilters: true,
			},
			wantsErr: false,
		},
		{
			name: "set assignee",
			cli:  "--author vilmibm",
			wants: ListOptions{
				Assignee:   "",
				Author:     "vilmibm",
				Labels:     []string{},
				Limit:      30,
				State:      "open",
				HasFilters: true,
			},
			wantsErr: false,
		},
		{
			name: "set labels",
			cli:  "--label 'bug,feature'",
			wants: ListOptions{
				Assignee:   "",
				Author:     "",
				Labels:     []string{"bug", "feature"},
				Limit:      30,
				State:      "open",
				HasFilters: true,
			},
			wantsErr: false,
		},
		{
			name: "negative limit",
			cli:  "--limit -10",
			wants: ListOptions{
				Assignee:   "",
				Author:     "",
				Labels:     []string{},
				Limit:      30,
				State:      "open",
				HasFilters: true,
			},
			wantsErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newListCmd(SharedOptions{}, func(o *ListOptions) {
				assert.Equal(t, tt.wants.Assignee, o.Assignee)
				assert.Equal(t, tt.wants.Author, o.Author)
				assert.Equal(t, tt.wants.Labels, o.Labels)
				assert.Equal(t, tt.wants.Limit, o.Limit)
				assert.Equal(t, tt.wants.State, o.State)
			})

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)
			cmd.SetArgs(argv)
			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestIssueList_success(t *testing.T) {
	tr := &httpmock.Registry{}
	tr.StubRepoResponse("OWNER", "REPO")
	jsonFile, _ := os.Open("../test/fixtures/issueList.json")
	defer jsonFile.Close()
	tr.StubResponse(200, jsonFile)

	i, _, stdout, stderr := iostreams.Test()
	opts := ListOptions{
		State:      "open",
		Assignee:   "",
		Limit:      30,
		Labels:     []string{},
		Author:     "",
		HasFilters: false,
		SharedOptions: SharedOptions{
			IO:           i,
			BaseRepo:     func() (ghrepo.Interface, error) { return ghrepo.New("OWNER", "REPO"), nil },
			HttpClient:   func() (*http.Client, error) { return &http.Client{Transport: tr}, nil },
			ColorableErr: func() io.Writer { return i.ErrOut },
			ColorableOut: func() io.Writer { return i.Out },
		},
	}

	fmt.Printf("🌭 err %+v\n", stderr.String())
	fmt.Printf("🌭 out %+v\n", stdout.String())

	err := list(opts)
	if err != nil {
		t.Errorf("error running command `issue list`: %v", err)
	}

	eq(t, stderr.String(), `
Showing 3 of 3 issues in OWNER/REPO

`)

	expectedIssues := []*regexp.Regexp{
		regexp.MustCompile(`(?m)^1\t.*won`),
		regexp.MustCompile(`(?m)^2\t.*too`),
		regexp.MustCompile(`(?m)^4\t.*fore`),
	}

	for _, r := range expectedIssues {
		if !r.MatchString(stdout.String()) {
			t.Errorf("output did not match regexp /%s/\n> output\n%s\n", r, stdout.String())
			return
		}
	}
}

// func TestIssueList_withFlags(t *testing.T) {
// 	initBlankContext("", "OWNER/REPO", "master")
// 	http := initFakeHTTP()
// 	http.StubRepoResponse("OWNER", "REPO")

// 	http.StubResponse(200, bytes.NewBufferString(`
// 	{ "data": {	"repository": {
// 		"hasIssuesEnabled": true,
// 		"issues": { "nodes": [] }
// 	} } }
// 	`))

// 	output, err := RunCommand("issue list -a probablyCher -l web,bug -s open -A foo")
// 	if err != nil {
// 		t.Errorf("error running command `issue list`: %v", err)
// 	}

// 	eq(t, output.String(), "")
// 	eq(t, output.Stderr(), `
// No issues match your search in OWNER/REPO

// `)

// 	bodyBytes, _ := ioutil.ReadAll(http.Requests[1].Body)
// 	reqBody := struct {
// 		Variables struct {
// 			Assignee string
// 			Labels   []string
// 			States   []string
// 			Author   string
// 		}
// 	}{}
// 	_ = json.Unmarshal(bodyBytes, &reqBody)

// 	eq(t, reqBody.Variables.Assignee, "probablyCher")
// 	eq(t, reqBody.Variables.Labels, []string{"web", "bug"})
// 	eq(t, reqBody.Variables.States, []string{"OPEN"})
// 	eq(t, reqBody.Variables.Author, "foo")
// }

// func TestIssueList_withInvalidLimitFlag(t *testing.T) {
// 	initBlankContext("", "OWNER/REPO", "master")
// 	http := initFakeHTTP()
// 	http.StubRepoResponse("OWNER", "REPO")

// 	_, err := RunCommand("issue list --limit=0")

// 	if err == nil || err.Error() != "invalid limit: 0" {
// 		t.Errorf("error running command `issue list`: %v", err)
// 	}
// }

// func TestIssueList_nullAssigneeLabels(t *testing.T) {
// 	initBlankContext("", "OWNER/REPO", "master")
// 	http := initFakeHTTP()
// 	http.StubRepoResponse("OWNER", "REPO")

// 	http.StubResponse(200, bytes.NewBufferString(`
// 	{ "data": {	"repository": {
// 		"hasIssuesEnabled": true,
// 		"issues": { "nodes": [] }
// 	} } }
// 	`))

// 	_, err := RunCommand("issue list")
// 	if err != nil {
// 		t.Errorf("error running command `issue list`: %v", err)
// 	}

// 	bodyBytes, _ := ioutil.ReadAll(http.Requests[1].Body)
// 	reqBody := struct {
// 		Variables map[string]interface{}
// 	}{}
// 	_ = json.Unmarshal(bodyBytes, &reqBody)

// 	_, assigneeDeclared := reqBody.Variables["assignee"]
// 	_, labelsDeclared := reqBody.Variables["labels"]
// 	eq(t, assigneeDeclared, false)
// 	eq(t, labelsDeclared, false)
// }

// func TestIssueList_disabledIssues(t *testing.T) {
// 	initBlankContext("", "OWNER/REPO", "master")
// 	http := initFakeHTTP()
// 	http.StubRepoResponse("OWNER", "REPO")

// 	http.StubResponse(200, bytes.NewBufferString(`
// 	{ "data": {	"repository": {
// 		"hasIssuesEnabled": false
// 	} } }
// 	`))

// 	_, err := RunCommand("issue list")
// 	if err == nil || err.Error() != "the 'OWNER/REPO' repository has disabled issues" {
// 		t.Errorf("error running command `issue list`: %v", err)
// 	}
// }

func eq(t *testing.T, got interface{}, expected interface{}) {
	t.Helper()
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("expected: %v, got: %v", expected, got)
	}
}
