package cabfbr

import (
	"fmt"
	"strings"
	"testing"

	"github.com/zmap/zlint/v3/lint"

	"github.com/letsencrypt/boulder/linter/lints/test"
)

func TestCrlValidityPeriod(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		want       lint.LintStatus
		wantSubStr string
	}{
		{
			name: "good", // CRL for subscriber certs
			want: lint.Pass,
		},
		{
			name: "good_subordinate_ca",
			want: lint.Pass,
		},
		{
			name:       "idp_distributionPoint_and_onlyUser_and_onlyCA", // What type of CRL is it (besides horrible)?!!??!
			want:       lint.Error,
			wantSubStr: "IssuingDistributionPoint should not have both onlyContainsUserCerts: TRUE and onlyContainsCACerts: TRUE",
		},
		{
			name:       "negative_validity",
			want:       lint.Warn,
			wantSubStr: "CRL missing IssuingDistributionPoint",
		},
		{
			name:       "negative_validity_subscriber_cert",
			want:       lint.Error,
			wantSubStr: "at or before",
		},
		{
			name:       "negative_validity_subordinate_ca",
			want:       lint.Error,
			wantSubStr: "at or before",
		},
		{
			name:       "long_validity_subscriber_cert", // 10 days + 1 second
			want:       lint.Error,
			wantSubStr: "CRL has validity period greater than 10 days",
		},
		{
			name:       "long_validity_subordinate_ca", // 1 year + 1 second
			want:       lint.Error,
			wantSubStr: "CRL has validity period greater than 365 days",
		},
		{
			// Technically this CRL is incorrect because Let's Encrypt does not
			// (yet) issue CRLs containing both the distributionPoint and
			// optional onlyContainsCACerts boolean, but we're still parsing the
			// correct BR validity in this lint.
			name: "long_validity_distributionPoint_and_subordinate_ca",
			want: lint.Pass,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			l := NewCrlValidityPeriod()
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
