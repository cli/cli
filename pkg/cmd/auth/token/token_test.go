package token

import (
	"bytes"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/require"
)

func TestNewCmdToken(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		output     TokenOptions
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:   "no flags",
			input:  "",
			output: TokenOptions{},
		},
		{
			name:   "with hostname",
			input:  "--hostname github.mycompany.com",
			output: TokenOptions{Hostname: "github.mycompany.com"},
		},
		{
			name:   "with user",
			input:  "--user test-user",
			output: TokenOptions{Username: "test-user"},
		},
		{
			name:   "with shorthand user",
			input:  "-u test-user",
			output: TokenOptions{Username: "test-user"},
		},
		{
			name:   "with shorthand hostname",
			input:  "-h github.mycompany.com",
			output: TokenOptions{Hostname: "github.mycompany.com"},
		},
		{
			name:   "with secure-storage",
			input:  "--secure-storage",
			output: TokenOptions{SecureStorage: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
				Config: func() (gh.Config, error) {
					cfg := config.NewBlankConfig()
					return cfg, nil
				},
			}
			argv, err := shlex.Split(tt.input)
			require.NoError(t, err)

			var cmdOpts *TokenOptions
			cmd := NewCmdToken(f, func(opts *TokenOptions) error {
				cmdOpts = opts
				return nil
			})
			// TODO cobra hack-around
			cmd.Flags().BoolP("help", "x", false, "")

			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantErr {
				require.Error(t, err)
				require.EqualError(t, err, tt.wantErrMsg)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.output.Hostname, cmdOpts.Hostname)
			require.Equal(t, tt.output.SecureStorage, cmdOpts.SecureStorage)
		})
	}
}

func TestTokenRun(t *testing.T) {
	tests := []struct {
		name       string
		opts       TokenOptions
		env        map[string]string
		cfgStubs   func(*testing.T, gh.Config)
		wantStdout string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "token",
			opts: TokenOptions{},
			cfgStubs: func(t *testing.T, cfg gh.Config) {
				login(t, cfg, "github.com", "test-user", "gho_ABCDEFG", "https", false)
			},
			wantStdout: "gho_ABCDEFG\n",
		},
		{
			name: "token by hostname",
			opts: TokenOptions{
				Hostname: "github.mycompany.com",
			},
			cfgStubs: func(t *testing.T, cfg gh.Config) {
				login(t, cfg, "github.com", "test-user", "gho_ABCDEFG", "https", false)
				login(t, cfg, "github.mycompany.com", "test-user", "gho_1234567", "https", false)
			},
			wantStdout: "gho_1234567\n",
		},
		{
			name:       "no token",
			opts:       TokenOptions{},
			wantErr:    true,
			wantErrMsg: "no oauth token found for github.com",
		},
		{
			name: "no token for hostname user",
			opts: TokenOptions{
				Hostname: "ghe.io",
				Username: "test-user",
			},
			wantErr:    true,
			wantErrMsg: "no oauth token found for ghe.io account test-user",
		},
		{
			name: "uses default host when one is not provided",
			opts: TokenOptions{},
			cfgStubs: func(t *testing.T, cfg gh.Config) {
				login(t, cfg, "github.com", "test-user", "gho_ABCDEFG", "https", false)
				login(t, cfg, "github.mycompany.com", "test-user", "gho_1234567", "https", false)
			},
			env:        map[string]string{"GH_HOST": "github.mycompany.com"},
			wantStdout: "gho_1234567\n",
		},
		{
			name: "token for user",
			opts: TokenOptions{
				Hostname: "github.com",
				Username: "test-user",
			},
			cfgStubs: func(t *testing.T, cfg gh.Config) {
				login(t, cfg, "github.com", "test-user", "gho_ABCDEFG", "https", false)
				login(t, cfg, "github.com", "test-user-2", "gho_1234567", "https", false)
			},
			wantStdout: "gho_ABCDEFG\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			tt.opts.IO = ios

			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			cfg, _ := config.NewIsolatedTestConfig(t)
			if tt.cfgStubs != nil {
				tt.cfgStubs(t, cfg)
			}

			tt.opts.Config = func() (gh.Config, error) {
				return cfg, nil
			}

			err := tokenRun(&tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				require.EqualError(t, err, tt.wantErrMsg)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantStdout, stdout.String())
		})
	}
}

func TestTokenRunSecureStorage(t *testing.T) {
	tests := []struct {
		name       string
		opts       TokenOptions
		cfgStubs   func(*testing.T, gh.Config)
		wantStdout string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "token",
			opts: TokenOptions{},
			cfgStubs: func(t *testing.T, cfg gh.Config) {
				login(t, cfg, "github.com", "test-user", "gho_ABCDEFG", "https", true)
			},
			wantStdout: "gho_ABCDEFG\n",
		},
		{
			name: "token by hostname",
			opts: TokenOptions{
				Hostname: "mycompany.com",
			},
			cfgStubs: func(t *testing.T, cfg gh.Config) {
				login(t, cfg, "mycompany.com", "test-user", "gho_1234567", "https", true)
			},
			wantStdout: "gho_1234567\n",
		},
		{
			name:       "no token",
			opts:       TokenOptions{},
			wantErr:    true,
			wantErrMsg: "no oauth token found for github.com",
		},
		{
			name: "no token for hostname user",
			opts: TokenOptions{
				Hostname: "ghe.io",
				Username: "test-user",
			},
			wantErr:    true,
			wantErrMsg: "no oauth token found for ghe.io account test-user",
		},
		{
			name: "token for user",
			opts: TokenOptions{
				Hostname: "github.com",
				Username: "test-user",
			},
			cfgStubs: func(t *testing.T, cfg gh.Config) {
				login(t, cfg, "github.com", "test-user", "gho_ABCDEFG", "https", true)
				login(t, cfg, "github.com", "test-user-2", "gho_1234567", "https", true)
			},
			wantStdout: "gho_ABCDEFG\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			tt.opts.IO = ios
			tt.opts.SecureStorage = true

			cfg, _ := config.NewIsolatedTestConfig(t)
			if tt.cfgStubs != nil {
				tt.cfgStubs(t, cfg)
			}

			tt.opts.Config = func() (gh.Config, error) {
				return cfg, nil
			}

			err := tokenRun(&tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				require.EqualError(t, err, tt.wantErrMsg)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantStdout, stdout.String())
		})
	}
}

func login(t *testing.T, c gh.Config, hostname, username, token, gitProtocol string, secureStorage bool) {
	t.Helper()
	_, err := c.Authentication().Login(hostname, username, token, gitProtocol, secureStorage)
	require.NoError(t, err)
}
