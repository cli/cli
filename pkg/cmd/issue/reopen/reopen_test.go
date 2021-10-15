package reopen

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"regexp"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
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

	cmd := NewCmdReopen(factory, nil)

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

func TestIssueReopen(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query IssueByNumber\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": {
				"hasIssuesEnabled": true,
				"issue": { "id": "THE-ID", "number": 2, "state": "CLOSED", "title": "The title of the issue"}
			} } }`),
	)
	http.Register(
		httpmock.GraphQL(`mutation IssueReopen\b`),
		httpmock.GraphQLMutation(`{"id": "THE-ID"}`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, inputs["issueId"], "THE-ID")
			}),
	)

	output, err := runCommand(http, true, "2")
	if err != nil {
		t.Fatalf("error running command `issue reopen`: %v", err)
	}

	r := regexp.MustCompile(`Reopened issue #2 \(The title of the issue\)`)

	if !r.MatchString(output.Stderr()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestIssueReopen_alreadyOpen(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query IssueByNumber\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": {
				"hasIssuesEnabled": true,
				"issue": { "number": 2, "state": "OPEN", "title": "The title of the issue"}
			} } }`),
	)

	output, err := runCommand(http, true, "2")
	if err != nil {
		t.Fatalf("error running command `issue reopen`: %v", err)
	}

	r := regexp.MustCompile(`Issue #2 \(The title of the issue\) is already open`)

	if !r.MatchString(output.Stderr()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestIssueReopen_issuesDisabled(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query IssueByNumber\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": {
				"hasIssuesEnabled": false
			} } }`),
	)

	_, err := runCommand(http, true, "2")
	if err == nil || err.Error() != "the 'OWNER/REPO' repository has disabled issues" {
		t.Fatalf("got error: %v", err)
	}
}
