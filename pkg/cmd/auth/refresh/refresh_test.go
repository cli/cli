package refresh

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/authflow"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	ghmock "github.com/cli/cli/v2/internal/gh/mock"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/telemetry"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared/gitcredentials"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
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
			name: "tty clipboard",
			tty:  true,
			cli:  "-c",
			wants: RefreshOptions{
				Hostname:  "",
				Clipboard: new(true),
			},
		},
		{
			name: "tty expiring",
			tty:  true,
			cli:  "--expiring",
			wants: RefreshOptions{
				Hostname: "",
				Expiring: new(true),
			},
		},
		{
			name: "tty expiring=false",
			tty:  true,
			cli:  "--expiring=false",
			wants: RefreshOptions{
				Hostname: "",
				Expiring: new(false),
			},
		},
		{
			name:     "nontty no arguments",
			wantsErr: true,
		},
		{
			name: "nontty hostname and clipboard",
			cli:  "-h aline.cedrac -c",
			wants: RefreshOptions{
				Hostname:  "aline.cedrac",
				Clipboard: new(true),
			},
		},
		{
			name: "nontty hostname",
			cli:  "-h aline.cedrac",
			wants: RefreshOptions{
				Hostname: "aline.cedrac",
			},
		},
		{
			name: "tty hostname and clipboard",
			tty:  true,
			cli:  "-h aline.cedrac -c",
			wants: RefreshOptions{
				Hostname:  "aline.cedrac",
				Clipboard: new(true),
			},
		},
		{
			name: "tty hostname and clipboard disabled",
			tty:  true,
			cli:  "-h aline.cedrac --clipboard=false",
			wants: RefreshOptions{
				Hostname:  "aline.cedrac",
				Clipboard: new(false),
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
			cmd := NewCmdRefresh(f, nil, func(opts *RefreshOptions) error {
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
			require.Equal(t, tt.wants.Clipboard, gotOpts.Clipboard)
			require.Equal(t, tt.wants.Expiring, gotOpts.Expiring)
		})
	}
}

type authArgs struct {
	hostname      string
	scopes        []string
	interactive   bool
	clipboard     bool
	secureStorage bool
	expiring      bool
}
type authOut struct {
	username    string
	token       string
	refreshable *gh.Credential
	err         error
}

