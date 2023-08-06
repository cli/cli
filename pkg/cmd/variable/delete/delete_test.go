package delete

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdDelete(t *testing.T) {
	tests := []struct {
		name     string
		cli      string
		wants    DeleteOptions
		wantsErr bool
	}{
		{
			name:     "no args",
			wantsErr: true,
		},
		{
			name: "repo",
			cli:  "cool",
			wants: DeleteOptions{
				VariableName: "cool",
			},
		},
		{
			name: "org",
			cli:  "cool --org anOrg",
			wants: DeleteOptions{
				VariableName: "cool",
				OrgName:      "anOrg",
			},
		},
		{
			name: "env",
			cli:  "cool --env anEnv",
			wants: DeleteOptions{
				VariableName: "cool",
				EnvName:      "anEnv",
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

			var gotOpts *DeleteOptions
			cmd := NewCmdDelete(f, func(opts *DeleteOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.VariableName, gotOpts.VariableName)
			assert.Equal(t, tt.wants.OrgName, gotOpts.OrgName)
			assert.Equal(t, tt.wants.EnvName, gotOpts.EnvName)
		})
	}
}

func TestRemoveRun(t *testing.T) {
	tests := []struct {
		name     string
		opts     *DeleteOptions
		wantPath string
	}{
		{
			name: "repo",
			opts: &DeleteOptions{
				VariableName: "cool_variable",
			},
			wantPath: "repos/owner/repo/actions/variables/cool_variable",
		},
		{
			name: "env",
			opts: &DeleteOptions{
				VariableName: "cool_variable",
				EnvName:      "development",
			},
			wantPath: "repos/owner/repo/environments/development/variables/cool_variable",
		},
		{
			name: "org",
			opts: &DeleteOptions{
				VariableName: "cool_variable",
				OrgName:      "UmbrellaCorporation",
			},
			wantPath: "orgs/UmbrellaCorporation/actions/variables/cool_variable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			reg.Register(httpmock.REST("DELETE", tt.wantPath),
				httpmock.StatusStringResponse(204, "No Content"))

			ios, _, _, _ := iostreams.Test()
			tt.opts.IO = ios

			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}

			tt.opts.Config = func() (config.Config, error) {
				return config.NewBlankConfig(), nil
			}

			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.FromFullName("owner/repo")
			}

			err := removeRun(tt.opts)
			assert.NoError(t, err)
		})
	}
}
