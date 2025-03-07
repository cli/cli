package shared

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/cmd/auth/shared/gitcredentials"
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

func (c tinyConfig) UsersForHost(hostname string) []string {
	return nil
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
				// Intentionally not instantiating anything in here because the tests do not hit this code path.
				// Right now it's better to panic if we write a test that hits the code than say, start calling
				// out to git unintentionally.
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
