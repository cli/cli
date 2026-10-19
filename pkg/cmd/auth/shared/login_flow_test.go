package shared

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/authflow"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/internal/telemetry"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared/gitcredentials"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type tinyConfig map[string]string

func (c tinyConfig) Login(host, username, token, gitProtocol string, encrypt bool) (bool, error) {
	c[fmt.Sprintf("%s:%s", host, "user")] = username
	c[fmt.Sprintf("%s:%s", host, "oauth_token")] = token
	c[fmt.Sprintf("%s:%s", host, "git_protocol")] = gitProtocol
	return false, nil
}

func (c tinyConfig) LoginRefreshable(host, username string, credential gh.Credential, gitProtocol string, encrypt bool) (bool, error) {
	c[fmt.Sprintf("%s:%s", host, "user")] = username
	c[fmt.Sprintf("%s:%s", host, "oauth_token")] = credential.Token
	c[fmt.Sprintf("%s:%s", host, "refresh_token")] = credential.RefreshToken
	c[fmt.Sprintf("%s:%s", host, "git_protocol")] = gitProtocol
	return false, nil
}

func (c tinyConfig) UsersForHost(hostname string) []string {
	return nil
}

// failingStorageConfig is a login config whose credential storage always fails.
type failingStorageConfig struct{ tinyConfig }

func (failingStorageConfig) Login(string, string, string, string, bool) (bool, error) {
	return false, errors.New("storage failed")
}

func (failingStorageConfig) LoginRefreshable(string, string, gh.Credential, string, bool) (bool, error) {
	return false, errors.New("storage failed")
}

func TestShouldCopyToClipboard(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		flagValue  *bool
		want       bool
	}{
		{
			name: "enabled by default",
			want: true,
		},
		{
			name:       "enabled by configuration",
			configured: "enabled",
			want:       true,
		},
		{
			name:       "explicit flag enables disabled configuration",
			configured: "disabled",
			flagValue:  new(true),
			want:       true,
		},
		{
			name:       "explicit flag disables enabled configuration",
			configured: "enabled",
			flagValue:  new(false),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.NewMockConfig()
			if tt.configured != "" {
				cfg.Set("", "clipboard", tt.configured)
			}

			require.Equal(t, tt.want, ShouldCopyToClipboard(cfg, tt.flagValue))
		})
	}
}

func TestShouldUseExpiring(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		flagValue  *bool
		want       bool
	}{
		{
			name: "disabled by default",
			want: false,
		},
		{
			name:       "enabled by configuration",
			configured: "enabled",
			want:       true,
		},
		{
			name:       "explicit flag enables disabled configuration",
			configured: "disabled",
			flagValue:  new(true),
			want:       true,
		},
		{
			name:       "explicit flag disables enabled configuration",
			configured: "enabled",
			flagValue:  new(false),
			want:       false,
		},
		{
			name:       "explicit flag and configuration both prefer expiring",
			configured: "enabled",
			flagValue:  new(true),
			want:       true,
		},
		{
			name:       "explicit flag and configuration both disable expiring",
			configured: "disabled",
			flagValue:  new(false),
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.NewMockConfig()
			if tt.configured != "" {
				cfg.Set("github.com", "expiring_credentials", tt.configured)
			}

			require.Equal(t, tt.want, ShouldUseExpiring(cfg, tt.flagValue, "github.com"))
		})
	}
}

