package close

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"regexp"
	"testing"

	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/test"
	"github.com/google/shlex"
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
		Branch: func() (string, error) {
			return "trunk", nil
		},
	}

	cmd := NewCmdClose(factory, nil)

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

func TestPrClose(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": {
			"pullRequest": { "number": 96, "title": "The title of the PR" }
		} } }
	`))

	http.StubResponse(200, bytes.NewBufferString(`{"id": "THE-ID"}`))

	output, err := runCommand(http, true, "96")
	if err != nil {
		t.Fatalf("error running command `pr close`: %v", err)
	}

	r := regexp.MustCompile(`Closed pull request #96 \(The title of the PR\)`)

	if !r.MatchString(output.Stderr()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestPrClose_alreadyClosed(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": {
			"pullRequest": { "number": 101, "title": "The title of the PR", "closed": true }
		} } }
	`))

	output, err := runCommand(http, true, "101")
	if err != nil {
		t.Fatalf("error running command `pr close`: %v", err)
	}

	r := regexp.MustCompile(`Pull request #101 \(The title of the PR\) is already closed`)

	if !r.MatchString(output.Stderr()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestPrClose_deleteBranch(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)
	http.StubResponse(200, bytes.NewBufferString(`
		{ "data": { "repository": {
			"pullRequest": { "number": 96, "title": "The title of the PR", "headRefName":"blueberries", "headRepositoryOwner": {"login": "OWNER"}}
		} } }
	`))
	http.StubResponse(200, bytes.NewBufferString(`{"id": "THE-ID"}`))
	http.Register(
		httpmock.REST("DELETE", "repos/OWNER/REPO/git/refs/heads/blueberries"),
		httpmock.StringResponse(`{}`))

	cs, cmdTeardown := test.InitCmdStubber()
	defer cmdTeardown()

	cs.Stub("") // git config --get-regexp ^branch\.blueberries\.(remote|merge)$
	cs.Stub("") // git rev-parse --verify blueberries`
	cs.Stub("") // git branch -d
	cs.Stub("") // git push origin --delete blueberries

	output, err := runCommand(http, true, `96 --delete-branch`)
	if err != nil {
		t.Fatalf("Got unexpected error running `pr close` %s", err)
	}

	test.ExpectLines(t, output.Stderr(), `Closed pull request #96 \(The title of the PR\)`, `Deleted branch blueberries`)
}
