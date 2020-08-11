package login

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"testing"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
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
		ghtoken  string
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
			cli:      "--hostname claire.redfield",
			wantsErr: true,
		},
		{
			name:     "nontty",
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
				Hostname: "barry.burton",
				Token:    "",
			},
		},
		{
			name:     "tty",
			stdinTTY: true,
			cli:      "",
			wants: LoginOptions{
				Hostname: "",
				Token:    "",
			},
		},
		{
			name:     "tty, GITHUB_TOKEN",
			stdinTTY: true,
			cli:      "",
			ghtoken:  "abc123",
			wants: LoginOptions{
				Hostname:     "github.com",
				Token:        "abc123",
				OnlyValidate: true,
			},
		},
		{
			name:     "nontty, GITHUB_TOKEN",
			stdinTTY: false,
			cli:      "",
			ghtoken:  "abc123",
			wants: LoginOptions{
				Hostname:     "github.com",
				Token:        "abc123",
				OnlyValidate: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ghtoken := os.Getenv("GITHUB_TOKEN")
			defer func() {
				os.Setenv("GITHUB_TOKEN", ghtoken)
			}()
			os.Setenv("GITHUB_TOKEN", tt.ghtoken)
			io, stdin, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: io,
			}

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
		})
	}
}

func scopesResponder(scopes string) func(*http.Request) (*http.Response, error) {
	return func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Request:    req,
			Header: map[string][]string{
				"X-Oauth-Scopes": {scopes},
			},
			Body: ioutil.NopCloser(bytes.NewBufferString("")),
		}, nil
	}
}

func Test_loginRun_nontty(t *testing.T) {
	tests := []struct {
		name      string
		opts      *LoginOptions
		httpStubs func(*httpmock.Registry)
		wantHosts string
		wantErr   *regexp.Regexp
	}{
		{
			name: "with token",
			opts: &LoginOptions{
				Hostname: "github.com",
				Token:    "abc123",
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
				reg.Register(httpmock.REST("GET", "api/v3/"), scopesResponder("repo,read:org"))
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
				reg.Register(httpmock.REST("GET", ""), scopesResponder("read:org"))
			},
			wantErr: regexp.MustCompile(`missing required scope 'repo'`),
		},
		{
			name: "missing read scope",
			opts: &LoginOptions{
				Hostname: "github.com",
				Token:    "abc456",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", ""), scopesResponder("repo"))
			},
			wantErr: regexp.MustCompile(`missing required scope 'read:org'`),
		},
		{
			name: "has admin scope",
			opts: &LoginOptions{
				Hostname: "github.com",
				Token:    "abc456",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", ""), scopesResponder("repo,admin:org"))
			},
			wantHosts: "github.com:\n    oauth_token: abc456\n",
		},
	}

	for _, tt := range tests {
		io, _, stdout, stderr := iostreams.Test()

		io.SetStdinTTY(false)
		io.SetStdoutTTY(false)

		tt.opts.Config = func() (config.Config, error) {
			return config.NewBlankConfig(), nil
		}

		tt.opts.IO = io
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			origClientFromCfg := clientFromCfg
			defer func() {
				clientFromCfg = origClientFromCfg
			}()
			clientFromCfg = func(_ string, _ config.Config) (*api.Client, error) {
				httpClient := &http.Client{Transport: reg}
				return api.NewClientFromHTTP(httpClient), nil
			}

			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			} else {
				reg.Register(httpmock.REST("GET", ""), scopesResponder("repo,read:org"))
			}

			mainBuf := bytes.Buffer{}
			hostsBuf := bytes.Buffer{}
			defer config.StubWriteConfig(&mainBuf, &hostsBuf)()

			err := loginRun(tt.opts)
			assert.Equal(t, tt.wantErr == nil, err == nil)
			if err != nil {
				if tt.wantErr != nil {
					assert.True(t, tt.wantErr.MatchString(err.Error()))
					return
				} else {
					t.Fatalf("unexpected error: %s", err)
				}
			}

			assert.Equal(t, "", stdout.String())
			assert.Equal(t, "", stderr.String())
			assert.Equal(t, tt.wantHosts, hostsBuf.String())
			reg.Verify(t)
		})
	}
}

func Test_loginRun_Survey(t *testing.T) {
	tests := []struct {
		name      string
		opts      *LoginOptions
		httpStubs func(*httpmock.Registry)
		askStubs  func(*prompt.AskStubber)
		wantHosts string
		cfg       func(config.Config)
	}{
		{
			name: "already authenticated",
			cfg: func(cfg config.Config) {
				_ = cfg.Set("github.com", "oauth_token", "ghi789")
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", ""), scopesResponder("repo,read:org,"))
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"jillv"}}}`))
			},
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(0)     // host type github.com
				as.StubOne(false) // do not continue
			},
			wantHosts: "", // nothing should have been written to hosts
		},
		{
			name: "hostname set",
			opts: &LoginOptions{
				Hostname: "rebecca.chambers",
			},
			wantHosts: "rebecca.chambers:\n    oauth_token: def456\n    git_protocol: https\n    user: jillv\n",
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(1)        // auth mode: token
				as.StubOne("def456") // auth token
				as.StubOne("HTTPS")  // git_protocol
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "api/v3/"), scopesResponder("repo,read:org,"))
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"jillv"}}}`))
			},
		},
		{
			name:      "choose enterprise",
			wantHosts: "brad.vickers:\n    oauth_token: def456\n    git_protocol: https\n    user: jillv\n",
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(1)              // host type enterprise
				as.StubOne("brad.vickers") // hostname
				as.StubOne(1)              // auth mode: token
				as.StubOne("def456")       // auth token
				as.StubOne("HTTPS")        // git_protocol
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "api/v3/"), scopesResponder("repo,read:org,"))
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{"login":"jillv"}}}`))
			},
		},
		{
			name:      "choose github.com",
			wantHosts: "github.com:\n    oauth_token: def456\n    git_protocol: https\n    user: jillv\n",
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(0)        // host type github.com
				as.StubOne(1)        // auth mode: token
				as.StubOne("def456") // auth token
				as.StubOne("HTTPS")  // git_protocol
			},
		},
		{
			name:      "sets git_protocol",
			wantHosts: "github.com:\n    oauth_token: def456\n    git_protocol: ssh\n    user: jillv\n",
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(0)        // host type github.com
				as.StubOne(1)        // auth mode: token
				as.StubOne("def456") // auth token
				as.StubOne("SSH")    // git_protocol
			},
		},
		// TODO how to test browser auth?
	}

	for _, tt := range tests {
		if tt.opts == nil {
			tt.opts = &LoginOptions{}
		}
		io, _, _, _ := iostreams.Test()

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
			origClientFromCfg := clientFromCfg
			defer func() {
				clientFromCfg = origClientFromCfg
			}()
			clientFromCfg = func(_ string, _ config.Config) (*api.Client, error) {
				httpClient := &http.Client{Transport: reg}
				return api.NewClientFromHTTP(httpClient), nil
			}
			if tt.httpStubs != nil {
				tt.httpStubs(reg)
			} else {
				reg.Register(httpmock.REST("GET", ""), scopesResponder("repo,read:org,"))
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

			err := loginRun(tt.opts)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			assert.Equal(t, tt.wantHosts, hostsBuf.String())
			reg.Verify(t)
		})
	}
}
