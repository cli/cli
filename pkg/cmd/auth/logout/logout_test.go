package logout

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/keyring"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	ghauth "github.com/cli/go-gh/v2/pkg/auth"
	"github.com/google/shlex"
	"github.com/stretchr/testify/require"
)

func Test_NewCmdLogout(t *testing.T) {
	tests := []struct {
		name  string
		cli   string
		wants LogoutOptions
		tty   bool
	}{
		{
			name:  "nontty no arguments",
			cli:   "",
			wants: LogoutOptions{},
		},
		{
			name:  "tty no arguments",
			tty:   true,
			cli:   "",
			wants: LogoutOptions{},
		},
		{
			name: "tty with hostname",
			tty:  true,
			cli:  "--hostname github.com",
			wants: LogoutOptions{
				Hostname: "github.com",
			},
		},
		{
			name: "nontty with hostname",
			cli:  "--hostname github.com",
			wants: LogoutOptions{
				Hostname: "github.com",
			},
		},
		{
			name: "tty with user",
			tty:  true,
			cli:  "--user monalisa",
			wants: LogoutOptions{
				Username: "monalisa",
			},
		},
		{
			name: "nontty with user",
			cli:  "--user monalisa",
			wants: LogoutOptions{
				Username: "monalisa",
			},
		},
		{
			name: "tty with hostname and user",
			tty:  true,
			cli:  "--hostname github.com --user monalisa",
			wants: LogoutOptions{
				Hostname: "github.com",
				Username: "monalisa",
			},
		},
		{
			name: "nontty with hostname and user",
			cli:  "--hostname github.com --user monalisa",
			wants: LogoutOptions{
				Hostname: "github.com",
				Username: "monalisa",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}
			ios.SetStdinTTY(tt.tty)
			ios.SetStdoutTTY(tt.tty)

			argv, err := shlex.Split(tt.cli)
			require.NoError(t, err)

			var gotOpts *LogoutOptions
			cmd := NewCmdLogout(f, func(opts *LogoutOptions) error {
				gotOpts = opts
				return nil
			})
			// TODO cobra hack-around
			cmd.Flags().BoolP("help", "x", false, "")

			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			require.NoError(t, err)

			require.Equal(t, tt.wants.Hostname, gotOpts.Hostname)
			require.Equal(t, tt.wants.Username, gotOpts.Username)
		})
	}
}

type user struct {
	name  string
	token string
}

type hostUsers struct {
	host  string
	users []user
}

type tokenAssertion func(t *testing.T, cfg gh.Config)

