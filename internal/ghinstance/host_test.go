package ghinstance

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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

func TestCategorizeHost(t *testing.T) {
	tests := []struct {
		name string
		host string
		want string
	}{
		{
			name: "github.com returns github.com",
			host: "github.com",
			want: "github.com",
		},
		{
			name: "classic GHES hostname returns ghes",
			host: "ghe.io",
			want: "ghes",
		},
		{
			name: "arbitrary enterprise hostname returns ghes",
			host: "enterprise.example.com",
			want: "ghes",
		},
		{
			name: "tenant subdomain of ghe.com returns tenancy",
			host: "tenant.ghe.com",
			want: "tenancy",
		},
		{
			name: "api subdomain under tenant returns tenancy",
			host: "api.tenant.ghe.com",
			want: "tenancy",
		},
		{
			name: "bare ghe.com returns ghes",
			host: "ghe.com",
			want: "ghes",
		},
		{
			name: "github.localhost returns uncategorized",
			host: "github.localhost",
			want: "uncategorized",
		},
		{
			name: "github.com subdomain returns uncategorized",
			host: "garage.github.com",
			want: "uncategorized",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, CategorizeHost(tt.host))
		})
	}
}
