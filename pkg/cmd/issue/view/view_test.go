package view

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/config"
	fd "github.com/cli/cli/v2/internal/featuredetection"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmd/issue/argparsetest"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/jsonfieldstest"
	"github.com/cli/cli/v2/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONFields(t *testing.T) {
	jsonfieldstest.ExpectCommandToSupportJSONFields(t, NewCmdView, []string{
		"assignees",
		"author",
		"body",
		"closed",
		"comments",
		"closedByPullRequestsReferences",
		"createdAt",
		"closedAt",
		"id",
		"labels",
		"milestone",
		"number",
		"projectCards",
		"projectItems",
		"reactionGroups",
		"state",
		"title",
		"updatedAt",
		"url",
		"isPinned",
		"stateReason",
		"issueType",
		"parent",
		"subIssues",
		"subIssuesSummary",
		"blockedBy",
		"blocking",
	})
}

func TestNewCmdView(t *testing.T) {
	// Test shared parsing of issue number / URL.
	argparsetest.TestArgParsing(t, NewCmdView)
}

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

	cmd := NewCmdView(factory, nil)

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

func TestIssueView_web(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(true)
	ios.SetStderrTTY(true)
	browser := &browser.Stub{}

	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.GraphQL(`query IssueByNumber\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": { "hasIssuesEnabled": true, "issue": {
				"number": 123,
				"url": "https://github.com/OWNER/REPO/issues/123"
			} } } }
		`))

	_, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	err := viewRun(&ViewOptions{
		IO:      ios,
		Browser: browser,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
		WebMode:     true,
		IssueNumber: 123,
	})
	if err != nil {
		t.Errorf("error running command `issue view`: %v", err)
	}

	assert.Equal(t, "", stdout.String())
	assert.Equal(t, "Opening https://github.com/OWNER/REPO/issues/123 in your browser.\n", stderr.String())
	browser.Verify(t, "https://github.com/OWNER/REPO/issues/123")
}

func TestIssueView_nontty_Preview(t *testing.T) {
	tests := map[string]struct {
		httpStubs       func(*httpmock.Registry)
		expectedOutputs []string
	}{
		"Open issue without metadata": {
			httpStubs: func(r *httpmock.Registry) {
				r.Register(httpmock.GraphQL(`query IssueByNumber\b`), httpmock.FileResponse("./fixtures/issueView_preview.json"))
				mockEmptyV2ProjectItems(t, r)
			},
			expectedOutputs: []string{
				`title:\tix of coins`,
				`state:\tOPEN`,
				`comments:\t9`,
				`author:\tmarseilles`,
				`assignees:`,
				`number:\t123\n`,
				`\*\*bold story\*\*`,
			},
		},
		"Open issue with metadata": {
			httpStubs: func(r *httpmock.Registry) {
				r.Register(httpmock.GraphQL(`query IssueByNumber\b`), httpmock.FileResponse("./fixtures/issueView_previewWithMetadata.json"))
				mockV2ProjectItems(t, r)
			},
			expectedOutputs: []string{
				`title:\tix of coins`,
				`assignees:\tmarseilles, monaco`,
				`author:\tmarseilles`,
				`state:\tOPEN`,
				`comments:\t9`,
				`labels:\tClosed: Duplicate, Closed: Won't Fix, help wanted, Status: In Progress, Type: Bug`,
				`projects:\tv2 Project 1 \(No Status\), v2 Project 2 \(Done\), Project 1 \(column A\), Project 2 \(column B\), Project 3 \(column C\), Project 4 \(Awaiting triage\)\n`,
				`milestone:\tuluru\n`,
				`number:\t123\n`,
				`\*\*bold story\*\*`,
			},
		},
		"Open issue with empty body": {
			httpStubs: func(r *httpmock.Registry) {
				r.Register(httpmock.GraphQL(`query IssueByNumber\b`), httpmock.FileResponse("./fixtures/issueView_previewWithEmptyBody.json"))
				mockEmptyV2ProjectItems(t, r)
			},
			expectedOutputs: []string{
				`title:\tix of coins`,
				`state:\tOPEN`,
				`author:\tmarseilles`,
				`labels:\ttarot`,
				`number:\t123\n`,
			},
		},
		"Closed issue": {
			httpStubs: func(r *httpmock.Registry) {
				r.Register(httpmock.GraphQL(`query IssueByNumber\b`), httpmock.FileResponse("./fixtures/issueView_previewClosedState.json"))
				mockEmptyV2ProjectItems(t, r)
			},
			expectedOutputs: []string{
				`title:\tix of coins`,
				`state:\tCLOSED`,
				`\*\*bold story\*\*`,
				`author:\tmarseilles`,
				`labels:\ttarot`,
				`number:\t123\n`,
				`\*\*bold story\*\*`,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			http := &httpmock.Registry{}
			defer http.Verify(t)
			if tc.httpStubs != nil {
				tc.httpStubs(http)
			}

			output, err := runCommand(http, false, "123")
			if err != nil {
				t.Errorf("error running `issue view`: %v", err)
			}

			assert.Equal(t, "", output.Stderr())

			//nolint:staticcheck // prefer exact matchers over ExpectLines
			test.ExpectLines(t, output.String(), tc.expectedOutputs...)
		})
	}
}