func TestLogin(t *testing.T) {
	tests := []struct {
		name         string
		opts         LoginOptions
		httpStubs    func(*testing.T, *httpmock.Registry)
		runStubs     func(*testing.T, *run.CommandStubber, *LoginOptions)
		wantsConfig  map[string]string
		wantsErr     string
		stdout       string
		stderr       string
		stderrAssert func(*testing.T, *LoginOptions, string)
	}{
		{
			name: "tty, prompt (protocol: ssh, create key: yes)",
			opts: LoginOptions{
				Prompter: &prompter.PrompterMock{
					SelectFunc: func(prompt, _ string, opts []string) (int, error) {
						switch prompt {
						case "What is your preferred protocol for Git operations on this host?":
							return prompter.IndexFor(opts, "SSH")
						case "How would you like to authenticate GitHub CLI?":
							return prompter.IndexFor(opts, "Paste an authentication token")
						}
						return -1, prompter.NoSuchPromptErr(prompt)
					},
					PasswordFunc: func(_ string) (string, error) {
						return "monkey", nil
					},
					ConfirmFunc: func(prompt string, _ bool) (bool, error) {
						return true, nil
					},
					AuthTokenFunc: func() (string, error) {
						return "ATOKEN", nil
					},
					InputFunc: func(_, _ string) (string, error) {
						return "Test Key", nil
					},
				},

				Hostname:    "example.com",
				Interactive: true,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "api/v3/"),
					httpmock.ScopesResponder("repo,read:org"))
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{ "login": "monalisa" }}}`))
				reg.Register(
					httpmock.REST("GET", "api/v3/user/keys"),
					httpmock.StringResponse(`[]`))
				reg.Register(
					httpmock.REST("POST", "api/v3/user/keys"),
					httpmock.StringResponse(`{}`))
			},
			runStubs: func(t *testing.T, cs *run.CommandStubber, opts *LoginOptions) {
				dir := t.TempDir()
				keyFile := filepath.Join(dir, "id_ed25519")
				cs.Register(`ssh-keygen`, 0, "", func(args []string) {
					expected := []string{
						"ssh-keygen", "-t", "ed25519",
						"-C", "",
						"-N", "monkey",
						"-f", keyFile,
					}
					assert.Equal(t, expected, args)
					// simulate that the public key file has been generated
					_ = os.WriteFile(keyFile+".pub", []byte("PUBKEY asdf"), 0600)
				})
				opts.sshContext = ssh.NewContextForTests(dir, "ssh-keygen")
			},
			wantsConfig: map[string]string{
				"example.com:user":         "monalisa",
				"example.com:oauth_token":  "ATOKEN",
				"example.com:git_protocol": "ssh",
			},
			stderrAssert: func(t *testing.T, opts *LoginOptions, stderr string) {
				sshDir, err := opts.sshContext.SshDir()
				if err != nil {
					t.Errorf("Could not load ssh config dir: %v", err)
				}

				assert.Equal(t, heredoc.Docf(`
				Tip: you can generate a Personal Access Token here https://example.com/settings/tokens
				The minimum required scopes are 'repo', 'read:org', 'admin:public_key'.
				- gh config set -h example.com git_protocol ssh
				✓ Configured git protocol
				✓ Uploaded the SSH key to your GitHub account: %s
				✓ Logged in as monalisa
			`, filepath.Join(sshDir, "id_ed25519.pub")), stderr)
			},
		},
		{
			name: "tty, --git-protocol ssh, prompt (create key: yes)",
			opts: LoginOptions{
				Prompter: &prompter.PrompterMock{
					SelectFunc: func(prompt, _ string, opts []string) (int, error) {
						switch prompt {
						case "How would you like to authenticate GitHub CLI?":
							return prompter.IndexFor(opts, "Paste an authentication token")
						}
						return -1, prompter.NoSuchPromptErr(prompt)
					},
					PasswordFunc: func(_ string) (string, error) {
						return "monkey", nil
					},
					ConfirmFunc: func(prompt string, _ bool) (bool, error) {
						return true, nil
					},
					AuthTokenFunc: func() (string, error) {
						return "ATOKEN", nil
					},
					InputFunc: func(_, _ string) (string, error) {
						return "Test Key", nil
					},
				},

				Hostname:    "example.com",
				Interactive: true,
				GitProtocol: "SSH",
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "api/v3/"),
					httpmock.ScopesResponder("repo,read:org"))
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{ "login": "monalisa" }}}`))
				reg.Register(
					httpmock.REST("GET", "api/v3/user/keys"),
					httpmock.StringResponse(`[]`))
				reg.Register(
					httpmock.REST("POST", "api/v3/user/keys"),
					httpmock.StringResponse(`{}`))
			},
			runStubs: func(t *testing.T, cs *run.CommandStubber, opts *LoginOptions) {
				dir := t.TempDir()
				keyFile := filepath.Join(dir, "id_ed25519")
				cs.Register(`ssh-keygen`, 0, "", func(args []string) {
					expected := []string{
						"ssh-keygen", "-t", "ed25519",
						"-C", "",
						"-N", "monkey",
						"-f", keyFile,
					}
					assert.Equal(t, expected, args)
					// simulate that the public key file has been generated
					_ = os.WriteFile(keyFile+".pub", []byte("PUBKEY asdf"), 0600)
				})
				opts.sshContext = ssh.NewContextForTests(dir, "ssh-keygen")
			},
			wantsConfig: map[string]string{
				"example.com:user":         "monalisa",
				"example.com:oauth_token":  "ATOKEN",
				"example.com:git_protocol": "ssh",
			},
			stderrAssert: func(t *testing.T, opts *LoginOptions, stderr string) {
				sshDir, err := opts.sshContext.SshDir()
				if err != nil {
					t.Errorf("Could not load ssh config dir: %v", err)
				}

				assert.Equal(t, heredoc.Docf(`
				Tip: you can generate a Personal Access Token here https://example.com/settings/tokens
				The minimum required scopes are 'repo', 'read:org', 'admin:public_key'.
				- gh config set -h example.com git_protocol ssh
				✓ Configured git protocol
				✓ Uploaded the SSH key to your GitHub account: %s
				✓ Logged in as monalisa
			`, filepath.Join(sshDir, "id_ed25519.pub")), stderr)
			},
		},
		{
			name: "tty, --git-protocol ssh, --skip-ssh-key",
			opts: LoginOptions{
				Prompter: &prompter.PrompterMock{
					SelectFunc: func(prompt, _ string, opts []string) (int, error) {
						if prompt == "How would you like to authenticate GitHub CLI?" {
							return prompter.IndexFor(opts, "Paste an authentication token")
						}
						return -1, prompter.NoSuchPromptErr(prompt)
					},
					AuthTokenFunc: func() (string, error) {
						return "ATOKEN", nil
					},
				},

				Hostname:         "example.com",
				Interactive:      true,
				GitProtocol:      "SSH",
				SkipSSHKeyPrompt: true,
			},
			httpStubs: func(t *testing.T, reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "api/v3/"),
					httpmock.ScopesResponder("repo,read:org"))
				reg.Register(
					httpmock.GraphQL(`query UserCurrent\b`),
					httpmock.StringResponse(`{"data":{"viewer":{ "login": "monalisa" }}}`))
			},
			wantsConfig: map[string]string{
				"example.com:user":         "monalisa",
				"example.com:oauth_token":  "ATOKEN",
				"example.com:git_protocol": "ssh",
			},
			stderr: heredoc.Doc(`
				Tip: you can generate a Personal Access Token here https://example.com/settings/tokens
				The minimum required scopes are 'repo', 'read:org'.
				- gh config set -h example.com git_protocol ssh
				✓ Configured git protocol
				✓ Logged in as monalisa
			`),
		},
		{
			name: "web flow, host issues a refreshable credential",
			opts: LoginOptions{
				Hostname: "example.com",
				Web:      true,
				Expiring: true,
				AuthFlow: func(_ *http.Client, _ string, _ *iostreams.IOStreams, _ string, _ []string, _ bool, _ browser.Browser, _ bool, requestRefreshToken bool) (*authflow.AuthResult, error) {
					if !requestRefreshToken {
						t.Error("expected requestRefreshToken to be true")
					}
					return &authflow.AuthResult{
						Token:       "gho_access",
						Username:    "monalisa",
						Refreshable: &gh.Credential{Token: "gho_access", RefreshToken: "ghr_refresh"},
					}, nil
				},
			},
			wantsConfig: map[string]string{
				"example.com:user":          "monalisa",
				"example.com:oauth_token":   "gho_access",
				"example.com:refresh_token": "ghr_refresh",
				"example.com:git_protocol":  "",
			},
			stderrAssert: func(t *testing.T, opts *LoginOptions, stderr string) {
				assert.Contains(t, stderr, "Authentication complete.")
				assert.Contains(t, stderr, "Received expiring refreshable token\n")
			},
		},
		{
			name: "web flow, host issues a refreshable credential without being asked",
			opts: LoginOptions{
				Hostname: "example.com",
				Web:      true,
				AuthFlow: func(_ *http.Client, _ string, _ *iostreams.IOStreams, _ string, _ []string, _ bool, _ browser.Browser, _ bool, requestRefreshToken bool) (*authflow.AuthResult, error) {
					if requestRefreshToken {
						t.Error("expected requestRefreshToken to be false")
					}
					return &authflow.AuthResult{
						Token:       "gho_access",
						Username:    "monalisa",
						Refreshable: &gh.Credential{Token: "gho_access", RefreshToken: "ghr_refresh"},
					}, nil
				},
			},
			wantsConfig: map[string]string{
				"example.com:user":          "monalisa",
				"example.com:oauth_token":   "gho_access",
				"example.com:refresh_token": "ghr_refresh",
				"example.com:git_protocol":  "",
			},
			stderrAssert: func(t *testing.T, opts *LoginOptions, stderr string) {
				assert.Contains(t, stderr, "Host issues expiring refreshable token\n")
			},
		},
		{
			name: "web flow, expiring requested but host issues a non-expiring token",
			opts: LoginOptions{
				Hostname: "example.com",
				Web:      true,
				Expiring: true,
				AuthFlow: func(_ *http.Client, _ string, _ *iostreams.IOStreams, _ string, _ []string, _ bool, _ browser.Browser, _ bool, _ bool) (*authflow.AuthResult, error) {
					return &authflow.AuthResult{Token: "gho_plain", Username: "monalisa"}, nil
				},
			},
			wantsConfig: map[string]string{
				"example.com:user":         "monalisa",
				"example.com:oauth_token":  "gho_plain",
				"example.com:git_protocol": "",
			},
			stderrAssert: func(t *testing.T, opts *LoginOptions, stderr string) {
				assert.Contains(t, stderr, "Host did not issue an expiring refreshable token\n")
			},
		},
		{
			name: "web flow, expiring on an enterprise host warns the token may not be expiring",
			opts: LoginOptions{
				Hostname: "example.com",
				Web:      true,
				Expiring: true,
				AuthFlow: func(_ *http.Client, _ string, _ *iostreams.IOStreams, _ string, _ []string, _ bool, _ browser.Browser, _ bool, _ bool) (*authflow.AuthResult, error) {
					return &authflow.AuthResult{Token: "gho_plain", Username: "monalisa"}, nil
				},
			},
			wantsConfig: map[string]string{
				"example.com:user":         "monalisa",
				"example.com:oauth_token":  "gho_plain",
				"example.com:git_protocol": "",
			},
			stderrAssert: func(t *testing.T, opts *LoginOptions, stderr string) {
				assert.Contains(t, stderr, "example.com may not support expiring tokens")
			},
		},
		{
			name: "interactive, explicit --expiring forces the web flow instead of offering token paste",
			opts: LoginOptions{
				Hostname:                    "example.com",
				Interactive:                 true,
				Expiring:                    true,
				ExpiringExplicitlyRequested: true,
				GitProtocol:                 "ssh",
				SkipSSHKeyPrompt:            true,
				Prompter: &prompter.PrompterMock{
					SelectFunc: func(prompt, _ string, _ []string) (int, error) {
						// --expiring must not reach the authentication-method choice, since a pasted token can
						// never be refreshable. Any such prompt here is a regression.
						t.Errorf("unexpected Select prompt under --expiring: %q", prompt)
						return -1, prompter.NoSuchPromptErr(prompt)
					},
				},
				AuthFlow: func(_ *http.Client, _ string, _ *iostreams.IOStreams, _ string, _ []string, _ bool, _ browser.Browser, _ bool, requestRefreshToken bool) (*authflow.AuthResult, error) {
					if !requestRefreshToken {
						t.Error("expected requestRefreshToken to be true")
					}
					return &authflow.AuthResult{
						Token:       "gho_access",
						Username:    "monalisa",
						Refreshable: &gh.Credential{Token: "gho_access", RefreshToken: "ghr_refresh"},
					}, nil
				},
			},
			wantsConfig: map[string]string{
				"example.com:user":          "monalisa",
				"example.com:oauth_token":   "gho_access",
				"example.com:refresh_token": "ghr_refresh",
				"example.com:git_protocol":  "ssh",
			},
			stderrAssert: func(t *testing.T, opts *LoginOptions, stderr string) {
				assert.Contains(t, stderr, "Received expiring refreshable token\n")
			},
		},
		{
			name: "interactive, config-derived expiring still offers token paste",
			opts: LoginOptions{
				Hostname:    "example.com",
				Interactive: true,
				// Expiring is true from the expiring_credentials config, but the flag was not passed, so the
				// authentication-method choice must still be offered and include the token-paste option.
				Expiring:         true,
				GitProtocol:      "ssh",
				SkipSSHKeyPrompt: true,
				Prompter: &prompter.PrompterMock{
					SelectFunc: func(prompt, _ string, options []string) (int, error) {
						if prompt == "How would you like to authenticate GitHub CLI?" {
							assert.Contains(t, options, "Paste an authentication token")
							return 0, nil
						}
						t.Errorf("unexpected Select prompt: %q", prompt)
						return -1, prompter.NoSuchPromptErr(prompt)
					},
				},
				AuthFlow: func(_ *http.Client, _ string, _ *iostreams.IOStreams, _ string, _ []string, _ bool, _ browser.Browser, _ bool, requestRefreshToken bool) (*authflow.AuthResult, error) {
					if !requestRefreshToken {
						t.Error("expected requestRefreshToken to be true")
					}
					return &authflow.AuthResult{
						Token:       "gho_access",
						Username:    "monalisa",
						Refreshable: &gh.Credential{Token: "gho_access", RefreshToken: "ghr_refresh"},
					}, nil
				},
			},
			wantsConfig: map[string]string{
				"example.com:user":          "monalisa",
				"example.com:oauth_token":   "gho_access",
				"example.com:refresh_token": "ghr_refresh",
				"example.com:git_protocol":  "ssh",
			},
			stderrAssert: func(t *testing.T, opts *LoginOptions, stderr string) {
				assert.Contains(t, stderr, "Received expiring refreshable token\n")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(t, reg)
			}

			cfg := tinyConfig{}
			ios, _, stdout, stderr := iostreams.Test()

			tt.opts.IO = ios
			tt.opts.Config = &cfg
			tt.opts.HTTPClient = &http.Client{Transport: reg}
			tt.opts.CredentialFlow = &GitCredentialFlow{
				IO: ios,
				// Helper detection now runs for every sign-in, including the non-interactive and SSH cases in this
				// table, so provide a fake helper config to keep the read hermetic instead of shelling out to git.
				HelperConfig: &gitcredentials.FakeHelperConfig{
					SelfExecutablePath: "/path/to/gh",
					Helpers:            map[string]gitcredentials.Helper{},
				},
			}

			if tt.runStubs != nil {
				rs, runRestore := run.Stub()
				defer runRestore(t)
				tt.runStubs(t, rs, &tt.opts)
			}

			err := Login(&tt.opts)

			if tt.wantsErr != "" {
				assert.EqualError(t, err, tt.wantsErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantsConfig, map[string]string(cfg))
			}

			assert.Equal(t, tt.stdout, stdout.String())

			if tt.stderrAssert != nil {
				tt.stderrAssert(t, &tt.opts, stderr.String())
			} else {
				assert.Equal(t, tt.stderr, stderr.String())
			}
		})
	}
}

