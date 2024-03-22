package ready

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
	"github.com/stretchr/testify/require"
)

func Test_NewCmdReady(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		isTTY   bool
		want    ReadyOptions
		wantErr string
	}{
		{
			name:  "number argument",
			args:  "123",
			isTTY: true,
			want: ReadyOptions{
				SelectorArg: "123",
			},
		},
		{
			name:  "no argument",
			args:  "",
			isTTY: true,
			want: ReadyOptions{
				SelectorArg: "",
			},
		},
		{
			name:    "no argument with --repo override",
			args:    "-R owner/repo",
			isTTY:   true,
			wantErr: "argument required when using the --repo flag",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStderrTTY(tt.isTTY)

			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			var opts *ReadyOptions
			cmd := NewCmdReady(f, func(o *ReadyOptions) error {
				opts = o
				return nil
			})
			cmd.PersistentFlags().StringP("repo", "R", "", "")

			argv, err := shlex.Split(tt.args)
			require.NoError(t, err)
			cmd.SetArgs(argv)

			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			_, err = cmd.ExecuteC()
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.want.SelectorArg, opts.SelectorArg)
		})
	}
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
	}

	cmd := NewCmdReady(factory, nil)

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

func TestPRReady(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.RunCommandFinder("123", &api.PullRequest{
		ID:      "THE-ID",
		Number:  123,
		State:   "OPEN",
		IsDraft: true,
	}, ghrepo.New("OWNER", "REPO"))

	http.Register(
		httpmock.GraphQL(`mutation PullRequestReadyForReview\b`),
		httpmock.GraphQLMutation(`{"id": "THE-ID"}`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, inputs["pullRequestId"], "THE-ID")
			}),
	)

	output, err := runCommand(http, true, "123")
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, "✓ Pull request OWNER/REPO#123 is marked as \"ready for review\"\n", output.Stderr())
}

func TestPRReady_alreadyReady(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.RunCommandFinder("123", &api.PullRequest{
		ID:      "THE-ID",
		Number:  123,
		State:   "OPEN",
		IsDraft: false,
	}, ghrepo.New("OWNER", "REPO"))

	output, err := runCommand(http, true, "123")
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, "! Pull request OWNER/REPO#123 is already \"ready for review\"\n", output.Stderr())
}

func TestPRReadyUndo(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.RunCommandFinder("123", &api.PullRequest{
		ID:      "THE-ID",
		Number:  123,
		State:   "OPEN",
		IsDraft: false,
	}, ghrepo.New("OWNER", "REPO"))

	http.Register(
		httpmock.GraphQL(`mutation ConvertPullRequestToDraft\b`),
		httpmock.GraphQLMutation(`{"id": "THE-ID"}`,
			func(inputs map[string]interface{}) {
				assert.Equal(t, inputs["pullRequestId"], "THE-ID")
			}),
	)

	output, err := runCommand(http, true, "123 --undo")
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, "✓ Pull request OWNER/REPO#123 is converted to \"draft\"\n", output.Stderr())
}

func TestPRReadyUndo_alreadyDraft(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.RunCommandFinder("123", &api.PullRequest{
		ID:      "THE-ID",
		Number:  123,
		State:   "OPEN",
		IsDraft: true,
	}, ghrepo.New("OWNER", "REPO"))

	output, err := runCommand(http, true, "123 --undo")
	assert.NoError(t, err)
	assert.Equal(t, "", output.String())
	assert.Equal(t, "! Pull request OWNER/REPO#123 is already \"in draft\"\n", output.Stderr())
}

func TestPRReady_closed(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	shared.RunCommandFinder("123", &api.PullRequest{
		ID:      "THE-ID",
		Number:  123,
		State:   "CLOSED",
		IsDraft: true,
	}, ghrepo.New("OWNER", "REPO"))

	output, err := runCommand(http, true, "123")
	assert.EqualError(t, err, "SilentError")
	assert.Equal(t, "", output.String())
	assert.Equal(t, "X Pull request OWNER/REPO#123 is closed. Only draft pull requests can be marked as \"ready for review\"\n", output.Stderr())
}