func Test_logoutRun_tty(t *testing.T) {
	tests := []struct {
		name          string
		opts          *LogoutOptions
		prompterStubs func(*prompter.PrompterMock)
		cfgHosts      []hostUsers
		secureStorage bool
		wantHosts     string
		assertToken   tokenAssertion
		wantErrOut    *regexp.Regexp
		wantErr       string
	}{
		{
			name: "logs out prompted user when multiple known hosts with one user each",
			opts: &LogoutOptions{},
			cfgHosts: []hostUsers{
				{"ghe.io", []user{
					{"monalisa-ghe", "abc123"},
				}},
				{"github.com", []user{
					{"monalisa", "abc123"},
				}},
			},
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(_, _ string, opts []string) (int, error) {
					return prompter.IndexFor(opts, "monalisa (github.com)")
				}
			},
			assertToken: hasNoToken("github.com"),
			wantHosts:   "ghe.io:\n    users:\n        monalisa-ghe:\n            oauth_token: abc123\n    git_protocol: ssh\n    oauth_token: abc123\n    user: monalisa-ghe\n",
			wantErrOut:  regexp.MustCompile(`Logged out of github.com account monalisa`),
		},
		{
			name: "logs out prompted user when multiple known hosts with multiple users each",
			opts: &LogoutOptions{},
			cfgHosts: []hostUsers{
				{"ghe.io", []user{
					{"monalisa-ghe", "abc123"},
					{"monalisa-ghe2", "abc123"},
				}},
				{"github.com", []user{
					{"monalisa", "monalisa-token"},
					{"monalisa2", "monalisa2-token"},
				}},
			},
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(_, _ string, opts []string) (int, error) {
					return prompter.IndexFor(opts, "monalisa (github.com)")
				}
			},
			assertToken: hasActiveToken("github.com", "monalisa2-token"),
			wantHosts:   "ghe.io:\n    users:\n        monalisa-ghe:\n            oauth_token: abc123\n        monalisa-ghe2:\n            oauth_token: abc123\n    git_protocol: ssh\n    user: monalisa-ghe2\n    oauth_token: abc123\ngithub.com:\n    users:\n        monalisa2:\n            oauth_token: monalisa2-token\n    git_protocol: ssh\n    user: monalisa2\n    oauth_token: monalisa2-token\n",
			wantErrOut:  regexp.MustCompile(`Logged out of github.com account monalisa`),
		},
		{
			name: "logs out only logged in user",
			opts: &LogoutOptions{},
			cfgHosts: []hostUsers{
				{"github.com", []user{
					{"monalisa", "abc123"},
				}},
			},
			wantHosts:   "{}\n",
			assertToken: hasNoToken("github.com"),
			wantErrOut:  regexp.MustCompile(`Logged out of github.com account monalisa`),
		},
		{
			name: "logs out prompted user when one known host with multiple users",
			opts: &LogoutOptions{},
			cfgHosts: []hostUsers{
				{"github.com", []user{
					{"monalisa", "monalisa-token"},
					{"monalisa2", "monalisa2-token"},
				}},
			},
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(_, _ string, opts []string) (int, error) {
					return prompter.IndexFor(opts, "monalisa (github.com)")
				}
			},
			wantHosts:   "github.com:\n    users:\n        monalisa2:\n            oauth_token: monalisa2-token\n    git_protocol: ssh\n    user: monalisa2\n    oauth_token: monalisa2-token\n",
			assertToken: hasActiveToken("github.com", "monalisa2-token"),
			wantErrOut:  regexp.MustCompile(`Logged out of github.com account monalisa`),
		},
		{
			name: "logs out specified user when multiple known hosts with one user each",
			opts: &LogoutOptions{
				Hostname: "ghe.io",
				Username: "monalisa-ghe",
			},
			cfgHosts: []hostUsers{
				{"ghe.io", []user{
					{"monalisa-ghe", "abc123"},
				}},
				{"github.com", []user{
					{"monalisa", "abc123"},
				}},
			},
			wantHosts:   "github.com:\n    users:\n        monalisa:\n            oauth_token: abc123\n    git_protocol: ssh\n    oauth_token: abc123\n    user: monalisa\n",
			assertToken: hasNoToken("ghe.io"),
			wantErrOut:  regexp.MustCompile(`Logged out of ghe.io account monalisa-ghe`),
		},
		{
			name:          "logs out specified user that is using secure storage",
			secureStorage: true,
			opts: &LogoutOptions{
				Hostname: "github.com",
				Username: "monalisa",
			},
			cfgHosts: []hostUsers{
				{"github.com", []user{
					{"monalisa", "abc123"},
				}},
			},
			wantHosts:   "{}\n",
			assertToken: hasNoToken("github.com"),
			wantErrOut:  regexp.MustCompile(`Logged out of github.com account monalisa`),
		},
		{
			name:    "errors when no known hosts",
			opts:    &LogoutOptions{},
			wantErr: `not logged in to any hosts`,
		},
		{
			name: "errors when specified host is not a known host",
			opts: &LogoutOptions{
				Hostname: "ghe.io",
				Username: "monalisa-ghe",
			},
			cfgHosts: []hostUsers{
				{"github.com", []user{
					{"monalisa", "abc123"},
				}},
			},
			wantErr: "not logged in to ghe.io",
		},
		{
			name: "errors when specified user is not logged in on specified host",
			opts: &LogoutOptions{
				Hostname: "ghe.io",
				Username: "unknown-user",
			},
			cfgHosts: []hostUsers{
				{"ghe.io", []user{
					{"monalisa-ghe", "abc123"},
				}},
			},
			wantErr: "not logged in to ghe.io account unknown-user",
		},
		{
			name: "errors when user is specified but doesn't exist on any host",
			opts: &LogoutOptions{
				Username: "unknown-user",
			},
			cfgHosts: []hostUsers{
				{"ghe.io", []user{
					{"monalisa-ghe", "abc123"},
				}},
				{"github.com", []user{
					{"monalisa", "abc123"},
				}},
			},
			wantErr: "no accounts matched that criteria",
		},
		{
			name: "switches user if there is another one available",
			opts: &LogoutOptions{
				Hostname: "github.com",
				Username: "monalisa2",
			},
			cfgHosts: []hostUsers{
				{"github.com", []user{
					{"monalisa", "monalisa-token"},
					{"monalisa2", "monalisa2-token"},
				}},
			},
			wantHosts:   "github.com:\n    users:\n        monalisa:\n            oauth_token: monalisa-token\n    git_protocol: ssh\n    user: monalisa\n    oauth_token: monalisa-token\n",
			assertToken: hasActiveToken("github.com", "monalisa-token"),
			wantErrOut:  regexp.MustCompile("✓ Switched active account for github.com to monalisa"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, readConfigs := config.NewIsolatedTestConfig(t, "")

			for _, hostUsers := range tt.cfgHosts {
				for _, user := range hostUsers.users {
					_, _ = cfg.Authentication().Login(
						string(hostUsers.host),
						user.name,
						user.token, "ssh", tt.secureStorage,
					)
				}
			}

			tt.opts.Config = func() (gh.Config, error) {
				return cfg, nil
			}

			ios, _, _, stderr := iostreams.Test()
			ios.SetStdinTTY(true)
			ios.SetStdoutTTY(true)
			tt.opts.IO = ios

			pm := &prompter.PrompterMock{}
			if tt.prompterStubs != nil {
				tt.prompterStubs(pm)
			}
			tt.opts.Prompter = pm
			tt.opts.GitClient = &git.Client{GitPath: "some/path/git"}

			cs, restoreRun := run.Stub()
			defer restoreRun(t)
			if tt.wantErr == "" {
				cs.Register(`git credential reject`, 0, "")
			}

			err := logoutRun(tt.opts)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			} else {
				require.NoError(t, err)
			}

			if tt.wantErrOut == nil {
				require.Equal(t, "", stderr.String())
			} else {
				require.True(t, tt.wantErrOut.MatchString(stderr.String()), stderr.String())
			}

			hostsBuf := bytes.Buffer{}
			readConfigs(io.Discard, &hostsBuf)

			require.Equal(t, tt.wantHosts, hostsBuf.String())

			if tt.assertToken != nil {
				tt.assertToken(t, cfg)
			}
		})
	}
}

