package list

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/run"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runCommand(rt http.RoundTripper, isTTY bool, cli string) (*test.CmdOut, error) {
	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(isTTY)
	ios.SetStdinTTY(isTTY)
	ios.SetStderrTTY(isTTY)

	browser := &browser.Stub{}
	factory := &cmdutil.Factory{
		IOStreams: ios,
		Browser:   browser,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: rt}, nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
	}

	fakeNow := func() time.Time {
		return time.Date(2022, time.August, 24, 23, 50, 0, 0, time.UTC)
	}

	cmd := NewCmdList(factory, func(opts *ListOptions) error {
		opts.Now = fakeNow
		return listRun(opts)
	})

	argv, err := shlex.Split(cli)
	if err != nil {
		return nil, err
	}
	cmd.SetArgs(argv)

	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

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

		ID   TITLE                  BRANCH         CREATED AT
		#32  New feature            feature        about 3 hours ago
		#29  Fixed bad bug          hubot:bug-fix  about 1 month ago
		#28  Improve documentation  docs           about 2 years ago
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

	assert.Equal(t, `32	New feature	feature	DRAFT	2022-08-24T20:01:12Z
29	Fixed bad bug	hubot:bug-fix	OPEN	2022-07-20T19:01:12Z
28	Improve documentation	docs	MERGED	2020-01-26T19:01:12Z
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
	assert.Error(t, err)

	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
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
	assert.Error(t, err)
}

func TestPRList_filteringHeadBranch(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query PullRequestList\b`),
		httpmock.GraphQLQuery(`{}`, func(_ string, params map[string]interface{}) {
			assert.Equal(t, interface{}("bug-fix"), params["headBranch"])
		}))

	_, err := runCommand(http, true, `-H bug-fix`)
	assert.Error(t, err)
}

