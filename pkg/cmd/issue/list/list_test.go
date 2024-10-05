package list

import (
	"bytes"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
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

	factory := &cmdutil.Factory{
		IOStreams: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: rt}, nil
		},
		Config: func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
	}

	fakeNow := func() time.Time {
		return time.Date(2022, time.August, 25, 23, 50, 0, 0, time.UTC)
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
		OutBuf: stdout,
		ErrBuf: stderr,
	}, err
}

func TestIssueList_nontty(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query IssueList\b`),
		httpmock.FileResponse("./fixtures/issueList.json"))

	output, err := runCommand(http, false, "")
	if err != nil {
		t.Errorf("error running command `issue list`: %v", err)
	}

	assert.Equal(t, "", output.Stderr())
	//nolint:staticcheck // prefer exact matchers over ExpectLines
	test.ExpectLines(t, output.String(),
		`1[\t]+number won[\t]+label[\t]+\d+`,
		`2[\t]+number too[\t]+label[\t]+\d+`,
		`4[\t]+number fore[\t]+label[\t]+\d+`)
}

func TestIssueList_tty(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query IssueList\b`),
		httpmock.FileResponse("./fixtures/issueList.json"))

	output, err := runCommand(http, true, "")
	if err != nil {
		t.Errorf("error running command `issue list`: %v", err)
	}

	assert.Equal(t, heredoc.Doc(`

		Showing 3 of 3 open issues in OWNER/REPO

		ID  TITLE        LABELS  UPDATED
		#1  number won   label   about 1 day ago
		#2  number too   label   about 1 month ago
		#4  number fore  label   about 2 years ago
	`), output.String())
	assert.Equal(t, ``, output.Stderr())
}

func TestIssueList_tty_withFlags(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query IssueList\b`),
		httpmock.GraphQLQuery(`
		{ "data": {	"repository": {
			"hasIssuesEnabled": true,
			"issues": { "nodes": [] }
		} } }`, func(_ string, params map[string]interface{}) {
			assert.Equal(t, "probablyCher", params["assignee"].(string))
			assert.Equal(t, "foo", params["author"].(string))
			assert.Equal(t, "me", params["mention"].(string))
			assert.Equal(t, []interface{}{"OPEN"}, params["states"].([]interface{}))
		}))

	output, err := runCommand(http, true, "-a probablyCher -s open -A foo --mention me")
	assert.EqualError(t, err, "no issues match your search in OWNER/REPO")

	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
}

func TestIssueList_tty_withAppFlag(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query IssueList\b`),
		httpmock.GraphQLQuery(`
		{ "data": {	"repository": {
			"hasIssuesEnabled": true,
			"issues": { "nodes": [] }
		} } }`, func(_ string, params map[string]interface{}) {
			assert.Equal(t, "app/dependabot", params["author"].(string))
		}))

	output, err := runCommand(http, true, "--app dependabot")
	assert.EqualError(t, err, "no issues match your search in OWNER/REPO")

	assert.Equal(t, "", output.String())
	assert.Equal(t, "", output.Stderr())
}

func TestIssueList_withInvalidLimitFlag(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	_, err := runCommand(http, true, "--limit=0")

	if err == nil || err.Error() != "invalid limit: 0" {
		t.Errorf("error running command `issue list`: %v", err)
	}
}

