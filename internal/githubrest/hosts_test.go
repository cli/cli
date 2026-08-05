package githubrest

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAPIBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		want     string
	}{
		{name: "github.com", hostname: "github.com", want: "https://api.github.com/"},
		{name: "uppercase github.com", hostname: "GitHub.com", want: "https://api.github.com/"},
		{name: "subdomain of github.com normalizes", hostname: "subdomain.github.com", want: "https://api.github.com/"},
		{name: "garage keeps its own host", hostname: "garage.github.com", want: "https://garage.github.com/api/v3/"},
		{name: "enterprise", hostname: "github.example.com", want: "https://github.example.com/api/v3/"},
		{name: "tenancy", hostname: "tenant.ghe.com", want: "https://api.tenant.ghe.com/"},
		{name: "localhost is plaintext", hostname: "github.localhost", want: "http://api.github.localhost/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, APIBaseURL(tt.hostname))
		})
	}
}

func TestUploadHost(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		want     string
	}{
		{name: "github.com splits uploads onto its own host", hostname: "github.com", want: "uploads.github.com"},
		{name: "subdomain of github.com normalizes", hostname: "subdomain.github.com", want: "uploads.github.com"},
		{name: "enterprise uploads on the same host", hostname: "github.example.com", want: "github.example.com"},
		{name: "garage uploads on the same host", hostname: "garage.github.com", want: "garage.github.com"},
		{name: "tenancy splits uploads onto its own host", hostname: "tenant.ghe.com", want: "uploads.tenant.ghe.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, UploadHost(tt.hostname))
		})
	}
}