func Test_refreshRun(t *testing.T) {
	tests := []struct {
		name                   string
		opts                   *RefreshOptions
		prompterStubs          func(*prompter.PrompterMock)
		cfgHosts               []string
		cfgRefreshable         bool
		cfgRefreshableExpired  bool
		refresher              *ghmock.TokenRefresherMock
		authOut                authOut
		oldScopes              string
		clipboard              string
		expiringCredentialsCfg string
		wantErr                string
		nontty                 bool
		wantAuthArgs           authArgs
		wantStoredToken        string
		wantStoredRefreshToken string
		wantStderrContains     string
		wantStderrOmits        string
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
				clipboard:     true,
			},
		},
		{
			name: "no hostname, one host configured, clipboard enabled",
			cfgHosts: []string{
				"github.com",
			},
			opts: &RefreshOptions{
				Hostname:  "",
				Clipboard: new(true),
			},
			wantAuthArgs: authArgs{
				hostname:      "github.com",
				scopes:        []string{},
				secureStorage: true,
				clipboard:     true,
			},
		},
		{
			name: "clipboard disabled by configuration",
			cfgHosts: []string{
				"github.com",
			},
			clipboard: "disabled",
			opts: &RefreshOptions{
				Hostname: "github.com",
			},
			wantAuthArgs: authArgs{
				hostname:      "github.com",
				scopes:        []string{},
				secureStorage: true,
				clipboard:     false,
			},
		},
		{
			name: "clipboard flag overrides enabled default",
			cfgHosts: []string{
				"github.com",
			},
			opts: &RefreshOptions{
				Hostname:  "github.com",
				Clipboard: new(false),
			},
			wantAuthArgs: authArgs{
				hostname:      "github.com",
				scopes:        []string{},
				secureStorage: true,
				clipboard:     false,
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
				clipboard:     true,
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
				clipboard:     true,
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
				clipboard:     true,
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
				clipboard:     true,
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
				clipboard:     true,
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
				hostname:  "obed.morton",
				scopes:    []string{},
				clipboard: true,
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
				clipboard:     true,
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
				clipboard:     true,
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
				clipboard:     true,
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
				clipboard:     true,
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
				clipboard:     true,
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
				clipboard:     true,
			},
		},
		{
			name: "expiring requested, host issues a refreshable credential",
			cfgHosts: []string{
				"github.com",
			},
			opts: &RefreshOptions{
				Hostname: "github.com",
				Expiring: new(true),
			},
			authOut: authOut{
				username:    "test-user",
				token:       "gho_access",
				refreshable: &gh.Credential{Token: "gho_access", RefreshToken: "ghr_refresh"},
			},
			wantAuthArgs: authArgs{
				hostname:      "github.com",
				scopes:        []string{},
				secureStorage: true,
				clipboard:     true,
				expiring:      true,
			},
			wantStoredToken:        "gho_access",
			wantStoredRefreshToken: "ghr_refresh",
		},
		{
			name: "expiring_credentials config is not consulted by refresh without the flag",
			cfgHosts: []string{
				"github.com",
			},
			expiringCredentialsCfg: "enabled",
			opts: &RefreshOptions{
				Hostname: "github.com",
			},
			wantAuthArgs: authArgs{
				hostname:      "github.com",
				scopes:        []string{},
				secureStorage: true,
				clipboard:     true,
				expiring:      false,
			},
			wantStoredToken: "xyz456",
		},
		{
			name: "--expiring=false requests a non-expiring token",
			cfgHosts: []string{
				"github.com",
			},
			expiringCredentialsCfg: "enabled",
			opts: &RefreshOptions{
				Hostname: "github.com",
				Expiring: new(false),
			},
			wantAuthArgs: authArgs{
				hostname:      "github.com",
				scopes:        []string{},
				secureStorage: true,
				clipboard:     true,
				expiring:      false,
			},
			wantStoredToken: "xyz456",
		},
		{
			name: "existing refreshable credential without --expiring keeps expiring",
			cfgHosts: []string{
				"github.com",
			},
			cfgRefreshable: true,
			opts:           &RefreshOptions{},
			authOut: authOut{
				username:    "test-user",
				token:       "gho_access",
				refreshable: &gh.Credential{Token: "gho_access", RefreshToken: "ghr_refresh"},
			},
			wantAuthArgs: authArgs{
				hostname:      "github.com",
				scopes:        []string{},
				secureStorage: true,
				clipboard:     true,
				expiring:      true,
			},
			wantStoredToken:        "gho_access",
			wantStoredRefreshToken: "ghr_refresh",
			wantStderrOmits:        "Original token was an expiring token",
		},
		{
			name: "existing refreshable credential with --expiring=false downgrades to non-expiring",
			cfgHosts: []string{
				"github.com",
			},
			cfgRefreshable: true,
			opts: &RefreshOptions{
				Expiring: new(false),
			},
			wantAuthArgs: authArgs{
				hostname:      "github.com",
				scopes:        []string{},
				secureStorage: true,
				clipboard:     true,
				expiring:      false,
			},
			wantStoredToken:        "xyz456",
			wantStoredRefreshToken: "",
			wantStderrOmits:        "Original token was an expiring token",
		},
		{
			name: "expired refreshable token is refreshed so its scopes are preserved",
			cfgHosts: []string{
				"github.com",
			},
			cfgRefreshableExpired: true,
			refresher:             renewingRefresher(),
			oldScopes:             "repo, read:org",
			opts:                  &RefreshOptions{},
			wantAuthArgs: authArgs{
				hostname:      "github.com",
				scopes:        []string{"repo", "read:org"},
				secureStorage: true,
				clipboard:     true,
				expiring:      true,
			},
			wantStoredToken:        "xyz456",
			wantStoredRefreshToken: "",
		},
		{
			name: "expired refreshable token that cannot be refreshed warns that scopes were not preserved",
			cfgHosts: []string{
				"github.com",
			},
			cfgRefreshableExpired: true,
			refresher:             failingRefresher(),
			opts:                  &RefreshOptions{},
			wantAuthArgs: authArgs{
				hostname:      "github.com",
				scopes:        []string{},
				secureStorage: true,
				clipboard:     true,
				expiring:      true,
			},
			wantStoredToken:        "xyz456",
			wantStoredRefreshToken: "",
			wantStderrContains:     "Could not read the current token's scopes",
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
			tt.opts.AuthFlow = func(_ *http.Client, _ *iostreams.IOStreams, hostname string, scopes []string, interactive, clipboard, requestRefreshToken bool) (*authflow.AuthResult, error) {
				aa.hostname = hostname
				aa.scopes = scopes
				aa.interactive = interactive
				aa.clipboard = clipboard
				aa.expiring = requestRefreshToken
				if tt.authOut != (authOut{}) {
					if tt.authOut.err != nil {
						return nil, tt.authOut.err
					}
					return &authflow.AuthResult{
						Token:       tt.authOut.token,
						Username:    tt.authOut.username,
						Refreshable: tt.authOut.refreshable,
					}, nil
				}
				return &authflow.AuthResult{Token: "xyz456", Username: "test-user"}, nil
			}

			cfg, _ := config.NewIsolatedTestConfig(t, "")
			if tt.clipboard != "" {
				cfg.Set("", "clipboard", tt.clipboard)
			}
			if tt.expiringCredentialsCfg != "" {
				cfg.Set("", "expiring_credentials", tt.expiringCredentialsCfg)
			}
			for _, hostname := range tt.cfgHosts {
				switch {
				case tt.cfgRefreshableExpired:
					// Store a refreshable credential whose access token has already expired and wire the refresher the
					// case provides, so the scope carry-forward path exercises ActiveTokenWithRefresh.
					authCfg := cfg.Authentication()
					authCfg.SetTokenRefresher(tt.refresher)
					expiry := time.Now().Add(-time.Hour)
					_, err := authCfg.LoginRefreshable(hostname, "test-user", gh.Credential{Token: "expired-access", RefreshToken: "ghr_old", ExpiresAt: &expiry}, "https", false)
					require.NoError(t, err)
				case tt.cfgRefreshable:
					_, err := cfg.Authentication().LoginRefreshable(hostname, "test-user", gh.Credential{Token: "abc123", RefreshToken: "ghr_old"}, "https", false)
					require.NoError(t, err)
				default:
					_, err := cfg.Authentication().Login(hostname, "test-user", "abc123", "https", false)
					require.NoError(t, err)
				}
			}
			tt.opts.Config = func() (gh.Config, error) {
				return cfg, nil
			}

			ios, _, _, stderr := iostreams.Test()
			ios.SetStdinTTY(!tt.nontty)
			ios.SetStdoutTTY(!tt.nontty)
			tt.opts.IO = ios

			httpReg := &httpmock.Registry{}
			httpReg.Register(
				httpmock.REST("GET", ""),
				func(req *http.Request) (*http.Response, error) {
					// The expired stored token 400s; the renewed token and the non-expiring token succeed.
					statusCode := 400
					switch req.Header.Get("Authorization") {
					case "token abc123", "token new-access":
						statusCode = 200
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
			tt.opts.PlainHttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: httpReg}, nil
			}

			pm := &prompter.PrompterMock{}
			if tt.prompterStubs != nil {
				tt.prompterStubs(pm)
			}
			tt.opts.Prompter = pm

			// Helper detection now runs for every refresh, so provide a fake helper config to keep the read hermetic
			// instead of shelling out to git.
			tt.opts.helperConfig = &gitcredentials.FakeHelperConfig{
				SelfExecutablePath: "/path/to/gh",
				Helpers:            map[string]gitcredentials.Helper{},
			}

			err := refreshRun(tt.opts)
			if tt.wantErr != "" {
				require.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)

			require.Equal(t, tt.wantAuthArgs.hostname, aa.hostname)
			require.Equal(t, tt.wantAuthArgs.scopes, aa.scopes)
			require.Equal(t, tt.wantAuthArgs.interactive, aa.interactive)
			require.Equal(t, tt.wantAuthArgs.clipboard, aa.clipboard)
			require.Equal(t, tt.wantAuthArgs.expiring, aa.expiring)

			authCfg := cfg.Authentication()
			activeUser, _ := authCfg.ActiveUser(aa.hostname)
			activeToken := authCfg.ActiveToken(aa.hostname).Token
			require.Equal(t, "test-user", activeUser)
			wantStoredToken := tt.wantStoredToken
			if wantStoredToken == "" {
				wantStoredToken = "xyz456"
			}
			require.Equal(t, wantStoredToken, activeToken)
			require.Equal(t, tt.wantStoredRefreshToken, authCfg.ActiveToken(aa.hostname).RefreshToken)

			if tt.wantStderrContains != "" {
				require.Contains(t, stderr.String(), tt.wantStderrContains)
			}
			if tt.wantStderrOmits != "" {
				require.NotContains(t, stderr.String(), tt.wantStderrOmits)
			}
		})
	}
}