func TestAuthenticatingGitCredentials(t *testing.T) {
	// Given we have no host or global credential helpers configured
	// And given they have chosen https as their git protocol
	// When they choose to authenticate git with their GitHub credentials
	// Then gh is configured as their credential helper for that host
	ios, _, _, _ := iostreams.Test()

	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("GET", "api/v3/"),
		httpmock.ScopesResponder("repo,read:org"))
	reg.Register(
		httpmock.GraphQL(`query UserCurrent\b`),
		httpmock.StringResponse(`{"data":{"viewer":{ "login": "monalisa" }}}`))

	opts := &LoginOptions{
		IO:          ios,
		Config:      tinyConfig{},
		HTTPClient:  &http.Client{Transport: reg},
		Hostname:    "example.com",
		Interactive: true,
		GitProtocol: "https",
		Prompter: &prompter.PrompterMock{
			SelectFunc: func(prompt, _ string, opts []string) (int, error) {
				if prompt == "How would you like to authenticate GitHub CLI?" {
					return prompter.IndexFor(opts, "Paste an authentication token")
				}
				return -1, prompter.NoSuchPromptErr(prompt)
			},
			AuthTokenFunc: func() (string, error) {
				return "ATOKEN", nil
			},
		},
		CredentialFlow: &GitCredentialFlow{
			Prompter: &prompter.PrompterMock{
				ConfirmFunc: func(prompt string, _ bool) (bool, error) {
					return true, nil
				},
			},
			HelperConfig: &gitcredentials.FakeHelperConfig{
				SelfExecutablePath: "/path/to/gh",
				Helpers:            map[string]gitcredentials.Helper{},
			},
			// Updater not required for this test as we will be setting gh as the helper
		},
	}

	require.NoError(t, Login(opts))

	helper, err := opts.CredentialFlow.HelperConfig.ConfiguredHelper("example.com")
	require.NoError(t, err)
	require.True(t, helper.IsOurs(), "expected gh to be the configured helper")
}

