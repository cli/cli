package cpcps

import (
	"fmt"
	"strings"
	"testing"

	"github.com/zmap/zlint/v3/lint"

	linttest "github.com/letsencrypt/boulder/linter/lints/test"
)

func TestCrlHasIDP(t *testing.T) {
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
			name:       "no_idp",
			want:       lint.Warn,
			wantSubStr: "CRL missing IssuingDistributionPoint",
		},
		{
			name:       "idp_no_dpn",
			want:       lint.Error,
			wantSubStr: "User certificate CRLs MUST have at least one DistributionPointName FullName",
		},
		{
			name:       "idp_no_fullname",
			want:       lint.Error,
			wantSubStr: "Failed to read IssuingDistributionPoint distributionPoint fullName",
		},
		{
			name:       "idp_no_uris",
			want:       lint.Error,
			wantSubStr: "IssuingDistributionPoint FullName URI MUST be present",
		},
		{
			name:       "idp_two_uris",
			want:       lint.Notice,
			wantSubStr: "IssuingDistributionPoint unexpectedly has more than one FullName",
		},
		{
			name:       "idp_https",
			want:       lint.Error,
			wantSubStr: "IssuingDistributionPoint URI MUST use http scheme",
		},
		{
			name:       "idp_no_usercerts",
			want:       lint.Error,
			wantSubStr: "Neither onlyContainsUserCerts nor onlyContainsCACerts was set",
		},
		{
			name:       "idp_some_reasons", // Subscriber cert
			want:       lint.Error,
			wantSubStr: "Unexpected IssuingDistributionPoint fields were found",
		},
		{
			name:       "idp_distributionPoint_and_onlyCA",
			want:       lint.Error,
			wantSubStr: "CA certificate CRLs SHOULD NOT have a DistributionPointName FullName",
		},
		{
			name:       "idp_distributionPoint_and_onlyUser_and_onlyCA",
			want:       lint.Error,
			wantSubStr: "IssuingDistributionPoint should not have both onlyContainsUserCerts: TRUE and onlyContainsCACerts: TRUE",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			l := NewCrlHasIDP()
			c := linttest.LoadPEMCRL(t, fmt.Sprintf("testdata/crl_%s.pem", tc.name))
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
