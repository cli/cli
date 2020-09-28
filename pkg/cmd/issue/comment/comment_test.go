package comment

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"reflect"
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

func eq(t *testing.T, got interface{}, expected interface{}) {
	t.Helper()
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("expected: %v, got: %v", expected, got)
	}
}
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

	cmd := NewCmdComment(factory, func(opts *CommentOptions) error {
		return commentRun(opts)
	})
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

func TestCommentCreate(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": {
		"hasIssuesEnabled": true,
		"issue": { "number": 96, "title": "The title of the issue"}
	} } }
	`))

	http.StubResponse(200, bytes.NewBufferString(`{"mutationId": "THE-ID"}`))

	output, err := runCommand(http, true, `13 -b "cash rules everything around me"`)
	if err != nil {
		t.Errorf("error running command `issue comment`: %v", err)
	}

	bodyBytes, _ := ioutil.ReadAll(http.Requests[1].Body)
	reqBody := struct {
		Variables struct {
			Input struct {
				Body string
			}
		}
	}{}
	_ = json.Unmarshal(bodyBytes, &reqBody)

	eq(t, reqBody.Variables.Input.Body, "cash rules everything around me")

	r := regexp.MustCompile(`Make a comment for The title of the issue`)

	if !r.MatchString(output.Stderr()) {
		t.Fatalf("output did not match regexp /%s/\n> output\n%q\n", r, output.Stderr())
	}
}

func TestIssue_Disabled(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.StubResponse(200, bytes.NewBufferString(`
	{ "data": { "repository": {
		"hasIssuesEnabled": false
	} } }
	`))

	_, err := runCommand(http, true, "13")
	if err == nil || err.Error() != "the 'OWNER/REPO' repository has disabled issues" {
		t.Fatalf("got error: %v", err)
	}
}
