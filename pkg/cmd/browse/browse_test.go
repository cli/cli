package browse

import (
	"bytes"
	//"io/ioutil"
	"net/http"
	"testing"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/internal/run"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

type args struct {
	repo ghrepo.Interface
	cli  string
}
type testCase struct {
	name           string
	args           args
	errorExpected  bool
	stdoutExpected string
	stderrExpected string
}

func runCommand(rt http.RoundTripper, t testCase) (*test.CmdOut, error) {
	io, _, stdout, stderr := iostreams.Test()

	browser := &cmdutil.TestBrowser{}
	factory := &cmdutil.Factory{
		IOStreams: io,
		Browser:   browser,

		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: rt}, nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
	}

	cmd := NewCmdBrowse(factory)

	argv, err := shlex.Split(t.args.cli)
	if err != nil {
		return nil, err
	}
	cmd.SetArgs(argv)

	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	_, err = cmd.ExecuteC()
	return &test.CmdOut{
		OutBuf:     stdout,
		ErrBuf:     stderr,
		BrowsedURL: browser.BrowsedURL(),
	}, err
}
func TestNewCmdBrowse(t *testing.T) {

	var tests = []testCase{
		{
			name: "test1",
			args: args{
				repo: ghrepo.New("bchadwic", "cli"),
				cli:  "--settings --projects",
			},
			errorExpected:  true,
			stdoutExpected: "",
			stderrExpected: "Error: accepts 1 flag, 2 flag(s) were recieved\nUse 'gh browse --help' for more information about browse\n\n",
		},
		// {
		// 	name: "test2",
		// 	args: args{
		// 		repo: ghrepo.New("bchadwic", "cli"),
		// 		cli:  "--settings",
		// 	},
		// 	errorExpected:  false,
		// 	stdoutExpected: "hello world",
		// 	stderrExpected: "hello world",
		// },
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			http := &httpmock.Registry{}
			defer http.Verify(t)

			_, cmdTeardown := run.Stub()
			defer cmdTeardown(t)

			output, err := runCommand(http, tt)

			if tt.errorExpected {
				assert.Error(t, err)
			}

			assert.Contains(t, output.OutBuf.String(), tt.stdoutExpected) // success outputs

			assert.Contains(t, output.ErrBuf.String(), tt.stderrExpected) // error outputs

		})
	}
}

// http := initFakeHTTP()
// defer http.Verify(t)

// _, cmdTeardown := run.Stub()
// defer cmdTeardown(t)

// output, err := runCommand(http, true, "--web -a peter -l bug -l docs -L 10 -s merged -B trunk")
// if err != nil {
// 	t.Errorf("error running command `pr list` with `--web` flag: %v", err)
// }

// assert.Equal(t, "", output.String())
// assert.Equal(t, "Opening github.com/OWNER/REPO/pulls in your browser.\n", output.Stderr())
// assert.Equal(t, "https://github.com/OWNER/REPO/pulls?q=is%3Apr+is%3Amerged+assignee%3Apeter+label%3Abug+label%3Adocs+base%3Atrunk", output.BrowsedURL)