func Test_refreshRun_recordsTelemetry(t *testing.T) {
	cfg, _ := config.NewIsolatedTestConfig(t, "")
	_, err := cfg.Authentication().Login("github.com", "test-user", "abc123", "https", false)
	require.NoError(t, err)

	recorder := &telemetry.InvocationRecorderSpy{}
	ios, _, _, _ := iostreams.Test()
	ios.SetStdinTTY(false)
	ios.SetStdoutTTY(false)

	opts := &RefreshOptions{
		IO:          ios,
		Hostname:    "github.com",
		Telemetry:   recorder,
		ResetScopes: true,
		Config: func() (gh.Config, error) {
			return cfg, nil
		},
		AuthFlow: func(_ *http.Client, _ *iostreams.IOStreams, _ string, _ []string, _, _, _ bool) (*authflow.AuthResult, error) {
			return &authflow.AuthResult{Token: "xyz456", Username: "test-user"}, nil
		},
		PlainHttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: &httpmock.Registry{}}, nil
		},
		Prompter: &prompter.PrompterMock{},
		helperConfig: &gitcredentials.FakeHelperConfig{
			SelfExecutablePath: "/path/to/gh",
			Helpers:            map[string]gitcredentials.Helper{},
		},
	}

	require.NoError(t, refreshRun(opts))

	events := recorder.Events()
	require.Len(t, events, 1)
	assert.Equal(t, ghtelemetry.SAMPLE_ALL, recorder.LastSampleRate)
	dims := events[0].Dimensions
	assert.Equal(t, "auth_invocation", events[0].Type)
	assert.Equal(t, "refresh", dims["auth_command"])
	assert.Equal(t, "false", dims["expiring_requested"])
	assert.Equal(t, "false", dims["expiring_setup_cred_helper_cancelled"])
	assert.Equal(t, "https", dims["git_protocol"])
	assert.Equal(t, "false", dims["expiring_received"])
	assert.Equal(t, "true", dims["auth_success"])
	// Helper telemetry is recorded even for this non-interactive refresh. No helper is configured and no setup runs,
	// so before and after are both "none".
	assert.Equal(t, "none", dims["git_credential_helper_before"])
	assert.Equal(t, "none", dims["git_credential_helper_after"])
	assert.Contains(t, []string{"keyring", "plaintext"}, dims["storage_type"])
}

