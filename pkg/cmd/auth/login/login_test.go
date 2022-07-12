package login

import (
	"bytes"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func stubHomeDir(t *testing.T, dir string) {
	homeEnv := "HOME"
	switch runtime.GOOS {
	case "windows":
		homeEnv = "USERPROFILE"
	case "plan9":
		homeEnv = "home"
	}
	oldHomeDir := os.Getenv(homeEnv)
	os.Setenv(homeEnv, dir)
	t.Cleanup(func() {
		os.Setenv(homeEnv, oldHomeDir)
	})
}

func Test_NewCmdLogin(t *testing.T) {
	tests := []struct {
		name     string
		cli      string
		stdin    string
		stdinTTY bool
		wants    LoginOptions
		wantsErr bool
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
		name       string
		opts       *LoginOptions
		httpStubs  func(*httpmock.Registry)
		cfgStubs   func(*config.ConfigMock)
		wantHosts  string
		wantErr    string
		wantStderr string
	}{
		{
			name: "with token",
			opts: &LoginOptions{
				Hostname: "github.com",
				Token:    "abc123",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", ""), httpmock.ScopesResponder("repo,read:org"))
			},
			wantHosts: "github.com:\n    oauth_token: abc123\n",
		},
		{
			name: "with token and non-default host",
			opts: &LoginOptions{
				Hostname: "albert.wesker",
				Token:    "abc123",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "api/v3/"), httpmock.ScopesResponder("repo,read:org"))
			},
			wantHosts: "albert.wesker:\n    oauth_token: abc123\n",
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
				Hostname: "github.com",
				Token:    "abc456",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", ""), httpmock.ScopesResponder("repo,admin:org"))
			},
			wantHosts: "github.com:\n    oauth_token: abc456\n",
		},
		{
			name: "github.com token from environment",
			opts: &LoginOptions{
				Hostname: "github.com",
				Token:    "abc456",
			},
			cfgStubs: func(c *config.ConfigMock) {
				c.AuthTokenFunc = func(string) (string, string) {
					return "value_from_env", "GH_TOKEN"
				}
			},
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
			cfgStubs: func(c *config.ConfigMock) {
				c.AuthTokenFunc = func(string) (string, string) {
					return "value_from_env", "GH_ENTERPRISE_TOKEN"
				}
			},
			wantErr: "SilentError",
			wantStderr: heredoc.Doc(`
				The value of the GH_ENTERPRISE_TOKEN environment variable is being used for authentication.
				To have GitHub CLI store credentials instead, first clear the value from the environment.
			`),
		},
	}

	for _, tt := range tests {
		ios, _, stdout, stderr := iostreams.Test()
		ios.SetStdinTTY(false)
		ios.SetStdoutTTY(false)
		tt.opts.IO = ios

		t.Run(tt.name, func(t *testing.T) {
			readConfigs := config.StubWriteConfig(t)
			cfg := config.NewBlankConfig()
			if tt.cfgStubs != nil {
				tt.cfgStubs(cfg)
			}
			tt.opts.Config = func() (config.Config, error) {
				return cfg, nil
			}

			reg := &httpmock.Registry{}
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
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

			assert.Equal(t, "", stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
			assert.Equal(t, tt.wantHosts, hostsBuf.String())
			reg.Verify(t)
		})
	}
}

