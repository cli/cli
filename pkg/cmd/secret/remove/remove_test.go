package remove

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

func TestNewCmdRemove(t *testing.T) {
	tests := []struct {
		name     string
		cli      string
		wants    RemoveOptions
		wantsErr bool
	}{
		{
			name:     "no args",
			wantsErr: true,
		},
		{
			name: "repo",
			cli:  "cool",
			wants: RemoveOptions{
				SecretName: "cool",
			},
		},
		{
			name: "org",
			cli:  "cool --org anOrg",
			wants: RemoveOptions{
				SecretName: "cool",
				OrgName:    "anOrg",
			},
		},
		{
			name: "env",
			cli:  "cool --env anEnv",
			wants: RemoveOptions{
				SecretName: "cool",
				EnvName:    "anEnv",
			},
		},
		{
			name: "user",
			cli:  "cool -u",
			wants: RemoveOptions{
				SecretName:  "cool",
				UserSecrets: true,
			},
		},
		{
			name: "Dependabot repo",
			cli:  "cool --app Dependabot",
			wants: RemoveOptions{
				SecretName:  "cool",
				Application: "Dependabot",
			},
		},
		{
			name: "Dependabot org",
			cli:  "cool --app Dependabot --org UmbrellaCorporation",
			wants: RemoveOptions{
				SecretName:  "cool",
				OrgName:     "UmbrellaCorporation",
				Application: "Dependabot",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: io,
			}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts *RemoveOptions
			cmd := NewCmdRemove(f, func(opts *RemoveOptions) error {
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

			assert.Equal(t, tt.wants.SecretName, gotOpts.SecretName)
			assert.Equal(t, tt.wants.OrgName, gotOpts.OrgName)
			assert.Equal(t, tt.wants.EnvName, gotOpts.EnvName)
		})
	}

}

func Test_removeRun_repo(t *testing.T) {
	tests := []struct {
		name     string
		opts     *RemoveOptions
		wantPath string
	}{
		{
			name: "Actions",
			opts: &RemoveOptions{
				Application: "actions",
				SecretName:  "cool_secret",
			},
			wantPath: "repos/owner/repo/actions/secrets/cool_secret",
		},
		{
			name: "Dependabot",
			opts: &RemoveOptions{
				Application: "dependabot",
				SecretName:  "cool_dependabot_secret",
			},
			wantPath: "repos/owner/repo/dependabot/secrets/cool_dependabot_secret",
		},
		{
			name: "defaults to Actions",
			opts: &RemoveOptions{
				SecretName: "cool_secret",
			},
			wantPath: "repos/owner/repo/actions/secrets/cool_secret",
		},
	}

	for _, tt := range tests {
		reg := &httpmock.Registry{}

		reg.Register(
			httpmock.REST("DELETE", tt.wantPath),
			httpmock.StatusStringResponse(204, "No Content"))

		io, _, _, _ := iostreams.Test()

		tt.opts.IO = io
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

		reg.Verify(t)
	}
}

func Test_removeRun_env(t *testing.T) {
	reg := &httpmock.Registry{}

	reg.Register(
		httpmock.REST("DELETE", "repos/owner/repo/environments/development/secrets/cool_secret"),
		httpmock.StatusStringResponse(204, "No Content"))

	io, _, _, _ := iostreams.Test()

	opts := &RemoveOptions{
		IO: io,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		},
		Config: func() (config.Config, error) {
			return config.NewBlankConfig(), nil
		},
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.FromFullName("owner/repo")
		},
		SecretName: "cool_secret",
		EnvName:    "development",
	}

	err := removeRun(opts)
	assert.NoError(t, err)

	reg.Verify(t)
}

func Test_removeRun_org(t *testing.T) {
	tests := []struct {
		name     string
		opts     *RemoveOptions
		wantPath string
	}{
		{
			name: "org",
			opts: &RemoveOptions{
				OrgName: "UmbrellaCorporation",
			},
			wantPath: "orgs/UmbrellaCorporation/actions/secrets/tVirus",
		},
		{
			name: "Dependabot org",
			opts: &RemoveOptions{
				Application: "dependabot",
				OrgName:     "UmbrellaCorporation",
			},
			wantPath: "orgs/UmbrellaCorporation/dependabot/secrets/tVirus",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}

			reg.Register(
				httpmock.REST("DELETE", tt.wantPath),
				httpmock.StatusStringResponse(204, "No Content"))

			io, _, _, _ := iostreams.Test()

			tt.opts.Config = func() (config.Config, error) {
				return config.NewBlankConfig(), nil
			}
			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.FromFullName("owner/repo")
			}
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			tt.opts.IO = io
			tt.opts.SecretName = "tVirus"

			err := removeRun(tt.opts)
			assert.NoError(t, err)

			reg.Verify(t)

		})
	}

}

func Test_removeRun_user(t *testing.T) {
	reg := &httpmock.Registry{}

	reg.Register(
		httpmock.REST("DELETE", "user/codespaces/secrets/cool_secret"),
		httpmock.StatusStringResponse(204, "No Content"))

	io, _, _, _ := iostreams.Test()

	opts := &RemoveOptions{
		IO: io,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		},
		Config: func() (config.Config, error) {
			return config.NewBlankConfig(), nil
		},
		SecretName:  "cool_secret",
		UserSecrets: true,
	}

	err := removeRun(opts)
	assert.NoError(t, err)

	reg.Verify(t)
}
