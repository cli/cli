package cabfbr

import (
	"fmt"
	"strings"
	"testing"

	"github.com/zmap/zlint/v3/lint"

	"github.com/letsencrypt/boulder/linter/lints/test"
)

func TestCrlCriticalReasonCodes(t *testing.T) {
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
			name:       "critical_reason",
			want:       lint.Error,
			wantSubStr: "reasonCode extension MUST NOT be marked critical",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			l := NewCrlCriticalReasonCodes()
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
