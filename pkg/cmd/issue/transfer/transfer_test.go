package transfer

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runCommand(rt http.RoundTripper, cli string) (*test.CmdOut, error) {
	ios, _, stdout, stderr := iostreams.Test()

	factory := &cmdutil.Factory{
		IOStreams: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: rt}, nil
		},
		Config: func() (gh.Config, error) {
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
		name         string
		cli          string
		wants        TransferOptions
		wantBaseRepo ghrepo.Interface
		wantErr      bool
	}{
		{
			name:    "no argument",
			cli:     "",
			wantErr: true,
		},
		{
			name: "issue number argument",
			cli:  "--repo cli/repo 23 OWNER/REPO",
			wants: TransferOptions{
				IssueNumber:      23,
				DestRepoSelector: "OWNER/REPO",
			},
			wantBaseRepo: ghrepo.New("cli", "repo"),
		},
		{
			name: "argument is hash prefixed number",
			// Escaping is required here to avoid what I think is shellex treating it as a comment.
			cli: "--repo cli/repo \\#23 OWNER/REPO",
			wants: TransferOptions{
				IssueNumber:      23,
				DestRepoSelector: "OWNER/REPO",
			},
			wantBaseRepo: ghrepo.New("cli", "repo"),
		},
		{
			name: "argument is a URL",
			cli:  "https://github.com/cli/cli/issues/23 OWNER/REPO",
			wants: TransferOptions{
				IssueNumber:      23,
				DestRepoSelector: "OWNER/REPO",
			},
			wantBaseRepo: ghrepo.New("cli", "cli"),
		},
		{
			name:    "argument cannot be parsed to an issue",
			cli:     "unparseable OWNER/REPO",
			wantErr: true,
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
			cmdutil.EnableRepoOverride(cmd, f)

			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, cErr := cmd.ExecuteC()
			if tt.wantErr {
				require.Error(t, cErr)
				return
			}

			require.NoError(t, cErr)
			assert.Equal(t, tt.wants.IssueNumber, gotOpts.IssueNumber)
			assert.Equal(t, tt.wants.DestRepoSelector, gotOpts.DestRepoSelector)
			actualBaseRepo, err := gotOpts.BaseRepo()
			require.NoError(t, err)
			assert.True(
				t,
				ghrepo.IsSame(tt.wantBaseRepo, actualBaseRepo),
				"expected base repo %+v, got %+v", tt.wantBaseRepo, actualBaseRepo,
			)
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
