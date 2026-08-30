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

func TestPRRevert_missingArgument(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.StubFinderForRunCommandStyleTests(t, "123", &api.PullRequest{
		ID:     "SOME-ID",
		Number: 123,
		State:  "MERGED",
		Title:  "The title of the PR",
	}, ghrepo.New("OWNER", "REPO"))

	// No arguments provided.
	_, err := runCommand(http, true, "")
	// Exits non-zero and prints an argument error.
	assert.EqualError(t, err, "cannot revert pull request: number, url, or branch required")
}

func TestPRRevert_acceptedIdentifierFormats(t *testing.T) {
	tests := []struct {
		name string
		args string
	}{
		{
			name: "Revert by pull request number",
			args: "123",
		},
		{
			name: "Revert by pull request identifier",
			args: "owner/repo#123",
		},
		{
			name: "Revert by pull request URL",
			args: "https://github.com/owner/repo/pull/123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			http := &httpmock.Registry{}
			defer http.Verify(t)

			shared.StubFinderForRunCommandStyleTests(t, tt.args, &api.PullRequest{
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
               "Number": 456,
               "URL": "https://github.com/OWNER/REPO/pull/456"
            } } } }
			`,
					func(inputs map[string]any) {
						assert.Equal(t, inputs["pullRequestId"], "SOME-ID")
					}),
			)

			output, err := runCommand(http, true, tt.args)
			// Revert PR is created and only its URL is printed.
			assert.NoError(t, err)
			assert.Equal(t, "https://github.com/OWNER/REPO/pull/456\n", output.String())
			assert.Equal(t, "", output.Stderr())
		})
	}
}

func TestPRRevert_notRevertable(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.StubFinderForRunCommandStyleTests(t, "123", &api.PullRequest{
		ID:     "SOME-ID",
		Number: 123,
		State:  "OPEN",
		Title:  "The title of the PR",
	}, ghrepo.New("OWNER", "REPO"))

	// Target PR is not merged.
	output, err := runCommand(http, true, "123")
	// API error, non-zero exit.
	assert.EqualError(t, err, "SilentError")
	assert.Equal(t, "X Pull request OWNER/REPO#123 (The title of the PR) can't be reverted because it has not been merged\n", output.Stderr())
	// No URL printed.
	assert.Equal(t, "", output.String())
}

func TestPRRevert_withTitleAndBody(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.StubFinderForRunCommandStyleTests(t, "123", &api.PullRequest{
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
               "Number": 456,
               "URL": "https://github.com/OWNER/REPO/pull/456"
            } } } }
			`,
			func(inputs map[string]any) {
				assert.Equal(t, inputs["pullRequestId"], "SOME-ID")
				assert.Equal(t, inputs["title"], "Revert PR title")
				assert.Equal(t, inputs["body"], "Revert PR body")
			}),
	)

	output, err := runCommand(http, true, "123 --title 'Revert PR title' --body 'Revert PR body'")
	// Revert PR created.
	assert.NoError(t, err)
	// Only URL printed.
	assert.Equal(t, "https://github.com/OWNER/REPO/pull/456\n", output.String())
	assert.Equal(t, "", output.Stderr())
}

func TestPRRevert_withDraft(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.StubFinderForRunCommandStyleTests(t, "123", &api.PullRequest{
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
               "Number": 456,
               "URL": "https://github.com/OWNER/REPO/pull/456"
            } } } }
			`,
			func(inputs map[string]any) {
				assert.Equal(t, inputs["pullRequestId"], "SOME-ID")
				assert.Equal(t, inputs["draft"], true)
			}),
	)

	output, err := runCommand(http, true, "123 --draft")
	// Revert PR created as a draft.
	assert.NoError(t, err)
	// Only URL printed.
	assert.Equal(t, "https://github.com/OWNER/REPO/pull/456\n", output.String())
	assert.Equal(t, "", output.Stderr())
}

func TestPRRevert_withBranch(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.StubFinderForRunCommandStyleTests(t, "123", &api.PullRequest{
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
               "Number": 456,
               "URL": "https://github.com/OWNER/REPO/pull/456",
               "HeadRefName": "revert-456-main"
            } } } }
			`,
			func(inputs map[string]any) {
				assert.Equal(t, inputs["pullRequestId"], "SOME-ID")
			}),
	)

	// The branch GitHub created for the revert is renamed to the requested name.
	http.Register(
		httpmock.REST("POST", "repos/OWNER/REPO/branches/revert-456-main/rename"),
		httpmock.RESTPayload(201, `{ "name": "custom-branch" }`,
			func(payload map[string]any) {
				assert.Equal(t, payload["new_name"], "custom-branch")
			}),
	)

	output, err := runCommand(http, true, "123 --branch custom-branch")
	// Revert PR created and its branch renamed.
	assert.NoError(t, err)
	// Only URL printed.
	assert.Equal(t, "https://github.com/OWNER/REPO/pull/456\n", output.String())
	assert.Equal(t, "", output.Stderr())
}

