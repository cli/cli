package login

import (
	"bytes"
	"fmt"
	"net/http"
	"regexp"
	"runtime"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stubHomeDir(t *testing.T, dir string) {
	homeEnv := "HOME"
	switch runtime.GOOS {
	case "windows":
		homeEnv = "USERPROFILE"
	case "plan9":
		homeEnv = "home"
	}
	t.Setenv(homeEnv, dir)
}

func Test_NewCmdLogin(t *testing.T) {
	tests := []struct {
		name        string
		cli         string
		stdin       string
		stdinTTY    bool
		defaultHost string
		wants       LoginOptions
		wantsErr    bool
	}{
		{
			name:  "nontty, with-token",
			stdin: "abc123\n",
			cli:   "--with-token",
			wants: LoginOptions{
				Hostname: "github.com",
				Token:    "abc123",
			},
		},
		{
			name:        "nontty, with-token, enterprise default host",
			stdin:       "abc123\n",
			cli:         "--with-token",
			defaultHost: "git.example.com",
			wants: LoginOptions{
				Hostname: "git.example.com",
				Token:    "abc123",
			},
		},
		{
			name:     "tty, with-token",
			stdinTTY: true,
			stdin:    "def456",
			cli:      "--with-token",
			wants: LoginOptions{
				Hostname: "github.com",
				Token:    "def456",
			},
		},
		{
			name:     "nontty, hostname",
			stdinTTY: false,
			cli:      "--hostname claire.redfield",
			wants: LoginOptions{
				Hostname: "claire.redfield",
				Token:    "",
			},
		},
		{
			name:     "nontty",
			stdinTTY: false,
			cli:      "",
			wants: LoginOptions{
				Hostname: "github.com",
				Token:    "",
			},
		},
		{
			name:  "nontty, with-token, hostname",
			cli:   "--hostname claire.redfield --with-token",
			stdin: "abc123\n",
			wants: LoginOptions{
				Hostname: "claire.redfield",
				Token:    "abc123",
			},
		},
		{
			name:     "tty, with-token, hostname",
			stdinTTY: true,
			stdin:    "ghi789",
			cli:      "--with-token --hostname brad.vickers",
			wants: LoginOptions{
				Hostname: "brad.vickers",
				Token:    "ghi789",
			},
		},
		{
			name:     "tty, hostname",
			stdinTTY: true,
			cli:      "--hostname barry.burton",
			wants: LoginOptions{
				Hostname:    "barry.burton",
				Token:       "",
				Interactive: true,
			},
		},
		{
			name:     "tty",
			stdinTTY: true,
			cli:      "",
			wants: LoginOptions{
				Hostname:    "",
				Token:       "",
				Interactive: true,
			},
		},
		{
			name:     "tty web",
			stdinTTY: true,
			cli:      "--web",
			wants: LoginOptions{
				Hostname:    "github.com",
				Web:         true,
				Interactive: true,
			},
		},
		{
			name: "nontty web",
			cli:  "--web",
			wants: LoginOptions{
				Hostname: "github.com",
				Web:      true,
			},
		},
		{
			name:     "web and with-token",
			cli:      "--web --with-token",
			wantsErr: true,
		},
		{
			name:     "tty one scope",
			stdinTTY: true,
			cli:      "--scopes repo:invite",
			wants: LoginOptions{
				Hostname:    "",
				Scopes:      []string{"repo:invite"},
				Token:       "",
				Interactive: true,
			},
		},
		{
			name:     "tty scopes",
			stdinTTY: true,
			cli:      "--scopes repo:invite,read:public_key",
			wants: LoginOptions{
				Hostname:    "",
				Scopes:      []string{"repo:invite", "read:public_key"},
				Token:       "",
				Interactive: true,
			},
		},
		{
			name:     "tty secure-storage",
			stdinTTY: true,
			cli:      "--secure-storage",
			wants: LoginOptions{
				Interactive: true,
			},
		},
		{
			name: "nontty secure-storage",
			cli:  "--secure-storage",
			wants: LoginOptions{
				Hostname: "github.com",
			},
		},
		{
			name:     "tty insecure-storage",
			stdinTTY: true,
			cli:      "--insecure-storage",
			wants: LoginOptions{
				Interactive:     true,
				InsecureStorage: true,
			},
		},
		{
			name: "nontty insecure-storage",
			cli:  "--insecure-storage",
			wants: LoginOptions{
				Hostname:        "github.com",
				InsecureStorage: true,
			},
		},
		{
			name:     "tty skip-ssh-key",
			stdinTTY: true,
			cli:      "--skip-ssh-key",
			wants: LoginOptions{
				SkipSSHKeyPrompt: true,
				Interactive:      true,
			},
		},
		{
			name: "nontty skip-ssh-key",
			cli:  "--skip-ssh-key",
			wants: LoginOptions{
				Hostname:         "github.com",
				SkipSSHKeyPrompt: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make sure there is a default host set so that
			// the local configuration file never read from.
			if tt.defaultHost == "" {
				tt.defaultHost = "github.com"
			}
			t.Setenv("GH_HOST", tt.defaultHost)

			ios, stdin, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(tt.stdinTTY)
			if tt.stdin != "" {
				stdin.WriteString(tt.stdin)
			}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts *LoginOptions
			cmd := NewCmdLogin(f, func(opts *LoginOptions) error {
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
			if tt.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.Token, gotOpts.Token)
			assert.Equal(t, tt.wants.Hostname, gotOpts.Hostname)
			assert.Equal(t, tt.wants.Web, gotOpts.Web)
			assert.Equal(t, tt.wants.Interactive, gotOpts.Interactive)
			assert.Equal(t, tt.wants.Scopes, gotOpts.Scopes)
		})
	}
}

func Test_loginRun_nontty(t *testing.T) {
	tests := []struct {
		name            string
		opts            *LoginOptions
		env             map[string]string
		httpStubs       func(*httpmock.Registry)
		cfgStubs        func(*testing.T, gh.Config)
		wantHosts       string
		wantErr         string
		wantStderr      string
		wantSecureToken string
	}{
		{
			name: "insecure with token",
			opts: &LoginOptions{
				Hostname:        "github.com",
				Token:           "abc123",
				InsecureStorage: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", ""), httpmock.ScopesResponder("repo,read:org"))
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"monalisa"}}}`))
			},
			wantHosts: "github.com:\n    users:\n        monalisa:\n            oauth_token: abc123\n    oauth_token: abc123\n    user: monalisa\n",
		},
		{
			name: "insecure with token and https git-protocol",
			opts: &LoginOptions{
				Hostname:        "github.com",
				Token:           "abc123",
				GitProtocol:     "https",
				InsecureStorage: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", ""), httpmock.ScopesResponder("repo,read:org"))
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"monalisa"}}}`))
			},
			wantHosts: "github.com:\n    users:\n        monalisa:\n            oauth_token: abc123\n    git_protocol: https\n    oauth_token: abc123\n    user: monalisa\n",
		},
		{
			name: "with token and non-default host",
			opts: &LoginOptions{
				Hostname:        "albert.wesker",
				Token:           "abc123",
				InsecureStorage: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "api/v3/"), httpmock.ScopesResponder("repo,read:org"))
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"monalisa"}}}`))
			},
			wantHosts: "albert.wesker:\n    users:\n        monalisa:\n            oauth_token: abc123\n    oauth_token: abc123\n    user: monalisa\n",
		},
		{
			name: "missing repo scope",
			opts: &LoginOptions{
				Hostname: "github.com",
				Token:    "abc456",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", ""), httpmock.ScopesResponder("read:org"))
			},
			wantErr: `error validating token: missing required scope 'repo'`,
		},
		{
			name: "missing read scope",
			opts: &LoginOptions{
				Hostname: "github.com",
				Token:    "abc456",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", ""), httpmock.ScopesResponder("repo"))
			},
			wantErr: `error validating token: missing required scope 'read:org'`,
		},
		{
			name: "has admin scope",
			opts: &LoginOptions{
				Hostname:        "github.com",
				Token:           "abc456",
				InsecureStorage: true,
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", ""), httpmock.ScopesResponder("repo,admin:org"))
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"monalisa"}}}`))
			},
			wantHosts: "github.com:\n    users:\n        monalisa:\n            oauth_token: abc456\n    oauth_token: abc456\n    user: monalisa\n",
		},
		{
			name: "github.com token from environment",
			opts: &LoginOptions{
				Hostname: "github.com",
				Token:    "abc456",
			},
			env:     map[string]string{"GH_TOKEN": "value_from_env"},
			wantErr: "SilentError",
			wantStderr: heredoc.Doc(`
                The value of the GH_TOKEN environment variable is being used for authentication.
                To have GitHub CLI store credentials instead, first clear the value from the environment.
            `),
		},
		{
			name: "GHE token from environment",
			opts: &LoginOptions{
				Hostname: "ghe.io",
				Token:    "abc456",
			},
			env:     map[string]string{"GH_ENTERPRISE_TOKEN": "value_from_env"},
			wantErr: "SilentError",
			wantStderr: heredoc.Doc(`
                The value of the GH_ENTERPRISE_TOKEN environment variable is being used for authentication.
                To have GitHub CLI store credentials instead, first clear the value from the environment.
            `),
		},
		{
			name: "with token and secure storage",
			opts: &LoginOptions{
				Hostname: "github.com",
				Token:    "abc123",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", ""), httpmock.ScopesResponder("repo,read:org"))
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"monalisa"}}}`))
			},
			wantHosts:       "github.com:\n    users:\n        monalisa:\n    user: monalisa\n",
			wantSecureToken: "abc123",
		},
		{
			name: "given we are already logged in, and log in as a new user, it is added to the config",
			opts: &LoginOptions{
				Hostname: "github.com",
				Token:    "newUserToken",
			},
			cfgStubs: func(t *testing.T, c gh.Config) {
				_, err := c.Authentication().Login("github.com", "monalisa", "abc123", "https", false)
				require.NoError(t, err)
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", ""), httpmock.ScopesResponder("repo,read:org"))
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"newUser"}}}`))
			},
			wantHosts: heredoc.Doc(`
                github.com:
                    users:
                        monalisa:
                            oauth_token: abc123
                        newUser:
                    git_protocol: https
                    user: newUser
            `),
			wantSecureToken: "newUserToken",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdinTTY(false)
			ios.SetStdoutTTY(false)
			tt.opts.IO = ios

			cfg, readConfigs := config.NewIsolatedTestConfig(t)
			if tt.cfgStubs != nil {
				tt.cfgStubs(t, cfg)
			}
			tt.opts.Config = func() (gh.Config, error) {
				return cfg, nil
			}

			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}

			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			_, restoreRun := run.Stub()
			defer restoreRun(t)

			err := loginRun(tt.opts)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}

			mainBuf := bytes.Buffer{}
			hostsBuf := bytes.Buffer{}
			readConfigs(&mainBuf, &hostsBuf)
			secureToken, _ := cfg.Authentication().TokenFromKeyring(tt.opts.Hostname)

			assert.Equal(t, "", stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
			assert.Equal(t, tt.wantHosts, hostsBuf.String())
			assert.Equal(t, tt.wantSecureToken, secureToken)
		})
	}
}

