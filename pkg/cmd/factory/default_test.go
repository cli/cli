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
			f.BaseRepo = BaseRepoFunc(f)
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

// Defined in pkg/cmdutil/repo_override.go but test it along with other BaseRepo functions
func Test_OverrideBaseRepo(t *testing.T) {
	orig_GH_HOST := os.Getenv("GH_REPO")
	t.Cleanup(func() {
		os.Setenv("GH_REPO", orig_GH_HOST)
	})

	tests := []struct {
		name        string
		remotes     git.RemoteSet
		config      config.Config
		envOverride string
		argOverride string
		wantsErr    bool
		wantsName   string
		wantsOwner  string
		wantsHost   string
	}{
		{
			name:        "override from argument",
			argOverride: "override/test",
			wantsHost:   "github.com",
			wantsOwner:  "override",
			wantsName:   "test",
		},
		{
			name:        "override from environment",
			envOverride: "somehost.com/override/test",
			wantsHost:   "somehost.com",
			wantsOwner:  "override",
			wantsName:   "test",
		},
		{
			name: "no override",
			remotes: git.RemoteSet{
				git.NewRemote("origin", "https://nonsense.com/owner/repo.git"),
			},
			config:     defaultConfig(),
			wantsHost:  "nonsense.com",
			wantsOwner: "owner",
			wantsName:  "repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envOverride != "" {
				os.Setenv("GH_REPO", tt.envOverride)
			} else {
				os.Unsetenv("GH_REPO")
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
			f.BaseRepo = cmdutil.OverrideBaseRepoFunc(f, tt.argOverride)
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

func Test_ioStreams_pager(t *testing.T) {
	tests := []struct {
		name      string
		env       map[string]string
		config    config.Config
		wantPager string
	}{
		{
			name: "GH_PAGER and PAGER set",
			env: map[string]string{
				"GH_PAGER": "GH_PAGER",
				"PAGER":    "PAGER",
			},
			wantPager: "GH_PAGER",
		},
		{
			name: "GH_PAGER and config pager set",
			env: map[string]string{
				"GH_PAGER": "GH_PAGER",
			},
			config:    pagerConfig(),
			wantPager: "GH_PAGER",
		},
		{
			name: "config pager and PAGER set",
			env: map[string]string{
				"PAGER": "PAGER",
			},
			config:    pagerConfig(),
			wantPager: "CONFIG_PAGER",
		},
		{
			name: "only PAGER set",
			env: map[string]string{
				"PAGER": "PAGER",
			},
			wantPager: "PAGER",
		},
		{
			name: "GH_PAGER set to blank string",
			env: map[string]string{
				"GH_PAGER": "",
				"PAGER":    "PAGER",
			},
			wantPager: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != nil {
				for k, v := range tt.env {
					old := os.Getenv(k)
					os.Setenv(k, v)
					if k == "GH_PAGER" {
						defer os.Unsetenv(k)
					} else {
						defer os.Setenv(k, old)
					}
				}
			}
			f := New("1")
			f.Config = func() (config.Config, error) {
				if tt.config == nil {
					return config.NewBlankConfig(), nil
				} else {
					return tt.config, nil
				}
			}
			io := ioStreams(f)
			assert.Equal(t, tt.wantPager, io.GetPager())
		})
	}
}

func Test_ioStreams_prompt(t *testing.T) {
	tests := []struct {
		name           string
		config         config.Config
		promptDisabled bool
	}{
		{
			name:           "default config",
			promptDisabled: false,
		},
		{
			name:           "config with prompt disabled",
			config:         disablePromptConfig(),
			promptDisabled: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := New("1")
			f.Config = func() (config.Config, error) {
				if tt.config == nil {
					return config.NewBlankConfig(), nil
				} else {
					return tt.config, nil
				}
			}
			io := ioStreams(f)
			assert.Equal(t, tt.promptDisabled, io.GetNeverPrompt())
		})
	}
}

func Test_browserLauncher(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		config      config.Config
		wantBrowser string
	}{
		{
			name: "GH_BROWSER set",
			env: map[string]string{
				"GH_BROWSER": "GH_BROWSER",
			},
			wantBrowser: "GH_BROWSER",
		},
		{
			name:        "config browser set",
			config:      config.NewFromString("browser: CONFIG_BROWSER"),
			wantBrowser: "CONFIG_BROWSER",
		},
		{
			name: "BROWSER set",
			env: map[string]string{
				"BROWSER": "BROWSER",
			},
			wantBrowser: "BROWSER",
		},
		{
			name: "GH_BROWSER and config browser set",
			env: map[string]string{
				"GH_BROWSER": "GH_BROWSER",
			},
			config:      config.NewFromString("browser: CONFIG_BROWSER"),
			wantBrowser: "GH_BROWSER",
		},
		{
			name: "config browser and BROWSER set",
			env: map[string]string{
				"BROWSER": "BROWSER",
			},
			config:      config.NewFromString("browser: CONFIG_BROWSER"),
			wantBrowser: "CONFIG_BROWSER",
		},
		{
			name: "GH_BROWSER and BROWSER set",
			env: map[string]string{
				"BROWSER":    "BROWSER",
				"GH_BROWSER": "GH_BROWSER",
			},
			wantBrowser: "GH_BROWSER",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != nil {
				for k, v := range tt.env {
					old := os.Getenv(k)
					os.Setenv(k, v)
					defer os.Setenv(k, old)
				}
			}
			f := New("1")
			f.Config = func() (config.Config, error) {
				if tt.config == nil {
					return config.NewBlankConfig(), nil
				} else {
					return tt.config, nil
				}
			}
			browser := browserLauncher(f)
			assert.Equal(t, tt.wantBrowser, browser)
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

func pagerConfig() config.Config {
	return config.NewFromString("pager: CONFIG_PAGER")
}

func disablePromptConfig() config.Config {
	return config.NewFromString("prompt: disabled")
}
