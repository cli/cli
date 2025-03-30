package factory

import (
	"errors"
	"net/url"
	"testing"

	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	ghmock "github.com/cli/cli/v2/internal/gh/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type identityTranslator struct{}

func (it identityTranslator) Translate(u *url.URL) *url.URL {
	return u
}

func Test_remoteResolver(t *testing.T) {
	tests := []struct {
		name     string
		remotes  func() (git.RemoteSet, error)
		config   gh.Config
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
			config: func() gh.Config {
				cfg := &ghmock.ConfigMock{}
				cfg.AuthenticationFunc = func() gh.AuthConfig {
					authCfg := &config.AuthConfig{}
					authCfg.SetHosts([]string{})
					authCfg.SetDefaultHost("github.com", "default")
					return authCfg
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
			config: func() gh.Config {
				cfg := &ghmock.ConfigMock{}
				cfg.AuthenticationFunc = func() gh.AuthConfig {
					authCfg := &config.AuthConfig{}
					authCfg.SetHosts([]string{"example.com"})
					authCfg.SetDefaultHost("example.com", "hosts")
					return authCfg
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
			config: func() gh.Config {
				cfg := &ghmock.ConfigMock{}
				cfg.AuthenticationFunc = func() gh.AuthConfig {
					authCfg := &config.AuthConfig{}
					authCfg.SetHosts([]string{"example.com"})
					authCfg.SetActiveToken("", "")
					authCfg.SetDefaultHost("example.com", "hosts")
					return authCfg
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
			config: func() gh.Config {
				cfg := &ghmock.ConfigMock{}
				cfg.AuthenticationFunc = func() gh.AuthConfig {
					authCfg := &config.AuthConfig{}
					authCfg.SetHosts([]string{"example.com"})
					authCfg.SetDefaultHost("example.com", "hosts")
					return authCfg
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
			config: func() gh.Config {
				cfg := &ghmock.ConfigMock{}
				cfg.AuthenticationFunc = func() gh.AuthConfig {
					authCfg := &config.AuthConfig{}
					authCfg.SetHosts([]string{"example.com"})
					authCfg.SetDefaultHost("example.com", "default")
					return authCfg
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
			config: func() gh.Config {
				cfg := &ghmock.ConfigMock{}
				cfg.AuthenticationFunc = func() gh.AuthConfig {
					authCfg := &config.AuthConfig{}
					authCfg.SetHosts([]string{"example.com"})
					authCfg.SetDefaultHost("example.com", "default")
					return authCfg
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
			config: func() gh.Config {
				cfg := &ghmock.ConfigMock{}
				cfg.AuthenticationFunc = func() gh.AuthConfig {
					authCfg := &config.AuthConfig{}
					authCfg.SetHosts([]string{"example.com", "github.com"})
					authCfg.SetActiveToken("", "")
					authCfg.SetDefaultHost("example.com", "default")
					return authCfg
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
			config: func() gh.Config {
				cfg := &ghmock.ConfigMock{}
				cfg.AuthenticationFunc = func() gh.AuthConfig {
					authCfg := &config.AuthConfig{}
					authCfg.SetHosts([]string{"example.com", "github.com"})
					authCfg.SetDefaultHost("github.com", "default")
					return authCfg
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
			config: func() gh.Config {
				cfg := &ghmock.ConfigMock{}
				cfg.AuthenticationFunc = func() gh.AuthConfig {
					authCfg := &config.AuthConfig{}
					authCfg.SetHosts([]string{"example.com", "github.com"})
					authCfg.SetDefaultHost("github.com", "default")
					return authCfg
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
			config: func() gh.Config {
				cfg := &ghmock.ConfigMock{}
				cfg.AuthenticationFunc = func() gh.AuthConfig {
					authCfg := &config.AuthConfig{}
					authCfg.SetHosts([]string{"example.com"})
					authCfg.SetDefaultHost("test.com", "GH_HOST")
					return authCfg
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
			config: func() gh.Config {
				cfg := &ghmock.ConfigMock{}
				cfg.AuthenticationFunc = func() gh.AuthConfig {
					authCfg := &config.AuthConfig{}
					authCfg.SetHosts([]string{"example.com"})
					authCfg.SetDefaultHost("test.com", "GH_HOST")
					return authCfg
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
			config: func() gh.Config {
				cfg := &ghmock.ConfigMock{}
				cfg.AuthenticationFunc = func() gh.AuthConfig {
					authCfg := &config.AuthConfig{}
					authCfg.SetHosts([]string{"example.com", "test.com"})
					authCfg.SetDefaultHost("test.com", "GH_HOST")
					return authCfg
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
				getConfig:     func() (gh.Config, error) { return tt.config, nil },
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

func Test_remoteResolver_Caching(t *testing.T) {
	t.Run("cache remotes", func(t *testing.T) {
		var readRemotesCalled bool

		rr := &remoteResolver{
			readRemotes: func() (git.RemoteSet, error) {
				if readRemotesCalled {
					return git.RemoteSet{}, errors.New("readRemotes should only be called once")
				}

				readRemotesCalled = true
				return git.RemoteSet{
					git.NewRemote("origin", "https://github.com/owner/repo.git"),
				}, nil
			},
			getConfig: func() (gh.Config, error) {
				cfg := &ghmock.ConfigMock{}
				cfg.AuthenticationFunc = func() gh.AuthConfig {
					authCfg := &config.AuthConfig{}
					authCfg.SetHosts([]string{"github.com"})
					authCfg.SetDefaultHost("github.com", "default")
					return authCfg
				}
				return cfg, nil
			},
			urlTranslator: identityTranslator{},
		}

		resolver := rr.Resolver()

		expectedRemoteNames := []string{"origin"}
		remotes, err := resolver()
		require.NoError(t, err)
		require.Equal(t, expectedRemoteNames, mapRemotesToNames(remotes))

		require.Equal(t, readRemotesCalled, true)

		cachedRemotes, err := resolver()
		require.NoError(t, err, "expected no error to be cached")
		require.Equal(t, expectedRemoteNames, mapRemotesToNames(cachedRemotes), "expected the remotes to be cached")
	})

	t.Run("cache error", func(t *testing.T) {
		var readRemotesCalled bool

		rr := &remoteResolver{
			readRemotes: func() (git.RemoteSet, error) {
				if readRemotesCalled {
					return git.RemoteSet{
						git.NewRemote("origin", "https://github.com/owner/repo.git"),
					}, nil
				}

				readRemotesCalled = true
				return git.RemoteSet{}, errors.New("error to be cached")
			},
			getConfig: func() (gh.Config, error) {
				cfg := &ghmock.ConfigMock{}
				cfg.AuthenticationFunc = func() gh.AuthConfig {
					authCfg := &config.AuthConfig{}
					authCfg.SetHosts([]string{"github.com"})
					authCfg.SetDefaultHost("github.com", "default")
					return authCfg
				}
				return cfg, nil
			},
			urlTranslator: identityTranslator{},
		}

		resolver := rr.Resolver()

		expectedErr := errors.New("error to be cached")
		remotes, err := resolver()
		require.Equal(t, expectedErr, err)
		require.Empty(t, remotes, "should return no remotes")

		require.Equal(t, readRemotesCalled, true)

		cachedRemotes, err := resolver()
		require.Equal(t, expectedErr, err, "expected the error to be cached")
		require.Empty(t, cachedRemotes, "should return no remotes")
	})
}

func mapRemotesToNames(remotes context.Remotes) []string {
	names := make([]string, len(remotes))
	for i, r := range remotes {
		names[i] = r.Name
	}
	return names
}
