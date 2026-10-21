package token

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	ghmock "github.com/cli/cli/v2/internal/gh/mock"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/require"
)

func TestNewCmdToken(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		output     TokenOptions
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:   "no flags",
			input:  "",
			output: TokenOptions{},
		},
		{
			name:   "with hostname",
			input:  "--hostname github.mycompany.com",
			output: TokenOptions{Hostname: "github.mycompany.com"},
		},
		{
			name:   "with user",
			input:  "--user test-user",
			output: TokenOptions{Username: "test-user"},
		},
		{
			name:   "with shorthand user",
			input:  "-u test-user",
			output: TokenOptions{Username: "test-user"},
		},
		{
			name:   "with shorthand hostname",
			input:  "-h github.mycompany.com",
			output: TokenOptions{Hostname: "github.mycompany.com"},
		},
		{
			name:   "with secure-storage",
			input:  "--secure-storage",
			output: TokenOptions{SecureStorage: true},
		},
		{
			name:   "with no-refresh",
			input:  "--no-refresh",
			output: TokenOptions{NoRefresh: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
				Config: func() (gh.Config, error) {
					cfg := config.NewMockConfig()
					return cfg, nil
				},
			}
			argv, err := shlex.Split(tt.input)
			require.NoError(t, err)

			var cmdOpts *TokenOptions
			cmd := NewCmdToken(f, func(opts *TokenOptions) error {
				cmdOpts = opts
				return nil
			})
			// TODO cobra hack-around
			cmd.Flags().BoolP("help", "x", false, "")

			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantErr {
				require.Error(t, err)
				require.EqualError(t, err, tt.wantErrMsg)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.output.Hostname, cmdOpts.Hostname)
			require.Equal(t, tt.output.SecureStorage, cmdOpts.SecureStorage)
			require.Equal(t, tt.output.NoRefresh, cmdOpts.NoRefresh)
		})
	}
}

func TestTokenRun(t *testing.T) {
	tests := []struct {
		name       string
		opts       TokenOptions
		env        map[string]string
		cfgStubs   func(*testing.T, gh.Config)
		wantStdout string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "token",
			opts: TokenOptions{},
			cfgStubs: func(t *testing.T, cfg gh.Config) {
				login(t, cfg, "github.com", "test-user", "gho_ABCDEFG", "https", false)
			},
			wantStdout: "gho_ABCDEFG\n",
		},
		{
			name: "token by hostname",
			opts: TokenOptions{
				Hostname: "github.mycompany.com",
			},
			cfgStubs: func(t *testing.T, cfg gh.Config) {
				login(t, cfg, "github.com", "test-user", "gho_ABCDEFG", "https", false)
				login(t, cfg, "github.mycompany.com", "test-user", "gho_1234567", "https", false)
			},
			wantStdout: "gho_1234567\n",
		},
		{
			name:       "no token",
			opts:       TokenOptions{},
			wantErr:    true,
			wantErrMsg: "no oauth token found for github.com",
		},
		{
			name: "no token for hostname user",
			opts: TokenOptions{
				Hostname: "ghe.io",
				Username: "test-user",
			},
			wantErr:    true,
			wantErrMsg: "no oauth token found for ghe.io account test-user",
		},
		{
			name: "uses default host when one is not provided",
			opts: TokenOptions{},
			cfgStubs: func(t *testing.T, cfg gh.Config) {
				login(t, cfg, "github.com", "test-user", "gho_ABCDEFG", "https", false)
				login(t, cfg, "github.mycompany.com", "test-user", "gho_1234567", "https", false)
			},
			env:        map[string]string{"GH_HOST": "github.mycompany.com"},
			wantStdout: "gho_1234567\n",
		},
		{
			name: "token for user",
			opts: TokenOptions{
				Hostname: "github.com",
				Username: "test-user",
			},
			cfgStubs: func(t *testing.T, cfg gh.Config) {
				login(t, cfg, "github.com", "test-user", "gho_ABCDEFG", "https", false)
				login(t, cfg, "github.com", "test-user-2", "gho_1234567", "https", false)
			},
			wantStdout: "gho_ABCDEFG\n",
		},
		{
			name: "returns a refreshable token's stored access token",
			opts: TokenOptions{},
			cfgStubs: func(t *testing.T, cfg gh.Config) {
				loginRefreshable(t, cfg, "github.com", "test-user", "gho_REFRESHABLE", "ghr_TOKEN", false)
			},
			wantStdout: "gho_REFRESHABLE\n",
		},
		{
			name: "no-refresh returns the stored token without refreshing",
			opts: TokenOptions{NoRefresh: true},
			cfgStubs: func(t *testing.T, cfg gh.Config) {
				login(t, cfg, "github.com", "test-user", "gho_ABCDEFG", "https", false)
			},
			wantStdout: "gho_ABCDEFG\n",
		},
		{
			name: "no-refresh returns a refreshable token's stored access token",
			opts: TokenOptions{NoRefresh: true},
			cfgStubs: func(t *testing.T, cfg gh.Config) {
				loginRefreshable(t, cfg, "github.com", "test-user", "gho_REFRESHABLE", "ghr_TOKEN", false)
			},
			wantStdout: "gho_REFRESHABLE\n",
		},
		{
			name:       "returns a token from the environment",
			opts:       TokenOptions{},
			env:        map[string]string{"GH_TOKEN": "gho_ENVTOKEN"},
			wantStdout: "gho_ENVTOKEN\n",
		},
		{
			name:       "no-refresh returns a token from the environment",
			opts:       TokenOptions{NoRefresh: true},
			env:        map[string]string{"GH_TOKEN": "gho_ENVTOKEN"},
			wantStdout: "gho_ENVTOKEN\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			tt.opts.IO = ios

			cfg, _ := config.NewIsolatedTestConfig(t, "")

			// Set after isolating the config, which clears the auth env vars.
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			if tt.cfgStubs != nil {
				tt.cfgStubs(t, cfg)
			}

			tt.opts.Config = func() (gh.Config, error) {
				return cfg, nil
			}

			err := tokenRun(&tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				require.EqualError(t, err, tt.wantErrMsg)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantStdout, stdout.String())
		})
	}
}

