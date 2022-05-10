package pin

import (
	"bytes"
	"io"
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
	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(isTTY)
	ios.SetStdinTTY(isTTY)
	ios.SetStderrTTY(isTTY)

	factory := &cmdutil.Factory{
		IOStreams: ios,
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

	cmd := NewCmdPin(factory, nil)

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

func TestIssuePin(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query IssueByNumber\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": {
				"hasIssuesEnabled": true,
				"issue": { "id": "ISSUE-ID", "number": 20, "title": "Issue Title", "isPinned": false}
			} } }`),
	)
	http.Register(
		httpmock.GraphQL(`mutation IssuePin\b`),
		httpmock.GraphQLMutation(`{"id": "ISSUE-ID"}`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, inputs["issueId"], "ISSUE-ID")
			}),
	)

	output, err := runCommand(http, true, "20")

	if err != nil {
		t.Fatalf("error running command `issue pin`: %v", err)
	}

	r := regexp.MustCompile(`Issue #20 \(Issue Title\) pinned to OWNER\/REPO`)

	if !r.MatchString(output.Stderr()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestIssuePin_unpinIssue(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query IssueByNumber\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": {
				"hasIssuesEnabled": true,
				"issue": { "id": "ISSUE-ID", "number": 20, "title": "Issue Title", "isPinned": true}
			} } }`),
	)

	http.Register(
		httpmock.GraphQL(`mutation IssueUnpin\b`),
		httpmock.GraphQLMutation(`{"id": "ISSUE-ID"}`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, inputs["issueId"], "ISSUE-ID")
			}),
	)

	output, err := runCommand(http, true, "20 --remove")

	if err != nil {
		t.Fatalf("error running command `issue pin`: %v", err)
	}

	r := regexp.MustCompile(`Issue #20 \(Issue Title\) was unpinned from OWNER\/REPO`)

	if !r.MatchString(output.Stderr()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestIssuePin_alreadyPinned(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query IssueByNumber\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": {
				"hasIssuesEnabled": true,
				"issue": { "id": "ISSUE-ID", "number": 20, "title": "Issue Title", "isPinned": true}
			} } }`),
	)

	output, err := runCommand(http, true, "20")

	if err != nil {
		t.Fatalf("error running command `issue pin`: %v", err)
	}

	r := regexp.MustCompile(`Issue #20 \(Issue Title\) is already pinned to OWNER\/REPO`)

	if !r.MatchString(output.Stderr()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestIssuePin_alreadyUnpinned(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query IssueByNumber\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": {
				"hasIssuesEnabled": true,
				"issue": { "id": "ISSUE-ID", "number": 20, "title": "Issue Title", "isPinned": false}
			} } }`),
	)

	output, err := runCommand(http, true, "20 --remove")

	if err != nil {
		t.Fatalf("error running command `issue pin`: %v", err)
	}

	r := regexp.MustCompile(`Issue #20 \(Issue Title\) is not pinned to OWNER\/REPO`)

	if !r.MatchString(output.Stderr()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}