func TestIssueView_tty_Preview(t *testing.T) {
	tests := map[string]struct {
		httpStubs       func(*httpmock.Registry)
		expectedOutputs []string
	}{
		"Open issue without metadata": {
			httpStubs: func(r *httpmock.Registry) {
				r.Register(httpmock.GraphQL(`query IssueByNumber\b`), httpmock.FileResponse("./fixtures/issueView_preview.json"))
				mockEmptyV2ProjectItems(t, r)
			},
			expectedOutputs: []string{
				`ix of coins OWNER/REPO#123`,
				`Open.*marseilles opened about 9 years ago.*9 comments`,
				`bold story`,
				`View this issue on GitHub: https://github.com/OWNER/REPO/issues/123`,
			},
		},
		"Open issue with metadata": {
			httpStubs: func(r *httpmock.Registry) {
				r.Register(httpmock.GraphQL(`query IssueByNumber\b`), httpmock.FileResponse("./fixtures/issueView_previewWithMetadata.json"))
				mockV2ProjectItems(t, r)
			},
			expectedOutputs: []string{
				`ix of coins OWNER/REPO#123`,
				`Open.*marseilles opened about 9 years ago.*9 comments`,
				`8 \x{1f615} • 7 \x{1f440} • 6 \x{2764}\x{fe0f} • 5 \x{1f389} • 4 \x{1f604} • 3 \x{1f680} • 2 \x{1f44e} • 1 \x{1f44d}`,
				`Assignees:.*marseilles, monaco\n`,
				`Labels:.*Closed: Duplicate, Closed: Won't Fix, help wanted, Status: In Progress, Type: Bug\n`,
				`Projects:.*v2 Project 1 \(No Status\), v2 Project 2 \(Done\), Project 1 \(column A\), Project 2 \(column B\), Project 3 \(column C\), Project 4 \(Awaiting triage\)\n`,
				`Milestone:.*uluru\n`,
				`bold story`,
				`View this issue on GitHub: https://github.com/OWNER/REPO/issues/123`,
			},
		},
		"Open issue with empty body": {
			httpStubs: func(r *httpmock.Registry) {
				r.Register(httpmock.GraphQL(`query IssueByNumber\b`), httpmock.FileResponse("./fixtures/issueView_previewWithEmptyBody.json"))
				mockEmptyV2ProjectItems(t, r)
			},
			expectedOutputs: []string{
				`ix of coins OWNER/REPO#123`,
				`Open.*marseilles opened about 9 years ago.*9 comments`,
				`No description provided`,
				`View this issue on GitHub: https://github.com/OWNER/REPO/issues/123`,
			},
		},
		"Closed issue": {
			httpStubs: func(r *httpmock.Registry) {
				r.Register(httpmock.GraphQL(`query IssueByNumber\b`), httpmock.FileResponse("./fixtures/issueView_previewClosedState.json"))
				mockEmptyV2ProjectItems(t, r)
			},
			expectedOutputs: []string{
				`ix of coins OWNER/REPO#123`,
				`Closed.*marseilles opened about 9 years ago.*9 comments`,
				`bold story`,
				`View this issue on GitHub: https://github.com/OWNER/REPO/issues/123`,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)

			httpReg := &httpmock.Registry{}
			defer httpReg.Verify(t)
			if tc.httpStubs != nil {
				tc.httpStubs(httpReg)
			}

			opts := ViewOptions{
				IO: ios,
				Now: func() time.Time {
					t, _ := time.Parse(time.RFC822, "03 Nov 20 15:04 UTC")
					return t
				},
				HttpClient: func() (*http.Client, error) {
					return &http.Client{Transport: httpReg}, nil
				},
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
				IssueNumber: 123,
			}

			err := viewRun(&opts)
			assert.NoError(t, err)

			assert.Equal(t, "", stderr.String())

			//nolint:staticcheck // prefer exact matchers over ExpectLines
			test.ExpectLines(t, stdout.String(), tc.expectedOutputs...)
		})
	}
}