func TestLogin_pastedTokenIgnoresConfiguredExpiringPreference(t *testing.T) {
	// A config-derived expiring preference must not leak into a login where the user pastes a token, which can never
	// be expiring: no expiring credential-helper prompt and no expiring request in telemetry.
	ios, _, _, stderr := iostreams.Test()
	recorder := &telemetry.InvocationRecorderSpy{}

	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.REST("GET", ""),
		httpmock.ScopesResponder("repo,read:org"))
	reg.Register(
		httpmock.GraphQL(`query UserCurrent\b`),
		httpmock.StringResponse(`{"data":{"viewer":{ "login": "monalisa" }}}`))

	opts := &LoginOptions{
		IO:          ios,
		Config:      tinyConfig{},
		HTTPClient:  &http.Client{Transport: reg},
		Hostname:    "github.com",
		Interactive: true,
		GitProtocol: "https",
		Expiring:    true,
		Telemetry:   recorder,
		Prompter: &prompter.PrompterMock{
			SelectFunc: func(prompt, _ string, opts []string) (int, error) {
				if prompt == "How would you like to authenticate GitHub CLI?" {
					return prompter.IndexFor(opts, "Paste an authentication token")
				}
				return -1, prompter.NoSuchPromptErr(prompt)
			},
			AuthTokenFunc: func() (string, error) {
				return "ATOKEN", nil
			},
		},
		CredentialFlow: &GitCredentialFlow{
			// The non-expiring helper prompt is a Confirm. The expiring one is a Select, which is left unset so the
			// mock panics if it is reached.
			Prompter: &prompter.PrompterMock{
				ConfirmFunc: func(prompt string, _ bool) (bool, error) {
					return true, nil
				},
			},
			HelperConfig: &gitcredentials.FakeHelperConfig{
				SelfExecutablePath: "/path/to/gh",
				Helpers:            map[string]gitcredentials.Helper{},
			},
		},
	}

	require.NoError(t, Login(opts))

	assert.NotContains(t, stderr.String(), "expiring")
	events := recorder.Events()
	require.Len(t, events, 1)
	assert.Equal(t, "false", events[0].Dimensions["expiring_requested"])
}

