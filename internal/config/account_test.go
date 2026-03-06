package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAccount(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantUser string
		wantHost string
		wantErr  string
	}{
		{
			name:     "valid github.com account",
			input:    "monalisa@github.com",
			wantUser: "monalisa",
			wantHost: "github.com",
		},
		{
			name:     "valid enterprise account",
			input:    "admin@github.enterprise.com",
			wantUser: "admin",
			wantHost: "github.enterprise.com",
		},
		{
			name:    "missing host",
			input:   "monalisa",
			wantErr: `invalid account format "monalisa". Expected "user@host" (e.g. "monalisa@github.com")`,
		},
		{
			name:    "missing user",
			input:   "@github.com",
			wantErr: `invalid account format "@github.com". Expected "user@host" (e.g. "monalisa@github.com")`,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: `invalid account format "". Expected "user@host" (e.g. "monalisa@github.com")`,
		},
		{
			name:    "trailing at sign",
			input:   "monalisa@",
			wantErr: `invalid account format "monalisa@". Expected "user@host" (e.g. "monalisa@github.com")`,
		},
		{
			name:     "host with port",
			input:    "admin@github.example.com:8443",
			wantUser: "admin",
			wantHost: "github.example.com:8443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, host, err := ParseAccount(tt.input)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantUser, user)
			require.Equal(t, tt.wantHost, host)
		})
	}
}