func TestPRRevert_withBase(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.StubFinderForRunCommandStyleTests(t, "123", &api.PullRequest{
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
               "Number": 456,
               "URL": "https://github.com/OWNER/REPO/pull/456",
               "HeadRefName": "revert-456-main"
            } } } }
			`,
			func(inputs map[string]any) {
				assert.Equal(t, inputs["pullRequestId"], "SOME-ID")
			}),
	)

	// The base branch of the newly created revert PR is updated.
	http.Register(
		httpmock.GraphQL(`mutation UpdatePullRequestBase\b`),
		httpmock.GraphQLMutation(`
			{ "data": { "updatePullRequest": { "clientMutationId": "" } } }
			`,
			func(inputs map[string]any) {
				assert.Equal(t, inputs["pullRequestId"], "NEW-ID")
				assert.Equal(t, inputs["baseRefName"], "main")
			}),
	)

	output, err := runCommand(http, true, "123 --base main")
	// Revert PR created and re-based.
	assert.NoError(t, err)
	// Only URL printed.
	assert.Equal(t, "https://github.com/OWNER/REPO/pull/456\n", output.String())
	assert.Equal(t, "", output.Stderr())
}

func TestPRRevert_withBranchAndBase(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.StubFinderForRunCommandStyleTests(t, "123", &api.PullRequest{
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
               "Number": 456,
               "URL": "https://github.com/OWNER/REPO/pull/456",
               "HeadRefName": "revert-456-main"
            } } } }
			`,
			func(inputs map[string]any) {
				assert.Equal(t, inputs["pullRequestId"], "SOME-ID")
			}),
	)

	http.Register(
		httpmock.REST("POST", "repos/OWNER/REPO/branches/revert-456-main/rename"),
		httpmock.RESTPayload(201, `{ "name": "custom-branch" }`,
			func(payload map[string]any) {
				assert.Equal(t, payload["new_name"], "custom-branch")
			}),
	)

	http.Register(
		httpmock.GraphQL(`mutation UpdatePullRequestBase\b`),
		httpmock.GraphQLMutation(`
			{ "data": { "updatePullRequest": { "clientMutationId": "" } } }
			`,
			func(inputs map[string]any) {
				assert.Equal(t, inputs["pullRequestId"], "NEW-ID")
				assert.Equal(t, inputs["baseRefName"], "trunk")
			}),
	)

	output, err := runCommand(http, true, "123 --branch custom-branch --base trunk")
	// Revert PR created, branch renamed and re-based.
	assert.NoError(t, err)
	// Only URL printed.
	assert.Equal(t, "https://github.com/OWNER/REPO/pull/456\n", output.String())
	assert.Equal(t, "", output.Stderr())
}

func TestPRRevert_withBranch_noHeadRefName(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.StubFinderForRunCommandStyleTests(t, "123", &api.PullRequest{
		ID:     "SOME-ID",
		Number: 123,
		State:  "MERGED",
		Title:  "The title of the PR",
	}, ghrepo.New("OWNER", "REPO"))

	// The API response carries no head branch name, so there is nothing to rename.
	http.Register(
		httpmock.GraphQL(`mutation PullRequestRevert\b`),
		httpmock.GraphQLMutation(`
			{ "data": { "revertPullRequest": { "pullRequest": {
				"ID": "SOME-ID"
			}, "revertPullRequest": {
               "ID": "NEW-ID",
               "Number": 456,
               "URL": "https://github.com/OWNER/REPO/pull/456"
            } } } }
			`,
			func(inputs map[string]any) {
				assert.Equal(t, inputs["pullRequestId"], "SOME-ID")
			}),
	)

	output, err := runCommand(http, true, "123 --branch custom-branch")
	// The rename is skipped with a warning instead of failing.
	assert.NoError(t, err)
	assert.Equal(t, "https://github.com/OWNER/REPO/pull/456\n", output.String())
	assert.Equal(t, "! could not rename branch: revert PR has no head branch name\n", output.Stderr())
}

