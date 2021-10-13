package delete

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
	"github.com/cli/cli/v2/pkg/prompt"
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

	cmd := NewCmdDelete(factory, nil)

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

func TestIssueDelete(t *testing.T) {
	httpRegistry := &httpmock.Registry{}
	defer httpRegistry.Verify(t)

	httpRegistry.Register(
		httpmock.GraphQL(`query IssueByNumber\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": {
				"hasIssuesEnabled": true,
				"issue": { "id": "THE-ID", "number": 13, "title": "The title of the issue"}
			} } }`),
	)
	httpRegistry.Register(
		httpmock.GraphQL(`mutation IssueDelete\b`),
		httpmock.GraphQLMutation(`{"id": "THE-ID"}`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, inputs["issueId"], "THE-ID")
			}),
	)
	as, teardown := prompt.InitAskStubber()
	defer teardown()
	as.StubOne("13")

	output, err := runCommand(httpRegistry, true, "13")
	if err != nil {
		t.Fatalf("error running command `issue delete`: %v", err)
	}

	r := regexp.MustCompile(`Deleted issue #13 \(The title of the issue\)`)

	if !r.MatchString(output.Stderr()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestIssueDelete_cancel(t *testing.T) {
	httpRegistry := &httpmock.Registry{}
	defer httpRegistry.Verify(t)

	httpRegistry.Register(
		httpmock.GraphQL(`query IssueByNumber\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": {
				"hasIssuesEnabled": true,
				"issue": { "id": "THE-ID", "number": 13, "title": "The title of the issue"}
			} } }`),
	)
	as, teardown := prompt.InitAskStubber()
	defer teardown()
	as.StubOne("14")

	output, err := runCommand(httpRegistry, true, "13")
	if err != nil {
		t.Fatalf("error running command `issue delete`: %v", err)
	}

	r := regexp.MustCompile(`Issue #13 was not deleted`)

	if !r.MatchString(output.String()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.String())
	}
}

func TestIssueDelete_doesNotExist(t *testing.T) {
	httpRegistry := &httpmock.Registry{}
	defer httpRegistry.Verify(t)

	httpRegistry.Register(
		httpmock.GraphQL(`query IssueByNumber\b`),
		httpmock.StringResponse(`
			{ "errors": [
				{ "message": "Could not resolve to an Issue with the number of 13." }
			] }
			`),
	)

	_, err := runCommand(httpRegistry, true, "13")
	if err == nil || err.Error() != "GraphQL error: Could not resolve to an Issue with the number of 13." {
		t.Errorf("error running command `issue delete`: %v", err)
	}
}

func TestIssueDelete_issuesDisabled(t *testing.T) {
	httpRegistry := &httpmock.Registry{}
	defer httpRegistry.Verify(t)

	httpRegistry.Register(
		httpmock.GraphQL(`query IssueByNumber\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": {
				"hasIssuesEnabled": false
			} } }`),
	)

	_, err := runCommand(httpRegistry, true, "13")
	if err == nil || err.Error() != "the 'OWNER/REPO' repository has disabled issues" {
		t.Fatalf("got error: %v", err)
	}
}
