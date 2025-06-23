package ratelimit

import (
	"os"
	"testing"
	"time"

	"github.com/letsencrypt/boulder/config"
	"github.com/letsencrypt/boulder/test"
)

func TestEnabled(t *testing.T) {
	policy := RateLimitPolicy{
		Threshold: 10,
	}
	if !policy.Enabled() {
		t.Errorf("Policy should have been enabled.")
	}
}

func TestNotEnabled(t *testing.T) {
	policy := RateLimitPolicy{
		Threshold: 0,
	}
	if policy.Enabled() {
		t.Errorf("Policy should not have been enabled.")
	}
}

func TestGetThreshold(t *testing.T) {
	policy := RateLimitPolicy{
		Threshold: 1,
		Overrides: map[string]int64{
			"key": 2,
			"baz": 99,
		},
		RegistrationOverrides: map[int64]int64{
			101: 3,
		},
	}

	testCases := []struct {
		Name     string
		Key      string
		RegID    int64
		Expected int64
	}{

		{
			Name:     "No key or reg overrides",
			Key:      "foo",
			RegID:    11,
			Expected: 1,
		},
		{
			Name:     "Key override, no reg override",
			Key:      "key",
			RegID:    11,
			Expected: 2,
		},
		{
			Name:     "No key override, reg override",
			Key:      "foo",
			RegID:    101,
			Expected: 3,
		},
		{
			Name:     "Key override, larger reg override",
			Key:      "foo",
			RegID:    101,
			Expected: 3,
		},
		{
			Name:     "Key override, smaller reg override",
			Key:      "baz",
			RegID:    101,
			Expected: 99,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			threshold, _ := policy.GetThreshold(tc.Key, tc.RegID)
			test.AssertEquals(t,
				threshold,
				tc.Expected)
		})
	}
}

func TestWindowBegin(t *testing.T) {
	policy := RateLimitPolicy{
		Window: config.Duration{Duration: 24 * time.Hour},
	}
	now := time.Date(2015, 9, 22, 0, 0, 0, 0, time.UTC)
	expected := time.Date(2015, 9, 21, 0, 0, 0, 0, time.UTC)
	actual := policy.WindowBegin(now)
	if actual != expected {
		t.Errorf("Incorrect WindowBegin: %s, expected %s", actual, expected)
	}
}

func TestLoadPolicies(t *testing.T) {
	policy := New()

	policyContent, readErr := os.ReadFile("../test/rate-limit-policies.yml")
	test.AssertNotError(t, readErr, "Failed to load rate-limit-policies.yml")

	// Test that loading a good policy from YAML doesn't error
	err := policy.LoadPolicies(policyContent)
	test.AssertNotError(t, err, "Failed to parse rate-limit-policies.yml")

	// Test that the CertificatesPerName section parsed correctly
	certsPerName := policy.CertificatesPerName()
	test.AssertEquals(t, certsPerName.Threshold, int64(2))
	test.AssertDeepEquals(t, certsPerName.Overrides, map[string]int64{
		"ratelimit.me":          1,
		"lim.it":                0,
		"le.wtf":                10000,
		"le1.wtf":               10000,
		"le2.wtf":               10000,
		"le3.wtf":               10000,
		"nginx.wtf":             10000,
		"good-caa-reserved.com": 10000,
		"bad-caa-reserved.com":  10000,
		"ecdsa.le.wtf":          10000,
		"must-staple.le.wtf":    10000,
	})
	test.AssertDeepEquals(t, certsPerName.RegistrationOverrides, map[int64]int64{
		101: 1000,
	})

	// Test that the RegistrationsPerIP section parsed correctly
	regsPerIP := policy.RegistrationsPerIP()
	test.AssertEquals(t, regsPerIP.Threshold, int64(10000))
	test.AssertDeepEquals(t, regsPerIP.Overrides, map[string]int64{
		"127.0.0.1": 1000000,
	})
	test.AssertEquals(t, len(regsPerIP.RegistrationOverrides), 0)

	// Test that the PendingAuthorizationsPerAccount section parsed correctly
	pendingAuthsPerAcct := policy.PendingAuthorizationsPerAccount()
	test.AssertEquals(t, pendingAuthsPerAcct.Threshold, int64(150))
	test.AssertEquals(t, len(pendingAuthsPerAcct.Overrides), 0)
	test.AssertEquals(t, len(pendingAuthsPerAcct.RegistrationOverrides), 0)

	// Test that the CertificatesPerFQDN section parsed correctly
	certsPerFQDN := policy.CertificatesPerFQDNSet()
	test.AssertEquals(t, certsPerFQDN.Threshold, int64(6))
	test.AssertDeepEquals(t, certsPerFQDN.Overrides, map[string]int64{
		"le.wtf":                10000,
		"le1.wtf":               10000,
		"le2.wtf":               10000,
		"le3.wtf":               10000,
		"le.wtf,le1.wtf":        10000,
		"good-caa-reserved.com": 10000,
		"nginx.wtf":             10000,
		"ecdsa.le.wtf":          10000,
		"must-staple.le.wtf":    10000,
	})
	test.AssertEquals(t, len(certsPerFQDN.RegistrationOverrides), 0)
	certsPerFQDNFast := policy.CertificatesPerFQDNSetFast()
	test.AssertEquals(t, certsPerFQDNFast.Threshold, int64(2))
	test.AssertDeepEquals(t, certsPerFQDNFast.Overrides, map[string]int64{
		"le.wtf": 100,
	})
	test.AssertEquals(t, len(certsPerFQDNFast.RegistrationOverrides), 0)

	// Test that loading invalid YAML generates an error
	err = policy.LoadPolicies([]byte("err"))
	test.AssertError(t, err, "Failed to generate error loading invalid yaml policy file")
	// Re-check a field of policy to make sure a LoadPolicies error doesn't
	// corrupt the existing policies
	test.AssertDeepEquals(t, policy.RegistrationsPerIP().Overrides, map[string]int64{
		"127.0.0.1": 1000000,
	})

	// Test that the RateLimitConfig accessors do not panic when there has been no
	// `LoadPolicy` call, and instead return empty RateLimitPolicy objects with default
	// values.
	emptyPolicy := New()
	test.AssertEquals(t, emptyPolicy.CertificatesPerName().Threshold, int64(0))
	test.AssertEquals(t, emptyPolicy.RegistrationsPerIP().Threshold, int64(0))
	test.AssertEquals(t, emptyPolicy.RegistrationsPerIP().Threshold, int64(0))
	test.AssertEquals(t, emptyPolicy.PendingAuthorizationsPerAccount().Threshold, int64(0))
	test.AssertEquals(t, emptyPolicy.CertificatesPerFQDNSet().Threshold, int64(0))
}
