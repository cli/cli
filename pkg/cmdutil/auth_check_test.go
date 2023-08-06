package cmdutil

import (
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/stretchr/testify/assert"
)

func Test_CheckAuth(t *testing.T) {
	tests := []struct {
		name     string
		cfgStubs func(*config.ConfigMock)
		expected bool
	}{
		{
			name:     "no known hosts, no env auth token",
			cfgStubs: func(c *config.ConfigMock) {},
			expected: false,
		},
		{
			name: "no known hosts, env auth token",
			cfgStubs: func(c *config.ConfigMock) {
				c.AuthenticationFunc = func() *config.AuthConfig {
					authCfg := &config.AuthConfig{}
					authCfg.SetToken("token", "GITHUB_TOKEN")
					return authCfg
				}
			},
			expected: true,
		},
		{
			name: "known host",
			cfgStubs: func(c *config.ConfigMock) {
				c.Set("github.com", "oauth_token", "token")
			},
			expected: true,
		},
		{
			name: "enterprise token",
			cfgStubs: func(c *config.ConfigMock) {
				t.Setenv("GH_ENTERPRISE_TOKEN", "token")
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.NewBlankConfig()
			tt.cfgStubs(cfg)
			result := CheckAuth(cfg)
			assert.Equal(t, tt.expected, result)
		})
	}
}
