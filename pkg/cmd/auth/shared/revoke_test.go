package shared

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRevokeOAuthTokenIfChanged(t *testing.T) {
	tests := []struct {
		name          string
		hostname      string
		previousToken string
		newToken      string
		revokeErr     error
		wantRevoke    bool
		wantErr       string
	}{
		{
			name:          "revokes replaced OAuth token",
			hostname:      "github.com",
			previousToken: "gho_previous",
			newToken:      "gho_new",
			wantRevoke:    true,
		},
		{
			name:          "revokes OAuth token replaced by PAT",
			hostname:      "github.com",
			previousToken: "gho_previous",
			newToken:      "ghp_new",
			wantRevoke:    true,
		},
		{
			name:          "does not revoke unchanged token",
			hostname:      "github.com",
			previousToken: "gho_same",
			newToken:      "gho_same",
		},
		{
			name:          "does not revoke PAT",
			hostname:      "github.com",
			previousToken: "ghp_previous",
			newToken:      "gho_new",
		},
		{
			name:          "does not revoke missing token",
			hostname:      "github.com",
			previousToken: "",
			newToken:      "gho_new",
		},
		{
			name:          "does not revoke token on GitHub Enterprise Server",
			hostname:      "example.com",
			previousToken: "gho_previous",
			newToken:      "gho_new",
		},
		{
			name:          "revokes token on GitHub Enterprise Cloud",
			hostname:      "tenant.ghe.com",
			previousToken: "gho_previous",
			newToken:      "gho_new",
			wantRevoke:    true,
		},
		{
			name:          "returns revocation error",
			hostname:      "github.com",
			previousToken: "gho_previous",
			newToken:      "gho_new",
			revokeErr:     errors.New("revocation failed"),
			wantRevoke:    true,
			wantErr:       "revocation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var revokedToken string
			err := RevokeOAuthTokenIfChanged(
				&http.Client{}, tt.hostname, tt.previousToken, tt.newToken,
				func(_ *http.Client, hostname, token string) error {
					require.Equal(t, tt.hostname, hostname)
					revokedToken = token
					return tt.revokeErr
				})

			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			if tt.wantRevoke {
				require.Equal(t, tt.previousToken, revokedToken)
			} else {
				require.Empty(t, revokedToken)
			}
		})
	}
}
