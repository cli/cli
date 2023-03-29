package ghinstance

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
			host: "github.localhost",
			want: false,
		},
		{
			host: "api.github.localhost",
			want: false,
		},
		{
			host: "garage.github.com",
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
			host: "GitHub.localhost",
			want: "github.localhost",
		},
		{
			host: "api.github.localhost",
			want: "github.localhost",
		},
		{
			host: "garage.github.com",
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
		input    string
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
			host: "github.localhost",
			want: "http://api.github.localhost/graphql",
		},
		{
			host: "garage.github.com",
			want: "https://garage.github.com/api/graphql",
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
			host: "github.localhost",
			want: "http://api.github.localhost/",
		},
		{
			host: "garage.github.com",
			want: "https://garage.github.com/api/v3/",
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