func TestLogin_recordsAuthTelemetry(t *testing.T) {
	// An interactive https login to github.com that configures gh as the credential helper should record a complete
	// auth_invocation event.
	ios, _, _, _ := iostreams.Test()
	recorder := &telemetry.InvocationRecorderSpy{}

	opts := &LoginOptions{
		IO:          ios,
		Config:      tinyConfig{},
		HTTPClient:  &http.Client{},
		Hostname:    "github.com",
		Interactive: true,
		GitProtocol: "https",
		Telemetry:   recorder,
		Prompter: &prompter.PrompterMock{
			SelectFunc: func(prompt, _ string, opts []string) (int, error) {
				if prompt == "How would you like to authenticate GitHub CLI?" {
					return prompter.IndexFor(opts, "Login with a web browser")
				}
				return -1, prompter.NoSuchPromptErr(prompt)
			},
		},
		AuthFlow: func(_ *http.Client, _ string, _ *iostreams.IOStreams, _ string, _ []string, _ bool, _ browser.Browser, _, _ bool) (*authflow.AuthResult, error) {
			return &authflow.AuthResult{Token: "ATOKEN", Username: "monalisa"}, nil
		},
		CredentialFlow: &GitCredentialFlow{
			Prompter: &prompter.PrompterMock{
				ConfirmFunc: func(prompt string, _ bool) (bool, error) {
					return true, nil
				},
			},
			HelperConfig: &gitcredentials.FakeHelperConfig{
				SelfExecutablePath: "/path/to/gh",
				Helpers:            map[string]gitcredentials.Helper{},
			},
		},
	}

	require.NoError(t, Login(opts))

	events := recorder.Events()
	require.Len(t, events, 1)
	assert.Equal(t, ghtelemetry.SAMPLE_ALL, recorder.LastSampleRate)
	dims := events[0].Dimensions
	assert.Equal(t, "auth_invocation", events[0].Type)
	assert.Equal(t, "login", dims["auth_command"])
	assert.Equal(t, "false", dims["expiring_requested"])
	assert.Equal(t, "false", dims["expiring_setup_cred_helper_cancelled"])
	assert.Equal(t, "https", dims["git_protocol"])
	assert.Equal(t, "none", dims["git_credential_helper_before"])
	assert.Equal(t, "ours", dims["git_credential_helper_after"])
	assert.Equal(t, "false", dims["expiring_received"])
	assert.Equal(t, "keyring", dims["storage_type"])
	assert.Equal(t, "true", dims["auth_success"])
}