func Test_loginRun_Survey(t *testing.T) {
	stubHomeDir(t, t.TempDir())

	tests := []struct {
		name            string
		opts            *LoginOptions
		httpStubs       func(*httpmock.Registry)
		prompterStubs   func(*prompter.PrompterMock)
		runStubs        func(*run.CommandStubber)
		cfgStubs        func(*testing.T, gh.Config)
		wantHosts       string
		wantErrOut      *regexp.Regexp
		wantSecureToken string
	}{
		{
			name: "hostname set",
			opts: &LoginOptions{
				Hostname:        "rebecca.chambers",
				Interactive:     true,
				InsecureStorage: true,
			},
			wantHosts: heredoc.Doc(`
                rebecca.chambers:
                    users:
                        jillv:
                            oauth_token: def456
                    git_protocol: https
                    oauth_token: def456
                    user: jillv
            `),
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(prompt, _ string, opts []string) (int, error) {
					switch prompt {
					case "What is your preferred protocol for Git operations on this host?":
						return prompter.IndexFor(opts, "HTTPS")
					case "How would you like to authenticate GitHub CLI?":
						return prompter.IndexFor(opts, "Paste an authentication token")
					}
					return -1, prompter.NoSuchPromptErr(prompt)
				}
			},
			runStubs: func(rs *run.CommandStubber) {
				rs.Register(`git config credential\.https:/`, 1, "")
				rs.Register(`git config credential\.helper`, 1, "")
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "api/v3/"), httpmock.ScopesResponder("repo,read:org"))
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"jillv"}}}`))
			},
			wantErrOut: regexp.MustCompile("Tip: you can generate a Personal Access Token here https://rebecca.chambers/settings/tokens"),
		},
		{
			name: "choose Other",
			wantHosts: heredoc.Doc(`
                brad.vickers:
                    users:
                        jillv:
                            oauth_token: def456
                    git_protocol: https
                    oauth_token: def456
                    user: jillv
            `),
			opts: &LoginOptions{
				Interactive:     true,
				InsecureStorage: true,
			},
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(prompt, _ string, opts []string) (int, error) {
					switch prompt {
					case "Where do you use GitHub?":
						return prompter.IndexFor(opts, "Other")
					case "What is your preferred protocol for Git operations on this host?":
						return prompter.IndexFor(opts, "HTTPS")
					case "How would you like to authenticate GitHub CLI?":
						return prompter.IndexFor(opts, "Paste an authentication token")
					}
					return -1, prompter.NoSuchPromptErr(prompt)
				}
				pm.InputHostnameFunc = func() (string, error) {
					return "brad.vickers", nil
				}
			},
			runStubs: func(rs *run.CommandStubber) {
				rs.Register(`git config credential\.https:/`, 1, "")
				rs.Register(`git config credential\.helper`, 1, "")
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "api/v3/"), httpmock.ScopesResponder("repo,read:org,read:public_key"))
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"jillv"}}}`))
			},
			wantErrOut: regexp.MustCompile("Tip: you can generate a Personal Access Token here https://brad.vickers/settings/tokens"),
		},
		{
			name: "choose github.com",
			wantHosts: heredoc.Doc(`
                github.com:
                    users:
                        jillv:
                            oauth_token: def456
                    git_protocol: https
                    oauth_token: def456
                    user: jillv
            `),
			opts: &LoginOptions{
				Interactive:     true,
				InsecureStorage: true,
			},
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(prompt, _ string, opts []string) (int, error) {
					switch prompt {
					case "Where do you use GitHub?":
						return prompter.IndexFor(opts, "GitHub.com")
					case "What is your preferred protocol for Git operations on this host?":
						return prompter.IndexFor(opts, "HTTPS")
					case "How would you like to authenticate GitHub CLI?":
						return prompter.IndexFor(opts, "Paste an authentication token")
					}
					return -1, prompter.NoSuchPromptErr(prompt)
				}
			},
			runStubs: func(rs *run.CommandStubber) {
				rs.Register(`git config credential\.https:/`, 1, "")
				rs.Register(`git config credential\.helper`, 1, "")
			},
			wantErrOut: regexp.MustCompile("Tip: you can generate a Personal Access Token here https://github.com/settings/tokens"),
		},
		{
			name: "sets git_protocol",
			wantHosts: heredoc.Doc(`
                github.com:
                    users:
                        jillv:
                            oauth_token: def456
                    git_protocol: ssh
                    oauth_token: def456
                    user: jillv
            `),
			opts: &LoginOptions{
				Interactive:     true,
				InsecureStorage: true,
			},
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(prompt, _ string, opts []string) (int, error) {
					switch prompt {
					case "Where do you use GitHub?":
						return prompter.IndexFor(opts, "GitHub.com")
					case "What is your preferred protocol for Git operations on this host?":
						return prompter.IndexFor(opts, "SSH")
					case "How would you like to authenticate GitHub CLI?":
						return prompter.IndexFor(opts, "Paste an authentication token")
					}
					return -1, prompter.NoSuchPromptErr(prompt)
				}
			},
			wantErrOut: regexp.MustCompile("Tip: you can generate a Personal Access Token here https://github.com/settings/tokens"),
		},
		{
			name: "secure storage",
			opts: &LoginOptions{
				Hostname:    "github.com",
				Interactive: true,
			},
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(prompt, _ string, opts []string) (int, error) {
					switch prompt {
					case "What is your preferred protocol for Git operations on this host?":
						return prompter.IndexFor(opts, "HTTPS")
					case "How would you like to authenticate GitHub CLI?":
						return prompter.IndexFor(opts, "Paste an authentication token")
					}
					return -1, prompter.NoSuchPromptErr(prompt)
				}
			},
			runStubs: func(rs *run.CommandStubber) {
				rs.Register(`git config credential\.https:/`, 1, "")
				rs.Register(`git config credential\.helper`, 1, "")
			},
			wantHosts: heredoc.Doc(`
                github.com:
                    git_protocol: https
                    users:
                        jillv:
                    user: jillv
            `),
			wantErrOut:      regexp.MustCompile("Logged in as jillv"),
			wantSecureToken: "def456",
		},
		{
			name: "given we log in as a user that is already in the config, we get an informational message",
			opts: &LoginOptions{
				Hostname:        "github.com",
				Interactive:     true,
				InsecureStorage: true,
			},
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(prompt, _ string, opts []string) (int, error) {
					switch prompt {
					case "What is your preferred protocol for Git operations on this host?":
						return prompter.IndexFor(opts, "HTTPS")
					case "How would you like to authenticate GitHub CLI?":
						return prompter.IndexFor(opts, "Paste an authentication token")
					}
					return -1, prompter.NoSuchPromptErr(prompt)
				}
			},
			cfgStubs: func(t *testing.T, c gh.Config) {
				_, err := c.Authentication().Login("github.com", "monalisa", "abc123", "https", false)
				require.NoError(t, err)
			},
			runStubs: func(rs *run.CommandStubber) {
				rs.Register(`git config credential\.https:/`, 1, "")
				rs.Register(`git config credential\.helper`, 1, "")
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", ""), httpmock.ScopesResponder("repo,read:org"))
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"monalisa"}}}`))
			},
			wantHosts: heredoc.Doc(`
            github.com:
                users:
                    monalisa:
                        oauth_token: def456
                git_protocol: https
                user: monalisa
                oauth_token: def456
            `),
			wantErrOut: regexp.MustCompile(`! You were already logged in to this account`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.opts == nil {
				tt.opts = &LoginOptions{}
			}
			ios, _, _, stderr := iostreams.Test()

			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)
			ios.SetStdoutTTY(true)

			tt.opts.IO = ios

			cfg, readConfigs := config.NewIsolatedTestConfig(t)
			if tt.cfgStubs != nil {
				tt.cfgStubs(t, cfg)
			}
			tt.opts.Config = func() (gh.Config, error) {
				return cfg, nil
			}

			reg := &httpmock.Registry{}
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			} else {
				reg.Register(httpmock.REST("GET", ""), httpmock.ScopesResponder("repo,read:org,read:public_key"))
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"jillv"}}}`))
			}

			pm := &prompter.PrompterMock{}
			pm.ConfirmFunc = func(_ string, _ bool) (bool, error) {
				return false, nil
			}
			pm.AuthTokenFunc = func() (string, error) {
				return "def456", nil
			}
			if tt.prompterStubs != nil {
				tt.prompterStubs(pm)
			}
			tt.opts.Prompter = pm

			tt.opts.GitClient = &git.Client{GitPath: "some/path/git"}

			rs, restoreRun := run.Stub()
			defer restoreRun(t)
			if tt.runStubs != nil {
				tt.runStubs(rs)
			}

			err := loginRun(tt.opts)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			mainBuf := bytes.Buffer{}
			hostsBuf := bytes.Buffer{}
			readConfigs(&mainBuf, &hostsBuf)
			secureToken, _ := cfg.Authentication().TokenFromKeyring(tt.opts.Hostname)

			assert.Equal(t, tt.wantHosts, hostsBuf.String())
			assert.Equal(t, tt.wantSecureToken, secureToken)
			if tt.wantErrOut == nil {
				assert.Equal(t, "", stderr.String())
			} else {
				assert.Regexp(t, tt.wantErrOut, stderr.String())
			}
			reg.Verify(t)
		})
	}
}