func Test_refreshRun_recordsTelemetryOnFailure(t *testing.T) {
	// Telemetry must reflect how far the refresh got: an issued expiring token is recorded even when storing it fails,
	// and a stored credential counts as a successful refresh even when the credential-helper setup fails afterwards.
	tests := []struct {
		name         string
		interactive  bool
		breakStorage bool
		helperErr    error
		wantSuccess  string
	}{
		{
			name:         "storage fails after an expiring token is issued",
			breakStorage: true,
			wantSuccess:  "false",
		},
		{
			name:        "credential helper setup fails after storage",
			interactive: true,
			helperErr:   errors.New("helper setup failed"),
			wantSuccess: "true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, _ := config.NewIsolatedTestConfig(t, "")
			_, err := cfg.Authentication().Login("github.com", "test-user", "abc123", "https", false)
			require.NoError(t, err)

			if tt.breakStorage {
				// Point the config directory under a regular file so persisting the refreshed credential fails
				// regardless of privileges.
				file := filepath.Join(t.TempDir(), "not-a-dir")
				require.NoError(t, os.WriteFile(file, nil, 0600))
				t.Setenv("GH_CONFIG_DIR", filepath.Join(file, "config"))
			}

			recorder := &telemetry.InvocationRecorderSpy{}
			ios, _, _, _ := iostreams.Test()

			opts := &RefreshOptions{
				IO:          ios,
				Hostname:    "github.com",
				Interactive: tt.interactive,
				Telemetry:   recorder,
				ResetScopes: true,
				Config: func() (gh.Config, error) {
					return cfg, nil
				},
				AuthFlow: func(_ *http.Client, _ *iostreams.IOStreams, _ string, _ []string, _, _, _ bool) (*authflow.AuthResult, error) {
					return &authflow.AuthResult{
						Token:       "xyz456",
						Username:    "test-user",
						Refreshable: &gh.Credential{Token: "xyz456", RefreshToken: "rt456"},
					}, nil
				},
				PlainHttpClient: func() (*http.Client, error) {
					return &http.Client{Transport: &httpmock.Registry{}}, nil
				},
				Prompter: &prompter.PrompterMock{
					ConfirmFunc: func(_ string, _ bool) (bool, error) {
						return true, nil
					},
				},
				helperConfig: &gitcredentials.FakeHelperConfig{
					SelfExecutablePath: "/path/to/gh",
					Helpers:            map[string]gitcredentials.Helper{},
					ConfigureOursErr:   tt.helperErr,
				},
			}

			require.Error(t, refreshRun(opts))

			events := recorder.Events()
			require.Len(t, events, 1)
			dims := events[0].Dimensions
			assert.Equal(t, "true", dims["expiring_received"])
			assert.Equal(t, tt.wantSuccess, dims["auth_success"])
		})
	}
}

