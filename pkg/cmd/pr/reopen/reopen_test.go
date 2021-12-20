package reopen

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/pr/shared"
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

func TestPRReopen(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.RunCommandFinder("123", &api.PullRequest{
		ID:     "THE-ID",
		Number: 123,
		State:  "CLOSED",
		Title:  "The title of the PR",
	}, ghrepo.New("OWNER", "REPO"))

	http.Register(
		httpmock.GraphQL(`mutation PullRequestReopen\b`),
		httpmock.GraphQLMutation(`{"id": "THE-ID"}`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, inputs["pullRequestId"], "THE-ID")
			}),
	)

	output, err := runCommand(http, true, "123")
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, "✓ Reopened pull request #123 (The title of the PR)\n", output.Stderr())
}

func TestPRReopen_alreadyOpen(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.RunCommandFinder("123", &api.PullRequest{
		ID:     "THE-ID",
		Number: 123,
		State:  "OPEN",
		Title:  "The title of the PR",
	}, ghrepo.New("OWNER", "REPO"))

	output, err := runCommand(http, true, "123")
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, "! Pull request #123 (The title of the PR) is already open\n", output.Stderr())
}

func TestPRReopen_alreadyMerged(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.RunCommandFinder("123", &api.PullRequest{
		ID:     "THE-ID",
		Number: 123,
		State:  "MERGED",
		Title:  "The title of the PR",
	}, ghrepo.New("OWNER", "REPO"))

	output, err := runCommand(http, true, "123")
	assert.EqualError(t, err, "SilentError")
	assert.Equal(t, "", output.String())
	assert.Equal(t, "X Pull request #123 (The title of the PR) can't be reopened because it was already merged\n", output.Stderr())
}
