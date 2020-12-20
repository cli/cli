package factory

import (
	"net/url"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_remoteResolver(t *testing.T) {
	rr := &remoteResolver{
		readRemotes: func() (git.RemoteSet, error) {
			return git.RemoteSet{
				git.NewRemote("fork", "https://example.org/ghe-owner/ghe-fork.git"),
				git.NewRemote("origin", "https://github.com/owner/repo.git"),
				git.NewRemote("upstream", "https://example.org/ghe-owner/ghe-repo.git"),
			}, nil
		},
		getConfig: func() (config.Config, error) {
			return config.NewFromString(heredoc.Doc(`
				hosts:
				  example.org:
				    oauth_token: GHETOKEN
			`)), nil
		},
		urlTranslator: func(u *url.URL) *url.URL {
			return u
		},
	}

	resolver := rr.Resolver("")
	remotes, err := resolver()
	require.NoError(t, err)
	require.Equal(t, 2, len(remotes))

	assert.Equal(t, "fork", remotes[0].Name)
	assert.Equal(t, "upstream", remotes[1].Name)
}

func Test_remoteResolverPriority(t *testing.T) {
	rr := &remoteResolver{
		readRemotes: func() (git.RemoteSet, error) {
			return git.RemoteSet{
				git.NewRemote("origin", "https://github.com/origin-owner/repo.git"),
				git.NewRemote("github", "https://github.com/gh-owner/gh-repo.git"),
				git.NewRemote("custom", "https://github.com/foo/bar.git"),
				git.NewRemote("upstream", "https://github.com/upstream-owner/repo.git"),
			}, nil
		},
		getConfig: func() (config.Config, error) {
			return config.NewFromString(heredoc.Doc(`
				hosts:
				  example.org:
				    oauth_token: GHETOKEN
			`)), nil
		},
		urlTranslator: func(u *url.URL) *url.URL {
			return u
		},
	}

	resolver := rr.Resolver("")
	remotes, err := resolver()
	require.NoError(t, err)
	require.Equal(t, 4, len(remotes))

	assert.Equal(t, "custom", remotes[0].Name)
	assert.Equal(t, "origin", remotes[1].Name)
	assert.Equal(t, "github", remotes[2].Name)
	assert.Equal(t, "upstream", remotes[3].Name)
}

func Test_remoteResolverOverride(t *testing.T) {
	rr := &remoteResolver{
		readRemotes: func() (git.RemoteSet, error) {
			return git.RemoteSet{
				git.NewRemote("fork", "https://example.org/ghe-owner/ghe-fork.git"),
				git.NewRemote("origin", "https://github.com/owner/repo.git"),
				git.NewRemote("upstream", "https://example.org/ghe-owner/ghe-repo.git"),
			}, nil
		},
		getConfig: func() (config.Config, error) {
			return config.NewFromString(heredoc.Doc(`
				hosts:
				  example.org:
				    oauth_token: GHETOKEN
			`)), nil
		},
		urlTranslator: func(u *url.URL) *url.URL {
			return u
		},
	}

	resolver := rr.Resolver("github.com")
	remotes, err := resolver()
	require.NoError(t, err)
	require.Equal(t, 1, len(remotes))

	assert.Equal(t, "origin", remotes[0].Name)
}