func TestPRRevert_withBase_noPullRequestID(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.StubFinderForRunCommandStyleTests(t, "123", &api.PullRequest{
		ID:     "SOME-ID",
		Number: 123,
		State:  "MERGED",
		Title:  "The title of the PR",
	}, ghrepo.New("OWNER", "REPO"))

	// The API response carries no revert PR ID, so the base can't be updated.
	http.Register(
		httpmock.GraphQL(`mutation PullRequestRevert\b`),
		httpmock.GraphQLMutation(`
			{ "data": { "revertPullRequest": { "pullRequest": {
				"ID": "SOME-ID"
			}, "revertPullRequest": {
               "Number": 456,
               "URL": "https://github.com/OWNER/REPO/pull/456",
               "HeadRefName": "revert-456-main"
            } } } }
			`,
			func(inputs map[string]any) {
				assert.Equal(t, inputs["pullRequestId"], "SOME-ID")
			}),
	)

	output, err := runCommand(http, true, "123 --base trunk")
	// The base update is skipped with a warning instead of failing.
	assert.NoError(t, err)
	assert.Equal(t, "https://github.com/OWNER/REPO/pull/456\n", output.String())
	assert.Equal(t, "! could not change base branch: revert PR has no ID\n", output.Stderr())
}

func TestPRRevert_branchRenameFailurePrintsURL(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.StubFinderForRunCommandStyleTests(t, "123", &api.PullRequest{
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
               "Number": 456,
               "URL": "https://github.com/OWNER/REPO/pull/456",
               "HeadRefName": "revert-456-main"
            } } } }
			`,
			func(inputs map[string]any) {
				assert.Equal(t, inputs["pullRequestId"], "SOME-ID")
			}),
	)

	http.Register(
		httpmock.REST("POST", "repos/OWNER/REPO/branches/revert-456-main/rename"),
		httpmock.StatusStringResponse(422, `{ "message": "Validation failed" }`),
	)

	output, err := runCommand(http, true, "123 --branch custom-branch")
	// Rename failure is fatal, but the revert PR URL has already been printed.
	assert.Error(t, err)
	assert.ErrorContains(t, err, "branch rename failed:")
	assert.Equal(t, "https://github.com/OWNER/REPO/pull/456\n", output.String())
	assert.Contains(t, output.Stderr(), "X Failed to rename branch revert-456-main to custom-branch:")
}

func TestPRRevert_APIFailure(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.StubFinderForRunCommandStyleTests(t, "123", &api.PullRequest{
		ID:     "SOME-ID",
		Number: 123,
		State:  "MERGED",
		Title:  "The title of the PR",
	}, ghrepo.New("OWNER", "REPO"))

	http.Register(
		httpmock.GraphQL(`mutation PullRequestRevert\b`),
		httpmock.GraphQLMutation(`
			{ "errors": [{
              "message": "Authorization error"
            }]}`,
			func(inputs map[string]any) {
				assert.Equal(t, inputs["pullRequestId"], "SOME-ID")
			}),
	)

	output, err := runCommand(http, true, "123")
	// Non-zero exit, stderr shows the API error, stdout empty.
	assert.EqualError(t, err, "API call failed: GraphQL: Authorization error")
	assert.Equal(t, "X GraphQL: Authorization error\n", output.Stderr())
	assert.Equal(t, "", output.String())
}

func TestPRRevert_multipleInvocations(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.StubFinderForRunCommandStyleTests(t, "123", &api.PullRequest{
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
               "Number": 456,
               "URL": "https://github.com/OWNER/REPO/pull/456"
            } } } }
			`,
			func(inputs map[string]any) {
				assert.Equal(t, inputs["pullRequestId"], "SOME-ID")
			}),
	)

	output, err := runCommand(http, true, "123")
	// Revert PR is created and only its URL is printed.
	assert.NoError(t, err)
	assert.Equal(t, "https://github.com/OWNER/REPO/pull/456\n", output.String())
	assert.Equal(t, "", output.Stderr())

	// Invoke the same command, behavior depends solely on API response
	shared.StubFinderForRunCommandStyleTests(t, "123", &api.PullRequest{
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
               "Number": 456,
               "URL": "https://github.com/OWNER/REPO/pull/456"
            } } } }
			`,
			func(inputs map[string]any) {
				assert.Equal(t, inputs["pullRequestId"], "SOME-ID")
			}),
	)

	output, err = runCommand(http, true, "123")
	// Revert PR is created and only its URL is printed.
	assert.NoError(t, err)
	assert.Equal(t, "https://github.com/OWNER/REPO/pull/456\n", output.String())
	assert.Equal(t, "", output.Stderr())

	// Invoke the same command, behavior depends solely on API response.
	shared.StubFinderForRunCommandStyleTests(t, "123", &api.PullRequest{
		ID:     "SOME-ID",
		Number: 123,
		State:  "OPEN",
		Title:  "The title of the PR",
	}, ghrepo.New("OWNER", "REPO"))

	output, err = runCommand(http, true, "123")
	// Revert PR is not created, API error, non-zero exit.
	assert.EqualError(t, err, "SilentError")
	assert.Equal(t, "X Pull request OWNER/REPO#123 (The title of the PR) can't be reverted because it has not been merged\n", output.Stderr())
	// No URL printed.
	assert.Equal(t, "", output.String())
}
