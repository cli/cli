package rfc

import (
	"fmt"
	"strings"
	"testing"

	"github.com/zmap/zlint/v3/lint"

	"github.com/letsencrypt/boulder/linter/lints/test"
)

func TestCrlHasValidTimestamps(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		want       lint.LintStatus
		wantSubStr string
	}{
		{
			name: "good",
			want: lint.Pass,
		},
		{
			name: "good_utctime_1950",
			want: lint.Pass,
		},
		{
			name: "good_gentime_2050",
			want: lint.Pass,
		},
		{
			name:       "gentime_2049",
			want:       lint.Error,
			wantSubStr: "timestamps prior to 2050 MUST be encoded using UTCTime",
		},
		{
			name:       "utctime_no_seconds",
			want:       lint.Error,
			wantSubStr: "timestamps encoded using UTCTime MUST be specified in the format \"YYMMDDHHMMSSZ\"",
		},
		{
			name:       "gentime_revoked_2049",
			want:       lint.Error,
			wantSubStr: "timestamps prior to 2050 MUST be encoded using UTCTime",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			l := NewCrlHasValidTimestamps()
			c := test.LoadPEMCRL(t, fmt.Sprintf("testdata/crl_%s.pem", tc.name))
			r := l.Execute(c)

			if r.Status != tc.want {
				t.Errorf("expected %q, got %q", tc.want, r.Status)
			}
			if !strings.Contains(r.Details, tc.wantSubStr) {
				t.Errorf("expected %q, got %q", tc.wantSubStr, r.Details)
			}
		})
	}
}
