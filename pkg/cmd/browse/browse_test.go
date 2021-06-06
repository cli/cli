package browse

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/test"
	"github.com/google/shlex"
	"github.com/spf13/cobra"
)

func TestNewCmdBrowse(t *testing.T) {

	type args struct {
		repo ghrepo.Interface
		cli  string
	}
	tests := []struct {
		name         	string
		args         	args
		errorExpected 	error
		stdoutExpected 	string
		stderrExpected 	string
	}{
		name: "test1",
		arg : args{
			repo: ghrepo.New("bchadwic","cli"),
			cli: "--settings",
		},
		errorExpected: nil,
		stdoutExpected: "now opening https://github.com/bchadwic/cli/settings in browser . . .\n",
		stderrExpected: "",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, stdout, stderr := iostreams.Test()
			//	tt.topic

			factory := &cmdutil.Factory{
				IOStreams: io,
				//os.Getenv("BROWSER")
				Browser: cmdutil.NewBrowser("", stdout, stderr),
				HttpClient: func() (*http.Client, error) {
					return &http.Client{Transport: rt}, nil
				},
				BaseRepo: tt.args.repo,
				},
			}
			cmd := NewCmdBrowse(factory)

			argv, err := shlex.Split(cli)
			if err != nil {
				return 
			}
			cmd.SetArgs(argv)
			cmd.SetOut(stdout)
			cmd.SetErr(stderr)

			_, err := cmd.ExecuteC()
			assert.Error(t, err)
			if stdoutExpected != "" {
				assert.Contains(t, stdout.String(), tt.outputExpected) // success outputs
			}
			if stderrExpected != "" {
				assert.Contains(t, stderr.String(), tt.outputExpected) // error outputs
			}
		})
	}
}



// func runCommand(isTTY bool, cli string) (*test.CmdOut, error) {
// 	io, _, stdout, stderr := iostreams.Test()
// 	io.SetStdoutTTY(isTTY)
// 	io.SetStdinTTY(isTTY)
// 	io.SetStderrTTY(isTTY)

// 	factory := &cmdutil.Factory{
// 		IOStreams: io,
// 		HttpClient: func() (*http.Client, error) {
// 			return &http.Client{Transport: rt}, nil
// 		},
// 		Config: func() (config.Config, error) {
// 			return config.NewBlankConfig(), nil
// 		},
// 		BaseRepo: func() (ghrepo.Interface, error) {
// 			return ghrepo.New("OWNER", "REPO"), nil
// 		},
// 	}

// 	cmd := NewCmdBrowse(factory)

// 	argv, err := shlex.Split(cli)
// 	if err != nil {
// 		return nil, err
// 	}
// 	cmd.SetArgs(argv)

// 	cmd.SetIn(&bytes.Buffer{})
// 	cmd.SetOut(ioutil.Discard)
// 	cmd.SetErr(ioutil.Discard)

// 	_, err = cmd.ExecuteC()
// 	return &test.CmdOut{
// 		OutBuf: stdout,
// 		ErrBuf: stderr,
// 	}, err
// }

// func TestBrowseOpen(t *testing.T) {
// 	runCommand(true, "")
// }

// func Test_browseList(t *testing.T) {
// 	for _, test := range tests {
// 		arg = test.args
// 		cmd := createCommand(arg.repo, arg.cli)
// 		err := cmd.RunE();
// 	}
// }

// func createCommand(repo ghrepo.Interface, cli string) *cobra.Command {
// 	io, _, stdout, stderr := iostreams.Test()
// 	io.SetStdoutTTY(false)
// 	io.SetStdinTTY(false) // Ask the team about TTY
// 	io.SetStderrTTY(false)

// 	factory := &cmdutil.Factory{
// 		IOStreams: io,
// 		Config: func() (config.Config, error) {
// 			return config.NewBlankConfig(), nil
// 		},
// 		BaseRepo: repo
// 		},
// 	}

// 	cmd := NewCmdBrowse(factory, nil)

// 	argv, err := shlex.Split(cli)
// 	if err != nil {
// 		return nil, err
// 	}
// 	cmd.SetArgs(argv)

// 	cmd.SetIn(&bytes.Buffer{})
// 	cmd.SetOut(ioutil.Discard)
// 	cmd.SetErr(ioutil.Discard)

// 	_, err = cmd.RunE()
// 	return &test.CmdOut{
// 		OutBuf: stdout,
// 		ErrBuf: stderr,
// 	}, err
// }