func TestTokenRunSecureStorage(t *testing.T) {
	tests := []struct {
		name       string
		opts       TokenOptions
		env        map[string]string
		cfgStubs   func(*testing.T, gh.Config)
		wantStdout string
		wantErr    bool
		wantErrMsg string
	}{
		// State 3: --secure-storage without --no-refresh (compatibility mode for go-gh).
		{
			name: "state 3: plain token in keyring is returned",
			opts: TokenOptions{},
			cfgStubs: func(t *testing.T, cfg gh.Config) {
				login(t, cfg, "github.com", "test-user", "gho_ABCDEFG", "https", true)
			},
			wantStdout: "gho_ABCDEFG\n",
		},
		{
			name: "state 3: plain token in keyring by hostname is returned",
			opts: TokenOptions{
				Hostname: "mycompany.com",
			},
			cfgStubs: func(t *testing.T, cfg gh.Config) {
				login(t, cfg, "mycompany.com", "test-user", "gho_1234567", "https", true)
			},
			wantStdout: "gho_1234567\n",
		},
		{
			name: "state 3: plain token for user in keyring is returned",
			opts: TokenOptions{
				Hostname: "github.com",
				Username: "test-user",
			},
			cfgStubs: func(t *testing.T, cfg gh.Config) {
				login(t, cfg, "github.com", "test-user", "gho_ABCDEFG", "https", true)
				login(t, cfg, "github.com", "test-user-2", "gho_1234567", "https", true)
			},
			wantStdout: "gho_ABCDEFG\n",
		},
		{
			name: "state 3: plain token in config is suppressed",
			opts: TokenOptions{},
			cfgStubs: func(t *testing.T, cfg gh.Config) {
				login(t, cfg, "github.com", "test-user", "gho_ABCDEFG", "https", false)
			},
			wantErr:    true,
			wantErrMsg: "no oauth token found for github.com",
		},
		{
			name:       "state 3: token from the environment is suppressed",
			opts:       TokenOptions{},
			env:        map[string]string{"GH_TOKEN": "gho_ENVTOKEN"},
			wantErr:    true,
			wantErrMsg: "no oauth token found for github.com",
		},
		{
			name: "state 3: refreshable token in config is returned despite secure-storage",
			opts: TokenOptions{},
			cfgStubs: func(t *testing.T, cfg gh.Config) {
				loginRefreshable(t, cfg, "github.com", "test-user", "gho_REFRESHABLE", "ghr_TOKEN", false)
			},
			wantStdout: "gho_REFRESHABLE\n",
		},
		{
			name: "state 3: refreshable token in keyring is returned",
			opts: TokenOptions{},
			cfgStubs: func(t *testing.T, cfg gh.Config) {
				loginRefreshable(t, cfg, "github.com", "test-user", "gho_REFRESHABLE", "ghr_TOKEN", true)
			},
			wantStdout: "gho_REFRESHABLE\n",
		},
		{
			name: "state 3: refreshable token in keyring is returned despite environment token",
			opts: TokenOptions{},
			env:  map[string]string{"GH_TOKEN": "gho_ENVTOKEN"},
			cfgStubs: func(t *testing.T, cfg gh.Config) {
				loginRefreshable(t, cfg, "github.com", "test-user", "gho_REFRESHABLE", "ghr_TOKEN", true)
			},
			wantStdout: "gho_REFRESHABLE\n",
		},
		{
			name: "state 3: plain token in keyring is returned despite environment token",
			opts: TokenOptions{},
			env:  map[string]string{"GH_TOKEN": "gho_ENVTOKEN"},
			cfgStubs: func(t *testing.T, cfg gh.Config) {
				login(t, cfg, "github.com", "test-user", "gho_ABCDEFG", "https", true)
			},
			wantStdout: "gho_ABCDEFG\n",
		},
		{
			name:       "state 3: no token is an error",
			opts:       TokenOptions{},
			wantErr:    true,
			wantErrMsg: "no oauth token found for github.com",
		},
		{
			name: "state 3: no token for hostname user is an error",
			opts: TokenOptions{
				Hostname: "ghe.io",
				Username: "test-user",
			},
			wantErr:    true,
			wantErrMsg: "no oauth token found for ghe.io account test-user",
		},

		// State 4: --secure-storage with --no-refresh (rare; return only keyring-sourced tokens).
		{
			name: "state 4: plain token in keyring is returned",
			opts: TokenOptions{NoRefresh: true},
			cfgStubs: func(t *testing.T, cfg gh.Config) {
				login(t, cfg, "github.com", "test-user", "gho_ABCDEFG", "https", true)
			},
			wantStdout: "gho_ABCDEFG\n",
		},
		{
			name: "state 4: plain token in config is suppressed",
			opts: TokenOptions{NoRefresh: true},
			cfgStubs: func(t *testing.T, cfg gh.Config) {
				login(t, cfg, "github.com", "test-user", "gho_ABCDEFG", "https", false)
			},
			wantErr:    true,
			wantErrMsg: "no oauth token found for github.com",
		},
		{
			name:       "state 4: token from the environment is suppressed",
			opts:       TokenOptions{NoRefresh: true},
			env:        map[string]string{"GH_TOKEN": "gho_ENVTOKEN"},
			wantErr:    true,
			wantErrMsg: "no oauth token found for github.com",
		},
		{
			name: "state 4: refreshable token in config is suppressed",
			opts: TokenOptions{NoRefresh: true},
			cfgStubs: func(t *testing.T, cfg gh.Config) {
				loginRefreshable(t, cfg, "github.com", "test-user", "gho_REFRESHABLE", "ghr_TOKEN", false)
			},
			wantErr:    true,
			wantErrMsg: "no oauth token found for github.com",
		},
		{
			name: "state 4: refreshable token in keyring is returned",
			opts: TokenOptions{NoRefresh: true},
			cfgStubs: func(t *testing.T, cfg gh.Config) {
				loginRefreshable(t, cfg, "github.com", "test-user", "gho_REFRESHABLE", "ghr_TOKEN", true)
			},
			wantStdout: "gho_REFRESHABLE\n",
		},
		{
			name: "state 4: refreshable token in keyring is returned despite environment token",
			opts: TokenOptions{NoRefresh: true},
			env:  map[string]string{"GH_TOKEN": "gho_ENVTOKEN"},
			cfgStubs: func(t *testing.T, cfg gh.Config) {
				loginRefreshable(t, cfg, "github.com", "test-user", "gho_REFRESHABLE", "ghr_TOKEN", true)
			},
			wantStdout: "gho_REFRESHABLE\n",
		},
		{
			name: "state 4: plain token in keyring is returned despite environment token",
			opts: TokenOptions{NoRefresh: true},
			env:  map[string]string{"GH_TOKEN": "gho_ENVTOKEN"},
			cfgStubs: func(t *testing.T, cfg gh.Config) {
				login(t, cfg, "github.com", "test-user", "gho_ABCDEFG", "https", true)
			},
			wantStdout: "gho_ABCDEFG\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			tt.opts.IO = ios
			tt.opts.SecureStorage = true

			cfg, _ := config.NewIsolatedTestConfig(t, "")

			// Set after isolating the config, which clears the auth env vars.
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			if tt.cfgStubs != nil {
				tt.cfgStubs(t, cfg)
			}

			tt.opts.Config = func() (gh.Config, error) {
				return cfg, nil
			}

			err := tokenRun(&tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				require.EqualError(t, err, tt.wantErrMsg)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantStdout, stdout.String())
		})
	}
}

