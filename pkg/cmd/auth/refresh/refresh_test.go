package refresh

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/require"
)

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
		{
			name: "reset scopes",
			tty:  true,
			cli:  "--reset-scopes",
			wants: RefreshOptions{
				ResetScopes: true,
			},
		},
		{
			name: "remove scope",
			tty:  true,
			cli:  "--remove-scopes read:public_key",
			wants: RefreshOptions{
				RemoveScopes: []string{"read:public_key"},
			},
		},
		{
			name: "remove multiple scopes",
			tty:  true,
			cli:  "--remove-scopes workflow,read:public_key",
			wants: RefreshOptions{
				RemoveScopes: []string{"workflow", "read:public_key"},
			},
		},
		{
			name: "remove scope shorthand",
			tty:  true,
			cli:  "-r read:public_key",
			wants: RefreshOptions{
				RemoveScopes: []string{"read:public_key"},
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
			require.NoError(t, err)

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
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wants.Hostname, gotOpts.Hostname)
			require.Equal(t, tt.wants.Scopes, gotOpts.Scopes)
		})
	}
}

type authArgs struct {
	hostname      string
	scopes        []string
	interactive   bool
	secureStorage bool
}

type authOut struct {
	username string
	token    string
	err      error
}

func Test_refreshRun(t *testing.T) {
	tests := []struct {
		name          string
		opts          *RefreshOptions
		prompterStubs func(*prompter.PrompterMock)
		cfgHosts      []string
		authOut       authOut
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
			name: "hostname given but not previously authenticated with it",
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
				scopes:        []string{},
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
				scopes:        []string{},
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
				scopes:        []string{},
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
				scopes:        []string{"delete_repo", "codespace", "repo:invite", "public_key:read"},
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
				scopes:        []string{},
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
				scopes:   []string{},
			},
		},
		{
			name: "reset scopes",
			cfgHosts: []string{
				"github.com",
			},
			oldScopes: "delete_repo, codespace",
			opts: &RefreshOptions{
				Hostname:    "github.com",
				ResetScopes: true,
			},
			wantAuthArgs: authArgs{
				hostname:      "github.com",
				scopes:        []string{},
				secureStorage: true,
			},
		},
		{
			name: "reset scopes and add some scopes",
			cfgHosts: []string{
				"github.com",
			},
			oldScopes: "repo:invite, delete_repo, codespace",
			opts: &RefreshOptions{
				Scopes:      []string{"public_key:read", "workflow"},
				ResetScopes: true,
			},
			wantAuthArgs: authArgs{
				hostname:      "github.com",
				scopes:        []string{"public_key:read", "workflow"},
				secureStorage: true,
			},
		},
		{
			name: "remove scopes",
			cfgHosts: []string{
				"github.com",
			},
			oldScopes: "delete_repo, codespace, repo:invite, public_key:read",
			opts: &RefreshOptions{
				Hostname:     "github.com",
				RemoveScopes: []string{"delete_repo", "repo:invite"},
			},
			wantAuthArgs: authArgs{
				hostname:      "github.com",
				scopes:        []string{"codespace", "public_key:read"},
				secureStorage: true,
			},
		},
		{
			name: "remove scope but no old scope",
			cfgHosts: []string{
				"github.com",
			},
			opts: &RefreshOptions{
				Hostname:     "github.com",
				RemoveScopes: []string{"delete_repo"},
			},
			wantAuthArgs: authArgs{
				hostname:      "github.com",
				scopes:        []string{},
				secureStorage: true,
			},
		},
		{
			name: "remove and add scopes at the same time",
			cfgHosts: []string{
				"github.com",
			},
			oldScopes: "repo:invite, delete_repo, codespace",
			opts: &RefreshOptions{
				Scopes:       []string{"repo:invite", "public_key:read", "workflow"},
				RemoveScopes: []string{"codespace", "repo:invite", "workflow"},
			},
			wantAuthArgs: authArgs{
				hostname:      "github.com",
				scopes:        []string{"delete_repo", "public_key:read"},
				secureStorage: true,
			},
		},
		{
			name: "remove scopes that don't exist",
			cfgHosts: []string{
				"github.com",
			},
			oldScopes: "repo:invite, delete_repo, codespace",
			opts: &RefreshOptions{
				RemoveScopes: []string{"codespace", "repo:invite", "public_key:read"},
			},
			wantAuthArgs: authArgs{
				hostname:      "github.com",
				scopes:        []string{"delete_repo"},
				secureStorage: true,
			},
		},
		{
			name: "errors when active user does not match user returned by auth flow",
			cfgHosts: []string{
				"github.com",
			},
			authOut: authOut{
				username: "not-test-user",
				token:    "xyz456",
			},
			opts:    &RefreshOptions{},
			wantErr: "error refreshing credentials for test-user, received credentials for not-test-user, did you use the correct account in the browser?",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aa := authArgs{}
			tt.opts.AuthFlow = func(_ *iostreams.IOStreams, hostname string, scopes []string, interactive bool) (token, username, error) {
				aa.hostname = hostname
				aa.scopes = scopes
				aa.interactive = interactive
				if tt.authOut != (authOut{}) {
					return token(tt.authOut.token), username(tt.authOut.username), tt.authOut.err
				}
				return token("xyz456"), username("test-user"), nil
			}

			cfg, _ := config.NewIsolatedTestConfig(t)
			for _, hostname := range tt.cfgHosts {
				_, err := cfg.Authentication().Login(hostname, "test-user", "abc123", "https", false)
				require.NoError(t, err)
			}
			tt.opts.Config = func() (gh.Config, error) {
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
				require.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)

			require.Equal(t, tt.wantAuthArgs.hostname, aa.hostname)
			require.Equal(t, tt.wantAuthArgs.scopes, aa.scopes)
			require.Equal(t, tt.wantAuthArgs.interactive, aa.interactive)

			authCfg := cfg.Authentication()
			activeUser, _ := authCfg.ActiveUser(aa.hostname)
			activeToken, _ := authCfg.ActiveToken(aa.hostname)
			require.Equal(t, "test-user", activeUser)
			require.Equal(t, "xyz456", activeToken)
		})
	}
}
