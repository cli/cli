package auth

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsHostSupported(t *testing.T) {
	testcases := []struct {
		name        string
		expectedErr bool
		host        string
	}{
		{
			name:        "Default github.com host",
			expectedErr: false,
			host:        "github.com",
		},
		{
			name:        "Localhost",
			expectedErr: false,
			host:        "github.localhost",
		},
		{
			name:        "No host set",
			expectedErr: false,
			host:        "",
		},
		{
			name:        "GHE tenant host",
			expectedErr: false,
			host:        "some-tenant.ghe.com",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GH_HOST", tc.host)

			err := IsHostSupported()
			if tc.expectedErr {
				require.ErrorIs(t, err, ErrUnsupportedHost)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