func login(t *testing.T, c gh.Config, hostname, username, token, gitProtocol string, secureStorage bool) {
	t.Helper()
	_, err := c.Authentication().Login(hostname, username, token, gitProtocol, secureStorage)
	require.NoError(t, err)
}

func loginRefreshable(t *testing.T, c gh.Config, hostname, username, accessToken, refreshToken string, secureStorage bool) {
	t.Helper()
	loginRefreshableWithExpiry(t, c, hostname, username, accessToken, refreshToken, secureStorage, nil)
}

func loginRefreshableWithExpiry(t *testing.T, c gh.Config, hostname, username, accessToken, refreshToken string, secureStorage bool, expiresAt *time.Time) {
	t.Helper()
	authCfg := c.Authentication().(*config.AuthConfig)
	_, err := authCfg.LoginRefreshable(hostname, username, gh.Credential{
		Token:        accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
	}, "https", secureStorage)
	require.NoError(t, err)
}

func setRefresher(t *testing.T, c gh.Config, r gh.TokenRefresher) {
	t.Helper()
	c.Authentication().(*config.AuthConfig).SetTokenRefresher(r)
}

// renewingRefresher returns a mock that renews to a fresh, long lived credential and records its calls.
func renewingRefresher(accessToken, refreshToken string) *ghmock.TokenRefresherMock {
	return &ghmock.TokenRefresherMock{
		RefreshFunc: func(_ string, _ string) (gh.RefreshableCredential, error) {
			future := time.Now().Add(time.Hour)
			return gh.RefreshableCredential{
				AccessToken:  accessToken,
				RefreshToken: refreshToken,
				ExpiresAt:    &future,
			}, nil
		},
	}
}

