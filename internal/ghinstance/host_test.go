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
			host: "github.localhost:8080",
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

func TestIsTenancy(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{
			host: "github.com",
			want: false,
		},
		{
			host: "github.localhost",
			want: false,
		},
		{
			host: "github.localhost:8080",
			want: false,
		},
		{
			host: "garage.github.com",
			want: false,
		},
		{
			host: "ghe.com",
			want: false,
		},
		{
			host: "tenant.ghe.com",
			want: true,
		},
		{
			host: "api.tenant.ghe.com",
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := IsTenancy(tt.host); got != tt.want {
				t.Errorf("IsTenancy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTenantName(t *testing.T) {
	tests := []struct {
		host       string
		wantTenant string
		wantFound  bool
	}{
		{
			host:       "github.com",
			wantTenant: "github.com",
		},
		{
			host:       "github.localhost",
			wantTenant: "github.localhost",
		},
		{
			host:       "github.localhost:8080",
			wantTenant: "github.localhost:8080",
		},
		{
			host:       "garage.github.com",
			wantTenant: "github.com",
		},
		{
			host:       "ghe.com",
			wantTenant: "ghe.com",
		},
		{
			host:       "tenant.ghe.com",
			wantTenant: "tenant",
			wantFound:  true,
		},
		{
			host:       "api.tenant.ghe.com",
			wantTenant: "tenant",
			wantFound:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if tenant, found := TenantName(tt.host); tenant != tt.wantTenant || found != tt.wantFound {
				t.Errorf("TenantName(%v) = %v %v, want %v %v", tt.host, tenant, found, tt.wantTenant, tt.wantFound)
			}
		})
	}
}

func TestIsLocal(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{
			host: "github.com",
			want: false,
		},
		{
			host: "garage.github.com",
			want: false,
		},
		{
			host: "ghe.com",
			want: false,
		},
		{
			host: "github.localhost",
			want: true,
		},
		{
			host: "github.localhost:8080",
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := IsLocal(tt.host); got != tt.want {
				t.Errorf("IsLocal() = %v, want %v", got, tt.want)
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
			host: "gitHub.LocalHost:8080",
			want: "github.localhost:8080",
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
		{
			host: "ghe.com",
			want: "ghe.com",
		},
		{
			host: "tenant.ghe.com",
			want: "tenant.ghe.com",
		},
		{
			host: "api.tenant.ghe.com",
			want: "tenant.ghe.com",
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
			host: "github.localhost:8080",
			want: "http://api.github.localhost:8080/graphql",
		},
		{
			host: "garage.github.com",
			want: "https://garage.github.com/api/graphql",
		},
		{
			host: "ghe.io",
			want: "https://ghe.io/api/graphql",
		},
		{
			host: "tenant.ghe.com",
			want: "https://api.tenant.ghe.com/graphql",
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
			host: "github.localhost:8080",
			want: "http://api.github.localhost:8080/",
		},
		{
			host: "garage.github.com",
			want: "https://garage.github.com/api/v3/",
		},
		{
			host: "ghe.io",
			want: "https://ghe.io/api/v3/",
		},
		{
			host: "tenant.ghe.com",
			want: "https://api.tenant.ghe.com/",
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