func TestIssueView_web_notFound(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query IssueByNumber\b`),
		httpmock.StringResponse(`
			{ "errors": [
				{ "message": "Could not resolve to an Issue with the number of 9999." }
			] }
			`),
	)

	_, cmdTeardown := run.Stub()
	defer cmdTeardown(t)

	_, err := runCommand(http, true, "-w 9999")
	if err == nil || err.Error() != "GraphQL: Could not resolve to an Issue with the number of 9999." {
		t.Errorf("error running command `issue view`: %v", err)
	}
}

func TestIssueView_disabledIssues(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query IssueByNumber\b`),
		httpmock.StringResponse(`
			{
				"data":
					{ "repository": {
						"id": "REPOID",
						"hasIssuesEnabled": false
					}
				},
				"errors": [
					{
						"type": "NOT_FOUND",
						"path": [
							"repository",
							"issue"
						],
						"message": "Could not resolve to an issue or pull request with the number of 6666."
					}
				]
			}
		`),
	)

	_, err := runCommand(http, true, `6666`)
	if err == nil || err.Error() != "the 'OWNER/REPO' repository has disabled issues" {
		t.Errorf("error running command `issue view`: %v", err)
	}
}

func TestIssueView_tty_Comments(t *testing.T) {
	tests := map[string]struct {
		cli             string
		httpStubs       func(*httpmock.Registry)
		expectedOutputs []string
		wantsErr        bool
	}{
		"without comments flag": {
			cli: "123",
			httpStubs: func(r *httpmock.Registry) {
				r.Register(httpmock.GraphQL(`query IssueByNumber\b`), httpmock.FileResponse("./fixtures/issueView_previewSingleComment.json"))
				mockEmptyV2ProjectItems(t, r)
			},
			expectedOutputs: []string{
				`some title OWNER/REPO#123`,
				`some body`,
				`———————— Not showing 5 comments ————————`,
				`marseilles \(Collaborator\) • Jan  1, 2020 • Newest comment`,
				`Comment 5`,
				`Use --comments to view the full conversation`,
				`View this issue on GitHub: https://github.com/OWNER/REPO/issues/123`,
			},
		},
		"with comments flag": {
			cli: "123 --comments",
			httpStubs: func(r *httpmock.Registry) {
				r.Register(httpmock.GraphQL(`query IssueByNumber\b`), httpmock.FileResponse("./fixtures/issueView_previewSingleComment.json"))
				r.Register(httpmock.GraphQL(`query CommentsForIssue\b`), httpmock.FileResponse("./fixtures/issueView_previewFullComments.json"))
				mockEmptyV2ProjectItems(t, r)
			},
			expectedOutputs: []string{
				`some title OWNER/REPO#123`,
				`some body`,
				`monalisa • Jan  1, 2020 • Edited`,
				`1 \x{1f615} • 2 \x{1f440} • 3 \x{2764}\x{fe0f} • 4 \x{1f389} • 5 \x{1f604} • 6 \x{1f680} • 7 \x{1f44e} • 8 \x{1f44d}`,
				`Comment 1`,
				`johnnytest \(Contributor\) • Jan  1, 2020`,
				`Comment 2`,
				`elvisp \(Member\) • Jan  1, 2020`,
				`Comment 3`,
				`loislane \(Owner\) • Jan  1, 2020`,
				`Comment 4`,
				`sam-spam • This comment has been marked as spam`,
				`marseilles \(Collaborator\) • Jan  1, 2020 • Newest comment`,
				`Comment 5`,
				`View this issue on GitHub: https://github.com/OWNER/REPO/issues/123`,
			},
		},
		"with invalid comments flag": {
			cli:      "123 --comments 3",
			wantsErr: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			http := &httpmock.Registry{}
			defer http.Verify(t)
			if tc.httpStubs != nil {
				tc.httpStubs(http)
			}
			output, err := runCommand(http, true, tc.cli)
			if tc.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, "", output.Stderr())
			//nolint:staticcheck // prefer exact matchers over ExpectLines
			test.ExpectLines(t, output.String(), tc.expectedOutputs...)
		})
	}
}

