package cmdutil

import (
	"testing"

	"github.com/cli/cli/internal/config"
	"github.com/stretchr/testify/assert"
)

func Test_CheckAuth(t *testing.T) {
	tests := []struct {
		name     string
		cfg      func(config.Config)
		expected bool
	}{
		{
			name:     "no hosts",
			cfg:      func(c config.Config) {},
			expected: false,
		},
		{
			name: "host, no token",
			cfg: func(c config.Config) {
				_ = c.Set("github.com", "oauth_token", "")
			},
			expected: false,
		},
		{
			name: "host, token",
			cfg: func(c config.Config) {
				_ = c.Set("github.com", "oauth_token", "a token")
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.NewBlankConfig()
			tt.cfg(cfg)
			result := CheckAuth(cfg)
			assert.Equal(t, tt.expected, result)
		})
	}
}