// failingRefresher returns a mock that always fails to refresh and records its calls.
func failingRefresher() *ghmock.TokenRefresherMock {
	return &ghmock.TokenRefresherMock{
		RefreshFunc: func(_ string, _ string) (gh.RefreshableCredential, error) {
			return gh.RefreshableCredential{}, errors.New("token endpoint unreachable")
		},
	}
}

// expiredRefresher returns a mock whose refresh token is rejected by the server, recorded via
// gh.ErrRefreshTokenInvalid, and records its calls.
func expiredRefresher() *ghmock.TokenRefresherMock {
	return &ghmock.TokenRefresherMock{
		RefreshFunc: func(_ string, _ string) (gh.RefreshableCredential, error) {
			return gh.RefreshableCredential{}, fmt.Errorf("server rejected token: %w", gh.ErrRefreshTokenInvalid)
		},
	}
}

func TestTokenRunRefresh(t *testing.T) {
	// past and future are relative to the 10 minute refresh leeway: past tokens (or those with no recorded expiry)
	// need refreshing, future tokens do not.
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	tests := []struct {
		name           string
		secureStorage  bool
		noRefresh      bool
		setup          func(*testing.T, gh.Config)
		refresher      *ghmock.TokenRefresherMock
		wantStdout     string
		wantErrMsg     string
		wantRefresh    int
		wantRefreshTok string
	}{
		{
			name: "state 1: expiring refreshable in config is refreshed",
			setup: func(t *testing.T, cfg gh.Config) {
				loginRefreshableWithExpiry(t, cfg, "github.com", "test-user", "gho_OLD", "ghr_OLD", false, &past)
			},
			refresher:      renewingRefresher("gho_NEW", "ghr_NEW"),
			wantStdout:     "gho_NEW\n",
			wantRefresh:    1,
			wantRefreshTok: "ghr_OLD",
		},
		{
			name: "state 1: expiring refreshable in keyring is refreshed",
			setup: func(t *testing.T, cfg gh.Config) {
				loginRefreshableWithExpiry(t, cfg, "github.com", "test-user", "gho_OLD", "ghr_OLD", true, &past)
			},
			refresher:      renewingRefresher("gho_NEW", "ghr_NEW"),
			wantStdout:     "gho_NEW\n",
			wantRefresh:    1,
			wantRefreshTok: "ghr_OLD",
		},
		{
			name: "state 1: unexpired refreshable is not refreshed",
			setup: func(t *testing.T, cfg gh.Config) {
				loginRefreshableWithExpiry(t, cfg, "github.com", "test-user", "gho_OLD", "ghr_OLD", false, &future)
			},
			refresher:   renewingRefresher("gho_NEW", "ghr_NEW"),
			wantStdout:  "gho_OLD\n",
			wantRefresh: 0,
		},
		{
			name: "state 1: failed refresh falls back to the stored token",
			setup: func(t *testing.T, cfg gh.Config) {
				loginRefreshableWithExpiry(t, cfg, "github.com", "test-user", "gho_OLD", "ghr_OLD", false, &past)
			},
			refresher:      failingRefresher(),
			wantStdout:     "gho_OLD\n",
			wantRefresh:    1,
			wantRefreshTok: "ghr_OLD",
		},
		{
			name: "state 1: no refresher leaves the stored token untouched",
			setup: func(t *testing.T, cfg gh.Config) {
				loginRefreshableWithExpiry(t, cfg, "github.com", "test-user", "gho_OLD", "ghr_OLD", false, &past)
			},
			wantStdout: "gho_OLD\n",
		},
		{
			name: "state 1: rejected refresh token errors and asks the user to re-authenticate",
			setup: func(t *testing.T, cfg gh.Config) {
				loginRefreshableWithExpiry(t, cfg, "github.com", "test-user", "gho_OLD", "ghr_OLD", false, &past)
			},
			refresher:      expiredRefresher(),
			wantErrMsg:     "the token for github.com has expired; please run 'gh auth login' to re-authenticate",
			wantRefresh:    1,
			wantRefreshTok: "ghr_OLD",
		},

		{
			name:      "state 2: no-refresh returns the stored expiring token without refreshing",
			noRefresh: true,
			setup: func(t *testing.T, cfg gh.Config) {
				loginRefreshableWithExpiry(t, cfg, "github.com", "test-user", "gho_OLD", "ghr_OLD", false, &past)
			},
			refresher:   renewingRefresher("gho_NEW", "ghr_NEW"),
			wantStdout:  "gho_OLD\n",
			wantRefresh: 0,
		},

		{
			name:          "state 3: expiring refreshable in config is refreshed despite secure-storage",
			secureStorage: true,
			setup: func(t *testing.T, cfg gh.Config) {
				loginRefreshableWithExpiry(t, cfg, "github.com", "test-user", "gho_OLD", "ghr_OLD", false, &past)
			},
			refresher:      renewingRefresher("gho_NEW", "ghr_NEW"),
			wantStdout:     "gho_NEW\n",
			wantRefresh:    1,
			wantRefreshTok: "ghr_OLD",
		},
		{
			name:          "state 3: expiring refreshable in keyring is refreshed",
			secureStorage: true,
			setup: func(t *testing.T, cfg gh.Config) {
				loginRefreshableWithExpiry(t, cfg, "github.com", "test-user", "gho_OLD", "ghr_OLD", true, &past)
			},
			refresher:      renewingRefresher("gho_NEW", "ghr_NEW"),
			wantStdout:     "gho_NEW\n",
			wantRefresh:    1,
			wantRefreshTok: "ghr_OLD",
		},
		{
			name:          "state 3: failed refresh falls back to the stored refreshable token",
			secureStorage: true,
			setup: func(t *testing.T, cfg gh.Config) {
				loginRefreshableWithExpiry(t, cfg, "github.com", "test-user", "gho_OLD", "ghr_OLD", false, &past)
			},
			refresher:      failingRefresher(),
			wantStdout:     "gho_OLD\n",
			wantRefresh:    1,
			wantRefreshTok: "ghr_OLD",
		},
		{
			name:          "state 3: plain keyring token is not refreshed",
			secureStorage: true,
			setup: func(t *testing.T, cfg gh.Config) {
				login(t, cfg, "github.com", "test-user", "gho_PLAIN", "https", true)
			},
			refresher:   renewingRefresher("gho_NEW", "ghr_NEW"),
			wantStdout:  "gho_PLAIN\n",
			wantRefresh: 0,
		},
		{
			name:          "state 3: rejected refresh token errors even under secure-storage",
			secureStorage: true,
			setup: func(t *testing.T, cfg gh.Config) {
				loginRefreshableWithExpiry(t, cfg, "github.com", "test-user", "gho_OLD", "ghr_OLD", true, &past)
			},
			refresher:      expiredRefresher(),
			wantErrMsg:     "the token for github.com has expired; please run 'gh auth login' to re-authenticate",
			wantRefresh:    1,
			wantRefreshTok: "ghr_OLD",
		},
		{
			name:          "state 3: plain config token is suppressed and not refreshed",
			secureStorage: true,
			setup: func(t *testing.T, cfg gh.Config) {
				login(t, cfg, "github.com", "test-user", "gho_PLAIN", "https", false)
			},
			refresher:   renewingRefresher("gho_NEW", "ghr_NEW"),
			wantErrMsg:  "no oauth token found for github.com",
			wantRefresh: 0,
		},

		{
			name:          "state 4: no-refresh returns the stored keyring token without refreshing",
			secureStorage: true,
			noRefresh:     true,
			setup: func(t *testing.T, cfg gh.Config) {
				loginRefreshableWithExpiry(t, cfg, "github.com", "test-user", "gho_OLD", "ghr_OLD", true, &past)
			},
			refresher:   renewingRefresher("gho_NEW", "ghr_NEW"),
			wantStdout:  "gho_OLD\n",
			wantRefresh: 0,
		},
		{
			name:          "state 4: no-refresh suppresses a config token and does not refresh",
			secureStorage: true,
			noRefresh:     true,
			setup: func(t *testing.T, cfg gh.Config) {
				loginRefreshableWithExpiry(t, cfg, "github.com", "test-user", "gho_OLD", "ghr_OLD", false, &past)
			},
			refresher:   renewingRefresher("gho_NEW", "ghr_NEW"),
			wantErrMsg:  "no oauth token found for github.com",
			wantRefresh: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			opts := TokenOptions{
				IO:            ios,
				SecureStorage: tt.secureStorage,
				NoRefresh:     tt.noRefresh,
			}

			cfg, _ := config.NewIsolatedTestConfig(t, "")
			if tt.setup != nil {
				tt.setup(t, cfg)
			}
			if tt.refresher != nil {
				setRefresher(t, cfg, tt.refresher)
			}

			opts.Config = func() (gh.Config, error) {
				return cfg, nil
			}

			err := tokenRun(&opts)
			if tt.wantErrMsg != "" {
				require.EqualError(t, err, tt.wantErrMsg)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.wantStdout, stdout.String())
			}

			if tt.refresher != nil {
				calls := tt.refresher.RefreshCalls()
				require.Len(t, calls, tt.wantRefresh)
				if tt.wantRefresh > 0 {
					require.Equal(t, tt.wantRefreshTok, calls[0].RefreshToken)
					require.Equal(t, "github.com", calls[0].Hostname)
				}
			}
		})
	}
}
