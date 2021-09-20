package list

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func runCommand(rt http.RoundTripper, isTTY bool, cli string) (*test.CmdOut, error) {
	io, _, stdout, stderr := iostreams.Test()
	io.SetStdoutTTY(isTTY)
	io.SetStdinTTY(isTTY)
	io.SetStderrTTY(isTTY)

	browser := &cmdutil.TestBrowser{}
	factory := &cmdutil.Factory{
		IOStreams: io,
		Browser:   browser,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: rt}, nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
	}

	cmd := NewCmdList(factory, nil)

	argv, err := shlex.Split(cli)
	if err != nil {
		return nil, err
	}
	cmd.SetArgs(argv)

	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(ioutil.Discard)
	cmd.SetErr(ioutil.Discard)

	_, err = cmd.ExecuteC()
	return &test.CmdOut{
		OutBuf:     stdout,
		ErrBuf:     stderr,
		BrowsedURL: browser.BrowsedURL(),
	}, err
}

func initFakeHTTP() *httpmock.Registry {
	return &httpmock.Registry{}
}

func TestPRList(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.Register(httpmock.GraphQL(`query PullRequestList\b`), httpmock.FileResponse("./fixtures/prList.json"))

	output, err := runCommand(http, true, "")
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, heredoc.Doc(`

		Showing 3 of 3 open pull requests in OWNER/REPO

		#32  New feature            feature
		#29  Fixed bad bug          hubot:bug-fix
		#28  Improve documentation  docs
	`), output.String())
	assert.Equal(t, ``, output.Stderr())
}

func TestPRList_nontty(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.Register(httpmock.GraphQL(`query PullRequestList\b`), httpmock.FileResponse("./fixtures/prList.json"))

	output, err := runCommand(http, false, "")
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "", output.Stderr())

	assert.Equal(t, `32	New feature	feature	DRAFT
29	Fixed bad bug	hubot:bug-fix	OPEN
28	Improve documentation	docs	MERGED
`, output.String())
}

func TestPRList_filtering(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query PullRequestList\b`),
		httpmock.GraphQLQuery(`{}`, func(_ string, params map[string]interface{}) {
			assert.Equal(t, []interface{}{"OPEN", "CLOSED", "MERGED"}, params["state"].([]interface{}))
		}))

	output, err := runCommand(http, true, `-s all`)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "", output.Stderr())
	assert.Equal(t, `
No pull requests match your search in OWNER/REPO

`, output.String())
}

func TestPRList_filteringRemoveDuplicate(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query PullRequestList\b`),
		httpmock.FileResponse("./fixtures/prListWithDuplicates.json"))

	output, err := runCommand(http, true, "")
	if err != nil {
		t.Fatal(err)
	}

	out := output.String()
	idx := strings.Index(out, "New feature")
	if idx < 0 {
		t.Fatalf("text %q not found in %q", "New feature", out)
	}
	assert.Equal(t, idx, strings.LastIndex(out, "New feature"))
}

func TestPRList_filteringClosed(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query PullRequestList\b`),
		httpmock.GraphQLQuery(`{}`, func(_ string, params map[string]interface{}) {
			assert.Equal(t, []interface{}{"CLOSED", "MERGED"}, params["state"].([]interface{}))
		}))

	_, err := runCommand(http, true, `-s closed`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPRList_filteringAssignee(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query PullRequestSearch\b`),
		httpmock.GraphQLQuery(`{}`, func(_ string, params map[string]interface{}) {
			assert.Equal(t, `repo:OWNER/REPO is:pr is:merged assignee:hubot label:"needs tests" base:develop`, params["q"].(string))
		}))

	_, err := runCommand(http, true, `-s merged -l "needs tests" -a hubot -B develop`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPRList_filteringDraft(t *testing.T) {
	tests := []struct {
		name          string
		cli           string
		expectedQuery string
	}{
		{
			name:          "draft",
			cli:           "--draft",
			expectedQuery: `repo:OWNER/REPO is:pr is:open draft:true`,
		},
		{
			name:          "non-draft",
			cli:           "--draft=false",
			expectedQuery: `repo:OWNER/REPO is:pr is:open draft:false`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			http := initFakeHTTP()
			defer http.Verify(t)

			http.Register(
				httpmock.GraphQL(`query PullRequestSearch\b`),
				httpmock.GraphQLQuery(`{}`, func(_ string, params map[string]interface{}) {
					assert.Equal(t, test.expectedQuery, params["q"].(string))
				}))

			_, err := runCommand(http, true, test.cli)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPRList_withInvalidLimitFlag(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)
	_, err := runCommand(http, true, `--limit=0`)
	assert.EqualError(t, err, "invalid value for --limit: 0")
}

func TestPRList_web(t *testing.T) {
	tests := []struct {
		name               string
		cli                string
		expectedBrowserURL string
	}{
		{
			name:               "filters",
			cli:                "-a peter -l bug -l docs -L 10 -s merged -B trunk",
			expectedBrowserURL: "https://github.com/OWNER/REPO/pulls?q=is%3Apr+is%3Amerged+assignee%3Apeter+label%3Abug+label%3Adocs+base%3Atrunk",
		},
		{
			name:               "draft",
			cli:                "--draft=true",
			expectedBrowserURL: "https://github.com/OWNER/REPO/pulls?q=is%3Apr+is%3Aopen+draft%3Atrue",
		},
		{
			name:               "non-draft",
			cli:                "--draft=0",
			expectedBrowserURL: "https://github.com/OWNER/REPO/pulls?q=is%3Apr+is%3Aopen+draft%3Afalse",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			http := initFakeHTTP()
			defer http.Verify(t)

			_, cmdTeardown := run.Stub()
			defer cmdTeardown(t)

			output, err := runCommand(http, true, "--web "+test.cli)
			if err != nil {
				t.Errorf("error running command `pr list` with `--web` flag: %v", err)
			}

			assert.Equal(t, "", output.String())
			assert.Equal(t, "Opening github.com/OWNER/REPO/pulls in your browser.\n", output.Stderr())
			assert.Equal(t, test.expectedBrowserURL, output.BrowsedURL)
		})
	}
}
