package cmdutil

import (
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func Test_CheckAuth(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		cfgStubs func(*testing.T, gh.Config)
		expected bool
	}{
		{
			name:     "no known hosts, no env auth token",
			expected: false,
		},
		{
			name:     "no known hosts, env auth token",
			env:      map[string]string{"GITHUB_TOKEN": "token"},
			expected: true,
		},
		{
			name: "known host",
			cfgStubs: func(t *testing.T, c gh.Config) {
				_, err := c.Authentication().Login("github.com", "test-user", "test-token", "https", false)
				require.NoError(t, err)
			},
			expected: true,
		},
		{
			name:     "enterprise token",
			env:      map[string]string{"GH_ENTERPRISE_TOKEN": "token"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, _ := config.NewIsolatedTestConfig(t)
			if tt.cfgStubs != nil {
				tt.cfgStubs(t, cfg)
			}

			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			require.Equal(t, tt.expected, CheckAuth(cfg))
		})
	}
}

func Test_IsAuthCheckEnabled(t *testing.T) {
	tests := []struct {
		name               string
		init               func() (*cobra.Command, error)
		isAuthCheckEnabled bool
	}{
		{
			name: "no annotations",
			init: func() (*cobra.Command, error) {
				cmd := &cobra.Command{}
				cmd.Flags().Bool("flag", false, "")
				return cmd, nil
			},
			isAuthCheckEnabled: true,
		},
		{
			name: "command-level disable",
			init: func() (*cobra.Command, error) {
				cmd := &cobra.Command{}
				DisableAuthCheck(cmd)
				return cmd, nil
			},
			isAuthCheckEnabled: false,
		},
		{
			name: "command with flag-level disable, flag not set",
			init: func() (*cobra.Command, error) {
				cmd := &cobra.Command{}
				cmd.Flags().Bool("flag", false, "")
				DisableAuthCheckFlag(cmd.Flag("flag"))
				return cmd, nil
			},
			isAuthCheckEnabled: true,
		},
		{
			name: "command with flag-level disable, flag set",
			init: func() (*cobra.Command, error) {
				cmd := &cobra.Command{}
				cmd.Flags().Bool("flag", false, "")
				if err := cmd.Flags().Set("flag", "true"); err != nil {
					return nil, err
				}

				DisableAuthCheckFlag(cmd.Flag("flag"))
				return cmd, nil
			},
			isAuthCheckEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := tt.init()
			require.NoError(t, err)

			// IsAuthCheckEnabled assumes commands under test are subcommands
			parent := &cobra.Command{Use: "root"}
			parent.AddCommand(cmd)
			require.Equal(t, tt.isAuthCheckEnabled, IsAuthCheckEnabled(cmd))
		})
	}
}