func TestLogin_recordsAuthTelemetryOnFailure(t *testing.T) {
	// Telemetry must reflect how far the login got: an issued expiring token is recorded even when storing it fails,
	// and a stored credential counts as a successful sign-in even when a later side effect fails.
	tests := []struct {
		name        string
		config      iconfig
		gitProtocol string
		helperErr   error
		setup       func(t *testing.T, opts *LoginOptions, reg *httpmock.Registry)
		wantSuccess string
	}{
		{
			name:        "storage fails after an expiring token is issued",
			config:      failingStorageConfig{tinyConfig{}},
			gitProtocol: "https",
			wantSuccess: "false",
		},
		{
			name:        "credential helper setup fails after storage",
			config:      tinyConfig{},
			gitProtocol: "https",
			helperErr:   errors.New("helper setup failed"),
			wantSuccess: "true",
		},
		{
			name:        "SSH key upload fails after storage",
			config:      tinyConfig{},
			gitProtocol: "ssh",
			setup: func(t *testing.T, opts *LoginOptions, reg *httpmock.Registry) {
				dir := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(dir, "id_ed25519.pub"), []byte("PUBKEY asdf"), 0600))
				opts.sshContext = ssh.NewContextForTests(dir, "")
				reg.Register(httpmock.REST("GET", "user/keys"), httpmock.StatusStringResponse(500, "{}"))
			},
			wantSuccess: "true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			recorder := &telemetry.InvocationRecorderSpy{}
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			opts := &LoginOptions{
				IO:          ios,
				Config:      tt.config,
				HTTPClient:  &http.Client{Transport: reg},
				Hostname:    "github.com",
				Interactive: true,
				GitProtocol: tt.gitProtocol,
				Telemetry:   recorder,
				Prompter: &prompter.PrompterMock{
					SelectFunc: func(prompt, _ string, opts []string) (int, error) {
						switch prompt {
						case "How would you like to authenticate GitHub CLI?":
							return prompter.IndexFor(opts, "Login with a web browser")
						case "Upload your SSH public key to your GitHub account?":
							return 0, nil
						}
						return -1, prompter.NoSuchPromptErr(prompt)
					},
					InputFunc: func(_, defaultValue string) (string, error) {
						return defaultValue, nil
					},
				},
				AuthFlow: func(_ *http.Client, _ string, _ *iostreams.IOStreams, _ string, _ []string, _ bool, _ browser.Browser, _, _ bool) (*authflow.AuthResult, error) {
					return &authflow.AuthResult{
						Token:       "ATOKEN",
						Username:    "monalisa",
						Refreshable: &gh.Credential{Token: "ATOKEN", RefreshToken: "RTOKEN"},
					}, nil
				},
				CredentialFlow: &GitCredentialFlow{
					Prompter: &prompter.PrompterMock{
						ConfirmFunc: func(_ string, _ bool) (bool, error) {
							return true, nil
						},
					},
					HelperConfig: &gitcredentials.FakeHelperConfig{
						SelfExecutablePath: "/path/to/gh",
						Helpers:            map[string]gitcredentials.Helper{},
						ConfigureOursErr:   tt.helperErr,
					},
				},
			}
			if tt.setup != nil {
				tt.setup(t, opts, reg)
			}

			require.Error(t, Login(opts))

			events := recorder.Events()
			require.Len(t, events, 1)
			dims := events[0].Dimensions
			assert.Equal(t, "true", dims["expiring_received"])
			assert.Equal(t, tt.wantSuccess, dims["auth_success"])
		})
	}
}