func TestIssueView_nontty_Comments(t *testing.T) {
	tests := map[string]struct {
		cli             string
		httpStubs       func(*httpmock.Registry)
		expectedOutputs []string
		wantsErr        bool
	}{
		"without comments flag": {
			cli: "123",
			httpStubs: func(r *httpmock.Registry) {
				r.Register(httpmock.GraphQL(`query IssueByNumber\b`), httpmock.FileResponse("./fixtures/issueView_previewSingleComment.json"))
				mockEmptyV2ProjectItems(t, r)
			},
			expectedOutputs: []string{
				`title:\tsome title`,
				`state:\tOPEN`,
				`author:\tmarseilles`,
				`comments:\t6`,
				`number:\t123`,
				`some body`,
			},
		},
		"with comments flag": {
			cli: "123 --comments",
			httpStubs: func(r *httpmock.Registry) {
				r.Register(httpmock.GraphQL(`query IssueByNumber\b`), httpmock.FileResponse("./fixtures/issueView_previewSingleComment.json"))
				r.Register(httpmock.GraphQL(`query CommentsForIssue\b`), httpmock.FileResponse("./fixtures/issueView_previewFullComments.json"))
				mockEmptyV2ProjectItems(t, r)
			},
			expectedOutputs: []string{
				`author:\tmonalisa`,
				`association:\t`,
				`edited:\ttrue`,
				`Comment 1`,
				`author:\tjohnnytest`,
				`association:\tcontributor`,
				`edited:\tfalse`,
				`Comment 2`,
				`author:\telvisp`,
				`association:\tmember`,
				`edited:\tfalse`,
				`Comment 3`,
				`author:\tloislane`,
				`association:\towner`,
				`edited:\tfalse`,
				`Comment 4`,
				`author:\tmarseilles`,
				`association:\tcollaborator`,
				`edited:\tfalse`,
				`Comment 5`,
			},
		},
		"with invalid comments flag": {
			cli:      "123 --comments 3",
			wantsErr: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			http := &httpmock.Registry{}
			defer http.Verify(t)
			if tc.httpStubs != nil {
				tc.httpStubs(http)
			}
			output, err := runCommand(http, false, tc.cli)
			if tc.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, "", output.Stderr())
			//nolint:staticcheck // prefer exact matchers over ExpectLines
			test.ExpectLines(t, output.String(), tc.expectedOutputs...)
		})
	}
}

// TODO projectsV1Deprecation
// Remove this test.
func TestProjectsV1Deprecation(t *testing.T) {
	t.Run("when projects v1 is supported, is included in query", func(t *testing.T) {
		ios, _, _, _ := iostreams.Test()

		reg := &httpmock.Registry{}
		reg.Register(
			httpmock.GraphQL(`projectCards`),
			// Simulate a GraphQL error to early exit the test.
			httpmock.StatusStringResponse(500, ""),
		)

		_, cmdTeardown := run.Stub()
		defer cmdTeardown(t)

		// Ignore the error because we have no way to really stub it without
		// fully stubbing a GQL error structure in the request body.
		_ = viewRun(&ViewOptions{
			IO: ios,
			HttpClient: func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			},
			BaseRepo: func() (ghrepo.Interface, error) {
				return ghrepo.New("OWNER", "REPO"), nil
			},

			Detector:    &fd.EnabledDetectorMock{},
			IssueNumber: 123,
		})

		// Verify that our request contained projectCards
		reg.Verify(t)
	})

	t.Run("when projects v1 is not supported, is not included in query", func(t *testing.T) {
		ios, _, _, _ := iostreams.Test()

		reg := &httpmock.Registry{}
		reg.Exclude(t, httpmock.GraphQL(`projectCards`))

		_, cmdTeardown := run.Stub()
		defer cmdTeardown(t)

		// Ignore the error because we're not really interested in it.
		_ = viewRun(&ViewOptions{
			IO: ios,
			HttpClient: func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			},
			BaseRepo: func() (ghrepo.Interface, error) {
				return ghrepo.New("OWNER", "REPO"), nil
			},

			Detector:    &fd.DisabledDetectorMock{},
			IssueNumber: 123,
		})

		// Verify that our request contained projectCards
		reg.Verify(t)
	})
}

