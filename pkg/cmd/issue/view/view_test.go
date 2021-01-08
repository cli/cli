package view

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"testing"

	"github.com/briandowns/spinner"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/internal/run"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/test"
	"github.com/cli/cli/utils"
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

	assert.Equal(t, "", output.String())
	assert.Equal(t, "Opening github.com/OWNER/REPO/issues/123 in your browser.\n", output.Stderr())

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	assert.Equal(t, "https://github.com/OWNER/REPO/issues/123", url)
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

	assert.Equal(t, "", output.String())
	assert.Equal(t, "Opening github.com/OWNER/REPO/issues/123 in your browser.\n", output.Stderr())

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	assert.Equal(t, "https://github.com/OWNER/REPO/issues/123", url)
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

			assert.Equal(t, "", output.Stderr())

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
				`Open.*marseilles opened about 9 years ago.*9 comments`,
				`bold story`,
				`View this issue on GitHub: https://github.com/OWNER/REPO/issues/123`,
			},
		},
		"Open issue with metadata": {
			fixture: "./fixtures/issueView_previewWithMetadata.json",
			expectedOutputs: []string{
				`ix of coins`,
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
				`ix of coins`,
				`Open.*marseilles opened about 9 years ago.*9 comments`,
				`No description provided`,
				`View this issue on GitHub: https://github.com/OWNER/REPO/issues/123`,
			},
		},
		"Closed issue": {
			fixture: "./fixtures/issueView_previewClosedState.json",
			expectedOutputs: []string{
				`ix of coins`,
				`Closed.*marseilles opened about 9 years ago.*9 comments`,
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

			assert.Equal(t, "", output.Stderr())

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

	assert.Equal(t, "", output.String())

	if seenCmd == nil {
		t.Fatal("expected a command to run")
	}
	url := seenCmd.Args[len(seenCmd.Args)-1]
	assert.Equal(t, "https://github.com/OWNER/REPO/issues/123", url)
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
				`some title`,
				`some body`,
				`———————— Not showing 4 comments ————————`,
				`marseilles \(collaborator\) • Jan  1, 2020 • Newest comment`,
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
				`some title`,
				`some body`,
				`monalisa • Jan  1, 2020 • edited`,
				`1 \x{1f615} • 2 \x{1f440} • 3 \x{2764}\x{fe0f} • 4 \x{1f389} • 5 \x{1f604} • 6 \x{1f680} • 7 \x{1f44e} • 8 \x{1f44d}`,
				`Comment 1`,
				`johnnytest \(contributor\) • Jan  1, 2020`,
				`Comment 2`,
				`elvisp \(member\) • Jan  1, 2020`,
				`Comment 3`,
				`loislane \(owner\) • Jan  1, 2020`,
				`Comment 4`,
				`marseilles \(collaborator\) • Jan  1, 2020 • Newest comment`,
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
			stubSpinner()
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
				`comments:\t5`,
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
			test.ExpectLines(t, output.String(), tc.expectedOutputs...)
		})
	}
}

func stubSpinner() {
	utils.StartSpinner = func(_ *spinner.Spinner) {}
	utils.StopSpinner = func(_ *spinner.Spinner) {}
}