func Test_refreshRun_doesNotRecordTelemetryForEnterprise(t *testing.T) {
	cfg, _ := config.NewIsolatedTestConfig(t, "")
	_, err := cfg.Authentication().Login("ghe.example.com", "test-user", "abc123", "https", false)
	require.NoError(t, err)

	recorder := &telemetry.InvocationRecorderSpy{}
	ios, _, _, _ := iostreams.Test()
	ios.SetStdinTTY(false)
	ios.SetStdoutTTY(false)

	opts := &RefreshOptions{
		IO:          ios,
		Hostname:    "ghe.example.com",
		Telemetry:   recorder,
		ResetScopes: true,
		Config: func() (gh.Config, error) {
			return cfg, nil
		},
		AuthFlow: func(_ *http.Client, _ *iostreams.IOStreams, _ string, _ []string, _, _, _ bool) (*authflow.AuthResult, error) {
			return &authflow.AuthResult{Token: "xyz456", Username: "test-user"}, nil
		},
		PlainHttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: &httpmock.Registry{}}, nil
		},
		Prompter: &prompter.PrompterMock{},
		helperConfig: &gitcredentials.FakeHelperConfig{
			SelfExecutablePath: "/path/to/gh",
			Helpers:            map[string]gitcredentials.Helper{},
		},
	}

	require.NoError(t, refreshRun(opts))

	assert.Empty(t, recorder.Events())
}

// renewingRefresher renews any refresh token to a fresh, long-lived credential whose access token is "new-access".
func renewingRefresher() *ghmock.TokenRefresherMock {
	future := time.Now().Add(time.Hour)
	return &ghmock.TokenRefresherMock{
		RefreshFunc: func(_, _ string) (gh.RefreshableCredential, error) {
			return gh.RefreshableCredential{
				AccessToken:  "new-access",
				RefreshToken: "new-refresh",
				ExpiresAt:    &future,
			}, nil
		},
	}
}

// failingRefresher always fails to renew, simulating a transient refresh error.
func failingRefresher() *ghmock.TokenRefresherMock {
	return &ghmock.TokenRefresherMock{
		RefreshFunc: func(_, _ string) (gh.RefreshableCredential, error) {
			return gh.RefreshableCredential{}, errors.New("refresh failed")
		},
	}
}