func TestLogin_recordsExpiringSetupCancelled(t *testing.T) {
	// An interactive https login that asks for an expiring token and then cancels at the credential-helper prompt
	// should record expiring_setup_cred_helper_cancelled=true and surface the warning.
	ios, _, _, stderr := iostreams.Test()
	recorder := &telemetry.InvocationRecorderSpy{}

	opts := &LoginOptions{
		IO:          ios,
		Config:      tinyConfig{},
		HTTPClient:  &http.Client{},
		Hostname:    "github.com",
		Interactive: true,
		GitProtocol: "https",
		Expiring:    true,
		Telemetry:   recorder,
		Prompter: &prompter.PrompterMock{
			SelectFunc: func(prompt, _ string, options []string) (int, error) {
				require.Equal(t, "How would you like to authenticate GitHub CLI?", prompt)
				return prompter.IndexFor(options, "Login with a web browser")
			},
		},
		CredentialFlow: &GitCredentialFlow{
			IO: ios,
			Prompter: &prompter.PrompterMock{
				SelectFunc: func(_, _ string, options []string) (int, error) {
					return prompter.IndexFor(options, "Cancel")
				},
			},
			HelperConfig: &gitcredentials.FakeHelperConfig{
				SelfExecutablePath: "/path/to/gh",
				Helpers: map[string]gitcredentials.Helper{
					"github.com": {Cmd: "osxkeychain"},
				},
			},
		},
	}

	err := Login(opts)
	assert.ErrorIs(t, err, cmdutil.CancelError)

	events := recorder.Events()
	require.Len(t, events, 1)
	dims := events[0].Dimensions
	assert.Equal(t, "true", dims["expiring_requested"])
	assert.Equal(t, "true", dims["expiring_setup_cred_helper_cancelled"])
	assert.Equal(t, "false", dims["auth_success"])
	assert.Contains(t, stderr.String(), "expiring token")
}

