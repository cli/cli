package list

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/test"
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

	cmd := NewCmdList(factory, nil)

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

func TestRepoList_nontty(t *testing.T) {
	io, _, stdout, stderr := iostreams.Test()
	io.SetStdoutTTY(false)
	io.SetStdinTTY(false)
	io.SetStderrTTY(false)

	httpReg := &httpmock.Registry{}
	defer httpReg.Verify(t)

	httpReg.Register(
		httpmock.GraphQL(`query UserCurrent\b`),
		httpmock.StringResponse(`{"data":{"viewer":{"login":"octocat"}}}`),
	)
	httpReg.Register(
		httpmock.GraphQL(`query RepoList\b`),
		httpmock.FileResponse("./fixtures/repoList.json"),
	)

	opts := ListOptions{
		IO: io,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: httpReg}, nil
		},
		Now: func() time.Time {
			t, _ := time.Parse(time.RFC822, "19 Feb 21 15:00 UTC")
			return t
		},
		Limit: 30,
	}

	err := listRun(&opts)
	assert.NoError(t, err)

	assert.Equal(t, "", stderr.String())

	assert.Equal(t, heredoc.Doc(`
		octocat/hello-world	My first repository	Public	2021-02-19T06:34:58Z
		octocat/cli	GitHub CLI	Public	2021-02-19T06:06:06Z
		octocat/testing		Private	2021-02-11T22:32:05Z
	`), stdout.String())
}

func TestRepoList_tty(t *testing.T) {
	io, _, stdout, stderr := iostreams.Test()
	io.SetStdoutTTY(true)
	io.SetStdinTTY(true)
	io.SetStderrTTY(true)

	httpReg := &httpmock.Registry{}
	defer httpReg.Verify(t)

	httpReg.Register(
		httpmock.GraphQL(`query UserCurrent\b`),
		httpmock.StringResponse(`{"data":{"viewer":{"login":"octocat"}}}`),
	)
	httpReg.Register(
		httpmock.GraphQL(`query RepoList\b`),
		httpmock.FileResponse("./fixtures/repoList.json"),
	)

	opts := ListOptions{
		IO: io,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: httpReg}, nil
		},
		Now: func() time.Time {
			t, _ := time.Parse(time.RFC822, "19 Feb 21 15:00 UTC")
			return t
		},
		Limit: 30,
	}

	err := listRun(&opts)
	assert.NoError(t, err)

	assert.Equal(t, "", stderr.String())

	assert.Equal(t, heredoc.Doc(`

		Showing 3 of 3 repositories in @octocat

		octocat/hello-world  My first repository           8h
		octocat/cli          GitHub CLI           Fork     8h
		octocat/testing                           Private  7d
	`), stdout.String())
}

func TestRepoList_filtering(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.GraphQL(`query UserCurrent\b`),
		httpmock.StringResponse(`{"data":{"viewer":{"login":"octocat"}}}`),
	)
	http.Register(
		httpmock.GraphQL(`query RepoList\b`),
		httpmock.GraphQLQuery(`{}`, func(_ string, params map[string]interface{}) {
			assert.Equal(t, "PRIVATE", params["privacy"])
			assert.Equal(t, float64(2), params["per_page"])
		}),
	)

	output, err := runCommand(http, true, `--private --limit 2 `)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "", output.Stderr())
	assert.Equal(t, "\nNo results match your search\n\n", output.String())
}

func TestRepoList_withInvalidFlagCombinations(t *testing.T) {
	tests := []struct {
		name       string
		cli        string
		wantStderr string
	}{
		{
			name:       "invalid limit",
			cli:        "--limit 0",
			wantStderr: "invalid limit: 0",
		},
		{
			name:       "both private and public",
			cli:        "--private --public",
			wantStderr: "specify only one of `--public` or `--private`",
		},
		{
			name:       "both source and fork",
			cli:        "--source --fork",
			wantStderr: "specify only one of `--source` or `--fork`",
		},
	}

	for _, tt := range tests {
		httpReg := &httpmock.Registry{}

		_, err := runCommand(httpReg, true, tt.cli)
		assert.EqualError(t, err, tt.wantStderr)
	}
}
