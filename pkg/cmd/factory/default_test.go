package factory

import (
	"net/url"
	"os"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/stretchr/testify/assert"
)

func Test_BaseRepo(t *testing.T) {
	orig_GH_HOST := os.Getenv("GH_HOST")
	t.Cleanup(func() {
		os.Setenv("GH_HOST", orig_GH_HOST)
	})

	tests := []struct {
		name       string
		remotes    git.RemoteSet
		config     config.Config
		override   string
		wantsErr   bool
		wantsName  string
		wantsOwner string
		wantsHost  string
	}{
		{
			name: "matching remote",
			remotes: git.RemoteSet{
				git.NewRemote("origin", "https://nonsense.com/owner/repo.git"),
			},
			config:     defaultConfig(),
			wantsName:  "repo",
			wantsOwner: "owner",
			wantsHost:  "nonsense.com",
		},
		{
			name: "no matching remote",
			remotes: git.RemoteSet{
				git.NewRemote("origin", "https://test.com/owner/repo.git"),
			},
			config:   defaultConfig(),
			wantsErr: true,
		},
		{
			name: "override with matching remote",
			remotes: git.RemoteSet{
				git.NewRemote("origin", "https://test.com/owner/repo.git"),
			},
			config:     defaultConfig(),
			override:   "test.com",
			wantsName:  "repo",
			wantsOwner: "owner",
			wantsHost:  "test.com",
		},
		{
			name: "override with no matching remote",
			remotes: git.RemoteSet{
				git.NewRemote("origin", "https://nonsense.com/owner/repo.git"),
			},
			config:   defaultConfig(),
			override: "test.com",
			wantsErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.override != "" {
				os.Setenv("GH_HOST", tt.override)
			} else {
				os.Unsetenv("GH_HOST")
			}
			f := New("1")
			rr := &remoteResolver{
				readRemotes: func() (git.RemoteSet, error) {
					return tt.remotes, nil
				},
				getConfig: func() (config.Config, error) {
					return tt.config, nil
				},
			}
			f.Remotes = rr.Resolver()
			f.BaseRepo = baseRepoFunc(f)
			repo, err := f.BaseRepo()
			if tt.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantsName, repo.RepoName())
			assert.Equal(t, tt.wantsOwner, repo.RepoOwner())
			assert.Equal(t, tt.wantsHost, repo.RepoHost())
		})
	}
}

func Test_SmartBaseRepo(t *testing.T) {
	pu, _ := url.Parse("https://test.com/newowner/newrepo.git")
	orig_GH_HOST := os.Getenv("GH_HOST")
	t.Cleanup(func() {
		os.Setenv("GH_HOST", orig_GH_HOST)
	})

	tests := []struct {
		name       string
		remotes    git.RemoteSet
		config     config.Config
		override   string
		wantsErr   bool
		wantsName  string
		wantsOwner string
		wantsHost  string
	}{
		{
			name: "override with matching remote",
			remotes: git.RemoteSet{
				git.NewRemote("origin", "https://test.com/owner/repo.git"),
			},
			config:     defaultConfig(),
			override:   "test.com",
			wantsName:  "repo",
			wantsOwner: "owner",
			wantsHost:  "test.com",
		},
		{
			name: "override with matching remote and base resolution",
			remotes: git.RemoteSet{
				&git.Remote{Name: "origin",
					Resolved: "base",
					FetchURL: pu,
					PushURL:  pu},
			},
			config:     defaultConfig(),
			override:   "test.com",
			wantsName:  "newrepo",
			wantsOwner: "newowner",
			wantsHost:  "test.com",
		},
		{
			name: "override with matching remote and nonbase resolution",
			remotes: git.RemoteSet{
				&git.Remote{Name: "origin",
					Resolved: "johnny/test",
					FetchURL: pu,
					PushURL:  pu},
			},
			config:     defaultConfig(),
			override:   "test.com",
			wantsName:  "test",
			wantsOwner: "johnny",
			wantsHost:  "test.com",
		},
		{
			name: "override with no matching remote",
			remotes: git.RemoteSet{
				git.NewRemote("origin", "https://example.com/owner/repo.git"),
			},
			config:   defaultConfig(),
			override: "test.com",
			wantsErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.override != "" {
				os.Setenv("GH_HOST", tt.override)
			} else {
				os.Unsetenv("GH_HOST")
			}
			f := New("1")
			rr := &remoteResolver{
				readRemotes: func() (git.RemoteSet, error) {
					return tt.remotes, nil
				},
				getConfig: func() (config.Config, error) {
					return tt.config, nil
				},
			}
			f.Remotes = rr.Resolver()
			f.BaseRepo = SmartBaseRepoFunc(f)
			repo, err := f.BaseRepo()
			if tt.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantsName, repo.RepoName())
			assert.Equal(t, tt.wantsOwner, repo.RepoOwner())
			assert.Equal(t, tt.wantsHost, repo.RepoHost())
		})
	}
}

// Defined in cmdutil repo_override but test it along with other BaseRepo functions
func Test_OverrideBaseRepo(t *testing.T) {
	orig_GH_HOST := os.Getenv("GH_HOST")
	t.Cleanup(func() {
		os.Setenv("GH_HOST", orig_GH_HOST)
	})

	tests := []struct {
		name       string
		config     config.Config
		host       string
		override   string
		wantsErr   bool
		wantsName  string
		wantsOwner string
		wantsHost  string
	}{
		{
			name:       "host, owner, and repo override",
			config:     defaultConfig(),
			host:       "test.com",
			override:   "owner/repo",
			wantsName:  "repo",
			wantsOwner: "owner",
			wantsHost:  "test.com",
		},
		{
			name:       "owner, and repo override",
			config:     defaultConfig(),
			override:   "owner/repo",
			wantsName:  "repo",
			wantsOwner: "owner",
			wantsHost:  "nonsense.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.host != "" {
				os.Setenv("GH_HOST", tt.host)
			} else {
				os.Unsetenv("GH_HOST")
			}
			f := New("1")
			f.Config = func() (config.Config, error) {
				return tt.config, nil
			}
			f.BaseRepo = cmdutil.OverrideBaseRepoFunc(f, tt.override)
			repo, err := f.BaseRepo()
			if tt.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantsName, repo.RepoName())
			assert.Equal(t, tt.wantsOwner, repo.RepoOwner())
			assert.Equal(t, tt.wantsHost, repo.RepoHost())
		})
	}
}

func defaultConfig() config.Config {
	return config.InheritEnv(config.NewFromString(heredoc.Doc(`
    hosts:
      nonsense.com:
        oauth_token: BLAH
		`)))
}