// mockEmptyV2ProjectItems registers GraphQL queries to report an issue is not contained on any v2 projects.
func mockEmptyV2ProjectItems(t *testing.T, r *httpmock.Registry) {
	r.Register(httpmock.GraphQL(`query IssueProjectItems\b`), httpmock.StringResponse(`
		{ "data": { "repository": { "issue": {
			"projectItems": {
				"totalCount": 0,
				"nodes": []
		} } } } }
	`))
}

// mockV2ProjectItems registers GraphQL queries to report an issue on multiple v2 projects in various states
// - `NO_STATUS_ITEM`: emulates this issue is on a project but is not given a status
// - `DONE_STATUS_ITEM`: emulates this issue is on a project and considered done
func mockV2ProjectItems(t *testing.T, r *httpmock.Registry) {
	r.Register(httpmock.GraphQL(`query IssueProjectItems\b`), httpmock.StringResponse(`
		{ "data": { "repository": { "issue": {
			"projectItems": {
				"totalCount": 2,
				"nodes": [
					{
						"id": "NO_STATUS_ITEM",
						"project": {
							"id": "PROJECT1",
							"title": "v2 Project 1"
						},
						"status": {
							"optionId": "",
							"name": ""
						}
					},
					{
						"id": "DONE_STATUS_ITEM",
						"project": {
							"id": "PROJECT2",
							"title": "v2 Project 2"
						},
						"status": {
							"optionId": "PROJECTITEMFIELD1",
							"name": "Done"
						}
					}
				]
			} } } } }
	`))
}

// issueResponseAllIssues2Fields returns a GraphQL response for an issue with all Issues 2.0 fields populated.
func issueResponseAllIssues2Fields() string {
	return `{ "data": { "repository": { "hasIssuesEnabled": true, "issue": {
		"id": "ISSUE_123",
		"number": 123,
		"title": "Implement OAuth flow",
		"state": "OPEN",
		"stateReason": "",
		"body": "The OAuth flow needs work.",
		"author": {"login": "user1"},
		"createdAt": "2024-01-01T00:00:00Z",
		"comments": {"nodes":[], "totalCount": 0},
		"assignees": {"nodes": [], "totalCount": 0},
		"labels": {"nodes": [], "totalCount": 0},
		"milestone": null,
		"reactionGroups": [],
		"projectCards": {"nodes": [], "totalCount": 0},
		"projectItems": {"nodes": [], "totalCount": 0},
		"url": "https://github.com/OWNER/REPO/issues/123",
		"issueType": {"id":"IT_1","name":"Bug","description":"Something is not working","color":"d73a4a"},
		"parent": {"number":100,"title":"Epic: Authentication overhaul","url":"https://github.com/OWNER/REPO/issues/100","state":"OPEN","repository":{"nameWithOwner":"OWNER/REPO"}},
		"subIssues": {
			"nodes": [
				{"number":101,"title":"Design auth module","url":"https://github.com/OWNER/REPO/issues/101","state":"CLOSED","repository":{"nameWithOwner":"OWNER/REPO"}},
				{"number":102,"title":"Token refresh logic","url":"https://github.com/OWNER/REPO/issues/102","state":"OPEN","repository":{"nameWithOwner":"OWNER/REPO"}}
			],
			"totalCount": 2
		},
		"subIssuesSummary": {"total":2,"completed":1,"percentCompleted":50.0},
		"blockedBy": {
			"nodes": [{"number":200,"title":"API rate limiting","url":"https://github.com/OWNER/REPO/issues/200","state":"OPEN","repository":{"nameWithOwner":"OWNER/REPO"}}],
			"totalCount": 1
		},
		"blocking": {
			"nodes": [{"number":300,"title":"Release v2.0","url":"https://github.com/OWNER/REPO/issues/300","state":"OPEN","repository":{"nameWithOwner":"OWNER/REPO"}}],
			"totalCount": 1
		}
	} } } }`
}

