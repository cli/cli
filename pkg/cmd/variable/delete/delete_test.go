package delete

import (
	"bytes"
	"net/http"
	"testing"

	ghContext "github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
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
		name      string
		opts      *DeleteOptions
		host      string
		httpStubs func(*httpmock.Registry)
	}{
		{
			name: "repo",
			opts: &DeleteOptions{
				VariableName: "cool_variable",
			},
			host: "github.com",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.WithHost(httpmock.REST("DELETE", "repos/owner/repo/actions/variables/cool_variable"), "api.github.com"),
					httpmock.StatusStringResponse(204, "No Content"))
			},
		},
		{
			name: "repo GHES",
			opts: &DeleteOptions{
				VariableName: "cool_variable",
			},
			host: "example.com",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.WithHost(httpmock.REST("DELETE", "api/v3/repos/owner/repo/actions/variables/cool_variable"), "example.com"),
					httpmock.StatusStringResponse(204, "No Content"))
			},
		},
		{
			name: "env",
			opts: &DeleteOptions{
				VariableName: "cool_variable",
				EnvName:      "development",
			},
			host: "github.com",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.WithHost(httpmock.REST("DELETE", "repos/owner/repo/environments/development/variables/cool_variable"), "api.github.com"),
					httpmock.StatusStringResponse(204, "No Content"))
			},
		},
		{
			name: "env GHES",
			opts: &DeleteOptions{
				VariableName: "cool_variable",
				EnvName:      "development",
			},
			host: "example.com",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.WithHost(httpmock.REST("DELETE", "api/v3/repos/owner/repo/environments/development/variables/cool_variable"), "example.com"),
					httpmock.StatusStringResponse(204, "No Content"))
			},
		},
		{
			name: "org",
			opts: &DeleteOptions{
				VariableName: "cool_variable",
				OrgName:      "UmbrellaCorporation",
			},
			host: "github.com",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.WithHost(httpmock.REST("DELETE", "orgs/UmbrellaCorporation/actions/variables/cool_variable"), "api.github.com"),
					httpmock.StatusStringResponse(204, "No Content"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
			require.NoError(t, err)
		})
	}
}

func TestNewCmdDeleteRemoteValidation(t *testing.T) {
	tests := []struct {
		name     string
		opts     *DeleteOptions
		wantPath string
		wantsErr bool
		errMsg   string
	}{
		{
			name: "single repo detected",
			opts: &DeleteOptions{
				VariableName: "cool",
				Remotes: func() (ghContext.Remotes, error) {
					remote := &ghContext.Remote{
						Remote: &git.Remote{
							Name: "origin",
						},
						Repo: ghrepo.New("owner", "repo"),
					}

					return ghContext.Remotes{
						remote,
					}, nil
				}},
			wantPath: "repos/owner/repo/actions/variables/cool",
		},
		{
			name: "multi repo detected",
			opts: &DeleteOptions{
				VariableName: "cool",
				Remotes: func() (ghContext.Remotes, error) {
					remote := &ghContext.Remote{
						Remote: &git.Remote{
							Name: "origin",
						},
						Repo: ghrepo.New("owner", "repo"),
					}
					remote2 := &ghContext.Remote{
						Remote: &git.Remote{
							Name: "upstream",
						},
						Repo: ghrepo.New("owner", "repo"),
					}

					return ghContext.Remotes{
						remote,
						remote2,
					}, nil
				}},
			wantsErr: true,
			errMsg:   "multiple remotes detected [origin upstream]. please specify which repo to use by providing the -R or --repo argument",
		},
		{
			name: "multi repo detected - single repo given",
			opts: &DeleteOptions{
				VariableName:    "cool",
				HasRepoOverride: true,
				Remotes: func() (ghContext.Remotes, error) {
					remote := &ghContext.Remote{
						Remote: &git.Remote{
							Name: "origin",
						},
						Repo: ghrepo.New("owner", "repo"),
					}
					remote2 := &ghContext.Remote{
						Remote: &git.Remote{
							Name: "upstream",
						},
						Repo: ghrepo.New("owner", "repo"),
					}

					return ghContext.Remotes{
						remote,
						remote2,
					}, nil
				}},
			wantPath: "repos/owner/repo/actions/variables/cool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			if tt.wantPath != "" {
				reg.Register(httpmock.REST("DELETE", tt.wantPath),
					httpmock.StatusStringResponse(204, "No Content"))
			}

			ios, _, _, _ := iostreams.Test()
			tt.opts.IO = ios

			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}

			tt.opts.Config = func() (gh.Config, error) {
				return config.NewBlankConfig(), nil
			}

			tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.FromFullName("owner/repo")
			}

			err := removeRun(tt.opts)
			if tt.wantsErr {
				assert.EqualError(t, err, tt.errMsg)

				return
			}

			assert.NoError(t, err)
		})
	}
}
