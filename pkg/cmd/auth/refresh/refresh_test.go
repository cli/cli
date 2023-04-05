package refresh

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

// TODO prompt cfg test

func Test_NewCmdRefresh(t *testing.T) {
	tests := []struct {
		name        string
		cli         string
		wants       RefreshOptions
		wantsErr    bool
		tty         bool
		neverPrompt bool
	}{
		{
			name: "tty no arguments",
			tty:  true,
			wants: RefreshOptions{
				Hostname: "",
			},
		},
		{
			name:     "nontty no arguments",
			wantsErr: true,
		},
		{
			name: "nontty hostname",
			cli:  "-h aline.cedrac",
			wants: RefreshOptions{
				Hostname: "aline.cedrac",
			},
		},
		{
			name: "tty hostname",
			tty:  true,
			cli:  "-h aline.cedrac",
			wants: RefreshOptions{
				Hostname: "aline.cedrac",
			},
		},
		{
			name:        "prompts disabled, no args",
			tty:         true,
			cli:         "",
			neverPrompt: true,
			wantsErr:    true,
		},
		{
			name:        "prompts disabled, hostname",
			tty:         true,
			cli:         "-h aline.cedrac",
			neverPrompt: true,
			wants: RefreshOptions{
				Hostname: "aline.cedrac",
			},
		},
		{
			name: "tty one scope",
			tty:  true,
			cli:  "--scopes repo:invite",
			wants: RefreshOptions{
				Scopes: []string{"repo:invite"},
			},
		},
		{
			name: "tty scopes",
			tty:  true,
			cli:  "--scopes repo:invite,read:public_key",
			wants: RefreshOptions{
				Scopes: []string{"repo:invite", "read:public_key"},
			},
		},
		{
			name:  "secure storage",
			tty:   true,
			cli:   "--secure-storage",
			wants: RefreshOptions{},
		},
		{
			name: "insecure storage",
			tty:  true,
			cli:  "--insecure-storage",
			wants: RefreshOptions{
				InsecureStorage: true,
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
			ios.SetNeverPrompt(tt.neverPrompt)

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts *RefreshOptions
			cmd := NewCmdRefresh(f, func(opts *RefreshOptions) error {
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
			assert.Equal(t, tt.wants.Scopes, gotOpts.Scopes)
		})
	}
}

type authArgs struct {
	hostname      string
	scopes        []string
	interactive   bool
	secureStorage bool
}

func Test_refreshRun(t *testing.T) {
	tests := []struct {
		name          string
		opts          *RefreshOptions
		prompterStubs func(*prompter.PrompterMock)
		cfgHosts      []string
		config        config.Config
		oldScopes     string
		wantErr       string
		nontty        bool
		wantAuthArgs  authArgs
	}{
		{
			name:    "no hosts configured",
			opts:    &RefreshOptions{},
			wantErr: `not logged in to any hosts`,
		},
		{
			name: "hostname given but dne",
			cfgHosts: []string{
				"github.com",
				"aline.cedrac",
			},
			opts: &RefreshOptions{
				Hostname: "obed.morton",
			},
			wantErr: `not logged in to obed.morton`,
		},
		{
			name: "hostname provided and is configured",
			cfgHosts: []string{
				"obed.morton",
				"github.com",
			},
			opts: &RefreshOptions{
				Hostname: "obed.morton",
			},
			wantAuthArgs: authArgs{
				hostname:      "obed.morton",
				scopes:        nil,
				secureStorage: true,
			},
		},
		{
			name: "no hostname, one host configured",
			cfgHosts: []string{
				"github.com",
			},
			opts: &RefreshOptions{
				Hostname: "",
			},
			wantAuthArgs: authArgs{
				hostname:      "github.com",
				scopes:        nil,
				secureStorage: true,
			},
		},
		{
			name: "no hostname, multiple hosts configured",
			cfgHosts: []string{
				"github.com",
				"aline.cedrac",
			},
			opts: &RefreshOptions{
				Hostname: "",
			},
			prompterStubs: func(pm *prompter.PrompterMock) {
				pm.SelectFunc = func(_, _ string, opts []string) (int, error) {
					return prompter.IndexFor(opts, "github.com")
				}
			},
			wantAuthArgs: authArgs{
				hostname:      "github.com",
				scopes:        nil,
				secureStorage: true,
			},
		},
		{
			name: "scopes provided",
			cfgHosts: []string{
				"github.com",
			},
			opts: &RefreshOptions{
				Scopes: []string{"repo:invite", "public_key:read"},
			},
			wantAuthArgs: authArgs{
				hostname:      "github.com",
				scopes:        []string{"repo:invite", "public_key:read"},
				secureStorage: true,
			},
		},
		{
			name: "more scopes provided",
			cfgHosts: []string{
				"github.com",
			},
			oldScopes: "delete_repo, codespace",
			opts: &RefreshOptions{
				Scopes: []string{"repo:invite", "public_key:read"},
			},
			wantAuthArgs: authArgs{
				hostname:      "github.com",
				scopes:        []string{"repo:invite", "public_key:read", "delete_repo", "codespace"},
				secureStorage: true,
			},
		},
		{
			name: "secure storage",
			cfgHosts: []string{
				"obed.morton",
			},
			opts: &RefreshOptions{
				Hostname: "obed.morton",
			},
			wantAuthArgs: authArgs{
				hostname:      "obed.morton",
				scopes:        nil,
				secureStorage: true,
			},
		},
		{
			name: "insecure storage",
			cfgHosts: []string{
				"obed.morton",
			},
			opts: &RefreshOptions{
				Hostname:        "obed.morton",
				InsecureStorage: true,
			},
			wantAuthArgs: authArgs{
				hostname: "obed.morton",
				scopes:   nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aa := authArgs{}
			tt.opts.AuthFlow = func(_ *config.AuthConfig, _ *iostreams.IOStreams, hostname string, scopes []string, interactive, secureStorage bool) error {
				aa.hostname = hostname
				aa.scopes = scopes
				aa.interactive = interactive
				aa.secureStorage = secureStorage
				return nil
			}

			var cfg config.Config
			if tt.config != nil {
				cfg = tt.config
			} else {
				cfg = config.NewFromString("")
				for _, hostname := range tt.cfgHosts {
					cfg.Set(hostname, "oauth_token", "abc123")
				}
			}
			tt.opts.Config = func() (config.Config, error) {
				return cfg, nil
			}

			ios, _, _, _ := iostreams.Test()
			ios.SetStdinTTY(!tt.nontty)
			ios.SetStdoutTTY(!tt.nontty)
			tt.opts.IO = ios

			httpReg := &httpmock.Registry{}
			httpReg.Register(
				httpmock.REST("GET", ""),
				func(req *http.Request) (*http.Response, error) {
					statusCode := 200
					if req.Header.Get("Authorization") != "token abc123" {
						statusCode = 400
					}
					return &http.Response{
						Request:    req,
						StatusCode: statusCode,
						Body:       io.NopCloser(strings.NewReader(``)),
						Header: http.Header{
							"X-Oauth-Scopes": {tt.oldScopes},
						},
					}, nil
				},
			)
			tt.opts.HttpClient = &http.Client{Transport: httpReg}

			pm := &prompter.PrompterMock{}
			if tt.prompterStubs != nil {
				tt.prompterStubs(pm)
			}
			tt.opts.Prompter = pm

			err := refreshRun(tt.opts)
			if tt.wantErr != "" {
				if assert.Error(t, err) {
					assert.Contains(t, err.Error(), tt.wantErr)
				}
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.wantAuthArgs.hostname, aa.hostname)
			assert.Equal(t, tt.wantAuthArgs.scopes, aa.scopes)
			assert.Equal(t, tt.wantAuthArgs.interactive, aa.interactive)
			assert.Equal(t, tt.wantAuthArgs.secureStorage, aa.secureStorage)
		})
	}
}
