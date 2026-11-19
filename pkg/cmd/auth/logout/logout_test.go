package logout

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	ghmock "github.com/cli/cli/v2/internal/gh/mock"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
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
		{
			name:  "local-only logout",
			cli:   "--no-revoke",
			wants: LogoutOptions{NoRevoke: true},
		},
		{
			name: "local-only logout with selected account",
			cli:  "--hostname github.com --user monalisa --no-revoke",
			wants: LogoutOptions{
				Hostname: "github.com", Username: "monalisa", NoRevoke: true,
			},
		},
		{
			name:  "explicit default revocation",
			cli:   "--no-revoke=false",
			wants: LogoutOptions{},
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
			require.Equal(t, tt.wants.NoRevoke, gotOpts.NoRevoke)
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

const logoutRetryHint = "\nLocal credentials were kept.\nRetry, or log out locally without revoking:\n  gh auth logout --hostname github.com --user monalisa --no-revoke"

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
		httpStatus    int
		httpErr       error
		wantHTTPHost  string
		wantHTTPToken string
		wantStderr    string
		retain        bool
		setupConfig   func(*testing.T, gh.Config)
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
		{
			name: "revokes the prompted inactive account",
			opts: &LogoutOptions{},
			cfgHosts: []hostUsers{
				{"github.com", []user{{"monalisa", "gho_selected"}, {"hubot", "ghp_other"}}},
			},
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(_, _ string, options []string) (int, error) {
					return prompter.IndexFor(options, "monalisa (github.com)")
				}
			},
			httpStatus:    204,
			wantHTTPHost:  "api.github.com",
			wantHTTPToken: "gho_selected",
			wantHosts:     "github.com:\n    users:\n        hubot:\n            oauth_token: ghp_other\n    git_protocol: ssh\n    user: hubot\n    oauth_token: ghp_other\n",
			assertToken:   hasActiveToken("github.com", "ghp_other"),
			wantStderr:    "✓ Revoked GitHub CLI OAuth token for monalisa (github.com)\n✓ Logged out of github.com account monalisa\n",
		},
		{
			name: "no-revoke still warns without constructing a client",
			opts: &LogoutOptions{NoRevoke: true},
			cfgHosts: []hostUsers{
				{"github.com", []user{{"monalisa", "gho_saved"}}},
			},
			wantHosts:   "{}\n",
			assertToken: hasNoToken("github.com"),
			wantStderr:  "✓ Logged out of github.com account monalisa\n! Authentication token was not revoked.\n",
		},
		{
			name: "not found retains the keyring credential",
			opts: &LogoutOptions{},
			cfgHosts: []hostUsers{
				{"github.com", []user{{"monalisa", "gho_saved"}}},
			},
			secureStorage: true,
			httpStatus:    404,
			wantHTTPHost:  "api.github.com",
			wantHTTPToken: "gho_saved",
			retain:        true,
			assertToken:   hasActiveToken("github.com", "gho_saved"),
			wantErr:       "failed to revoke GitHub CLI OAuth token: HTTP 404 (Not Found)" + logoutRetryHint,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, readConfigs := config.NewIsolatedTestConfig(t, "")

			for _, hostUsers := range tt.cfgHosts {
				for _, user := range hostUsers.users {
					_, err := cfg.Authentication().Login(
						string(hostUsers.host),
						user.name,
						user.token, "ssh", tt.secureStorage,
					)
					require.NoError(t, err)
				}
			}
			if tt.setupConfig != nil {
				tt.setupConfig(t, cfg)
			}
			var before bytes.Buffer
			readConfigs(io.Discard, &before)

			tt.opts.Config = func() (gh.Config, error) {
				return cfg, nil
			}

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdinTTY(true)
			ios.SetStdoutTTY(true)
			tt.opts.IO = ios

			pm := &prompter.PrompterMock{}
			if tt.prompterStubs != nil {
				tt.prompterStubs(pm)
			}
			tt.opts.Prompter = pm
			tt.opts.Context = context.Background()
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			requests := 0
			if tt.wantHTTPHost != "" {
				reg.Register(httpmock.MatchAny, func(req *http.Request) (*http.Response, error) {
					defer req.Body.Close()
					requests++
					require.Equal(t, http.MethodDelete, req.Method)
					require.Equal(t, tt.wantHTTPHost, req.URL.Host)
					require.True(t, strings.HasSuffix(req.URL.Path, "/token"))
					body, err := io.ReadAll(req.Body)
					require.NoError(t, err)
					require.JSONEq(t, fmt.Sprintf(`{"access_token":%q}`, tt.wantHTTPToken), string(body))
					if tt.httpErr != nil {
						return nil, tt.httpErr
					}
					return &http.Response{
						StatusCode: tt.httpStatus, Header: http.Header{"Content-Type": {"application/json"}},
						Body: io.NopCloser(strings.NewReader(`{"message":"Not Found"}`)), Request: req,
					}, nil
				})
			}
			tt.opts.OAuthAppHttpClient = func() (*http.Client, error) {
				require.NotEmpty(t, tt.wantHTTPHost, "unexpected revocation client construction")
				return &http.Client{Transport: reg}, nil
			}

			err := logoutRun(tt.opts)
			require.Empty(t, stdout.String())
			if tt.wantHTTPHost != "" {
				require.Equal(t, 1, requests)
			}
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				if tt.retain {
					var after bytes.Buffer
					readConfigs(io.Discard, &after)
					require.Equal(t, before.String(), after.String())
					require.Empty(t, stderr.String())
					if tt.assertToken != nil {
						tt.assertToken(t, cfg)
					}
				}
				if tt.wantStderr != "" {
					require.Equal(t, tt.wantStderr, stderr.String())
				}
				return
			} else {
				require.NoError(t, err)
			}

			if tt.wantStderr != "" {
				require.Equal(t, tt.wantStderr, stderr.String())
			} else if tt.wantErrOut == nil {
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
		wantErrMatch  *regexp.Regexp
		httpStatus    int
		httpErr       error
		wantHTTPHost  string
		wantHTTPToken string
		wantStderr    string
		retain        bool
		setupConfig   func(*testing.T, gh.Config)
		initialConfig string
		clientErr     error
		lockErr       error
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
		{
			name:          "revokes a saved OAuth token",
			opts:          &LogoutOptions{},
			cfgHosts:      []hostUsers{{"github.com", []user{{"monalisa", "gho_saved"}}}},
			httpStatus:    204,
			wantHTTPHost:  "api.github.com",
			wantHTTPToken: "gho_saved",
			wantHosts:     "{}\n",
			assertToken:   hasNoToken("github.com"),
			wantStderr:    "✓ Revoked GitHub CLI OAuth token for monalisa (github.com)\n✓ Logged out of github.com account monalisa\n",
		},
		{
			name:          "revokes a keyring OAuth token on GHES",
			opts:          &LogoutOptions{},
			cfgHosts:      []hostUsers{{"enterprise.example.com", []user{{"monalisa", "gho_saved"}}}},
			secureStorage: true,
			httpStatus:    204,
			wantHTTPHost:  "enterprise.example.com",
			wantHTTPToken: "gho_saved",
			wantHosts:     "{}\n",
			assertToken:   hasNoToken("enterprise.example.com"),
			wantStderr:    "✓ Revoked GitHub CLI OAuth token for monalisa (enterprise.example.com)\n✓ Logged out of enterprise.example.com account monalisa\n",
		},
		{
			name: "revokes before switching the active account",
			opts: &LogoutOptions{Username: "monalisa"},
			cfgHosts: []hostUsers{
				{"github.com", []user{{"hubot", "ghp_other"}, {"monalisa", "gho_saved"}}},
			},
			httpStatus:    204,
			wantHTTPHost:  "api.github.com",
			wantHTTPToken: "gho_saved",
			wantHosts:     "github.com:\n    users:\n        hubot:\n            oauth_token: ghp_other\n    git_protocol: ssh\n    user: hubot\n    oauth_token: ghp_other\n",
			assertToken:   hasActiveToken("github.com", "ghp_other"),
			wantStderr:    "✓ Revoked GitHub CLI OAuth token for monalisa (github.com)\n✓ Logged out of github.com account monalisa\n✓ Switched active account for github.com to hubot\n",
		},
		{
			name:       "classic PAT warns without HTTP",
			opts:       &LogoutOptions{},
			cfgHosts:   []hostUsers{{"github.com", []user{{"monalisa", "ghp_saved"}}}},
			wantHosts:  "{}\n",
			wantStderr: "✓ Logged out of github.com account monalisa\n! Personal access token was not revoked.\n  Revoke it at https://github.com/settings/tokens\n",
		},
		{
			name:       "fine-grained PAT warning uses the selected host",
			opts:       &LogoutOptions{},
			cfgHosts:   []hostUsers{{"enterprise.example.com", []user{{"monalisa", "github_pat_saved"}}}},
			wantHosts:  "{}\n",
			wantStderr: "✓ Logged out of enterprise.example.com account monalisa\n! Personal access token was not revoked.\n  Revoke it at https://enterprise.example.com/settings/tokens\n",
		},
		{
			name:       "GitHub App token warns without HTTP",
			opts:       &LogoutOptions{},
			cfgHosts:   []hostUsers{{"github.com", []user{{"monalisa", "ghu_saved"}}}},
			wantHosts:  "{}\n",
			wantStderr: "✓ Logged out of github.com account monalisa\n! GitHub App token was not revoked.\n  Revoke it through the app that issued it.\n",
		},
		{
			name:       "GitHub App installation token warns without HTTP",
			opts:       &LogoutOptions{},
			cfgHosts:   []hostUsers{{"github.com", []user{{"monalisa", "ghs_saved"}}}},
			wantHosts:  "{}\n",
			wantStderr: "✓ Logged out of github.com account monalisa\n! GitHub App token was not revoked.\n  Revoke it through the app that issued it.\n",
		},
		{
			name:       "unrecognized token warns without probing",
			opts:       &LogoutOptions{},
			cfgHosts:   []hostUsers{{"github.com", []user{{"monalisa", "opaque-saved-token"}}}},
			wantHosts:  "{}\n",
			wantStderr: "✓ Logged out of github.com account monalisa\n! Authentication token was not revoked.\n",
		},
		{
			name:       "no-revoke keeps its warning for an OAuth token",
			opts:       &LogoutOptions{NoRevoke: true},
			cfgHosts:   []hostUsers{{"github.com", []user{{"monalisa", "gho_saved"}}}},
			wantHosts:  "{}\n",
			wantStderr: "✓ Logged out of github.com account monalisa\n! Authentication token was not revoked.\n",
		},
		{
			name:       "no-revoke keeps its warning for a PAT",
			opts:       &LogoutOptions{NoRevoke: true},
			cfgHosts:   []hostUsers{{"github.com", []user{{"monalisa", "ghp_saved"}}}},
			wantHosts:  "{}\n",
			wantStderr: "✓ Logged out of github.com account monalisa\n! Authentication token was not revoked.\n",
		},
		{
			name:          "no-revoke does not require resolving the saved credential",
			opts:          &LogoutOptions{NoRevoke: true},
			initialConfig: "hosts:\n  github.com:\n    user: monalisa\n    users:\n      monalisa: {}\n",
			assertToken: func(t *testing.T, cfg gh.Config) {
				require.Empty(t, cfg.Authentication().UsersForHost("github.com"))
				require.Empty(t, cfg.Authentication().Hosts())
			},
			wantStderr: "✓ Logged out of github.com account monalisa\n! Authentication token was not revoked.\n",
		},
		{
			name:        "OAuth revocation requires the advisory lock",
			opts:        &LogoutOptions{},
			cfgHosts:    []hostUsers{{"github.com", []user{{"monalisa", "gho_saved"}}}},
			lockErr:     errors.New("locking unavailable"),
			retain:      true,
			assertToken: hasActiveToken("github.com", "gho_saved"),
			wantErr:     "could not lock authentication credentials: locking unavailable" + logoutRetryHint,
		},
		{
			name:        "PAT logout does not require the advisory lock",
			opts:        &LogoutOptions{},
			cfgHosts:    []hostUsers{{"github.com", []user{{"monalisa", "ghp_saved"}}}},
			lockErr:     errors.New("locking unavailable"),
			wantHosts:   "{}\n",
			assertToken: hasNoToken("github.com"),
			wantStderr:  "✓ Logged out of github.com account monalisa\n! Personal access token was not revoked.\n  Revoke it at https://github.com/settings/tokens\n",
		},
		{
			name:        "client setup failure retains credentials",
			opts:        &LogoutOptions{},
			cfgHosts:    []hostUsers{{"github.com", []user{{"monalisa", "gho_saved"}}}},
			clientErr:   errors.New("client construction failed"),
			retain:      true,
			assertToken: hasActiveToken("github.com", "gho_saved"),
			wantErr:     "client construction failed" + logoutRetryHint,
		},
		{
			name:          "network failure retains credentials",
			opts:          &LogoutOptions{},
			cfgHosts:      []hostUsers{{"github.com", []user{{"monalisa", "gho_saved"}}}},
			httpErr:       context.DeadlineExceeded,
			wantHTTPHost:  "api.github.com",
			wantHTTPToken: "gho_saved",
			retain:        true,
			assertToken:   hasActiveToken("github.com", "gho_saved"),
			wantErr:       "failed to revoke GitHub CLI OAuth token: request timed out" + logoutRetryHint,
		},
		{
			name:     "server failure retains a refreshable credential",
			opts:     &LogoutOptions{},
			cfgHosts: []hostUsers{{"github.com", []user{{"monalisa", "gho_saved"}}}},
			setupConfig: func(t *testing.T, cfg gh.Config) {
				_, err := cfg.Authentication().LoginRefreshable("github.com", "monalisa",
					gh.Credential{Token: "gho_saved", RefreshToken: "ghr_saved"}, "ssh", false)
				require.NoError(t, err)
			},
			httpStatus:    500,
			wantHTTPHost:  "api.github.com",
			wantHTTPToken: "gho_saved",
			retain:        true,
			assertToken: func(t *testing.T, cfg gh.Config) {
				credential, err := cfg.Authentication().TokenForUser("github.com", "monalisa")
				require.NoError(t, err)
				require.Equal(t, "gho_saved", credential.Token)
				require.Equal(t, "ghr_saved", credential.RefreshToken)
			},
			wantErr: "failed to revoke GitHub CLI OAuth token: HTTP 500 (Internal Server Error)" + logoutRetryHint,
		},
		{
			name:     "removes an expired access and refresh pair without refreshing",
			opts:     &LogoutOptions{},
			cfgHosts: []hostUsers{{"github.com", []user{{"monalisa", "gho_saved"}}}},
			setupConfig: func(t *testing.T, cfg gh.Config) {
				_, err := cfg.Authentication().LoginRefreshable("github.com", "monalisa",
					gh.Credential{
						Token: "gho_saved", RefreshToken: "ghr_saved",
						ExpiresAt: new(time.Unix(1, 0)),
					}, "ssh", true)
				require.NoError(t, err)
				cfg.Authentication().SetTokenRefresher(&ghmock.TokenRefresherMock{
					RefreshFunc: func(string, string) (gh.RefreshableCredential, error) {
						t.Error("logout must not refresh the saved credential")
						return gh.RefreshableCredential{}, errors.New("unexpected token refresh")
					},
				})
			},
			httpStatus:    204,
			wantHTTPHost:  "api.github.com",
			wantHTTPToken: "gho_saved",
			wantHosts:     "{}\n",
			assertToken: func(t *testing.T, cfg gh.Config) {
				_, err := cfg.Authentication().TokenForUser("github.com", "monalisa")
				require.Error(t, err)
				require.Empty(t, cfg.Authentication().ActiveToken("github.com").Token)
			},
			wantStderr: "✓ Revoked GitHub CLI OAuth token for monalisa (github.com)\n✓ Logged out of github.com account monalisa\n",
		},
		{
			name:     "reports revocation even when local configuration cannot be written",
			opts:     &LogoutOptions{},
			cfgHosts: []hostUsers{{"github.com", []user{{"monalisa", "gho_saved"}}}},
			setupConfig: func(t *testing.T, cfg gh.Config) {
				configFile := filepath.Join(os.Getenv("GH_CONFIG_DIR"), "config.yml")
				require.NoError(t, os.Remove(configFile))
				require.NoError(t, os.Mkdir(configFile, 0o700))
			},
			httpStatus:    204,
			wantHTTPHost:  "api.github.com",
			wantHTTPToken: "gho_saved",
			wantErrMatch:  regexp.MustCompile(`^[^\r\n]*config\.yml[^\r\n]*$`),
			wantStderr:    "✓ Revoked GitHub CLI OAuth token for monalisa (github.com)\n",
		},
		{
			name:     "environment token blocks even local-only logout",
			opts:     &LogoutOptions{NoRevoke: true},
			cfgHosts: []hostUsers{{"github.com", []user{{"monalisa", "gho_saved"}}}},
			setupConfig: func(t *testing.T, cfg gh.Config) {
				cfg.Authentication().SetActiveToken("gho_environment", "GH_TOKEN")
			},
			retain: true,
			assertToken: func(t *testing.T, cfg gh.Config) {
				credential, err := cfg.Authentication().TokenForUser("github.com", "monalisa")
				require.NoError(t, err)
				require.Equal(t, "gho_saved", credential.Token)
			},
			wantErr:    cmdutil.SilentError.Error(),
			wantStderr: "The value of the GH_TOKEN environment variable is being used for authentication.\nTo erase credentials stored in GitHub CLI, first clear the value from the environment.\n",
		},
		{
			name:     "configured API gateway receives the request",
			opts:     &LogoutOptions{},
			cfgHosts: []hostUsers{{"github.com", []user{{"monalisa", "gho_saved"}}}},
			setupConfig: func(t *testing.T, cfg gh.Config) {
				cfg.Set("github.com", "api_host", "gateway.example.com")
			},
			httpStatus:    204,
			wantHTTPHost:  "gateway.example.com",
			wantHTTPToken: "gho_saved",
			wantHosts:     "{}\n",
			wantStderr:    "✓ Revoked GitHub CLI OAuth token for monalisa (github.com)\n✓ Logged out of github.com account monalisa\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, readConfigs := config.NewIsolatedTestConfig(t, tt.initialConfig)

			for _, hostUsers := range tt.cfgHosts {
				for _, user := range hostUsers.users {
					_, err := cfg.Authentication().Login(
						string(hostUsers.host),
						user.name,
						user.token, "ssh", tt.secureStorage,
					)
					require.NoError(t, err)
				}
			}
			if tt.setupConfig != nil {
				tt.setupConfig(t, cfg)
			}
			var before bytes.Buffer
			readConfigs(io.Discard, &before)
			var commandCfg gh.Config = cfg
			if tt.lockErr != nil {
				commandCfg = &ghmock.ConfigMock{
					AuthenticationFunc: func() gh.AuthConfig {
						return logoutLockErrorAuthConfig{AuthConfig: cfg.Authentication(), lockErr: tt.lockErr}
					},
				}
			}
			tt.opts.Config = func() (gh.Config, error) {
				return commandCfg, nil
			}

			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdinTTY(false)
			ios.SetStdoutTTY(false)
			tt.opts.IO = ios
			tt.opts.Context = context.Background()
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			requests := 0
			if tt.wantHTTPHost != "" {
				reg.Register(httpmock.MatchAny, func(req *http.Request) (*http.Response, error) {
					defer req.Body.Close()
					requests++
					require.Equal(t, http.MethodDelete, req.Method)
					require.Equal(t, tt.wantHTTPHost, req.URL.Host)
					require.True(t, strings.HasSuffix(req.URL.Path, "/token"))
					body, err := io.ReadAll(req.Body)
					require.NoError(t, err)
					require.JSONEq(t, fmt.Sprintf(`{"access_token":%q}`, tt.wantHTTPToken), string(body))
					if tt.httpErr != nil {
						return nil, tt.httpErr
					}
					return &http.Response{
						StatusCode: tt.httpStatus, Header: http.Header{"Content-Type": {"application/json"}},
						Body: io.NopCloser(strings.NewReader(`{"message":"Not Found"}`)), Request: req,
					}, nil
				})
			}
			tt.opts.OAuthAppHttpClient = func() (*http.Client, error) {
				if tt.clientErr != nil {
					return nil, tt.clientErr
				}
				require.NotEmpty(t, tt.wantHTTPHost, "unexpected revocation client construction")
				return &http.Client{Transport: reg}, nil
			}

			err := logoutRun(tt.opts)
			require.Empty(t, stdout.String())
			if tt.wantHTTPHost != "" {
				require.Equal(t, 1, requests)
			}
			if tt.wantErr != "" || tt.wantErrMatch != nil {
				if tt.wantErrMatch != nil {
					require.Error(t, err)
					require.Regexp(t, tt.wantErrMatch, err.Error())
					require.NotContains(t, err.Error(), "Local credentials were kept")
				} else {
					require.EqualError(t, err, tt.wantErr)
				}
				if tt.retain {
					var after bytes.Buffer
					readConfigs(io.Discard, &after)
					require.Equal(t, before.String(), after.String())
					if tt.wantStderr == "" {
						require.Empty(t, stderr.String())
					}
					if tt.assertToken != nil {
						tt.assertToken(t, cfg)
					}
				}
				if tt.wantStderr != "" {
					require.Equal(t, tt.wantStderr, stderr.String())
				}
				return
			} else {
				require.NoError(t, err)
			}

			if tt.wantStderr != "" {
				require.Equal(t, tt.wantStderr, stderr.String())
			} else if tt.wantErrOut == nil {
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

type logoutLockErrorAuthConfig struct {
	gh.AuthConfig
	lockErr error
}

func (c logoutLockErrorAuthConfig) LogoutWithCredential(hostname, username string, beforeLogout func(gh.Credential, error) error) error {
	return c.AuthConfig.LogoutWithCredential(hostname, username, func(credential gh.Credential, _ error) error {
		return beforeLogout(credential, c.lockErr)
	})
}

func hasNoToken(hostname string) tokenAssertion {
	return func(t *testing.T, cfg gh.Config) {
		t.Helper()

		token := cfg.Authentication().ActiveToken(hostname).Token
		require.Empty(t, token)
	}
}

func hasActiveToken(hostname string, expectedToken string) tokenAssertion {
	return func(t *testing.T, cfg gh.Config) {
		t.Helper()

		token := cfg.Authentication().ActiveToken(hostname).Token
		require.Equal(t, expectedToken, token)
	}
}
