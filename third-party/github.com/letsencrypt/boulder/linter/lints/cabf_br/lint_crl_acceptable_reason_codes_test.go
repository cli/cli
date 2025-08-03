package cabfbr

import (
	"fmt"
	"strings"
	"testing"

	"github.com/zmap/zlint/v3/lint"

	"github.com/letsencrypt/boulder/linter/lints/test"
)

func TestCrlAcceptableReasonCodes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		want       lint.LintStatus
		wantSubStr string
	}{
		{
			// crl_good.pem contains a revocation entry with no reason code extension.
			name: "good",
			want: lint.Pass,
		},
		{
			name:       "reason_0",
			want:       lint.Error,
			wantSubStr: "MUST NOT include reasonCodes other than",
		},
		{
			name: "reason_1",
			want: lint.Pass,
		},
		{
			name:       "reason_2",
			want:       lint.Error,
			wantSubStr: "MUST NOT include reasonCodes other than",
		},
		{
			name: "reason_3",
			want: lint.Pass,
		},
		{
			name: "reason_4",
			want: lint.Pass,
		},
		{
			name: "reason_5",
			want: lint.Pass,
		},
		{
			name:       "reason_6",
			want:       lint.Error,
			wantSubStr: "MUST NOT include reasonCodes other than",
		},
		{
			name:       "reason_8",
			want:       lint.Error,
			wantSubStr: "MUST NOT include reasonCodes other than",
		},
		{
			name: "reason_9",
			want: lint.Pass,
		},
		{
			name:       "reason_10",
			want:       lint.Error,
			wantSubStr: "MUST NOT include reasonCodes other than",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			l := NewCrlAcceptableReasonCodes()
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
