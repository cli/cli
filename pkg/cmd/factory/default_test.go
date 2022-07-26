package factory

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_BaseRepo(t *testing.T) {
	tests := []struct {
		name       string
		remotes    git.RemoteSet
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
			wantsName:  "repo",
			wantsOwner: "owner",
			wantsHost:  "nonsense.com",
		},
		{
			name: "no matching remote",
			remotes: git.RemoteSet{
				git.NewRemote("origin", "https://test.com/owner/repo.git"),
			},
			wantsErr: true,
		},
		{
			name: "override with matching remote",
			remotes: git.RemoteSet{
				git.NewRemote("origin", "https://test.com/owner/repo.git"),
			},
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
			override: "test.com",
			wantsErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := New("1")
			rr := &remoteResolver{
				readRemotes: func() (git.RemoteSet, error) {
					return tt.remotes, nil
				},
				getConfig: func() (config.Config, error) {
					cfg := &config.ConfigMock{}
					cfg.HostsFunc = func() []string {
						hosts := []string{"nonsense.com"}
						if tt.override != "" {
							hosts = append([]string{tt.override}, hosts...)
						}
						return hosts
					}
					cfg.DefaultHostFunc = func() (string, string) {
						if tt.override != "" {
							return tt.override, "GH_HOST"
						}
						return "nonsense.com", "hosts"
					}
					cfg.AuthTokenFunc = func(string) (string, string) {
						return "", ""
					}
					return cfg, nil
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

	tests := []struct {
		name       string
		remotes    git.RemoteSet
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
			override: "test.com",
			wantsErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := New("1")
			rr := &remoteResolver{
				readRemotes: func() (git.RemoteSet, error) {
					return tt.remotes, nil
				},
				getConfig: func() (config.Config, error) {
					cfg := &config.ConfigMock{}
					cfg.HostsFunc = func() []string {
						hosts := []string{"nonsense.com"}
						if tt.override != "" {
							hosts = append([]string{tt.override}, hosts...)
						}
						return hosts
					}
					cfg.DefaultHostFunc = func() (string, string) {
						if tt.override != "" {
							return tt.override, "GH_HOST"
						}
						return "nonsense.com", "hosts"
					}
					return cfg, nil
				},
			}
			f.HttpClient = func() (*http.Client, error) { return nil, nil }
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
				old := os.Getenv("GH_REPO")
				os.Setenv("GH_REPO", tt.envOverride)
				defer os.Setenv("GH_REPO", old)
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

func TestSSOURL(t *testing.T) {
	tests := []struct {
		name       string
		host       string
		sso        string
		wantStderr string
		wantSSO    string
	}{
		{
			name:       "SSO challenge in response header",
			host:       "github.com",
			sso:        "required; url=https://github.com/login/sso?return_to=xyz&param=123abc; another",
			wantStderr: "",
			wantSSO:    "https://github.com/login/sso?return_to=xyz&param=123abc",
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sso := r.URL.Query().Get("sso"); sso != "" {
			w.Header().Set("X-GitHub-SSO", sso)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := New("1")
			f.Config = func() (config.Config, error) {
				return config.NewBlankConfig(), nil
			}
			ios, _, _, stderr := iostreams.Test()
			f.IOStreams = ios
			client, err := httpClientFunc(f, "v1.2.3")()
			require.NoError(t, err)
			req, err := http.NewRequest("GET", ts.URL, nil)
			if tt.sso != "" {
				q := req.URL.Query()
				q.Set("sso", tt.sso)
				req.URL.RawQuery = q.Encode()
			}
			req.Host = tt.host
			require.NoError(t, err)

			res, err := client.Do(req)
			require.NoError(t, err)

			assert.Equal(t, 204, res.StatusCode)
			assert.Equal(t, tt.wantStderr, stderr.String())
			assert.Equal(t, tt.wantSSO, SSOURL())
		})
	}
}

func defaultConfig() *config.ConfigMock {
	cfg := config.NewFromString("")
	cfg.Set("nonsense.com", "oauth_token", "BLAH")
	return cfg
}

func pagerConfig() config.Config {
	return config.NewFromString("pager: CONFIG_PAGER")
}

func disablePromptConfig() config.Config {
	return config.NewFromString("prompt: disabled")
}