// issueResponseNoIssues2Fields returns a GraphQL response for an issue with no Issues 2.0 fields.
func issueResponseNoIssues2Fields() string {
	return `{ "data": { "repository": { "hasIssuesEnabled": true, "issue": {
		"id": "ISSUE_456",
		"number": 456,
		"title": "Fix login page",
		"state": "OPEN",
		"stateReason": "",
		"body": "The login page is broken.",
		"author": {"login": "user2"},
		"createdAt": "2024-01-01T00:00:00Z",
		"comments": {"nodes":[], "totalCount": 2},
		"assignees": {"nodes": [], "totalCount": 0},
		"labels": {"nodes": [], "totalCount": 0},
		"milestone": null,
		"reactionGroups": [],
		"projectCards": {"nodes": [], "totalCount": 0},
		"projectItems": {"nodes": [], "totalCount": 0},
		"url": "https://github.com/OWNER/REPO/issues/456"
	} } } }`
}

func TestIssueView_tty_Issues2AllFields(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(true)
	ios.SetStdinTTY(true)
	ios.SetStderrTTY(true)

	httpReg := &httpmock.Registry{}
	defer httpReg.Verify(t)

	httpReg.Register(
		httpmock.GraphQL(`query IssueByNumber\b`),
		httpmock.StringResponse(issueResponseAllIssues2Fields()),
	)
	mockEmptyV2ProjectItems(t, httpReg)

	opts := ViewOptions{
		IO: ios,
		Now: func() time.Time {
			t, _ := time.Parse(time.RFC822, "03 Nov 24 15:04 UTC")
			return t
		},
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: httpReg}, nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
		IssueNumber: 123,
	}

	err := viewRun(&opts)
	require.NoError(t, err)

	assert.Equal(t, "", stderr.String())

	out := stdout.String()

	// Title
	assert.Contains(t, out, "Implement OAuth flow")
	assert.Contains(t, out, "OWNER/REPO#123")

	// State line includes issue type prefix
	assert.Contains(t, out, "Bug · Open")

	// Type metadata row
	assert.Contains(t, out, "Type:")
	assert.Contains(t, out, "Bug")

	// Parent metadata row
	assert.Contains(t, out, "Parent:")
	assert.Contains(t, out, "OWNER/REPO#100 Epic: Authentication overhaul")

	// Blocked by metadata row
	assert.Contains(t, out, "Blocked by:")
	assert.Contains(t, out, "OWNER/REPO#200 API rate limiting")

	// Blocking metadata row
	assert.Contains(t, out, "Blocking:")
	assert.Contains(t, out, "OWNER/REPO#300 Release v2.0")

	// Sub-issues section
	assert.Contains(t, out, "Sub-issues")
	assert.Contains(t, out, "1/2 (50%)")
	assert.Contains(t, out, "OWNER/REPO#101")
	assert.Contains(t, out, "Design auth module")
	assert.Contains(t, out, "OWNER/REPO#102")
	assert.Contains(t, out, "Token refresh logic")

	// Body
	assert.Contains(t, out, "The OAuth flow needs work.")

	// Footer
	assert.Contains(t, out, "View this issue on GitHub: https://github.com/OWNER/REPO/issues/123")
}

