package browse

import (
	"bytes"
	"io/ioutil"
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

func runCommand(rt http.RoundTripper, repo ghrepo.Interface, cli string) (*test.CmdOut, error) {
	io, _, stdout, stderr := iostreams.Test()

	browser := &cmdutil.TestBrowser{}
	factory := &cmdutil.Factory{
		IOStreams: io,
		Browser:   browser,

		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: rt}, nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return repo, nil
		},
	}

	cmd := NewCmdBrowse(factory)

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
		OutBuf:     stdout,
		ErrBuf:     stderr,
		BrowsedURL: browser.BrowsedURL(),
	}, err
}
func TestNewCmdBrowse(t *testing.T) {

	type args struct {
		repo ghrepo.Interface
		cli  string
	}

	tests := []struct {
		name           string
		args           args
		errorExpected  bool
		stdoutExpected string
		stderrExpected string
		urlExpected    string
	}{
		{
			name: "multiple flag",
			args: args{
				repo: ghrepo.New("jessica", "cli"),
				cli:  "--settings --projects",
			},
			errorExpected:  true,
			stdoutExpected: "",
			stderrExpected: "these two flags are incompatible, see below for instructions",
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
		// {
		// 	name: "branch flag",
		// 	args: args{
		// 		repo: ghrepo.New("ken", "cli"),
		// 		cli:  "--branch",
		// 	},
		// 	errorExpected:  false,
		// 	stdoutExpected: "",
		// 	stderrExpected: "Aa",
		// 	urlExpected:    "",
		// },
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			http := &httpmock.Registry{}
			defer http.Verify(t)

			_, cmdTeardown := run.Stub()
			defer cmdTeardown(t)

			output, err := runCommand(http, tt.args.repo, tt.args.cli)

			if tt.errorExpected {
				assert.Equal(t, err.Error(), tt.stderrExpected)
			} else {
				assert.Equal(t, err, nil)
			}
			assert.Equal(t, tt.stdoutExpected, output.OutBuf.String())
			assert.Equal(t, tt.urlExpected, output.BrowsedURL)
		})
	}
}
func TestFileArgParsing(t *testing.T) {
	tests := []struct {
		name           string
		arg            string
		errorExpected  bool
		fileArg        string
		lineArg        string
		stderrExpected string
	}{
		{
			name:          "non line number",
			arg:           "main.go",
			errorExpected: false,
			fileArg:       "main.go",
		},
		{
			name:          "line number",
			arg:           "main.go:32",
			errorExpected: false,
			fileArg:       "main.go",
			lineArg:       "32",
		},
		{
			name:           "non line number error",
			arg:            "ma:in.go",
			errorExpected:  true,
			stderrExpected: "invalid line number after colon\nUse 'gh browse --help' for more information about browse\n",
		},
	}
	for _, tt := range tests {
		err, arr := parseFileArg(tt.arg)
		if tt.errorExpected {
			assert.Equal(t, err.Error(), tt.stderrExpected)
		} else {
			assert.Equal(t, err, nil)
			if len(arr) > 1 {
				assert.Equal(t, tt.lineArg, arr[1])
			}
			assert.Equal(t, tt.fileArg, arr[0])
		}

	}
}
