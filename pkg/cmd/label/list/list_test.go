package list

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"strings"
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

func runCommand(rt http.RoundTripper, isTTY bool, cli string) (*test.CmdOut, *cmdutil.TestBrowser, error) {
	io, _, stdout, stderr := iostreams.Test()
	io.SetStdoutTTY(isTTY)
	io.SetStdinTTY(isTTY)
	io.SetStderrTTY(isTTY)

	browser := &cmdutil.TestBrowser{}

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
		Browser: browser,
	}

	cmd := NewCmdList(factory, nil)

	argv, err := shlex.Split(cli)
	if err != nil {
		return nil, nil, err
	}
	cmd.SetArgs(argv)

	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(ioutil.Discard)
	cmd.SetErr(ioutil.Discard)

	_, err = cmd.ExecuteC()
	return &test.CmdOut{
		OutBuf: stdout,
		ErrBuf: stderr,
	}, browser, err
}

func TestLabelList_nontty(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/labels"),
		httpmock.JSONResponse([]Label{
			{
				Name:        "bug",
				Color:       "#B60205",
				Description: "This is a bug label",
			},
			{
				Name:        "docs",
				Color:       "#D93F0B",
				Description: "This is a docs label",
			},
		}),
	)

	output, _, err := runCommand(http, false, "")
	if err != nil {
		t.Errorf("error running command `label list`: %v", err)
	}

	assert.Equal(t, "", output.Stderr())

	outputLines := strings.Split(output.String(), "\n")

	assert.Equal(t, 3, len(outputLines))
	assert.Equal(t, "bug\tThis is a bug label", outputLines[0])
	assert.Equal(t, "docs\tThis is a docs label", outputLines[1])
	assert.Equal(t, "", outputLines[2])
}

func TestLabelList_tty(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/labels"),
		httpmock.JSONResponse([]Label{
			{
				Name:        "bug",
				Color:       "#B60205",
				Description: "This is a bug label",
			},
			{
				Name:        "docs",
				Color:       "#D93F0B",
				Description: "This is a docs label",
			},
		}),
	)

	output, _, err := runCommand(http, true, "")
	if err != nil {
		t.Errorf("error running command `label list`: %v", err)
	}

	assert.Equal(t, "", output.Stderr())

	outputLines := strings.Split(output.String(), "\n")

	assert.Equal(t, 3, len(outputLines))
	assert.Equal(t, "bug   This is a bug label", outputLines[0])
	assert.Equal(t, "docs  This is a docs label", outputLines[1])
	assert.Equal(t, "", outputLines[2])
}

func TestLabelList_web(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	output, browser, err := runCommand(http, true, "-w")
	if err != nil {
		t.Errorf("error running command `label list`: %v", err)
	}

	assert.Equal(t, "", output.String())
	assert.Equal(t, "Opening github.com/OWNER/REPO/labels in your browser.\n", output.ErrBuf.String())
	browser.Verify(t, "https://github.com/OWNER/REPO/labels")

}
