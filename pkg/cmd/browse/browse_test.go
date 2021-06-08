package browse

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/cli/cli/internal/config"
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
	urlExpected    string
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
		Config: func() (config.Config, error) {
			return config.NewBlankConfig(), nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return t.args.repo, nil
		},
	}

	cmd := NewCmdBrowse(factory)

	argv, err := shlex.Split(t.args.cli)
	if err != nil {
		return nil, err
	}
	cmd.SetArgs(argv)

	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(ioutil.Discard)
	cmd.SetErr(ioutil.Discard)

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
			name: "multiple flag",
			args: args{
				repo: ghrepo.New("jessica", "cli"),
				cli:  "--settings --projects",
			},
			errorExpected:  true,
			stdoutExpected: "",
			stderrExpected: "accepts 1 flag, 2 flag(s) were recieved\nUse 'gh browse --help' for more information about browse\n",
			urlExpected:    "",
		},
		{
			name: "settings flag",
			args: args{
				repo: ghrepo.New("husrav", "cli"),
				cli:  "--settings",
			},
			errorExpected:  false,
			stdoutExpected: "now opening https://github.com/husrav/cli/settings in browser . . .\n",
			stderrExpected: "",
			urlExpected:    "https://github.com/husrav/cli/settings",
		},
		{
			name: "projects flag",
			args: args{
				repo: ghrepo.New("ben", "cli"),
				cli:  "--projects",
			},
			errorExpected:  false,
			stdoutExpected: "now opening https://github.com/ben/cli/projects in browser . . .\n",
			stderrExpected: "",
			urlExpected:    "https://github.com/ben/cli/projects",
		},
		{
			name: "wiki flag",
			args: args{
				repo: ghrepo.New("thanh", "cli"),
				cli:  "--wiki",
			},
			errorExpected:  false,
			stdoutExpected: "now opening https://github.com/thanh/cli/wiki in browser . . .\n",
			stderrExpected: "",
			urlExpected:    "https://github.com/thanh/cli/wiki",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			http := &httpmock.Registry{}
			defer http.Verify(t)

			_, cmdTeardown := run.Stub()
			defer cmdTeardown(t)

			output, err := runCommand(http, tt)

			if tt.errorExpected {
				assert.Equal(t, err.Error(), tt.stderrExpected)
			} else {
				assert.Equal(t, err, nil)
			}
			assert.Equal(t, output.OutBuf.String(), tt.stdoutExpected)
			assert.Equal(t, tt.urlExpected, output.BrowsedURL)
		})
	}
}
