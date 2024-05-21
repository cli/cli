package get

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdList(t *testing.T) {
	tests := []struct {
		name  string
		cli   string
		wants GetOptions
	}{
		{
			name: "repo",
			cli:  "FOO",
			wants: GetOptions{
				OrgName:      "",
				VariableName: "FOO",
			},
		},
		{
			name: "org",
			cli:  "-oUmbrellaCorporation BAR",
			wants: GetOptions{
				OrgName:      "UmbrellaCorporation",
				VariableName: "BAR",
			},
		},
		{
			name: "env",
			cli:  "-eDevelopment BAZ",
			wants: GetOptions{
				EnvName:      "Development",
				VariableName: "BAZ",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts *GetOptions
			cmd := NewCmdGet(f, func(opts *GetOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.OrgName, gotOpts.OrgName)
			assert.Equal(t, tt.wants.EnvName, gotOpts.EnvName)
			assert.Equal(t, tt.wants.VariableName, gotOpts.VariableName)
		})
	}
}

func Test_getRun(t *testing.T) {
	tests := []struct {
		name    string
		tty     bool
		opts    *GetOptions
		wantOut string
	}{
		{
			name: "repo tty",
			tty:  true,
			opts: &GetOptions{
				VariableName: "VARIABLE_ONE",
			},
			wantOut: "one",
		},
		{
			name: "repo not tty",
			tty:  false,
			opts: &GetOptions{
				VariableName: "VARIABLE_ONE",
			},
			wantOut: "one",
		},
		{
			name: "org tty",
			tty:  true,
			opts: &GetOptions{
				OrgName:      "UmbrellaCorporation",
				VariableName: "VARIABLE_ONE",
			},
			wantOut: "org_one",
		},
		{
			name: "org not tty",
			tty:  false,
			opts: &GetOptions{
				OrgName:      "UmbrellaCorporation",
				VariableName: "VARIABLE_ONE",
			},
			wantOut: "org_one",
		},
		{
			name: "env tty",
			tty:  true,
			opts: &GetOptions{
				EnvName:      "Development",
				VariableName: "VARIABLE_ONE",
			},
			wantOut: "one",
		},
		{
			name: "env not tty",
			tty:  false,
			opts: &GetOptions{
				EnvName:      "Development",
				VariableName: "VARIABLE_ONE",
			},
			wantOut: "one",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			path := fmt.Sprintf("repos/owner/repo/actions/variables/%s", tt.opts.VariableName)
			if tt.opts.EnvName != "" {
				path = fmt.Sprintf("repos/owner/repo/environments/%s/variables/%s", tt.opts.EnvName, tt.opts.VariableName)
			} else if tt.opts.OrgName != "" {
				path = fmt.Sprintf("orgs/%s/actions/variables/%s", tt.opts.OrgName, tt.opts.VariableName)
			}

			payload := Variable{
				Name:  "VARIABLE_ONE",
				Value: "one",
			}
			if tt.opts.OrgName != "" {
				payload = Variable{
					Name:  "VARIABLE_ONE",
					Value: "org_one",
				}
			}

			reg.Register(httpmock.REST("GET", path), httpmock.JSONResponse(payload))

			ios, _, stdout, _ := iostreams.Test()

			ios.SetStdoutTTY(tt.tty)

			tt.opts.IO = ios
			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.FromFullName("owner/repo")
			}
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			tt.opts.Config = func() (gh.Config, error) {
				return config.NewBlankConfig(), nil
			}

			err := getRun(tt.opts)
			assert.NoError(t, err)

			assert.Equal(t, tt.wantOut, stdout.String())
		})
	}
}
