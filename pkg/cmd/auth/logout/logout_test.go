package logout

import (
	"bytes"
	"net/http"
	"os"
	"regexp"
	"testing"

	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func Test_NewCmdLogout(t *testing.T) {
	tests := []struct {
		name  string
		cli   string
		wants LogoutOptions
	}{
		{
			name: "with hostname",
			cli:  "--hostname harry.mason",
			wants: LogoutOptions{
				Hostname: "harry.mason",
			},
		},
		{
			name: "no arguments",
			cli:  "",
			wants: LogoutOptions{
				Hostname: "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: io,
			}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

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
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.Hostname, gotOpts.Hostname)
		})

	}
}

func Test_logoutRun_tty(t *testing.T) {
	tests := []struct {
		name       string
		opts       *LogoutOptions
		askStubs   func(*prompt.AskStubber)
		cfgHosts   []string
		wantHosts  string
		wantErrOut *regexp.Regexp
		wantErr    *regexp.Regexp
	}{
		{
			name:      "no arguments, multiple hosts",
			opts:      &LogoutOptions{},
			cfgHosts:  []string{"cheryl.mason", "github.com"},
			wantHosts: "cheryl.mason:\n    oauth_token: abc123\n",
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne("github.com")
				as.StubOne(true)
			},
			wantErrOut: regexp.MustCompile(`Logged out of github.com account 'cybilb'`),
		},
		{
			name:     "no arguments, one host",
			opts:     &LogoutOptions{},
			cfgHosts: []string{"github.com"},
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(true)
			},
			wantErrOut: regexp.MustCompile(`Logged out of github.com account 'cybilb'`),
		},
		{
			name:    "no arguments, no hosts",
			opts:    &LogoutOptions{},
			wantErr: regexp.MustCompile(`not logged in to any hosts`),
		},
		{
			name: "hostname",
			opts: &LogoutOptions{
				Hostname: "cheryl.mason",
			},
			cfgHosts:  []string{"cheryl.mason", "github.com"},
			wantHosts: "github.com:\n    oauth_token: abc123\n",
			askStubs: func(as *prompt.AskStubber) {
				as.StubOne(true)
			},
			wantErrOut: regexp.MustCompile(`Logged out of cheryl.mason account 'cybilb'`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, _, stderr := iostreams.Test()

			io.SetStdinTTY(true)
			io.SetStdoutTTY(true)

			tt.opts.IO = io
			cfg := config.NewBlankConfig()
			tt.opts.Config = func() (config.Config, error) {
				return cfg, nil
			}

			for _, hostname := range tt.cfgHosts {
				_ = cfg.Set(hostname, "oauth_token", "abc123")
			}

			reg := &httpmock.Registry{}
			reg.Register(
				httpmock.GraphQL(`query UserCurrent\b`),
				httpmock.StringResponse(`{"data":{"viewer":{"login":"cybilb"}}}`))

			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}

			mainBuf := bytes.Buffer{}
			hostsBuf := bytes.Buffer{}
			defer config.StubWriteConfig(&mainBuf, &hostsBuf)()

			as, teardown := prompt.InitAskStubber()
			defer teardown()
			if tt.askStubs != nil {
				tt.askStubs(as)
			}

			err := logoutRun(tt.opts)
			assert.Equal(t, tt.wantErr == nil, err == nil)
			if err != nil {
				if tt.wantErr != nil {
					assert.True(t, tt.wantErr.MatchString(err.Error()))
					return
				} else {
					t.Fatalf("unexpected error: %s", err)
				}
			}

			if tt.wantErrOut == nil {
				assert.Equal(t, "", stderr.String())
			} else {
				assert.True(t, tt.wantErrOut.MatchString(stderr.String()))
			}

			assert.Equal(t, tt.wantHosts, hostsBuf.String())
			reg.Verify(t)
		})
	}
}

func Test_logoutRun_nontty(t *testing.T) {
	tests := []struct {
		name      string
		opts      *LogoutOptions
		cfgHosts  []string
		wantHosts string
		wantErr   *regexp.Regexp
		ghtoken   string
	}{
		{
			name:    "no arguments",
			wantErr: regexp.MustCompile(`hostname required when not`),
			opts:    &LogoutOptions{},
		},
		{
			name: "hostname, one host",
			opts: &LogoutOptions{
				Hostname: "harry.mason",
			},
			cfgHosts: []string{"harry.mason"},
		},
		{
			name: "hostname, multiple hosts",
			opts: &LogoutOptions{
				Hostname: "harry.mason",
			},
			cfgHosts:  []string{"harry.mason", "cheryl.mason"},
			wantHosts: "cheryl.mason:\n    oauth_token: abc123\n",
		},
		{
			name: "hostname, no hosts",
			opts: &LogoutOptions{
				Hostname: "harry.mason",
			},
			wantErr: regexp.MustCompile(`not logged in to any hosts`),
		},
		{
			name:    "gh token is set",
			opts:    &LogoutOptions{},
			ghtoken: "abc123",
			wantErr: regexp.MustCompile(`GITHUB_TOKEN is set in your environment`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ghtoken := os.Getenv("GITHUB_TOKEN")
			defer func() {
				os.Setenv("GITHUB_TOKEN", ghtoken)
			}()
			os.Setenv("GITHUB_TOKEN", tt.ghtoken)
			io, _, _, stderr := iostreams.Test()

			io.SetStdinTTY(false)
			io.SetStdoutTTY(false)

			tt.opts.IO = io
			cfg := config.NewBlankConfig()
			tt.opts.Config = func() (config.Config, error) {
				return cfg, nil
			}

			for _, hostname := range tt.cfgHosts {
				_ = cfg.Set(hostname, "oauth_token", "abc123")
			}

			reg := &httpmock.Registry{}
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}

			mainBuf := bytes.Buffer{}
			hostsBuf := bytes.Buffer{}
			defer config.StubWriteConfig(&mainBuf, &hostsBuf)()

			err := logoutRun(tt.opts)
			assert.Equal(t, tt.wantErr == nil, err == nil)
			if err != nil {
				if tt.wantErr != nil {
					assert.True(t, tt.wantErr.MatchString(err.Error()))
					return
				} else {
					t.Fatalf("unexpected error: %s", err)
				}
			}

			assert.Equal(t, "", stderr.String())

			assert.Equal(t, tt.wantHosts, hostsBuf.String())
			reg.Verify(t)
		})
	}
}
