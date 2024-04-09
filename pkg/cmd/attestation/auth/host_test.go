package auth

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
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
		{
			name:        "Unsupported host",
			expectedErr: true,
			host:        "my-unsupported-host.github.com",
		},
	}

	for _, tc := range testcases {
		err := os.Setenv("GH_HOST", tc.host)
		require.NoError(t, err)

		err = IsHostSupported()
		if tc.expectedErr {
			assert.Error(t, err)
			assert.ErrorIs(t, err, ErrUnsupportedHost)
		} else {
			assert.NoError(t, err)
		}
	}
}
