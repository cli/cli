package view

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/config"
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

	factory := &cmdutil.Factory{
		IOStreams: io,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: rt}, nil
		},
		Config: func() (config.Config, error) {
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
	cmd.SetOut(ioutil.Discard)
	cmd.SetErr(ioutil.Discard)

	_, err = cmd.ExecuteC()
	return &test.CmdOut{
		OutBuf: stdout,
		ErrBuf: stderr,
	}, err
}

func TestIssueView_web(t *testing.T) {
	io, _, stdout, stderr := iostreams.Test()
	io.SetStdoutTTY(true)
	io.SetStderrTTY(true)
	browser := &cmdutil.TestBrowser{}

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
		IO:      io,
		Browser: browser,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
		WebMode:     true,
		SelectorArg: "123",
	})
	if err != nil {
		t.Errorf("error running command `issue view`: %v", err)
	}

	assert.Equal(t, "", stdout.String())
	assert.Equal(t, "Opening github.com/OWNER/REPO/issues/123 in your browser.\n", stderr.String())
	browser.Verify(t, "https://github.com/OWNER/REPO/issues/123")
}

func TestIssueView_nontty_Preview(t *testing.T) {
	tests := map[string]struct {
		fixture         string
		expectedOutputs []string
	}{
		"Open issue without metadata": {
			fixture: "./fixtures/issueView_preview.json",
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
			fixture: "./fixtures/issueView_previewWithMetadata.json",
			expectedOutputs: []string{
				`title:\tix of coins`,
				`assignees:\tmarseilles, monaco`,
				`author:\tmarseilles`,
				`state:\tOPEN`,
				`comments:\t9`,
				`labels:\tone, two, three, four, five`,
				`projects:\tProject 1 \(column A\), Project 2 \(column B\), Project 3 \(column C\), Project 4 \(Awaiting triage\)\n`,
				`milestone:\tuluru\n`,
				`number:\t123\n`,
				`\*\*bold story\*\*`,
			},
		},
		"Open issue with empty body": {
			fixture: "./fixtures/issueView_previewWithEmptyBody.json",
			expectedOutputs: []string{
				`title:\tix of coins`,
				`state:\tOPEN`,
				`author:\tmarseilles`,
				`labels:\ttarot`,
				`number:\t123\n`,
			},
		},
		"Closed issue": {
			fixture: "./fixtures/issueView_previewClosedState.json",
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

			http.Register(httpmock.GraphQL(`query IssueByNumber\b`), httpmock.FileResponse(tc.fixture))

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
		fixture         string
		expectedOutputs []string
	}{
		"Open issue without metadata": {
			fixture: "./fixtures/issueView_preview.json",
			expectedOutputs: []string{
				`ix of coins #123`,
				`Open.*marseilles opened about 9 years ago.*9 comments`,
				`bold story`,
				`View this issue on GitHub: https://github.com/OWNER/REPO/issues/123`,
			},
		},
		"Open issue with metadata": {
			fixture: "./fixtures/issueView_previewWithMetadata.json",
			expectedOutputs: []string{
				`ix of coins #123`,
				`Open.*marseilles opened about 9 years ago.*9 comments`,
				`8 \x{1f615} • 7 \x{1f440} • 6 \x{2764}\x{fe0f} • 5 \x{1f389} • 4 \x{1f604} • 3 \x{1f680} • 2 \x{1f44e} • 1 \x{1f44d}`,
				`Assignees:.*marseilles, monaco\n`,
				`Labels:.*one, two, three, four, five\n`,
				`Projects:.*Project 1 \(column A\), Project 2 \(column B\), Project 3 \(column C\), Project 4 \(Awaiting triage\)\n`,
				`Milestone:.*uluru\n`,
				`bold story`,
				`View this issue on GitHub: https://github.com/OWNER/REPO/issues/123`,
			},
		},
		"Open issue with empty body": {
			fixture: "./fixtures/issueView_previewWithEmptyBody.json",
			expectedOutputs: []string{
				`ix of coins #123`,
				`Open.*marseilles opened about 9 years ago.*9 comments`,
				`No description provided`,
				`View this issue on GitHub: https://github.com/OWNER/REPO/issues/123`,
			},
		},
		"Closed issue": {
			fixture: "./fixtures/issueView_previewClosedState.json",
			expectedOutputs: []string{
				`ix of coins #123`,
				`Closed.*marseilles opened about 9 years ago.*9 comments`,
				`bold story`,
				`View this issue on GitHub: https://github.com/OWNER/REPO/issues/123`,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			io, _, stdout, stderr := iostreams.Test()
			io.SetStdoutTTY(true)
			io.SetStdinTTY(true)
			io.SetStderrTTY(true)

			httpReg := &httpmock.Registry{}
			defer httpReg.Verify(t)

			httpReg.Register(httpmock.GraphQL(`query IssueByNumber\b`), httpmock.FileResponse(tc.fixture))

			opts := ViewOptions{
				IO: io,
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
				SelectorArg: "123",
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
		fixtures        map[string]string
		expectedOutputs []string
		wantsErr        bool
	}{
		"without comments flag": {
			cli: "123",
			fixtures: map[string]string{
				"IssueByNumber": "./fixtures/issueView_previewSingleComment.json",
			},
			expectedOutputs: []string{
				`some title #123`,
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
			fixtures: map[string]string{
				"IssueByNumber":    "./fixtures/issueView_previewSingleComment.json",
				"CommentsForIssue": "./fixtures/issueView_previewFullComments.json",
			},
			expectedOutputs: []string{
				`some title #123`,
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
			for name, file := range tc.fixtures {
				name := fmt.Sprintf(`query %s\b`, name)
				http.Register(httpmock.GraphQL(name), httpmock.FileResponse(file))
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
		fixtures        map[string]string
		expectedOutputs []string
		wantsErr        bool
	}{
		"without comments flag": {
			cli: "123",
			fixtures: map[string]string{
				"IssueByNumber": "./fixtures/issueView_previewSingleComment.json",
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
			fixtures: map[string]string{
				"IssueByNumber":    "./fixtures/issueView_previewSingleComment.json",
				"CommentsForIssue": "./fixtures/issueView_previewFullComments.json",
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
			for name, file := range tc.fixtures {
				name := fmt.Sprintf(`query %s\b`, name)
				http.Register(httpmock.GraphQL(name), httpmock.FileResponse(file))
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
