package transfer

import (
	"bytes"
	"io"
	"net/http"
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

func runCommand(rt http.RoundTripper, cli string) (*test.CmdOut, error) {
	ios, _, stdout, stderr := iostreams.Test()

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

	ios.SetStdoutTTY(true)

	cmd := NewCmdTransfer(factory, nil)

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

func TestNewCmdTransfer(t *testing.T) {
	tests := []struct {
		name    string
		cli     string
		wants   TransferOptions
		wantErr string
	}{
		{
			name: "issue name",
			cli:  "3252 OWNER/REPO",
			wants: TransferOptions{
				IssueSelector:    "3252",
				DestRepoSelector: "OWNER/REPO",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &cmdutil.Factory{}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts *TransferOptions
			cmd := NewCmdTransfer(f, func(opts *TransferOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, cErr := cmd.ExecuteC()
			assert.NoError(t, cErr)
			assert.Equal(t, tt.wants.IssueSelector, gotOpts.IssueSelector)
			assert.Equal(t, tt.wants.DestRepoSelector, gotOpts.DestRepoSelector)
		})
	}
}

func Test_transferRun_noflags(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	output, err := runCommand(http, "")

	if err != nil {
		assert.Equal(t, "issue and destination repository are required", err.Error())
	}

	assert.Equal(t, "", output.String())
}

func Test_transferRunSuccessfulIssueTransfer(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query IssueByNumber\b`),
		httpmock.StringResponse(`
			{ "data": { "repository": {
				"hasIssuesEnabled": true,
				"issue": { "id": "THE-ID", "number": 1234, "title": "The title of the issue"}
			} } }`))

	http.Register(
		httpmock.GraphQL(`query RepositoryInfo\b`),
		httpmock.StringResponse(`
				{ "data": { "repository": {
						"id": "dest-id",
						"name": "REPO1",
						"owner": { "login": "OWNER1" },
						"viewerPermission": "WRITE",
						"hasIssuesEnabled": true
				}}}`))

	http.Register(
		httpmock.GraphQL(`mutation IssueTransfer\b`),
		httpmock.GraphQLMutation(`{"data":{"transferIssue":{"issue":{"url":"https://github.com/OWNER1/REPO1/issues/1"}}}}`, func(input map[string]interface{}) {
			assert.Equal(t, input["issueId"], "THE-ID")
			assert.Equal(t, input["repositoryId"], "dest-id")
		}))

	output, err := runCommand(http, "1234 OWNER1/REPO1")
	if err != nil {
		t.Errorf("error running command `issue transfer`: %v", err)
	}
	assert.Equal(t, "https://github.com/OWNER1/REPO1/issues/1\n", output.String())
}
