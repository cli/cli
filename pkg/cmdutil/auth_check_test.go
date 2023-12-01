package cmdutil

import (
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/stretchr/testify/require"
)

func Test_CheckAuth(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		cfgStubs func(*testing.T, config.Config)
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
			cfgStubs: func(t *testing.T, c config.Config) {
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
