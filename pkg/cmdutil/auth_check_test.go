package cmdutil

import (
	"os"
	"testing"

	"github.com/cli/cli/internal/config"
	"github.com/stretchr/testify/assert"
)

func Test_CheckAuth(t *testing.T) {
	orig_GITHUB_TOKEN := os.Getenv("GITHUB_TOKEN")
	t.Cleanup(func() {
		os.Setenv("GITHUB_TOKEN", orig_GITHUB_TOKEN)
	})

	tests := []struct {
		name     string
		cfg      func(config.Config)
		envToken bool
		expected bool
	}{
		{
			name:     "no hosts",
			cfg:      func(c config.Config) {},
			envToken: false,
			expected: false,
		},
		{name: "no hosts, env auth token",
			cfg:      func(c config.Config) {},
			envToken: true,
			expected: true,
		},
		{
			name: "host, no token",
			cfg: func(c config.Config) {
				_ = c.Set("github.com", "oauth_token", "")
			},
			envToken: false,
			expected: false,
		},
		{
			name: "host, token",
			cfg: func(c config.Config) {
				_ = c.Set("github.com", "oauth_token", "a token")
			},
			envToken: false,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envToken {
				os.Setenv("GITHUB_TOKEN", "TOKEN")
			} else {
				os.Setenv("GITHUB_TOKEN", "")
			}

			cfg := config.NewBlankConfig()
			tt.cfg(cfg)
			result := CheckAuth(cfg)
			assert.Equal(t, tt.expected, result)
		})
	}
}