func TestPRList_filteringAssignee(t *testing.T) {
	http := initFakeHTTP()
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query PullRequestSearch\b`),
		httpmock.GraphQLQuery(`{}`, func(_ string, params map[string]interface{}) {
			assert.Equal(t, `assignee:hubot base:develop is:merged label:"needs tests" repo:OWNER/REPO type:pr`, params["q"].(string))
		}))

	_, err := runCommand(http, true, `-s merged -l "needs tests" -a hubot -B develop`)
	assert.Error(t, err)
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
			expectedQuery: `draft:true repo:OWNER/REPO state:open type:pr`,
		},
		{
			name:          "non-draft",
			cli:           "--draft=false",
			expectedQuery: `draft:false repo:OWNER/REPO state:open type:pr`,
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
			assert.Error(t, err)
		})
	}
}

func TestPRList_filteringAuthor(t *testing.T) {
	tests := []struct {
		name          string
		cli           string
		expectedQuery string
	}{
		{
			name:          "author @me",
			cli:           `--author "@me"`,
			expectedQuery: `author:@me repo:OWNER/REPO state:open type:pr`,
		},
		{
			name:          "author user",
			cli:           `--author "monalisa"`,
			expectedQuery: `author:monalisa repo:OWNER/REPO state:open type:pr`,
		},
		{
			name:          "app author",
			cli:           `--author "app/dependabot"`,
			expectedQuery: `author:app/dependabot repo:OWNER/REPO state:open type:pr`,
		},
		{
			name:          "app author with app option",
			cli:           `--app "dependabot"`,
			expectedQuery: `author:app/dependabot repo:OWNER/REPO state:open type:pr`,
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
			assert.Error(t, err)
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
			expectedBrowserURL: "https://github.com/OWNER/REPO/pulls?q=assignee%3Apeter+base%3Atrunk+is%3Amerged+label%3Abug+label%3Adocs+type%3Apr",
		},
		{
			name:               "draft",
			cli:                "--draft=true",
			expectedBrowserURL: "https://github.com/OWNER/REPO/pulls?q=draft%3Atrue+state%3Aopen+type%3Apr",
		},
		{
			name:               "non-draft",
			cli:                "--draft=0",
			expectedBrowserURL: "https://github.com/OWNER/REPO/pulls?q=draft%3Afalse+state%3Aopen+type%3Apr",
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
			assert.Equal(t, "Opening https://github.com/OWNER/REPO/pulls in your browser.\n", output.Stderr())
			assert.Equal(t, test.expectedBrowserURL, output.BrowsedURL)
		})
	}
}

func TestPRList_withProjectItems(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.GraphQL(`query PullRequestList\b`),
		httpmock.GraphQLQuery(`{
			"data": {
			  "repository": {
				"pullRequests": {
				  "totalCount": 1,
				  "nodes": [
					{
					  "projectItems": {
						"nodes": [
						  {
							"id": "PVTI_lAHOAA3WC84AW6WNzgJ8rnQ",
							"project": {
							  "id": "PVT_kwHOAA3WC84AW6WN",
							  "title": "Test Public Project"
							},
							"status": {
							  "optionId": "47fc9ee4",
							  "name": "In Progress"
							}
						  }
						],
						"totalCount": 1
					  }
					}
				  ]
				}
			  }
			}
		  }`, func(_ string, params map[string]interface{}) {
			require.Equal(t, map[string]interface{}{
				"owner": "OWNER",
				"repo":  "REPO",
				"limit": float64(30),
				"state": []interface{}{"OPEN"},
			}, params)
		}))

	client := &http.Client{Transport: reg}
	prsAndTotalCount, err := listPullRequests(
		client,
		ghrepo.New("OWNER", "REPO"),
		prShared.FilterOptions{
			Entity: "pr",
			State:  "open",
		},
		30,
	)

	require.NoError(t, err)
	require.Len(t, prsAndTotalCount.PullRequests, 1)
	require.Len(t, prsAndTotalCount.PullRequests[0].ProjectItems.Nodes, 1)

	require.Equal(t, prsAndTotalCount.PullRequests[0].ProjectItems.Nodes[0].ID, "PVTI_lAHOAA3WC84AW6WNzgJ8rnQ")

	expectedProject := api.ProjectV2ItemProject{
		ID:    "PVT_kwHOAA3WC84AW6WN",
		Title: "Test Public Project",
	}
	require.Equal(t, prsAndTotalCount.PullRequests[0].ProjectItems.Nodes[0].Project, expectedProject)

	expectedStatus := api.ProjectV2ItemStatus{
		OptionID: "47fc9ee4",
		Name:     "In Progress",
	}
	require.Equal(t, prsAndTotalCount.PullRequests[0].ProjectItems.Nodes[0].Status, expectedStatus)
}

func TestPRList_Search_withProjectItems(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.GraphQL(`query PullRequestSearch\b`),
		httpmock.GraphQLQuery(`{
			"data": {
			  "search": {
				"issueCount": 1,
				"nodes": [
				  {
					"projectItems": {
					  "nodes": [
						{
						  "id": "PVTI_lAHOAA3WC84AW6WNzgJ8rl0",
						  "project": {
							"id": "PVT_kwHOAA3WC84AW6WN",
							"title": "Test Public Project"
						  },
						  "status": {
							"optionId": "47fc9ee4",
							"name": "In Progress"
						  }
						}
					  ],
					  "totalCount": 1
					}
				  }
				]
			  }
			}
		  }`, func(_ string, params map[string]interface{}) {
			require.Equal(t, map[string]interface{}{
				"limit": float64(30),
				"q":     "just used to force the search API branch repo:OWNER/REPO state:open type:pr",
			}, params)
		}))

	client := &http.Client{Transport: reg}
	prsAndTotalCount, err := listPullRequests(
		client,
		ghrepo.New("OWNER", "REPO"),
		prShared.FilterOptions{
			Entity: "pr",
			State:  "open",
			Search: "just used to force the search API branch",
		},
		30,
	)

	require.NoError(t, err)
	require.Len(t, prsAndTotalCount.PullRequests, 1)
	require.Len(t, prsAndTotalCount.PullRequests[0].ProjectItems.Nodes, 1)

	require.Equal(t, prsAndTotalCount.PullRequests[0].ProjectItems.Nodes[0].ID, "PVTI_lAHOAA3WC84AW6WNzgJ8rl0")

	expectedProject := api.ProjectV2ItemProject{
		ID:    "PVT_kwHOAA3WC84AW6WN",
		Title: "Test Public Project",
	}
	require.Equal(t, prsAndTotalCount.PullRequests[0].ProjectItems.Nodes[0].Project, expectedProject)

	expectedStatus := api.ProjectV2ItemStatus{
		OptionID: "47fc9ee4",
		Name:     "In Progress",
	}
	require.Equal(t, prsAndTotalCount.PullRequests[0].ProjectItems.Nodes[0].Status, expectedStatus)
}