func TestIssueList_disabledIssues(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query IssueList\b`),
		httpmock.StringResponse(`
			{ "data": {	"repository": {
				"hasIssuesEnabled": false
			} } }`),
	)

	_, err := runCommand(http, true, "")
	if err == nil || err.Error() != "the 'OWNER/REPO' repository has disabled issues" {
		t.Errorf("error running command `issue list`: %v", err)
	}
}

func TestIssueList_web(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(true)
	ios.SetStderrTTY(true)
	browser := &browser.Stub{}

	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	_, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	err := listRun(&ListOptions{
		IO:      ios,
		Browser: browser,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
		WebMode:      true,
		State:        "all",
		Assignee:     "peter",
		Author:       "john",
		Labels:       []string{"bug", "docs"},
		Mention:      "frank",
		Milestone:    "v1.1",
		LimitResults: 10,
	})
	if err != nil {
		t.Errorf("error running command `issue list` with `--web` flag: %v", err)
	}

	assert.Equal(t, "", stdout.String())
	assert.Equal(t, "Opening https://github.com/OWNER/REPO/issues in your browser.\n", stderr.String())
	browser.Verify(t, "https://github.com/OWNER/REPO/issues?q=assignee%3Apeter+author%3Ajohn+label%3Abug+label%3Adocs+mentions%3Afrank+milestone%3Av1.1+type%3Aissue")
}

func Test_issueList(t *testing.T) {
	type args struct {
		repo    ghrepo.Interface
		filters prShared.FilterOptions
		limit   int
	}
	tests := []struct {
		name      string
		args      args
		httpStubs func(*httpmock.Registry)
		wantErr   bool
	}{
		{
			name: "default",
			args: args{
				limit: 30,
				repo:  ghrepo.New("OWNER", "REPO"),
				filters: prShared.FilterOptions{
					Entity: "issue",
					State:  "open",
				},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query IssueList\b`),
					httpmock.GraphQLQuery(`
					{ "data": {	"repository": {
						"hasIssuesEnabled": true,
						"issues": { "nodes": [] }
					} } }`, func(_ string, params map[string]interface{}) {
						assert.Equal(t, map[string]interface{}{
							"owner":  "OWNER",
							"repo":   "REPO",
							"limit":  float64(30),
							"states": []interface{}{"OPEN"},
						}, params)
					}))
			},
		},
		{
			name: "milestone by number",
			args: args{
				limit: 30,
				repo:  ghrepo.New("OWNER", "REPO"),
				filters: prShared.FilterOptions{
					Entity:    "issue",
					State:     "open",
					Milestone: "13",
				},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query RepositoryMilestoneByNumber\b`),
					httpmock.StringResponse(`
					{ "data": { "repository": { "milestone": {
						"title": "1.x"
					} } } }
					`))
				reg.Register(
					httpmock.GraphQL(`query IssueSearch\b`),
					httpmock.GraphQLQuery(`
					{ "data": {
						"repository": { "hasIssuesEnabled": true },
						"search": {
							"issueCount": 0,
							"nodes": []
						}
					} }`, func(_ string, params map[string]interface{}) {
						assert.Equal(t, map[string]interface{}{
							"owner": "OWNER",
							"repo":  "REPO",
							"limit": float64(30),
							"query": "milestone:1.x repo:OWNER/REPO state:open type:issue",
							"type":  "ISSUE",
						}, params)
					}))
			},
		},
		{
			name: "milestone by title",
			args: args{
				limit: 30,
				repo:  ghrepo.New("OWNER", "REPO"),
				filters: prShared.FilterOptions{
					Entity:    "issue",
					State:     "open",
					Milestone: "1.x",
				},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query IssueSearch\b`),
					httpmock.GraphQLQuery(`
					{ "data": {
						"repository": { "hasIssuesEnabled": true },
						"search": {
							"issueCount": 0,
							"nodes": []
						}
					} }`, func(_ string, params map[string]interface{}) {
						assert.Equal(t, map[string]interface{}{
							"owner": "OWNER",
							"repo":  "REPO",
							"limit": float64(30),
							"query": "milestone:1.x repo:OWNER/REPO state:open type:issue",
							"type":  "ISSUE",
						}, params)
					}))
			},
		},
		{
			name: "@me syntax",
			args: args{
				limit: 30,
				repo:  ghrepo.New("OWNER", "REPO"),
				filters: prShared.FilterOptions{
					Entity:   "issue",
					State:    "open",
					Author:   "@me",
					Assignee: "@me",
					Mention:  "@me",
				},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data": {"viewer": {"login": "monalisa"} } }`))
				reg.Register(
					httpmock.GraphQL(`query IssueList\b`),
					httpmock.GraphQLQuery(`
					{ "data": {	"repository": {
						"hasIssuesEnabled": true,
						"issues": { "nodes": [] }
					} } }`, func(_ string, params map[string]interface{}) {
						assert.Equal(t, map[string]interface{}{
							"owner":    "OWNER",
							"repo":     "REPO",
							"limit":    float64(30),
							"states":   []interface{}{"OPEN"},
							"assignee": "monalisa",
							"author":   "monalisa",
							"mention":  "monalisa",
						}, params)
					}))
			},
		},
		{
			name: "@me with search",
			args: args{
				limit: 30,
				repo:  ghrepo.New("OWNER", "REPO"),
				filters: prShared.FilterOptions{
					Entity:   "issue",
					State:    "open",
					Author:   "@me",
					Assignee: "@me",
					Mention:  "@me",
					Search:   "auth bug",
				},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query IssueSearch\b`),
					httpmock.GraphQLQuery(`
					{ "data": {
						"repository": { "hasIssuesEnabled": true },
						"search": {
							"issueCount": 0,
							"nodes": []
						}
					} }`, func(_ string, params map[string]interface{}) {
						assert.Equal(t, map[string]interface{}{
							"owner": "OWNER",
							"repo":  "REPO",
							"limit": float64(30),
							"query": "auth bug assignee:@me author:@me mentions:@me repo:OWNER/REPO state:open type:issue",
							"type":  "ISSUE",
						}, params)
					}))
			},
		},
		{
			name: "with labels",
			args: args{
				limit: 30,
				repo:  ghrepo.New("OWNER", "REPO"),
				filters: prShared.FilterOptions{
					Entity: "issue",
					State:  "open",
					Labels: []string{"hello", "one world"},
				},
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.GraphQL(`query IssueSearch\b`),
					httpmock.GraphQLQuery(`
					{ "data": {
						"repository": { "hasIssuesEnabled": true },
						"search": {
							"issueCount": 0,
							"nodes": []
						}
					} }`, func(_ string, params map[string]interface{}) {
						assert.Equal(t, map[string]interface{}{
							"owner": "OWNER",
							"repo":  "REPO",
							"limit": float64(30),
							"query": `label:"one world" label:hello repo:OWNER/REPO state:open type:issue`,
							"type":  "ISSUE",
						}, params)
					}))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpreg := &httpmock.Registry{}
			defer httpreg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(httpreg)
			}
			client := &http.Client{Transport: httpreg}
			_, err := issueList(client, tt.args.repo, tt.args.filters, tt.args.limit)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIssueList_withProjectItems(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.GraphQL(`query IssueList\b`),
		httpmock.GraphQLQuery(`{
			"data": {
			  "repository": {
				"hasIssuesEnabled": true,
				"issues": {
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
				"owner":  "OWNER",
				"repo":   "REPO",
				"limit":  float64(30),
				"states": []interface{}{"OPEN"},
			}, params)
		}))

	client := &http.Client{Transport: reg}
	issuesAndTotalCount, err := issueList(
		client,
		ghrepo.New("OWNER", "REPO"),
		prShared.FilterOptions{
			Entity: "issue",
		},
		30,
	)

	require.NoError(t, err)
	require.Len(t, issuesAndTotalCount.Issues, 1)
	require.Len(t, issuesAndTotalCount.Issues[0].ProjectItems.Nodes, 1)

	require.Equal(t, issuesAndTotalCount.Issues[0].ProjectItems.Nodes[0].ID, "PVTI_lAHOAA3WC84AW6WNzgJ8rnQ")

	expectedProject := api.ProjectV2ItemProject{
		ID:    "PVT_kwHOAA3WC84AW6WN",
		Title: "Test Public Project",
	}
	require.Equal(t, issuesAndTotalCount.Issues[0].ProjectItems.Nodes[0].Project, expectedProject)

	expectedStatus := api.ProjectV2ItemStatus{
		OptionID: "47fc9ee4",
		Name:     "In Progress",
	}
	require.Equal(t, issuesAndTotalCount.Issues[0].ProjectItems.Nodes[0].Status, expectedStatus)
}