func TestIssueView_nontty_Issues2AllFields(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()

	httpReg := &httpmock.Registry{}
	defer httpReg.Verify(t)

	httpReg.Register(
		httpmock.GraphQL(`query IssueByNumber\b`),
		httpmock.StringResponse(issueResponseAllIssues2Fields()),
	)
	mockEmptyV2ProjectItems(t, httpReg)

	opts := ViewOptions{
		IO: ios,
		Now: func() time.Time {
			t, _ := time.Parse(time.RFC822, "03 Nov 24 15:04 UTC")
			return t
		},
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: httpReg}, nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
		IssueNumber: 123,
	}

	err := viewRun(&opts)
	require.NoError(t, err)

	assert.Equal(t, "", stderr.String())

	out := stdout.String()

	assert.Contains(t, out, "issue-type:\tBug\n")
	assert.Contains(t, out, "parent:\tOWNER/REPO#100\n")
	assert.Contains(t, out, "sub-issues:\tOWNER/REPO#101, OWNER/REPO#102\n")
	assert.Contains(t, out, "sub-issues-completed:\t1/2\n")
	assert.Contains(t, out, "blocked-by:\tOWNER/REPO#200\n")
	assert.Contains(t, out, "blocking:\tOWNER/REPO#300\n")
}

func TestIssueView_tty_Issues2NoFields(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(true)
	ios.SetStdinTTY(true)
	ios.SetStderrTTY(true)

	httpReg := &httpmock.Registry{}
	defer httpReg.Verify(t)

	httpReg.Register(
		httpmock.GraphQL(`query IssueByNumber\b`),
		httpmock.StringResponse(issueResponseNoIssues2Fields()),
	)
	mockEmptyV2ProjectItems(t, httpReg)

	opts := ViewOptions{
		IO: ios,
		Now: func() time.Time {
			t, _ := time.Parse(time.RFC822, "03 Nov 24 15:04 UTC")
			return t
		},
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: httpReg}, nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
		IssueNumber: 456,
	}

	err := viewRun(&opts)
	require.NoError(t, err)

	assert.Equal(t, "", stderr.String())

	out := stdout.String()

	// Standard fields are still present
	assert.Contains(t, out, "Fix login page")
	assert.Contains(t, out, "OWNER/REPO#456")
	assert.Contains(t, out, "Open")
	assert.Contains(t, out, "The login page is broken.")
	assert.Contains(t, out, "View this issue on GitHub: https://github.com/OWNER/REPO/issues/456")

	// Issues 2.0 sections must NOT appear
	assert.NotContains(t, out, "Type:")
	assert.NotContains(t, out, "Parent:")
	assert.NotContains(t, out, "Blocked by:")
	assert.NotContains(t, out, "Blocking:")
	assert.NotContains(t, out, "Sub-issues")
}

func TestIssueView_nontty_Issues2NoFields(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()

	httpReg := &httpmock.Registry{}
	defer httpReg.Verify(t)

	httpReg.Register(
		httpmock.GraphQL(`query IssueByNumber\b`),
		httpmock.StringResponse(issueResponseNoIssues2Fields()),
	)
	mockEmptyV2ProjectItems(t, httpReg)

	opts := ViewOptions{
		IO: ios,
		Now: func() time.Time {
			t, _ := time.Parse(time.RFC822, "03 Nov 24 15:04 UTC")
			return t
		},
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: httpReg}, nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
		IssueNumber: 456,
	}

	err := viewRun(&opts)
	require.NoError(t, err)

	assert.Equal(t, "", stderr.String())

	out := stdout.String()

	// Issues 2.0 keys appear with empty values to keep line counts stable
	// for `head | grep` workflows.
	assert.Contains(t, out, "issue-type:\t\n")
	assert.Contains(t, out, "parent:\t\n")
	assert.Contains(t, out, "sub-issues:\t\n")
	assert.Contains(t, out, "sub-issues-completed:\t\n")
	assert.Contains(t, out, "blocked-by:\t\n")
	assert.Contains(t, out, "blocking:\t\n")
}

func TestIssueView_json_IssueType(t *testing.T) {
	httpReg := &httpmock.Registry{}
	defer httpReg.Verify(t)

	httpReg.Register(
		httpmock.GraphQL(`query IssueByNumber\b`),
		httpmock.StringResponse(issueResponseAllIssues2Fields()),
	)

	output, err := runCommand(httpReg, false, `123 --json issueType`)
	require.NoError(t, err)

	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(output.OutBuf.Bytes(), &data))

	issueType, ok := data["issueType"].(map[string]interface{})
	require.True(t, ok, "issueType should be an object")
	assert.Equal(t, "IT_1", issueType["id"])
	assert.Equal(t, "Bug", issueType["name"])
	assert.Equal(t, "Something is not working", issueType["description"])
	assert.Equal(t, "d73a4a", issueType["color"])
}

