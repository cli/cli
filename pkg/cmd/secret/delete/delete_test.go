package delete

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	ghContext "github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/secret/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
				SecretName: "cool",
			},
		},
		{
			name: "org",
			cli:  "cool --org anOrg",
			wants: DeleteOptions{
				SecretName: "cool",
				OrgName:    "anOrg",
			},
		},
		{
			name: "env",
			cli:  "cool --env anEnv",
			wants: DeleteOptions{
				SecretName: "cool",
				EnvName:    "anEnv",
			},
		},
		{
			name: "user",
			cli:  "cool -u",
			wants: DeleteOptions{
				SecretName:  "cool",
				UserSecrets: true,
			},
		},
		{
			name: "Dependabot repo",
			cli:  "cool --app Dependabot",
			wants: DeleteOptions{
				SecretName:  "cool",
				Application: "Dependabot",
			},
		},
		{
			name: "Dependabot org",
			cli:  "cool --app Dependabot --org UmbrellaCorporation",
			wants: DeleteOptions{
				SecretName:  "cool",
				OrgName:     "UmbrellaCorporation",
				Application: "Dependabot",
			},
		},
		{
			name: "Codespaces org",
			cli:  "cool --app codespaces --org UmbrellaCorporation",
			wants: DeleteOptions{
				SecretName:  "cool",
				OrgName:     "UmbrellaCorporation",
				Application: "Codespaces",
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

			assert.Equal(t, tt.wants.SecretName, gotOpts.SecretName)
			assert.Equal(t, tt.wants.OrgName, gotOpts.OrgName)
			assert.Equal(t, tt.wants.EnvName, gotOpts.EnvName)
		})
	}
}

func TestNewCmdDeleteBaseRepoFuncs(t *testing.T) {
	remotes := ghContext.Remotes{
		&ghContext.Remote{
			Remote: &git.Remote{
				Name: "origin",
			},
			Repo: ghrepo.New("owner", "fork"),
		},
		&ghContext.Remote{
			Remote: &git.Remote{
				Name: "upstream",
			},
			Repo: ghrepo.New("owner", "repo"),
		},
	}

	tests := []struct {
		name          string
		args          string
		prompterStubs func(*prompter.MockPrompter)
		wantRepo      ghrepo.Interface
		wantErr       error
	}{
		{
			name:     "when there is a repo flag provided, the factory base repo func is used",
			args:     "SECRET_NAME --repo owner/repo",
			wantRepo: ghrepo.New("owner", "repo"),
		},
		{
			name: "when there is no repo flag provided, and no prompting, the base func requiring no ambiguity is used",
			args: "SECRET_NAME",
			wantErr: shared.AmbiguousBaseRepoError{
				Remotes: remotes,
			},
		},
		{
			name: "when there is no repo flag provided, and can prompt, the base func resolving ambiguity is used",
			args: "SECRET_NAME",
			prompterStubs: func(pm *prompter.MockPrompter) {
				pm.RegisterSelect(
					"Select a repo",
					[]string{"owner/fork", "owner/repo"},
					func(_, _ string, opts []string) (int, error) {
						return prompter.IndexFor(opts, "owner/fork")
					},
				)
			},
			wantRepo: ghrepo.New("owner", "fork"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			var pm *prompter.MockPrompter
			if tt.prompterStubs != nil {
				ios.SetStdinTTY(true)
				ios.SetStdoutTTY(true)
				ios.SetStderrTTY(true)
				pm = prompter.NewMockPrompter(t)
				tt.prompterStubs(pm)
			}

			f := &cmdutil.Factory{
				IOStreams: ios,
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.FromFullName("owner/repo")
				},
				Prompter: pm,
				Remotes: func() (ghContext.Remotes, error) {
					return remotes, nil
				},
			}

			argv, err := shlex.Split(tt.args)
			assert.NoError(t, err)

			var gotOpts *DeleteOptions
			cmd := NewCmdDelete(f, func(opts *DeleteOptions) error {
				gotOpts = opts
				return nil
			})
			// Require to support --repo flag
			cmdutil.EnableRepoOverride(cmd, f)
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)

			_, err = cmd.ExecuteC()
			require.NoError(t, err)

			baseRepo, err := gotOpts.BaseRepo()
			if tt.wantErr != nil {
				require.Equal(t, tt.wantErr, err)
				return
			}
			require.True(t, ghrepo.IsSame(tt.wantRepo, baseRepo))
		})
	}
}