func TestIssueList_Search_withProjectItems(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.GraphQL(`query IssueSearch\b`),
		httpmock.GraphQLQuery(`{
			"data": {
			  "repository": {
				"hasIssuesEnabled": true
			  },
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
				"owner": "OWNER",
				"repo":  "REPO",
				"type":  "ISSUE",
				"limit": float64(30),
				"query": "just used to force the search API branch repo:OWNER/REPO type:issue",
			}, params)
		}))

	client := &http.Client{Transport: reg}
	issuesAndTotalCount, err := issueList(
		client,
		ghrepo.New("OWNER", "REPO"),
		prShared.FilterOptions{
			Entity: "issue",
			Search: "just used to force the search API branch",
		},
		30,
	)

	require.NoError(t, err)
	require.Len(t, issuesAndTotalCount.Issues, 1)
	require.Len(t, issuesAndTotalCount.Issues[0].ProjectItems.Nodes, 1)

	require.Equal(t, issuesAndTotalCount.Issues[0].ProjectItems.Nodes[0].ID, "PVTI_lAHOAA3WC84AW6WNzgJ8rl0")

	expectedProject := api.ProjectV2ItemProject{
		ID:    "PVT_kwHOAA3WC84AW6WN",
		Title: "Test Public Project",
	}
	require.Equal(t, issuesAndTotalCount.Issues[0].ProjectItems.Nodes[0].Project, expectedProject)

	expectedStatus := api.ProjectV2ItemStatus{
		OptionID: "47fc9ee4",
		Name:     "In Progress",
	}
	require.Equal(t, issuesAndTotalCount.Issues[0].ProjectItems.Nodes[0].Status, expectedStatus)
}
