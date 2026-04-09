package authsetuser

import (
	"bytes"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdSetUser(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantErr  string
		wantOpts SetUserOptions
	}{
		{
			name:  "owner and username parsed",
			input: "--owner work-org work-user",
			wantOpts: SetUserOptions{
				Owner:    "work-org",
				Username: "work-user",
			},
		},
		{
			name:  "hostname flag parsed",
			input: "--owner work-org --hostname ghe.io work-user",
			wantOpts: SetUserOptions{
				Owner:    "work-org",
				Username: "work-user",
				Hostname: "ghe.io",
			},
		},
		{
			name:    "errors when owner flag is missing",
			input:   "work-user",
			wantErr: `required flag(s) "owner" not set`,
		},
		{
			name:    "errors when username arg is missing",
			input:   "--owner work-org",
			wantErr: "accepts 1 arg(s), received 0",
		},
		{
			name:    "errors when too many args",
			input:   "--owner work-org user1 user2",
			wantErr: "accepts 1 arg(s), received 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &cmdutil.Factory{}

			argv, err := shlex.Split(tt.input)
			require.NoError(t, err)

			var gotOpts *SetUserOptions
			cmd := NewCmdSetUser(f, func(opts *SetUserOptions) error {
				gotOpts = opts
				return nil
			})
			// Override help so -h can be used for hostname
			cmd.Flags().BoolP("help", "x", false, "")
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantOpts.Owner, gotOpts.Owner)
			assert.Equal(t, tt.wantOpts.Username, gotOpts.Username)
			assert.Equal(t, tt.wantOpts.Hostname, gotOpts.Hostname)
		})
	}
}

func TestSetUserRun(t *testing.T) {
	tests := []struct {
		name        string
		owner       string
		username    string
		hostname    string
		setupConfig func(*config.AuthConfig)
		wantErr     string
		wantMapping string
	}{
		{
			name:     "maps owner to authenticated user",
			owner:    "work-org",
			username: "work-user",
			setupConfig: func(authCfg *config.AuthConfig) {
				_, err := authCfg.Login("github.com", "work-user", "work-token", "", false)
				require.NoError(t, err)
			},
			wantMapping: "work-user",
		},
		{
			name:     "maps personal owner to personal user",
			owner:    "personal-user",
			username: "personal-user",
			setupConfig: func(authCfg *config.AuthConfig) {
				_, err := authCfg.Login("github.com", "personal-user", "personal-token", "", false)
				require.NoError(t, err)
			},
			wantMapping: "personal-user",
		},
		{
			name:     "errors when not logged in to host",
			owner:    "work-org",
			username: "work-user",
			setupConfig: func(authCfg *config.AuthConfig) {
				// no users logged in
			},
			wantErr: "not logged in to github.com",
		},
		{
			name:     "errors when username not an authenticated user on host",
			owner:    "work-org",
			username: "unknown-user",
			setupConfig: func(authCfg *config.AuthConfig) {
				_, err := authCfg.Login("github.com", "work-user", "work-token", "", false)
				require.NoError(t, err)
			},
			wantErr: "not logged in to github.com as unknown-user",
		},
		{
			name:     "uses explicit hostname flag",
			owner:    "work-org",
			username: "work-user",
			hostname: "ghe.io",
			setupConfig: func(authCfg *config.AuthConfig) {
				_, err := authCfg.Login("ghe.io", "work-user", "work-token", "", false)
				require.NoError(t, err)
			},
			wantMapping: "work-user",
		},
		{
			name:     "errors when hostname not known",
			owner:    "work-org",
			username: "work-user",
			hostname: "unknown.internal",
			setupConfig: func(authCfg *config.AuthConfig) {
				_, err := authCfg.Login("github.com", "work-user", "work-token", "", false)
				require.NoError(t, err)
			},
			wantErr: "not logged in to unknown.internal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, _ := config.NewIsolatedTestConfig(t)
			authCfg := cfg.Authentication().(*config.AuthConfig)
			tt.setupConfig(authCfg)

			ios, _, _, _ := iostreams.Test()

			opts := &SetUserOptions{
				IO: ios,
				Config: func() (gh.Config, error) {
					return cfg, nil
				},
				Owner:    tt.owner,
				Username: tt.username,
				Hostname: tt.hostname,
			}

			err := setUserRun(opts)

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			hostname := tt.hostname
			if hostname == "" {
				hostname = "github.com"
			}
			user, err := authCfg.UserForOwner(hostname, tt.owner)
			require.NoError(t, err)
			assert.Equal(t, tt.wantMapping, user)
		})
	}
}