func Test_removeRun_repo(t *testing.T) {
	tests := []struct {
		name      string
		opts      *DeleteOptions
		host      string
		httpStubs func(*httpmock.Registry)
	}{
		{
			name: "Actions",
			opts: &DeleteOptions{
				Application: "actions",
				SecretName:  "cool_secret",
			},
			host: "github.com",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.WithHost(httpmock.REST("DELETE", "repos/owner/repo/actions/secrets/cool_secret"), "api.github.com"), httpmock.StatusStringResponse(204, "No Content"))
			},
		},
		{
			name: "Actions GHES",
			opts: &DeleteOptions{
				Application: "actions",
				SecretName:  "cool_secret",
			},
			host: "example.com",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.WithHost(httpmock.REST("DELETE", "api/v3/repos/owner/repo/actions/secrets/cool_secret"), "example.com"), httpmock.StatusStringResponse(204, "No Content"))
			},
		},
		{
			name: "Dependabot",
			opts: &DeleteOptions{
				Application: "dependabot",
				SecretName:  "cool_dependabot_secret",
			},
			host: "github.com",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.WithHost(httpmock.REST("DELETE", "repos/owner/repo/dependabot/secrets/cool_dependabot_secret"), "api.github.com"), httpmock.StatusStringResponse(204, "No Content"))
			},
		},
		{
			name: "Dependabot GHES",
			opts: &DeleteOptions{
				Application: "dependabot",
				SecretName:  "cool_dependabot_secret",
			},
			host: "example.com",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.WithHost(httpmock.REST("DELETE", "api/v3/repos/owner/repo/dependabot/secrets/cool_dependabot_secret"), "example.com"), httpmock.StatusStringResponse(204, "No Content"))
			},
		},
		{
			name: "defaults to Actions",
			opts: &DeleteOptions{
				SecretName: "cool_secret",
			},
			host: "github.com",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.WithHost(httpmock.REST("DELETE", "repos/owner/repo/actions/secrets/cool_secret"), "api.github.com"), httpmock.StatusStringResponse(204, "No Content"))
			},
		},
	}

	for _, tt := range tests {
		reg := &httpmock.Registry{}
		tt.httpStubs(reg)
		defer reg.Verify(t)

		ios, _, _, _ := iostreams.Test()

		tt.opts.IO = ios
		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}
		tt.opts.Config = func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		}
		tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
			return ghrepo.FromFullNameWithHost("owner/repo", tt.host)
		}

		err := removeRun(tt.opts)
		assert.NoError(t, err)
	}
}

func Test_removeRun_env(t *testing.T) {
	tests := []struct {
		name      string
		opts      *DeleteOptions
		host      string
		httpStubs func(*httpmock.Registry)
	}{
		{
			name: "delete environment secret",
			opts: &DeleteOptions{
				SecretName: "cool_secret",
				EnvName:    "development",
			},
			host: "github.com",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.WithHost(httpmock.REST("DELETE", "repos/owner/repo/environments/development/secrets/cool_secret"), "api.github.com"),
					httpmock.StatusStringResponse(204, "No Content"))
			},
		},
		{
			name: "delete environment secret GHES",
			opts: &DeleteOptions{
				SecretName: "cool_secret",
				EnvName:    "development",
			},
			host: "example.com",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.WithHost(httpmock.REST("DELETE", "api/v3/repos/owner/repo/environments/development/secrets/cool_secret"), "example.com"),
					httpmock.StatusStringResponse(204, "No Content"))
			},
		},
	}

	for _, tt := range tests {
		reg := &httpmock.Registry{}
		tt.httpStubs(reg)
		defer reg.Verify(t)

		ios, _, _, _ := iostreams.Test()
		tt.opts.IO = ios
		tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
			if tt.host != "" {
				return ghrepo.FromFullNameWithHost("owner/repo", tt.host)
			}
			return ghrepo.FromFullName("owner/repo")
		}
		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}
		tt.opts.Config = func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		}

		err := removeRun(tt.opts)
		require.NoError(t, err)
	}
}

func Test_removeRun_org(t *testing.T) {
	tests := []struct {
		name     string
		opts     *DeleteOptions
		wantPath string
	}{
		{
			name: "org",
			opts: &DeleteOptions{
				OrgName: "UmbrellaCorporation",
			},
			wantPath: "orgs/UmbrellaCorporation/actions/secrets/tVirus",
		},
		{
			name: "Dependabot org",
			opts: &DeleteOptions{
				Application: "dependabot",
				OrgName:     "UmbrellaCorporation",
			},
			wantPath: "orgs/UmbrellaCorporation/dependabot/secrets/tVirus",
		},
		{
			name: "Codespaces org",
			opts: &DeleteOptions{
				Application: "codespaces",
				OrgName:     "UmbrellaCorporation",
			},
			wantPath: "orgs/UmbrellaCorporation/codespaces/secrets/tVirus",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}

			reg.Register(
				httpmock.REST("DELETE", tt.wantPath),
				httpmock.StatusStringResponse(204, "No Content"))

			ios, _, _, _ := iostreams.Test()

			tt.opts.Config = func() (gh.Config, error) {
				return config.NewBlankConfig(), nil
			}
			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.FromFullName("owner/repo")
			}
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			tt.opts.IO = ios
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

	ios, _, _, _ := iostreams.Test()

	opts := &DeleteOptions{
		IO: ios,
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		},
		Config: func() (gh.Config, error) {
			return config.NewBlankConfig(), nil
		},
		SecretName:  "cool_secret",
		UserSecrets: true,
	}

	err := removeRun(opts)
	assert.NoError(t, err)

	reg.Verify(t)
}