func Test_loginRun_Survey(t *testing.T) {
	stubHomeDir(t, t.TempDir())

	tests := []struct {
		name       string
		opts       *LoginOptions
		httpStubs  func(*httpmock.Registry)
		askStubs   func(*prompt.AskStubber)
		runStubs   func(*run.CommandStubber)
		wantHosts  string
		wantErrOut *regexp.Regexp
		cfgStubs   func(*config.ConfigMock)
	}{
		{
			name: "already authenticated",
			opts: &LoginOptions{
				Interactive: true,
			},
			cfgStubs: func(c *config.ConfigMock) {
				c.AuthTokenFunc = func(h string) (string, string) {
					return "ghi789", "oauth_token"
				}
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", ""), httpmock.ScopesResponder("repo,read:org"))
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubPrompt("What account do you want to log into?").AnswerWith("GitHub.com")
				as.StubPrompt("You're already logged into github.com. Do you want to re-authenticate?").AnswerWith(false)
			},
			wantHosts:  "",
			wantErrOut: nil,
		},
		{
			name: "hostname set",
			opts: &LoginOptions{
				Hostname:    "rebecca.chambers",
				Interactive: true,
			},
			wantHosts: heredoc.Doc(`
				rebecca.chambers:
				    oauth_token: def456
				    user: jillv
				    git_protocol: https
			`),
			askStubs: func(as *prompt.AskStubber) {
				as.StubPrompt("What is your preferred protocol for Git operations?").AnswerWith("HTTPS")
				as.StubPrompt("Authenticate Git with your GitHub credentials?").AnswerWith(false)
				as.StubPrompt("How would you like to authenticate GitHub CLI?").AnswerWith("Paste an authentication token")
				as.StubPrompt("Paste your authentication token:").AnswerWith("def456")
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
			name: "choose enterprise",
			wantHosts: heredoc.Doc(`
				brad.vickers:
				    oauth_token: def456
				    user: jillv
				    git_protocol: https
			`),
			opts: &LoginOptions{
				Interactive: true,
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubPrompt("What account do you want to log into?").AnswerWith("GitHub Enterprise Server")
				as.StubPrompt("GHE hostname:").AnswerWith("brad.vickers")
				as.StubPrompt("What is your preferred protocol for Git operations?").AnswerWith("HTTPS")
				as.StubPrompt("Authenticate Git with your GitHub credentials?").AnswerWith(false)
				as.StubPrompt("How would you like to authenticate GitHub CLI?").AnswerWith("Paste an authentication token")
				as.StubPrompt("Paste your authentication token:").AnswerWith("def456")
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
				    oauth_token: def456
				    user: jillv
				    git_protocol: https
			`),
			opts: &LoginOptions{
				Interactive: true,
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubPrompt("What account do you want to log into?").AnswerWith("GitHub.com")
				as.StubPrompt("What is your preferred protocol for Git operations?").AnswerWith("HTTPS")
				as.StubPrompt("Authenticate Git with your GitHub credentials?").AnswerWith(false)
				as.StubPrompt("How would you like to authenticate GitHub CLI?").AnswerWith("Paste an authentication token")
				as.StubPrompt("Paste your authentication token:").AnswerWith("def456")
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
				    oauth_token: def456
				    user: jillv
				    git_protocol: ssh
			`),
			opts: &LoginOptions{
				Interactive: true,
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubPrompt("What account do you want to log into?").AnswerWith("GitHub.com")
				as.StubPrompt("What is your preferred protocol for Git operations?").AnswerWith("SSH")
				as.StubPrompt("Generate a new SSH key to add to your GitHub account?").AnswerWith(false)
				as.StubPrompt("How would you like to authenticate GitHub CLI?").AnswerWith("Paste an authentication token")
				as.StubPrompt("Paste your authentication token:").AnswerWith("def456")
			},
			wantErrOut: regexp.MustCompile("Tip: you can generate a Personal Access Token here https://github.com/settings/tokens"),
		},
		// TODO how to test browser auth?
	}

	for _, tt := range tests {
		if tt.opts == nil {
			tt.opts = &LoginOptions{}
		}
		ios, _, _, stderr := iostreams.Test()

		ios.SetStdinTTY(true)
		ios.SetStderrTTY(true)
		ios.SetStdoutTTY(true)

		tt.opts.IO = ios

		readConfigs := config.StubWriteConfig(t)

		cfg := config.NewBlankConfig()
		if tt.cfgStubs != nil {
			tt.cfgStubs(cfg)
		}
		tt.opts.Config = func() (config.Config, error) {
			return cfg, nil
		}

		t.Run(tt.name, func(t *testing.T) {
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

			as := prompt.NewAskStubber(t)
			if tt.askStubs != nil {
				tt.askStubs(as)
			}

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

			assert.Equal(t, tt.wantHosts, hostsBuf.String())
			if tt.wantErrOut == nil {
				assert.Equal(t, "", stderr.String())
			} else {
				assert.Regexp(t, tt.wantErrOut, stderr.String())
			}
			reg.Verify(t)
		})
	}
}
