package checks

import (
	"testing"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/require"
)

func TestPrintSummary(t *testing.T) {
	tests := []struct {
		name   string
		counts checkCounts
		want   string
	}{
		{
			name:   "no checks",
			counts: checkCounts{},
			want:   "\n\n",
		},
		{
			name:   "all successful",
			counts: checkCounts{Passed: 3},
			want:   "All checks were successful\n0 cancelled, 0 failing, 3 successful, 0 skipped, and 0 pending checks\n\n",
		},
		{
			name:   "some failed",
			counts: checkCounts{Failed: 1, Passed: 2},
			want:   "Some checks were not successful\n0 cancelled, 1 failing, 2 successful, 0 skipped, and 0 pending checks\n\n",
		},
		{
			name:   "some pending",
			counts: checkCounts{Pending: 1, Passed: 2},
			want:   "Some checks are still pending\n0 cancelled, 0 failing, 2 successful, 0 skipped, and 1 pending checks\n\n",
		},
		{
			// Regression: before the fix, the guard omitted counts.Canceled, so a
			// cancelled-only result printed an empty summary.
			name:   "only cancelled",
			counts: checkCounts{Canceled: 2},
			want:   "Some checks were cancelled\n2 cancelled, 0 failing, 0 successful, 0 skipped, and 0 pending checks\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			ios.SetStdoutTTY(true)

			printSummary(ios, tt.counts)

			require.Equal(t, tt.want, stdout.String())
		})
	}
}
