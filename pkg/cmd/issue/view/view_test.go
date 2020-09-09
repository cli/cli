package view

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"os/exec"
	"reflect"
	"testing"

	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/internal/run"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/test"
	"github.com/google/shlex"
)

func eq(t *testing.T, got interface{}, expected interface{}) {
	t.Helper()
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("expected: %v, got: %v", expected, got)
	}
}

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
	http := &httpmock.Registry{}
	defer http.Verify(t)

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

	output, err := runCommand(http, true, "-w 123")
	if err != nil {
		t.Errorf("error running command `issue view`: %v", err)
	}

	eq(t, output.String(), "")
	eq(t, output.Stderr(), "Opening github.com/OWNER/REPO/issues/123 in your browser.\n")

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	eq(t, url, "https://github.com/OWNER/REPO/issues/123")
}

func TestIssueView_web_numberArgWithHash(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

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

	output, err := runCommand(http, true, "-w \"#123\"")
	if err != nil {
		t.Errorf("error running command `issue view`: %v", err)
	}

	eq(t, output.String(), "")
	eq(t, output.Stderr(), "Opening github.com/OWNER/REPO/issues/123 in your browser.\n")

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	eq(t, url, "https://github.com/OWNER/REPO/issues/123")
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

			eq(t, output.Stderr(), "")

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
				`ix of coins`,
				`Open.*marseilles opened about 292 years ago.*9 comments`,
				`bold story`,
				`View this issue on GitHub: https://github.com/OWNER/REPO/issues/123`,
			},
		},
		"Open issue with metadata": {
			fixture: "./fixtures/issueView_previewWithMetadata.json",
			expectedOutputs: []string{
				`ix of coins`,
				`Open.*marseilles opened about 292 years ago.*9 comments`,
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
				`ix of coins`,
				`Open.*marseilles opened about 292 years ago.*9 comments`,
				`View this issue on GitHub: https://github.com/OWNER/REPO/issues/123`,
			},
		},
		"Closed issue": {
			fixture: "./fixtures/issueView_previewClosedState.json",
			expectedOutputs: []string{
				`ix of coins`,
				`Closed.*marseilles opened about 292 years ago.*9 comments`,
				`bold story`,
				`View this issue on GitHub: https://github.com/OWNER/REPO/issues/123`,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			http := &httpmock.Registry{}
			defer http.Verify(t)

			http.Register(httpmock.GraphQL(`query IssueByNumber\b`), httpmock.FileResponse(tc.fixture))

			output, err := runCommand(http, true, "123")
			if err != nil {
				t.Errorf("error running `issue view`: %v", err)
			}

			eq(t, output.Stderr(), "")

			test.ExpectLines(t, output.String(), tc.expectedOutputs...)
		})
	}
}

func TestIssueView_web_notFound(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

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

	_, err := runCommand(http, true, "-w 9999")
	if err == nil || err.Error() != "GraphQL error: Could not resolve to an Issue with the number of 9999." {
		t.Errorf("error running command `issue view`: %v", err)
	}

	if seenCmd != nil {
		t.Fatal("did not expect any command to run")
	}
}

func TestIssueView_disabledIssues(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": {
			"id": "REPOID",
			"hasIssuesEnabled": false
		} } }
	`))

	_, err := runCommand(http, true, `6666`)
	if err == nil || err.Error() != "the 'OWNER/REPO' repository has disabled issues" {
		t.Errorf("error running command `issue view`: %v", err)
	}
}

func TestIssueView_web_urlArg(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

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

	output, err := runCommand(http, true, "-w https://github.com/OWNER/REPO/issues/123")
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
