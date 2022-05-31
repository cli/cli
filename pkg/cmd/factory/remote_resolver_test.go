package factory

import (
	"net/url"
	"os"
	"testing"

	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/stretchr/testify/assert"
)

type identityTranslator struct{}

func (it identityTranslator) Translate(u *url.URL) *url.URL {
	return u
}

func Test_remoteResolver(t *testing.T) {
	orig_GH_HOST := os.Getenv("GH_HOST")
	t.Cleanup(func() {
		os.Setenv("GH_HOST", orig_GH_HOST)
	})

	tests := []struct {
		name     string
		remotes  func() (git.RemoteSet, error)
		config   config.Config
		output   []string
		wantsErr bool
	}{
		{
			name: "no authenticated hosts",
			remotes: func() (git.RemoteSet, error) {
				return git.RemoteSet{
					git.NewRemote("origin", "https://github.com/owner/repo.git"),
				}, nil
			},
			config: func() config.Config {
				cfg := &config.ConfigMock{}
				cfg.HostsFunc = func() []string {
					return []string{}
				}
				cfg.DefaultHostFunc = func() (string, string) {
					return "github.com", "default"
				}
				return cfg
			}(),
			wantsErr: true,
		},
		{
			name: "no git remotes",
			remotes: func() (git.RemoteSet, error) {
				return git.RemoteSet{}, nil
			},
			config: func() config.Config {
				cfg := &config.ConfigMock{}
				cfg.HostsFunc = func() []string {
					return []string{"example.com"}
				}
				cfg.DefaultHostFunc = func() (string, string) {
					return "example.com", "hosts"
				}
				return cfg
			}(),
			wantsErr: true,
		},
		{
			name: "one authenticated host with no matching git remote and no fallback remotes",
			remotes: func() (git.RemoteSet, error) {
				return git.RemoteSet{
					git.NewRemote("origin", "https://test.com/owner/repo.git"),
				}, nil
			},
			config: func() config.Config {
				cfg := &config.ConfigMock{}
				cfg.HostsFunc = func() []string {
					return []string{"example.com"}
				}
				cfg.DefaultHostFunc = func() (string, string) {
					return "example.com", "hosts"
				}
				cfg.AuthTokenFunc = func(string) (string, string) {
					return "", ""
				}
				return cfg
			}(),
			wantsErr: true,
		},
		{
			name: "one authenticated host with no matching git remote and fallback remotes",
			remotes: func() (git.RemoteSet, error) {
				return git.RemoteSet{
					git.NewRemote("origin", "https://github.com/owner/repo.git"),
				}, nil
			},
			config: func() config.Config {
				cfg := &config.ConfigMock{}
				cfg.HostsFunc = func() []string {
					return []string{"example.com"}
				}
				cfg.DefaultHostFunc = func() (string, string) {
					return "example.com", "hosts"
				}
				return cfg
			}(),
			output: []string{"origin"},
		},
		{
			name: "one authenticated host with matching git remote",
			remotes: func() (git.RemoteSet, error) {
				return git.RemoteSet{
					git.NewRemote("origin", "https://example.com/owner/repo.git"),
				}, nil
			},
			config: func() config.Config {
				cfg := &config.ConfigMock{}
				cfg.HostsFunc = func() []string {
					return []string{"example.com"}
				}
				cfg.DefaultHostFunc = func() (string, string) {
					return "example.com", "default"
				}
				return cfg
			}(),
			output: []string{"origin"},
		},
		{
			name: "one authenticated host with multiple matching git remotes",
			remotes: func() (git.RemoteSet, error) {
				return git.RemoteSet{
					git.NewRemote("upstream", "https://example.com/owner/repo.git"),
					git.NewRemote("github", "https://example.com/owner/repo.git"),
					git.NewRemote("origin", "https://example.com/owner/repo.git"),
					git.NewRemote("fork", "https://example.com/owner/repo.git"),
				}, nil
			},
			config: func() config.Config {
				cfg := &config.ConfigMock{}
				cfg.HostsFunc = func() []string {
					return []string{"example.com"}
				}
				cfg.DefaultHostFunc = func() (string, string) {
					return "example.com", "default"
				}
				return cfg
			}(),
			output: []string{"upstream", "github", "origin", "fork"},
		},
		{
			name: "multiple authenticated hosts with no matching git remote",
			remotes: func() (git.RemoteSet, error) {
				return git.RemoteSet{
					git.NewRemote("origin", "https://test.com/owner/repo.git"),
				}, nil
			},
			config: func() config.Config {
				cfg := &config.ConfigMock{}
				cfg.HostsFunc = func() []string {
					return []string{"example.com", "github.com"}
				}
				cfg.DefaultHostFunc = func() (string, string) {
					return "github.com", "default"
				}
				cfg.AuthTokenFunc = func(string) (string, string) {
					return "", ""
				}
				return cfg
			}(),
			wantsErr: true,
		},
		{
			name: "multiple authenticated hosts with one matching git remote",
			remotes: func() (git.RemoteSet, error) {
				return git.RemoteSet{
					git.NewRemote("upstream", "https://test.com/owner/repo.git"),
					git.NewRemote("origin", "https://example.com/owner/repo.git"),
				}, nil
			},
			config: func() config.Config {
				cfg := &config.ConfigMock{}
				cfg.HostsFunc = func() []string {
					return []string{"example.com", "github.com"}
				}
				cfg.DefaultHostFunc = func() (string, string) {
					return "github.com", "default"
				}
				return cfg
			}(),
			output: []string{"origin"},
		},
		{
			name: "multiple authenticated hosts with multiple matching git remotes",
			remotes: func() (git.RemoteSet, error) {
				return git.RemoteSet{
					git.NewRemote("upstream", "https://example.com/owner/repo.git"),
					git.NewRemote("github", "https://github.com/owner/repo.git"),
					git.NewRemote("origin", "https://example.com/owner/repo.git"),
					git.NewRemote("fork", "https://github.com/owner/repo.git"),
					git.NewRemote("test", "https://test.com/owner/repo.git"),
				}, nil
			},
			config: func() config.Config {
				cfg := &config.ConfigMock{}
				cfg.HostsFunc = func() []string {
					return []string{"example.com", "github.com"}
				}
				cfg.DefaultHostFunc = func() (string, string) {
					return "github.com", "default"
				}
				return cfg
			}(),
			output: []string{"upstream", "github", "origin", "fork"},
		},
		{
			name: "override host with no matching git remotes",
			remotes: func() (git.RemoteSet, error) {
				return git.RemoteSet{
					git.NewRemote("origin", "https://example.com/owner/repo.git"),
				}, nil
			},
			config: func() config.Config {
				cfg := &config.ConfigMock{}
				cfg.HostsFunc = func() []string {
					return []string{"example.com"}
				}
				cfg.DefaultHostFunc = func() (string, string) {
					return "test.com", "GH_HOST"
				}
				return cfg
			}(),
			wantsErr: true,
		},
		{
			name: "override host with one matching git remote",
			remotes: func() (git.RemoteSet, error) {
				return git.RemoteSet{
					git.NewRemote("upstream", "https://example.com/owner/repo.git"),
					git.NewRemote("origin", "https://test.com/owner/repo.git"),
				}, nil
			},
			config: func() config.Config {
				cfg := &config.ConfigMock{}
				cfg.HostsFunc = func() []string {
					return []string{"example.com"}
				}
				cfg.DefaultHostFunc = func() (string, string) {
					return "test.com", "GH_HOST"
				}
				return cfg
			}(),
			output: []string{"origin"},
		},
		{
			name: "override host with multiple matching git remotes",
			remotes: func() (git.RemoteSet, error) {
				return git.RemoteSet{
					git.NewRemote("upstream", "https://test.com/owner/repo.git"),
					git.NewRemote("github", "https://example.com/owner/repo.git"),
					git.NewRemote("origin", "https://test.com/owner/repo.git"),
				}, nil
			},
			config: func() config.Config {
				cfg := &config.ConfigMock{}
				cfg.HostsFunc = func() []string {
					return []string{"example.com", "test.com"}
				}
				cfg.DefaultHostFunc = func() (string, string) {
					return "test.com", "GH_HOST"
				}
				return cfg
			}(),
			output: []string{"upstream", "origin"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := &remoteResolver{
				readRemotes:   tt.remotes,
				getConfig:     func() (config.Config, error) { return tt.config, nil },
				urlTranslator: identityTranslator{},
			}
			resolver := rr.Resolver()
			remotes, err := resolver()
			if tt.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			names := []string{}
			for _, r := range remotes {
				names = append(names, r.Name)
			}
			assert.Equal(t, tt.output, names)
		})
	}
}