func Test_promptForHostname(t *testing.T) {
	tests := []struct {
		name          string
		options       []string
		selectedIndex int
		// This is so we can test that the options in the function don't change
		expectedSelection string
		inputHostname     string
		expect            string
	}{
		{
			name:              "select 'GitHub.com'",
			selectedIndex:     0,
			expectedSelection: "GitHub.com",
			expect:            "github.com",
		},
		{
			name:              "select 'Other'",
			selectedIndex:     1,
			expectedSelection: "Other",
			inputHostname:     "github.enterprise.com",
			expect:            "github.enterprise.com",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			promptMock := &prompter.PrompterMock{
				SelectFunc: func(_ string, _ string, options []string) (int, error) {
					if options[tt.selectedIndex] != tt.expectedSelection {
						return 0, fmt.Errorf("expected %s at index %d, but got %s", tt.expectedSelection, tt.selectedIndex, options[tt.selectedIndex])
					}
					return tt.selectedIndex, nil
				},
				InputHostnameFunc: func() (string, error) {
					return tt.inputHostname, nil
				},
			}
			opts := &LoginOptions{
				Prompter: promptMock,
			}
			hostname, err := promptForHostname(opts)
			require.NoError(t, err)
			require.Equal(t, tt.expect, hostname)
		})
	}
}