func TestLogin_doesNotRecordAuthTelemetryForEnterprise(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	recorder := &telemetry.InvocationRecorderSpy{}

	opts := &LoginOptions{
		IO:          ios,
		Config:      tinyConfig{},
		HTTPClient:  &http.Client{},
		Hostname:    "ghe.example.com",
		Interactive: true,
		GitProtocol: "https",
		Web:         true,
		Telemetry:   recorder,
		AuthFlow: func(_ *http.Client, _ string, _ *iostreams.IOStreams, _ string, _ []string, _ bool, _ browser.Browser, _, _ bool) (*authflow.AuthResult, error) {
			return &authflow.AuthResult{Token: "ATOKEN", Username: "monalisa"}, nil
		},
		CredentialFlow: &GitCredentialFlow{
			Prompter: &prompter.PrompterMock{
				ConfirmFunc: func(prompt string, _ bool) (bool, error) {
					return false, nil
				},
			},
			HelperConfig: &gitcredentials.FakeHelperConfig{
				SelfExecutablePath: "/path/to/gh",
				Helpers:            map[string]gitcredentials.Helper{},
			},
		},
	}

	require.NoError(t, Login(opts))

	assert.Empty(t, recorder.Events())
}

func Test_scopesSentence(t *testing.T) {
	type args struct {
		scopes []string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "basic scopes",
			args: args{
				scopes: []string{"repo", "read:org"},
			},
			want: "'repo', 'read:org'",
		},
		{
			name: "empty",
			args: args{
				scopes: []string(nil),
			},
			want: "",
		},
		{
			name: "workflow scope",
			args: args{
				scopes: []string{"repo", "workflow"},
			},
			want: "'repo', 'workflow'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scopesSentence(tt.args.scopes); got != tt.want {
				t.Errorf("scopesSentence() = %q, want %q", got, tt.want)
			}
		})
	}
}