func Test_logoutRun_nontty(t *testing.T) {
	tests := []struct {
		name          string
		opts          *LogoutOptions
		cfgHosts      []hostUsers
		secureStorage bool
		wantHosts     string
		assertToken   tokenAssertion
		wantErrOut    *regexp.Regexp
		wantErr       string
	}{
		{
			name: "logs out specified user when one known host",
			opts: &LogoutOptions{
				Hostname: "github.com",
				Username: "monalisa",
			},
			cfgHosts: []hostUsers{
				{"github.com", []user{
					{"monalisa", "abc123"},
				}},
			},
			wantHosts:   "{}\n",
			assertToken: hasNoToken("github.com"),
			wantErrOut:  regexp.MustCompile(`Logged out of github.com account monalisa`),
		},
		{
			name: "logs out specified user when multiple known hosts",
			opts: &LogoutOptions{
				Hostname: "github.com",
				Username: "monalisa",
			},
			cfgHosts: []hostUsers{
				{"github.com", []user{
					{"monalisa", "abc123"},
				}},
				{"ghe.io", []user{
					{"monalisa-ghe", "abc123"},
				}},
			},
			wantHosts:   "ghe.io:\n    users:\n        monalisa-ghe:\n            oauth_token: abc123\n    git_protocol: ssh\n    oauth_token: abc123\n    user: monalisa-ghe\n",
			assertToken: hasNoToken("github.com"),
			wantErrOut:  regexp.MustCompile(`Logged out of github.com account monalisa`),
		},
		{
			name:          "logs out specified user that is using secure storage",
			secureStorage: true,
			opts: &LogoutOptions{
				Hostname: "github.com",
				Username: "monalisa",
			},
			cfgHosts: []hostUsers{
				{"github.com", []user{
					{"monalisa", "abc123"},
				}},
			},
			wantHosts:   "{}\n",
			assertToken: hasNoToken("github.com"),
			wantErrOut:  regexp.MustCompile(`Logged out of github.com account monalisa`),
		},
		{
			name: "errors when no known hosts",
			opts: &LogoutOptions{
				Hostname: "github.com",
				Username: "monalisa",
			},
			wantErr: `not logged in to any hosts`,
		},
		{
			name: "errors when specified host is not a known host",
			opts: &LogoutOptions{
				Hostname: "ghe.io",
				Username: "monalisa-ghe",
			},
			cfgHosts: []hostUsers{
				{"github.com", []user{
					{"monalisa", "abc123"},
				}},
			},
			wantErr: "not logged in to ghe.io",
		},
		{
			name: "errors when specified user is not logged in on specified host",
			opts: &LogoutOptions{
				Hostname: "ghe.io",
				Username: "unknown-user",
			},
			cfgHosts: []hostUsers{
				{"ghe.io", []user{
					{"monalisa-ghe", "abc123"},
				}},
			},
			wantErr: "not logged in to ghe.io account unknown-user",
		},
		{
			name: "errors when host is specified but user is ambiguous",
			opts: &LogoutOptions{
				Hostname: "ghe.io",
			},
			cfgHosts: []hostUsers{
				{"ghe.io", []user{
					{"monalisa-ghe", "abc123"},
					{"monalisa-ghe2", "abc123"},
				}},
			},
			wantErr: "unable to determine which account to log out of, please specify `--hostname` and `--user`",
		},
		{
			name: "errors when user is specified but host is ambiguous",
			opts: &LogoutOptions{
				Username: "monalisa",
			},
			cfgHosts: []hostUsers{
				{"github.com", []user{
					{"monalisa", "abc123"},
				}},
				{"ghe.io", []user{
					{"monalisa", "abc123"},
				}},
			},
			wantErr: "unable to determine which account to log out of, please specify `--hostname` and `--user`",
		},
		{
			name: "switches user if there is another one available",
			opts: &LogoutOptions{
				Hostname: "github.com",
				Username: "monalisa2",
			},
			cfgHosts: []hostUsers{
				{"github.com", []user{
					{"monalisa", "monalisa-token"},
					{"monalisa2", "monalisa2-token"},
				}},
			},
			wantHosts:   "github.com:\n    users:\n        monalisa:\n            oauth_token: monalisa-token\n    git_protocol: ssh\n    user: monalisa\n    oauth_token: monalisa-token\n",
			assertToken: hasActiveToken("github.com", "monalisa-token"),
			wantErrOut:  regexp.MustCompile("✓ Switched active account for github.com to monalisa"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, readConfigs := config.NewIsolatedTestConfig(t, "")

			for _, hostUsers := range tt.cfgHosts {
				for _, user := range hostUsers.users {
					_, _ = cfg.Authentication().Login(
						string(hostUsers.host),
						user.name,
						user.token, "ssh", tt.secureStorage,
					)
				}
			}
			tt.opts.Config = func() (gh.Config, error) {
				return cfg, nil
			}

			ios, _, _, stderr := iostreams.Test()
			ios.SetStdinTTY(false)
			ios.SetStdoutTTY(false)
			tt.opts.IO = ios
			tt.opts.GitClient = &git.Client{GitPath: "some/path/git"}

			cs, restoreRun := run.Stub()
			defer restoreRun(t)
			if tt.wantErr == "" {
				cs.Register(`git credential reject`, 0, "")
			}

			err := logoutRun(tt.opts)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			} else {
				require.NoError(t, err)
			}

			if tt.wantErrOut == nil {
				require.Equal(t, "", stderr.String())
			} else {
				require.True(t, tt.wantErrOut.MatchString(stderr.String()), stderr.String())
			}

			hostsBuf := bytes.Buffer{}
			readConfigs(io.Discard, &hostsBuf)

			require.Equal(t, tt.wantHosts, hostsBuf.String())

			if tt.assertToken != nil {
				tt.assertToken(t, cfg)
			}
		})
	}
}

