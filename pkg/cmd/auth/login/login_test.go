package login

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

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
			wantsErr: true,
			cli:      "--with-token",
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
			wantsErr: true,
			cli:      "--with-token",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

func Test_loginRun_Nontty(t *testing.T) {
	tests := []struct {
		name      string
		opts      *LoginOptions
		httpStubs func(*httpmock.Registry)
		wantHosts string
		wantsErr  bool
	}{
		{
			name:     "no arguments",
			opts:     &LoginOptions{},
			wantsErr: true,
		},
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
			wantHosts: "albert.wesker:\n    oauth_token: abc123\n",
		},
		{
			name: "insufficient scopes",
			opts: &LoginOptions{
				Hostname: "github.com",
				Token:    "abc456",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(httpmock.REST("GET", "user"), scopesResponder("read:org"))
			},
			wantsErr: true,
		},
	}

	for _, tt := range tests {
		io, _, stdout, stderr := iostreams.Test()

		io.SetStdinTTY(false)
		io.SetStdoutTTY(false)

		tt.opts.Config = func() (config.Config, error) {
			return config.NewBlankConfig(), nil
		}

		reg := &httpmock.Registry{}
		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}

		if tt.httpStubs != nil {
			tt.httpStubs(reg)
		} else {
			reg.Register(httpmock.REST("GET", "user"), scopesResponder("repo,read:org,"))
		}

		tt.opts.IO = io
		t.Run(tt.name, func(t *testing.T) {
			mainBuf := bytes.Buffer{}
			hostsBuf := bytes.Buffer{}
			defer config.StubWriteConfig(&mainBuf, &hostsBuf)()

			err := loginRun(tt.opts)
			assert.Equal(t, tt.wantsErr, (err != nil))
			if err != nil {
				return
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
		rr        bool
	}{
		{
			name: "hostname set",
			opts: &LoginOptions{
				Hostname: "rebecca.chambers",
			},
			wantHosts: "rebecca.chambers:\n    oauth_token: def456\n    git_protocol: https\n",
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(1)        // auth mode: token
				as.StubOne("def456") // auth token
				as.StubOne("HTTPS")  // git_protocol
			},
		},
		{
			name:      "choose enterprise",
			wantHosts: "brad.vickers:\n    oauth_token: def456\n    git_protocol: https\n",
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(1)              // host type enterprise
				as.StubOne("brad.vickers") // hostname
				as.StubOne(1)              // auth mode: token
				as.StubOne("def456")       // auth token
				as.StubOne("HTTPS")        // git_protocol
			},
		},
		{
			name:      "choose github.com",
			wantHosts: "github.com:\n    oauth_token: def456\n    git_protocol: https\n",
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(0)        // host type github.com
				as.StubOne(1)        // auth mode: token
				as.StubOne("def456") // auth token
				as.StubOne("HTTPS")  // git_protocol
			},
		},
		{
			name:      "sets git_protocol",
			wantHosts: "github.com:\n    oauth_token: def456\n    git_protocol: ssh\n",
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

		tt.opts.Config = func() (config.Config, error) {
			return config.NewBlankConfig(), nil
		}

		reg := &httpmock.Registry{}
		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}

		if tt.httpStubs != nil {
			tt.httpStubs(reg)
		} else {
			reg.Register(httpmock.REST("GET", "user"), scopesResponder("repo,read:org,"))
			reg.Register(
				httpmock.GraphQL(`query UserCurrent\b`),
				httpmock.StringResponse(`{"data":{"viewer":{"login":"jillv"}}}`))
		}

		tt.opts.IO = io
		t.Run(tt.name, func(t *testing.T) {
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

			// TODO is there value in checking the output of stderr? It's full of stuff and largely I
			// don't think is important to test.
			assert.Equal(t, tt.wantHosts, hostsBuf.String())
			reg.Verify(t)
		})
	}
}
