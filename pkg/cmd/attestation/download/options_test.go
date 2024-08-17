package download

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAreFlagsValid(t *testing.T) {
	tests := []struct {
		name  string
		limit int
	}{
		{
			name:  "Limit is too low",
			limit: 0,
		},
		{
			name:  "Limit is too high",
			limit: 1001,
		},
	}
	for _, tc := range tests {
		opts := Options{
			Limit: tc.limit,
		}

		err := opts.AreFlagsValid()
		require.Error(t, err)
		expectedErrMsg := fmt.Sprintf("limit %d not allowed, must be between 1 and 1000", tc.limit)
		require.ErrorContains(t, err, expectedErrMsg)
	}
}
