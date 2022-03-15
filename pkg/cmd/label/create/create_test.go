package create

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
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
		Config: func() (config.Config, error) {
			return config.NewBlankConfig(), nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
	}

	cmd := NewCmdCreate(factory, nil)

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

func TestLabelCreate_nontty_non_interactive(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.REST("POST", "repos/OWNER/REPO/labels"),
		httpmock.StatusStringResponse(201, "{}"),
	)

	output, err := runCommand(http, false, "-n bugs -d \"Something isn't working\" -c \"0052cc\"")

	if err != nil {
		t.Errorf("error running command `label create`: %v", err)
	}

	assert.Equal(t, "", output.Stderr())
	assert.Equal(t, "", output.String())

	bodyBytes, _ := ioutil.ReadAll(http.Requests[0].Body)
	reqBody := CreateLabelRequest{}
	err = json.Unmarshal(bodyBytes, &reqBody)
	if err != nil {
		t.Fatalf("error decoding JSON: %v", err)
	}

	expectReqBody := CreateLabelRequest{
		Name:        "bugs",
		Description: "Something isn't working",
		Color:       "0052cc",
	}

	assert.Equal(t, expectReqBody, reqBody)
}

func TestLabelCreate_tty_non_interactive(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.REST("POST", "repos/OWNER/REPO/labels"),
		httpmock.StatusStringResponse(201, "{}"),
	)

	output, err := runCommand(http, true, "-n bugs -d \"Something isn't working\" -c \"0052cc\"")

	if err != nil {
		t.Errorf("error running command `label create`: %v", err)
	}

	assert.Equal(t, "\n✓ Label created.\n", output.Stderr())
	assert.Equal(t, "", output.String())

	bodyBytes, _ := ioutil.ReadAll(http.Requests[0].Body)
	reqBody := CreateLabelRequest{}
	err = json.Unmarshal(bodyBytes, &reqBody)
	if err != nil {
		t.Fatalf("error decoding JSON: %v", err)
	}

	expectReqBody := CreateLabelRequest{
		Name:        "bugs",
		Description: "Something isn't working",
		Color:       "0052cc",
	}

	assert.Equal(t, expectReqBody, reqBody)
}

func TestLabelCreate_tty_interactive(t *testing.T) {
	tests := []struct {
		Cli      string
		AskStubs func(*prompt.AskStubber)
	}{
		{
			Cli: "",
			AskStubs: func(as *prompt.AskStubber) {
				as.StubPrompt("Label name").AnswerWith("bugs")
				as.StubPrompt("Label description").AnswerWith("Something isn't working")
				as.StubPrompt("Label color").AnswerWith("0052cc")
			},
		},
		{
			Cli: "-d \"Something isn't working\"",
			AskStubs: func(as *prompt.AskStubber) {
				as.StubPrompt("Label name").AnswerWith("bugs")
				as.StubPrompt("Label color").AnswerWith("0052cc")
			},
		},
		{
			Cli: "-c \"0052cc\"",
			AskStubs: func(as *prompt.AskStubber) {
				as.StubPrompt("Label name").AnswerWith("bugs")
				as.StubPrompt("Label description").AnswerWith("Something isn't working")
			},
		},
		{
			Cli: "-d \"Something isn't working\" -c \"0052cc\"",
			AskStubs: func(as *prompt.AskStubber) {
				as.StubPrompt("Label name").AnswerWith("bugs")
			},
		},
	}

	for _, test := range tests {
		http := &httpmock.Registry{}
		defer http.Verify(t)

		http.Register(
			httpmock.REST("POST", "repos/OWNER/REPO/labels"),
			httpmock.StatusStringResponse(201, "{}"),
		)

		as := prompt.NewAskStubber(t)
		test.AskStubs(as)

		output, err := runCommand(http, true, test.Cli)

		if err != nil {
			t.Errorf("error running command `label create`: %v", err)
		}

		assert.Equal(t, "\n✓ Label created.\n", output.Stderr())
		assert.Equal(t, "", output.String())

		bodyBytes, _ := ioutil.ReadAll(http.Requests[0].Body)
		reqBody := CreateLabelRequest{}
		err = json.Unmarshal(bodyBytes, &reqBody)
		if err != nil {
			t.Fatalf("error decoding JSON: %v", err)
		}

		expectReqBody := CreateLabelRequest{
			Name:        "bugs",
			Description: "Something isn't working",
			Color:       "0052cc",
		}

		assert.Equal(t, expectReqBody, reqBody)
	}
}

func TestLabelCreate_random_color(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	http.Register(
		httpmock.REST("POST", "repos/OWNER/REPO/labels"),
		httpmock.StatusStringResponse(201, "{}"),
	)

	for i := 0; i < 2; i++ {

	}
	output, err := runCommand(http, false, "-n bugs")

	if err != nil {
		t.Errorf("error running command `label create`: %v", err)
	}

	assert.Equal(t, "", output.Stderr())
	assert.Equal(t, "", output.String())

	bodyBytes, _ := ioutil.ReadAll(http.Requests[0].Body)
	reqBody := CreateLabelRequest{}
	err = json.Unmarshal(bodyBytes, &reqBody)
	if err != nil {
		t.Fatalf("error decoding JSON: %v", err)
	}

	assert.Contains(t, randomColor, reqBody.Color)
}

func TestLabelCreate_failed_when_not_provide_name(t *testing.T) {
	http := &httpmock.Registry{}
	defer http.Verify(t)

	_, err := runCommand(http, false, "")

	assert.NotNil(t, err)
	assert.Equal(t, "must provide name when not running interactively", fmt.Sprintf("%v", err))
}
