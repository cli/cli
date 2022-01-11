package login

import (
	"bytes"
	"net/http"
	"os"
	"regexp"
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
			wantsErr: true,
		},
		{
			name:     "nontty",
			stdinTTY: false,
			cli:      "",
			wantsErr: true,
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
				Hostname: "github.com",
				Web:      true,
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
			io, stdin, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: io,
			}

			io.SetStdoutTTY(true)
			io.SetStdinTTY(tt.stdinTTY)
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
		env        map[string]string
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
			env: map[string]string{
				"GH_TOKEN": "value_from_env",
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
			env: map[string]string{
				"GH_ENTERPRISE_TOKEN": "value_from_env",
			},
			wantErr: "SilentError",
			wantStderr: heredoc.Doc(`
				The value of the GH_ENTERPRISE_TOKEN environment variable is being used for authentication.
				To have GitHub CLI store credentials instead, first clear the value from the environment.
			`),
		},
	}

	for _, tt := range tests {
		io, _, stdout, stderr := iostreams.Test()

		io.SetStdinTTY(false)
		io.SetStdoutTTY(false)

		tt.opts.Config = func() (config.Config, error) {
			cfg := config.NewBlankConfig()
			return config.InheritEnv(cfg), nil
		}

		tt.opts.IO = io
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}

			old_GH_TOKEN := os.Getenv("GH_TOKEN")
			os.Setenv("GH_TOKEN", tt.env["GH_TOKEN"])
			old_GITHUB_TOKEN := os.Getenv("GITHUB_TOKEN")
			os.Setenv("GITHUB_TOKEN", tt.env["GITHUB_TOKEN"])
			old_GH_ENTERPRISE_TOKEN := os.Getenv("GH_ENTERPRISE_TOKEN")
			os.Setenv("GH_ENTERPRISE_TOKEN", tt.env["GH_ENTERPRISE_TOKEN"])
			old_GITHUB_ENTERPRISE_TOKEN := os.Getenv("GITHUB_ENTERPRISE_TOKEN")
			os.Setenv("GITHUB_ENTERPRISE_TOKEN", tt.env["GITHUB_ENTERPRISE_TOKEN"])
			defer func() {
				os.Setenv("GH_TOKEN", old_GH_TOKEN)
				os.Setenv("GITHUB_TOKEN", old_GITHUB_TOKEN)
				os.Setenv("GH_ENTERPRISE_TOKEN", old_GH_ENTERPRISE_TOKEN)
				os.Setenv("GITHUB_ENTERPRISE_TOKEN", old_GITHUB_ENTERPRISE_TOKEN)
			}()

			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			}

			_, restoreRun := run.Stub()
			defer restoreRun(t)

			mainBuf := bytes.Buffer{}
			hostsBuf := bytes.Buffer{}
			defer config.StubWriteConfig(&mainBuf, &hostsBuf)()

			err := loginRun(tt.opts)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, "", stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
			assert.Equal(t, tt.wantHosts, hostsBuf.String())
			reg.Verify(t)
		})
	}
}

func Test_loginRun_Survey(t *testing.T) {
	tests := []struct {
		name       string
		opts       *LoginOptions
		httpStubs  func(*httpmock.Registry)
		askStubs   func(*prompt.AskStubber)
		runStubs   func(*run.CommandStubber)
		wantHosts  string
		wantErrOut *regexp.Regexp
		cfg        func(config.Config)
	}{
		{
			name: "already authenticated",
			opts: &LoginOptions{
				Interactive: true,
			},
			cfg: func(cfg config.Config) {
				_ = cfg.Set("github.com", "oauth_token", "ghi789")
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", ""), httpmock.ScopesResponder("repo,read:org"))
				// reg.Register(
				// 	httpmock.GraphQL(`query UserCurrent\b`),
				// 	httpmock.StringResponse(`{"data":{"viewer":{"login":"jillv"}}}`))
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(0)     // host type github.com
				as.StubOne(false) // do not continue
			},
			wantHosts:  "", // nothing should have been written to hosts
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
				as.StubOne("HTTPS")  // git_protocol
				as.StubOne(false)    // cache credentials
				as.StubOne(1)        // auth mode: token
				as.StubOne("def456") // auth token
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
				as.StubOne(1)              // host type enterprise
				as.StubOne("brad.vickers") // hostname
				as.StubOne("HTTPS")        // git_protocol
				as.StubOne(false)          // cache credentials
				as.StubOne(1)              // auth mode: token
				as.StubOne("def456")       // auth token
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
				as.StubOne(0)        // host type github.com
				as.StubOne("HTTPS")  // git_protocol
				as.StubOne(false)    // cache credentials
				as.StubOne(1)        // auth mode: token
				as.StubOne("def456") // auth token
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
				as.StubOne(0)        // host type github.com
				as.StubOne("SSH")    // git_protocol
				as.StubOne(10)       // TODO: SSH key selection
				as.StubOne(1)        // auth mode: token
				as.StubOne("def456") // auth token
			},
			wantErrOut: regexp.MustCompile("Tip: you can generate a Personal Access Token here https://github.com/settings/tokens"),
		},
		// TODO how to test browser auth?
	}

	for _, tt := range tests {
		if tt.opts == nil {
			tt.opts = &LoginOptions{}
		}
		io, _, _, stderr := iostreams.Test()

		io.SetStdinTTY(true)
		io.SetStderrTTY(true)
		io.SetStdoutTTY(true)

		tt.opts.IO = io

		cfg := config.NewBlankConfig()

		if tt.cfg != nil {
			tt.cfg(cfg)
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

			mainBuf := bytes.Buffer{}
			hostsBuf := bytes.Buffer{}
			defer config.StubWriteConfig(&mainBuf, &hostsBuf)()

			as, teardown := prompt.InitAskStubber()
			defer teardown()
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
