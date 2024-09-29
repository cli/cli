package list

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdList(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
		tty     bool
	}{
		{
			name:    "happy path no arguments",
			args:    []string{},
			wantErr: false,
			tty:     false,
		},
		{
			name:    "too many arguments",
			args:    []string{"foo"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdinTTY(tt.tty)
			ios.SetStdoutTTY(tt.tty)
			ios.SetStderrTTY(tt.tty)

			f := &cmdutil.Factory{
				IOStreams: ios,
			}
			cmd := NewCmdList(f, func(*ListOptions) error {
				return nil
			})
			cmd.SetArgs(tt.args)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err := cmd.ExecuteC()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestListRun(t *testing.T) {
	tests := []struct {
		name       string
		opts       *ListOptions
		isTTY      bool
		httpStubs  func(t *testing.T, reg *httpmock.Registry)
		wantStdout string
		wantStderr string
		wantErr    bool
		errMsg     string
	}{
		{
			name:  "gitignore list tty",
			isTTY: true,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "gitignore/templates"),
					httpmock.StringResponse(`[
						"AL",
						"Actionscript",
						"Ada",
						"Agda",
						"Android",
						"AppEngine",
						"AppceleratorTitanium",
						"ArchLinuxPackages",
						"Autotools",
						"Ballerina",
						"C",
						"C++",
						"CFWheels",
						"CMake",
						"CUDA",
						"CakePHP",
						"ChefCookbook",
						"Clojure",
						"CodeIgniter",
						"CommonLisp",
						"Composer",
						"Concrete5",
						"Coq",
						"CraftCMS",
						"D"
						]`,
					))
			},
			wantStdout: heredoc.Doc(`
				GITIGNORE
				AL
				Actionscript
				Ada
				Agda
				Android
				AppEngine
				AppceleratorTitanium
				ArchLinuxPackages
				Autotools
				Ballerina
				C
				C++
				CFWheels
				CMake
				CUDA
				CakePHP
				ChefCookbook
				Clojure
				CodeIgniter
				CommonLisp
				Composer
				Concrete5
				Coq
				CraftCMS
				D
				`),
			wantStderr: "",
			opts:       &ListOptions{},
		},
		{
			name:  "gitignore list no .gitignore templates tty",
			isTTY: true,
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "gitignore/templates"),
					httpmock.StringResponse(`[]`),
				)
			},
			wantStdout: "",
			wantStderr: "",
			wantErr:    true,
			errMsg:     "No gitignore templates found",
			opts:       &ListOptions{},
		},
	}

	for _, tt := range tests {
		reg := &httpmock.Registry{}
		if tt.httpStubs != nil {
			tt.httpStubs(t, reg)
		}
		tt.opts.Config = func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		}
		tt.opts.HTTPClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}
		ios, _, stdout, stderr := iostreams.Test()
		ios.SetStdoutTTY(tt.isTTY)
		ios.SetStdinTTY(tt.isTTY)
		ios.SetStderrTTY(tt.isTTY)
		tt.opts.IO = ios

		t.Run(tt.name, func(t *testing.T) {
			defer reg.Verify(t)
			err := listRun(tt.opts)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tt.errMsg, err.Error())
				return
			}

			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}
