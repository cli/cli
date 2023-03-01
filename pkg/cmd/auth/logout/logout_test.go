package logout

import (
	"bytes"
	"fmt"
	"net/http"
	"regexp"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/zalando/go-keyring"
)

func Test_NewCmdLogout(t *testing.T) {
	tests := []struct {
		name     string
		cli      string
		wants    LogoutOptions
		wantsErr bool
		tty      bool
	}{
		{
			name:     "nontty no arguments",
			cli:      "",
			wantsErr: true,
		},
		{
			name: "tty no arguments",
			tty:  true,
			cli:  "",
			wants: LogoutOptions{
				Hostname: "",
			},
		},
		{
			name: "tty with hostname",
			tty:  true,
			cli:  "--hostname harry.mason",
			wants: LogoutOptions{
				Hostname: "harry.mason",
			},
		},
		{
			name: "nontty with hostname",
			cli:  "--hostname harry.mason",
			wants: LogoutOptions{
				Hostname: "harry.mason",
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
			if tt.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.Hostname, gotOpts.Hostname)
		})

	}
}

func Test_logoutRun_tty(t *testing.T) {
	tests := []struct {
		name          string
		opts          *LogoutOptions
		prompterStubs func(*prompter.PrompterMock)
		cfgHosts      []string
		secureStorage bool
		wantHosts     string
		wantErrOut    *regexp.Regexp
		wantErr       string
	}{
		{
			name:      "no arguments, multiple hosts",
			opts:      &LogoutOptions{},
			cfgHosts:  []string{"cheryl.mason", "github.com"},
			wantHosts: "cheryl.mason:\n    oauth_token: abc123\n",
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(_, _ string, opts []string) (int, error) {
					return prompter.IndexFor(opts, "github.com")
				}
			},
			wantErrOut: regexp.MustCompile(`Logged out of github.com account 'cybilb'`),
		},
		{
			name:       "no arguments, one host",
			opts:       &LogoutOptions{},
			cfgHosts:   []string{"github.com"},
			wantHosts:  "{}\n",
			wantErrOut: regexp.MustCompile(`Logged out of github.com account 'cybilb'`),
		},
		{
			name:    "no arguments, no hosts",
			opts:    &LogoutOptions{},
			wantErr: `not logged in to any hosts`,
		},
		{
			name: "hostname",
			opts: &LogoutOptions{
				Hostname: "cheryl.mason",
			},
			cfgHosts:   []string{"cheryl.mason", "github.com"},
			wantHosts:  "github.com:\n    oauth_token: abc123\n",
			wantErrOut: regexp.MustCompile(`Logged out of cheryl.mason account 'cybilb'`),
		},
		{
			name:          "secure storage",
			secureStorage: true,
			opts: &LogoutOptions{
				Hostname: "github.com",
			},
			cfgHosts:   []string{"github.com"},
			wantHosts:  "{}\n",
			wantErrOut: regexp.MustCompile(`Logged out of github.com account 'cybilb'`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyring.MockInit()
			readConfigs := config.StubWriteConfig(t)
			cfg := config.NewFromString("")
			for _, hostname := range tt.cfgHosts {
				if tt.secureStorage {
					cfg.Set(hostname, "user", "monalisa")
					_ = keyring.Set(fmt.Sprintf("gh:%s", hostname), "", "abc123")
					cfg.Authentication().SetToken("abc123", "keyring")
				} else {
					cfg.Set(hostname, "oauth_token", "abc123")
				}
			}
			tt.opts.Config = func() (config.Config, error) {
				return cfg, nil
			}

			ios, _, _, stderr := iostreams.Test()
			ios.SetStdinTTY(true)
			ios.SetStdoutTTY(true)
			tt.opts.IO = ios

			reg := &httpmock.Registry{}
			reg.Register(
				httpmock.GraphQL(`query UserCurrent\b`),
				httpmock.StringResponse(`{"data":{"viewer":{"login":"cybilb"}}}`),
			)
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}

			pm := &prompter.PrompterMock{}
			if tt.prompterStubs != nil {
				tt.prompterStubs(pm)
			}
			tt.opts.Prompter = pm

			err := logoutRun(tt.opts)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				return
			} else {
				assert.NoError(t, err)
			}

			if tt.wantErrOut == nil {
				assert.Equal(t, "", stderr.String())
			} else {
				assert.True(t, tt.wantErrOut.MatchString(stderr.String()))
			}

			mainBuf := bytes.Buffer{}
			hostsBuf := bytes.Buffer{}
			readConfigs(&mainBuf, &hostsBuf)
			secureToken, _ := cfg.Authentication().TokenFromKeyring(tt.opts.Hostname)

			assert.Equal(t, tt.wantHosts, hostsBuf.String())
			assert.Equal(t, "", secureToken)
			reg.Verify(t)
		})
	}
}

func Test_logoutRun_nontty(t *testing.T) {
	tests := []struct {
		name          string
		opts          *LogoutOptions
		cfgHosts      []string
		secureStorage bool
		ghtoken       string
		wantHosts     string
		wantErr       string
	}{
		{
			name: "hostname, one host",
			opts: &LogoutOptions{
				Hostname: "harry.mason",
			},
			cfgHosts:  []string{"harry.mason"},
			wantHosts: "{}\n",
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
			wantErr: `not logged in to any hosts`,
		},
		{
			name:          "secure storage",
			secureStorage: true,
			opts: &LogoutOptions{
				Hostname: "harry.mason",
			},
			cfgHosts:  []string{"harry.mason"},
			wantHosts: "{}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyring.MockInit()
			readConfigs := config.StubWriteConfig(t)
			cfg := config.NewFromString("")
			for _, hostname := range tt.cfgHosts {
				if tt.secureStorage {
					cfg.Set(hostname, "user", "monalisa")
					_ = keyring.Set(fmt.Sprintf("gh:%s", hostname), "", "abc123")
					cfg.Authentication().SetToken("abc123", "keyring")
				} else {
					cfg.Set(hostname, "oauth_token", "abc123")
				}
			}
			tt.opts.Config = func() (config.Config, error) {
				return cfg, nil
			}

			ios, _, _, stderr := iostreams.Test()
			ios.SetStdinTTY(false)
			ios.SetStdoutTTY(false)
			tt.opts.IO = ios

			reg := &httpmock.Registry{}
			tt.opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}

			err := logoutRun(tt.opts)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, "", stderr.String())

			mainBuf := bytes.Buffer{}
			hostsBuf := bytes.Buffer{}
			readConfigs(&mainBuf, &hostsBuf)
			secureToken, _ := cfg.Authentication().TokenFromKeyring(tt.opts.Hostname)

			assert.Equal(t, tt.wantHosts, hostsBuf.String())
			assert.Equal(t, "", secureToken)
			reg.Verify(t)
		})
	}
}
