package factory

import (
	"net/url"
	"os"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/config"
	"github.com/stretchr/testify/assert"
)

func Test_remoteResolver(t *testing.T) {
	orig_GH_HOST := os.Getenv("GH_HOST")
	t.Cleanup(func() {
		os.Setenv("GH_HOST", orig_GH_HOST)
	})

	tests := []struct {
		name     string
		remotes  func() (git.RemoteSet, error)
		config   func() (config.Config, error)
		override string
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
			config: func() (config.Config, error) {
				return config.NewFromString(heredoc.Doc(`hosts:`)), nil
			},
			wantsErr: true,
		},
		{
			name: "no git remotes",
			remotes: func() (git.RemoteSet, error) {
				return git.RemoteSet{}, nil
			},
			config: func() (config.Config, error) {
				return config.NewFromString(heredoc.Doc(`
				  hosts:
				    example.com:
				      oauth_token: GHETOKEN
				`)), nil
			},
			wantsErr: true,
		},
		{
			name: "one authenticated host with no matching git remote and no fallback remotes",
			remotes: func() (git.RemoteSet, error) {
				return git.RemoteSet{
					git.NewRemote("origin", "https://test.com/owner/repo.git"),
				}, nil
			},
			config: func() (config.Config, error) {
				return config.NewFromString(heredoc.Doc(`
				  hosts:
				    example.com:
				      oauth_token: GHETOKEN
				`)), nil
			},
			wantsErr: true,
		},
		{
			name: "one authenticated host with no matching git remote and fallback remotes",
			remotes: func() (git.RemoteSet, error) {
				return git.RemoteSet{
					git.NewRemote("origin", "https://github.com/owner/repo.git"),
				}, nil
			},
			config: func() (config.Config, error) {
				return config.NewFromString(heredoc.Doc(`
				  hosts:
				    example.com:
				      oauth_token: GHETOKEN
				`)), nil
			},
			output: []string{"origin"},
		},
		{
			name: "one authenticated host with matching git remote",
			remotes: func() (git.RemoteSet, error) {
				return git.RemoteSet{
					git.NewRemote("upstream", "https://github.com/owner/repo.git"),
					git.NewRemote("origin", "https://example.com/owner/repo.git"),
				}, nil
			},
			config: func() (config.Config, error) {
				return config.NewFromString(heredoc.Doc(`
				  hosts:
				    example.com:
				      oauth_token: GHETOKEN
				`)), nil
			},
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
			config: func() (config.Config, error) {
				return config.NewFromString(heredoc.Doc(`
				  hosts:
				    example.com:
				      oauth_token: GHETOKEN
				`)), nil
			},
			output: []string{"upstream", "github", "origin", "fork"},
		},
		{
			name: "multiple authenticated hosts with no matching git remote",
			remotes: func() (git.RemoteSet, error) {
				return git.RemoteSet{
					git.NewRemote("origin", "https://test.com/owner/repo.git"),
				}, nil
			},
			config: func() (config.Config, error) {
				return config.NewFromString(heredoc.Doc(`
				  hosts:
				    example.com:
				      oauth_token: GHETOKEN
				    github.com:
				      oauth_token: GHTOKEN
				`)), nil
			},
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
			config: func() (config.Config, error) {
				return config.NewFromString(heredoc.Doc(`
				  hosts:
				    example.com:
				      oauth_token: GHETOKEN
				    github.com:
				      oauth_token: GHTOKEN
				`)), nil
			},
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
			config: func() (config.Config, error) {
				return config.NewFromString(heredoc.Doc(`
				  hosts:
				    example.com:
				      oauth_token: GHETOKEN
				    github.com:
				      oauth_token: GHTOKEN
				`)), nil
			},
			output: []string{"upstream", "github", "origin", "fork"},
		},
		{
			name: "override host with no matching git remotes",
			remotes: func() (git.RemoteSet, error) {
				return git.RemoteSet{
					git.NewRemote("origin", "https://example.com/owner/repo.git"),
				}, nil
			},
			config: func() (config.Config, error) {
				return config.InheritEnv(config.NewFromString(heredoc.Doc(`
				  hosts:
				    example.com:
				      oauth_token: GHETOKEN
				`))), nil
			},
			override: "test.com",
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
			config: func() (config.Config, error) {
				return config.InheritEnv(config.NewFromString(heredoc.Doc(`
				  hosts:
				    example.com:
				      oauth_token: GHETOKEN
				`))), nil
			},
			override: "test.com",
			output:   []string{"origin"},
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
			config: func() (config.Config, error) {
				return config.InheritEnv(config.NewFromString(heredoc.Doc(`
				  hosts:
				    example.com:
				      oauth_token: GHETOKEN
				`))), nil
			},
			override: "test.com",
			output:   []string{"upstream", "origin"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.override != "" {
				os.Setenv("GH_HOST", tt.override)
			}
			rr := &remoteResolver{
				readRemotes: tt.remotes,
				getConfig:   tt.config,
				urlTranslator: func(u *url.URL) *url.URL {
					return u
				},
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
