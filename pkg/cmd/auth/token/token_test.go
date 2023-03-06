package token

import (
	"bytes"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/zalando/go-keyring"
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
				Config: func() (config.Config, error) {
					cfg := config.NewBlankConfig()
					return cfg, nil
				},
			}
			argv, err := shlex.Split(tt.input)
			assert.NoError(t, err)

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
				assert.Error(t, err)
				assert.EqualError(t, err, tt.wantErrMsg)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.output.Hostname, cmdOpts.Hostname)
			assert.Equal(t, tt.output.SecureStorage, cmdOpts.SecureStorage)
		})
	}
}

func TestTokenRun(t *testing.T) {
	tests := []struct {
		name       string
		opts       TokenOptions
		wantStdout string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "token",
			opts: TokenOptions{
				Config: func() (config.Config, error) {
					cfg := config.NewBlankConfig()
					cfg.Set("github.com", "oauth_token", "gho_ABCDEFG")
					return cfg, nil
				},
			},
			wantStdout: "gho_ABCDEFG\n",
		},
		{
			name: "token by hostname",
			opts: TokenOptions{
				Config: func() (config.Config, error) {
					cfg := config.NewBlankConfig()
					cfg.Set("github.com", "oauth_token", "gho_ABCDEFG")
					cfg.Set("github.mycompany.com", "oauth_token", "gho_1234567")
					return cfg, nil
				},
				Hostname: "github.mycompany.com",
			},
			wantStdout: "gho_1234567\n",
		},
		{
			name: "no token",
			opts: TokenOptions{
				Config: func() (config.Config, error) {
					cfg := config.NewBlankConfig()
					return cfg, nil
				},
			},
			wantErr:    true,
			wantErrMsg: "no oauth token",
		},
		{
			name: "uses default host when one is not provided",
			opts: TokenOptions{
				Config: func() (config.Config, error) {
					cfg := config.NewBlankConfig()
					cfg.AuthenticationFunc = func() *config.AuthConfig {
						authCfg := &config.AuthConfig{}
						authCfg.SetDefaultHost("github.mycompany.com", "GH_HOST")
						authCfg.SetToken("gho_1234567", "default")
						return authCfg
					}
					return cfg, nil
				},
			},
			wantStdout: "gho_1234567\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			tt.opts.IO = ios
			err := tokenRun(&tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
				assert.EqualError(t, err, tt.wantErrMsg)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantStdout, stdout.String())
		})
	}
}

func TestTokenRunSecureStorage(t *testing.T) {
	tests := []struct {
		name       string
		opts       TokenOptions
		wantStdout string
		wantErr    bool
		wantErrMsg string
	}{
		{
			name: "token",
			opts: TokenOptions{
				Config: func() (config.Config, error) {
					cfg := config.NewBlankConfig()
					_ = keyring.Set("gh:github.com", "", "gho_ABCDEFG")
					return cfg, nil
				},
			},
			wantStdout: "gho_ABCDEFG\n",
		},
		{
			name: "token by hostname",
			opts: TokenOptions{
				Config: func() (config.Config, error) {
					cfg := config.NewBlankConfig()
					_ = keyring.Set("gh:mycompany.com", "", "gho_1234567")
					return cfg, nil
				},
				Hostname: "mycompany.com",
			},
			wantStdout: "gho_1234567\n",
		},
		{
			name: "no token",
			opts: TokenOptions{
				Config: func() (config.Config, error) {
					cfg := config.NewBlankConfig()
					return cfg, nil
				},
			},
			wantErr:    true,
			wantErrMsg: "no oauth token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keyring.MockInit()
			ios, _, stdout, _ := iostreams.Test()
			tt.opts.IO = ios
			tt.opts.SecureStorage = true
			err := tokenRun(&tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
				assert.EqualError(t, err, tt.wantErrMsg)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.wantStdout, stdout.String())
		})
	}
}
