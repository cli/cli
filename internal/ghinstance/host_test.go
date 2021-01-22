package ghinstance

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOverridableDefault(t *testing.T) {
	oldOverride := hostnameOverride
	t.Cleanup(func() {
		hostnameOverride = oldOverride
	})

	host := OverridableDefault()
	if host != "github.com" {
		t.Errorf("expected github.com, got %q", host)
	}

	OverrideDefault("example.org")

	host = OverridableDefault()
	if host != "example.org" {
		t.Errorf("expected example.org, got %q", host)
	}
	host = Default()
	if host != "github.com" {
		t.Errorf("expected github.com, got %q", host)
	}
}

func TestIsEnterprise(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{
			host: "github.com",
			want: false,
		},
		{
			host: "api.github.com",
			want: false,
		},
		{
			host: "ghe.io",
			want: true,
		},
		{
			host: "example.com",
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := IsEnterprise(tt.host); got != tt.want {
				t.Errorf("IsEnterprise() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeHostname(t *testing.T) {
	tests := []struct {
		host string
		want string
	}{
		{
			host: "GitHub.com",
			want: "github.com",
		},
		{
			host: "api.github.com",
			want: "github.com",
		},
		{
			host: "ssh.github.com",
			want: "github.com",
		},
		{
			host: "upload.github.com",
			want: "github.com",
		},
		{
			host: "GHE.IO",
			want: "ghe.io",
		},
		{
			host: "git.my.org",
			want: "git.my.org",
		},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := NormalizeHostname(tt.host); got != tt.want {
				t.Errorf("NormalizeHostname() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHostnameValidator(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		wantsErr bool
	}{
		{
			name:     "valid hostname",
			input:    "internal.instance",
			wantsErr: false,
		},
		{
			name:     "hostname with slashes",
			input:    "//internal.instance",
			wantsErr: true,
		},
		{
			name:     "empty hostname",
			input:    "   ",
			wantsErr: true,
		},
		{
			name:     "hostname with colon",
			input:    "internal.instance:2205",
			wantsErr: true,
		},
		{
			name:     "non-string hostname",
			input:    62,
			wantsErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := HostnameValidator(tt.input)
			if tt.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}
func TestGraphQLEndpoint(t *testing.T) {
	tests := []struct {
		host string
		want string
	}{
		{
			host: "github.com",
			want: "https://api.github.com/graphql",
		},
		{
			host: "ghe.io",
			want: "https://ghe.io/api/graphql",
		},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := GraphQLEndpoint(tt.host); got != tt.want {
				t.Errorf("GraphQLEndpoint() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRESTPrefix(t *testing.T) {
	tests := []struct {
		host string
		want string
	}{
		{
			host: "github.com",
			want: "https://api.github.com/",
		},
		{
			host: "ghe.io",
			want: "https://ghe.io/api/v3/",
		},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := RESTPrefix(tt.host); got != tt.want {
				t.Errorf("RESTPrefix() = %v, want %v", got, tt.want)
			}
		})
	}
}