func Test_logoutRun_credentialCleanup(t *testing.T) {
	tests := []struct {
		name       string
		hostname   string
		token      string
		revokeErr  error
		staleToken string
		expectGit  bool
		gitExit    int
		wantErr    string
		wantErrOut string
		wantToken  bool
	}{
		{
			name:       "revokes OAuth token before local cleanup",
			hostname:   "github.com",
			token:      "gho_test-token",
			expectGit:  true,
			wantErrOut: "Logged out of github.com account monalisa",
		},
		{
			name:       "revokes active OAuth token and leftover keyring token",
			hostname:   "github.com",
			token:      "gho_current-token",
			staleToken: "gho_stale-token",
			expectGit:  true,
			wantErrOut: "Logged out of github.com account monalisa",
		},
		{
			name:      "retains OAuth token when revocation fails",
			hostname:  "github.com",
			token:     "gho_test-token",
			revokeErr: errors.New("HTTP 422: Validation Failed"),
			wantErr:   "failed to revoke authentication token: HTTP 422: Validation Failed",
			wantToken: true,
		},
		{
			name:       "skips revocation for GitHub Enterprise Server",
			hostname:   "example.com",
			token:      "gho_test-token",
			expectGit:  true,
			wantErrOut: "Logged out of example.com account monalisa",
		},
		{
			name:       "warns when git credential cleanup fails",
			hostname:   "github.com",
			token:      "ghp_test-token",
			expectGit:  true,
			gitExit:    1,
			wantErrOut: "Failed to remove credentials from git credential helper",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs, restoreRun := run.Stub()
			defer restoreRun(t)
			if tt.expectGit {
				cs.Register(`git credential reject`, tt.gitExit, "")
			}

			cfg, _ := config.NewIsolatedTestConfig(t, "")
			secureStorage := true
			if tt.staleToken != "" {
				secureStorage = false
			}
			_, err := cfg.Authentication().Login(tt.hostname, "monalisa", tt.token, "https", secureStorage)
			require.NoError(t, err)
			if tt.staleToken != "" {
				require.NoError(t, keyring.Set("gh:"+tt.hostname, "monalisa", tt.staleToken))
			}

			httpClient := &http.Client{}
			var revokedTokens []string
			ios, _, _, stderr := iostreams.Test()
			opts := &LogoutOptions{
				IO:              ios,
				Config:          func() (gh.Config, error) { return cfg, nil },
				PlainHttpClient: func() (*http.Client, error) { return httpClient, nil },
				RevokeToken: func(_ *http.Client, gotHostname, token string) error {
					require.Equal(t, tt.hostname, gotHostname)
					revokedTokens = append(revokedTokens, token)
					return tt.revokeErr
				},
				GitClient: &git.Client{GitPath: "some/path/git"},
				Hostname:  tt.hostname,
				Username:  "monalisa",
			}

			err = logoutRun(opts)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			if tt.wantErrOut != "" {
				require.Contains(t, stderr.String(), tt.wantErrOut)
			}

			if strings.HasPrefix(tt.token, string(gh.TokenTypeOAuth)) && !ghauth.IsEnterprise(tt.hostname) {
				wantTokens := []string{tt.token}
				if tt.staleToken != "" {
					wantTokens = []string{tt.staleToken, tt.token}
				}
				require.Equal(t, wantTokens, revokedTokens)
			} else {
				require.Empty(t, revokedTokens)
			}

			token, _, tokenErr := cfg.Authentication().TokenForUser(tt.hostname, "monalisa")
			if tt.wantToken {
				require.NoError(t, tokenErr)
				require.Equal(t, tt.token, token)
			} else {
				require.Error(t, tokenErr)
			}
		})
	}
}

func hasNoToken(hostname string) tokenAssertion {
	return func(t *testing.T, cfg gh.Config) {
		t.Helper()

		token, _ := cfg.Authentication().ActiveToken(hostname)
		require.Empty(t, token)
	}
}

func hasActiveToken(hostname string, expectedToken string) tokenAssertion {
	return func(t *testing.T, cfg gh.Config) {
		t.Helper()

		token, _ := cfg.Authentication().ActiveToken(hostname)
		require.Equal(t, expectedToken, token)
	}
}