func TestIssueView_json_ParentSubIssues(t *testing.T) {
	httpReg := &httpmock.Registry{}
	defer httpReg.Verify(t)

	httpReg.Register(
		httpmock.GraphQL(`query IssueByNumber\b`),
		httpmock.StringResponse(issueResponseAllIssues2Fields()),
	)

	output, err := runCommand(httpReg, false, `123 --json parent,subIssues,subIssuesSummary`)
	require.NoError(t, err)

	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(output.OutBuf.Bytes(), &data))

	// Parent
	parent, ok := data["parent"].(map[string]interface{})
	require.True(t, ok, "parent should be an object")
	assert.Equal(t, float64(100), parent["number"])
	assert.Equal(t, "Epic: Authentication overhaul", parent["title"])
	assert.Equal(t, "https://github.com/OWNER/REPO/issues/100", parent["url"])
	assert.Equal(t, "OPEN", parent["state"])

	// Sub-issues
	subIssuesObj, ok := data["subIssues"].(map[string]interface{})
	require.True(t, ok, "subIssues should be an object")
	assert.Equal(t, float64(2), subIssuesObj["totalCount"])

	subIssues, ok := subIssuesObj["nodes"].([]interface{})
	require.True(t, ok, "subIssues.nodes should be an array")
	require.Len(t, subIssues, 2)

	sub0 := subIssues[0].(map[string]interface{})
	assert.Equal(t, float64(101), sub0["number"])
	assert.Equal(t, "Design auth module", sub0["title"])
	assert.Equal(t, "CLOSED", sub0["state"])

	sub1 := subIssues[1].(map[string]interface{})
	assert.Equal(t, float64(102), sub1["number"])
	assert.Equal(t, "Token refresh logic", sub1["title"])
	assert.Equal(t, "OPEN", sub1["state"])

	// Sub-issues summary
	summary, ok := data["subIssuesSummary"].(map[string]interface{})
	require.True(t, ok, "subIssuesSummary should be an object")
	assert.Equal(t, float64(2), summary["total"])
	assert.Equal(t, float64(1), summary["completed"])
	assert.Equal(t, float64(50), summary["percentCompleted"])
}

func TestIssueView_json_BlockedByBlocking(t *testing.T) {
	httpReg := &httpmock.Registry{}
	defer httpReg.Verify(t)

	httpReg.Register(
		httpmock.GraphQL(`query IssueByNumber\b`),
		httpmock.StringResponse(issueResponseAllIssues2Fields()),
	)

	output, err := runCommand(httpReg, false, `123 --json blockedBy,blocking`)
	require.NoError(t, err)

	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(output.OutBuf.Bytes(), &data))

	// Blocked by
	blockedByObj, ok := data["blockedBy"].(map[string]interface{})
	require.True(t, ok, "blockedBy should be an object")
	assert.Equal(t, float64(1), blockedByObj["totalCount"])

	blockedBy, ok := blockedByObj["nodes"].([]interface{})
	require.True(t, ok, "blockedBy.nodes should be an array")
	require.Len(t, blockedBy, 1)

	blocked0 := blockedBy[0].(map[string]interface{})
	assert.Equal(t, float64(200), blocked0["number"])
	assert.Equal(t, "API rate limiting", blocked0["title"])
	assert.Equal(t, "https://github.com/OWNER/REPO/issues/200", blocked0["url"])
	assert.Equal(t, "OPEN", blocked0["state"])

	// Blocking
	blockingObj, ok := data["blocking"].(map[string]interface{})
	require.True(t, ok, "blocking should be an object")
	assert.Equal(t, float64(1), blockingObj["totalCount"])

	blocking, ok := blockingObj["nodes"].([]interface{})
	require.True(t, ok, "blocking.nodes should be an array")
	require.Len(t, blocking, 1)

	blocking0 := blocking[0].(map[string]interface{})
	assert.Equal(t, float64(300), blocking0["number"])
	assert.Equal(t, "Release v2.0", blocking0["title"])
	assert.Equal(t, "https://github.com/OWNER/REPO/issues/300", blocking0["url"])
	assert.Equal(t, "OPEN", blocking0["state"])
}
