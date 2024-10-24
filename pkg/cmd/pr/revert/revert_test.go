package revert

import (
	"bytes"
	"io"
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
	ios, _, stdout, stderr := iostreams.Test()
	ios.SetStdoutTTY(isTTY)
	ios.SetStdinTTY(isTTY)
	ios.SetStderrTTY(isTTY)

	factory := &cmdutil.Factory{
		IOStreams: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: rt}, nil
		},
	}

	cmd := NewCmdRevert(factory, nil)

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

func TestPRRevert(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.RunCommandFinder("123", &api.PullRequest{
		ID:     "SOME-ID",
		Number: 123,
		State:  "MERGED",
		Title:  "The title of the PR",
	}, ghrepo.New("OWNER", "REPO"))

	http.Register(
		httpmock.GraphQL(`mutation PullRequestRevert\b`),
		httpmock.GraphQLMutation(`
			{ "data": { "revertPullRequest": { "pullRequest": {
				"ID": "SOME-ID"
			}, "revertPullRequest": {
               "ID": "NEW-ID",
			   "Title": "Revert PR title",
               "Number": 456
            } } } }
			`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, inputs["pullRequestId"], "SOME-ID")
			}),
	)

	output, err := runCommand(http, true, "123")
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, "✓ Created pull request OWNER/REPO#456 (Revert PR title) that reverts OWNER/REPO#123 (The title of the PR)\n", output.Stderr())
}

func TestPRRevert_notRevertable(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.RunCommandFinder("123", &api.PullRequest{
		ID:     "SOME-ID",
		Number: 123,
		State:  "OPEN",
		Title:  "The title of the PR",
	}, ghrepo.New("OWNER", "REPO"))

	output, err := runCommand(http, true, "123")
	assert.Error(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, "X Pull request OWNER/REPO#123 (The title of the PR) can't be reverted because it has not been merged\n", output.Stderr())
}

func TestPRRevert_withAllOpts(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.RunCommandFinder("123", &api.PullRequest{
		ID:     "SOME-ID",
		Number: 123,
		State:  "MERGED",
		Title:  "The title of the PR",
	}, ghrepo.New("OWNER", "REPO"))

	http.Register(
		httpmock.GraphQL(`mutation PullRequestRevert\b`),
		httpmock.GraphQLMutation(`
			{ "data": { "revertPullRequest": { "pullRequest": {
				"ID": "SOME-ID"
			}, "revertPullRequest": {
               "ID": "NEW-ID",
			   "Title": "Revert PR title",
               "Number": 456
            } } } }
			`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, inputs["pullRequestId"], "SOME-ID")
				assert.Equal(t, inputs["title"], "Revert PR title")
				assert.Equal(t, inputs["body"], "Revert PR body")
				assert.Equal(t, inputs["draft"], true)
			}),
	)

	output, err := runCommand(http, true, "123 --title 'Revert PR title' --body 'Revert PR body' --draft")
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, "✓ Created pull request OWNER/REPO#456 (Revert PR title) that reverts OWNER/REPO#123 (The title of the PR)\n", output.Stderr())
}
