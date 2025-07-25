package rfc

import (
	"fmt"
	"strings"
	"testing"

	"github.com/zmap/zlint/v3/lint"

	"github.com/letsencrypt/boulder/linter/lints/test"
)

func TestCrlHasNumber(t *testing.T) {
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
			name:       "no_number",
			want:       lint.Error,
			wantSubStr: "MUST include the CRL number",
		},
		{
			name:       "critical_number",
			want:       lint.Error,
			wantSubStr: "MUST NOT be marked critical",
		},
		{
			name:       "long_number",
			want:       lint.Error,
			wantSubStr: "MUST NOT be longer than 20 octets",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			l := NewCrlHasNumber()
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
