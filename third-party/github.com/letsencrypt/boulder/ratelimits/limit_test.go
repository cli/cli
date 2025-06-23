package ratelimits

import (
	"os"
	"testing"
	"time"

	"github.com/letsencrypt/boulder/config"
	"github.com/letsencrypt/boulder/test"
)

func TestParseOverrideNameId(t *testing.T) {
	// 'enum:ipv4'
	// Valid IPv4 address.
	name, id, err := parseOverrideNameId(NewRegistrationsPerIPAddress.String() + ":10.0.0.1")
	test.AssertNotError(t, err, "should not error")
	test.AssertEquals(t, name, NewRegistrationsPerIPAddress)
	test.AssertEquals(t, id, "10.0.0.1")

	// 'enum:ipv6range'
	// Valid IPv6 address range.
	name, id, err = parseOverrideNameId(NewRegistrationsPerIPv6Range.String() + ":2001:0db8:0000::/48")
	test.AssertNotError(t, err, "should not error")
	test.AssertEquals(t, name, NewRegistrationsPerIPv6Range)
	test.AssertEquals(t, id, "2001:0db8:0000::/48")

	// Missing colon (this should never happen but we should avoid panicking).
	_, _, err = parseOverrideNameId(NewRegistrationsPerIPAddress.String() + "10.0.0.1")
	test.AssertError(t, err, "missing colon")

	// Empty string.
	_, _, err = parseOverrideNameId("")
	test.AssertError(t, err, "empty string")

	// Only a colon.
	_, _, err = parseOverrideNameId(NewRegistrationsPerIPAddress.String() + ":")
	test.AssertError(t, err, "only a colon")

	// Invalid enum.
	_, _, err = parseOverrideNameId("lol:noexist")
	test.AssertError(t, err, "invalid enum")
}

func TestValidateLimit(t *testing.T) {
	err := validateLimit(limit{Burst: 1, Count: 1, Period: config.Duration{Duration: time.Second}})
	test.AssertNotError(t, err, "valid limit")

	// All of the following are invalid.
	for _, l := range []limit{
		{Burst: 0, Count: 1, Period: config.Duration{Duration: time.Second}},
		{Burst: 1, Count: 0, Period: config.Duration{Duration: time.Second}},
		{Burst: 1, Count: 1, Period: config.Duration{Duration: 0}},
	} {
		err = validateLimit(l)
		test.AssertError(t, err, "limit should be invalid")
	}
}

func TestLoadAndParseOverrideLimits(t *testing.T) {
	// Load a single valid override limit with Id formatted as 'enum:RegId'.
	l, err := loadAndParseOverrideLimits("testdata/working_override.yml")
	test.AssertNotError(t, err, "valid single override limit")
	expectKey := joinWithColon(NewRegistrationsPerIPAddress.EnumString(), "10.0.0.2")
	test.AssertEquals(t, l[expectKey].Burst, int64(40))
	test.AssertEquals(t, l[expectKey].Count, int64(40))
	test.AssertEquals(t, l[expectKey].Period.Duration, time.Second)

	// Load single valid override limit with a 'domain' Id.
	l, err = loadAndParseOverrideLimits("testdata/working_override_regid_domain.yml")
	test.AssertNotError(t, err, "valid single override limit with Id of regId:domain")
	expectKey = joinWithColon(CertificatesPerDomain.EnumString(), "example.com")
	test.AssertEquals(t, l[expectKey].Burst, int64(40))
	test.AssertEquals(t, l[expectKey].Count, int64(40))
	test.AssertEquals(t, l[expectKey].Period.Duration, time.Second)

	// Load multiple valid override limits with 'regId' Ids.
	l, err = loadAndParseOverrideLimits("testdata/working_overrides.yml")
	test.AssertNotError(t, err, "multiple valid override limits")
	expectKey1 := joinWithColon(NewRegistrationsPerIPAddress.EnumString(), "10.0.0.2")
	test.AssertEquals(t, l[expectKey1].Burst, int64(40))
	test.AssertEquals(t, l[expectKey1].Count, int64(40))
	test.AssertEquals(t, l[expectKey1].Period.Duration, time.Second)
	expectKey2 := joinWithColon(NewRegistrationsPerIPv6Range.EnumString(), "2001:0db8:0000::/48")
	test.AssertEquals(t, l[expectKey2].Burst, int64(50))
	test.AssertEquals(t, l[expectKey2].Count, int64(50))
	test.AssertEquals(t, l[expectKey2].Period.Duration, time.Second*2)

	// Load multiple valid override limits with 'fqdnSet' Ids, as follows:
	//   - CertificatesPerFQDNSet:example.com
	//   - CertificatesPerFQDNSet:example.com,example.net
	//   - CertificatesPerFQDNSet:example.com,example.net,example.org
	firstEntryKey, err := newFQDNSetBucketKey(CertificatesPerFQDNSet, []string{"example.com"})
	test.AssertNotError(t, err, "valid fqdnSet with one domain should not fail")
	secondEntryKey, err := newFQDNSetBucketKey(CertificatesPerFQDNSet, []string{"example.com", "example.net"})
	test.AssertNotError(t, err, "valid fqdnSet with two domains should not fail")
	thirdEntryKey, err := newFQDNSetBucketKey(CertificatesPerFQDNSet, []string{"example.com", "example.net", "example.org"})
	test.AssertNotError(t, err, "valid fqdnSet with three domains should not fail")
	l, err = loadAndParseOverrideLimits("testdata/working_overrides_regid_fqdnset.yml")
	test.AssertNotError(t, err, "multiple valid override limits with 'fqdnSet' Ids")
	test.AssertEquals(t, l[firstEntryKey].Burst, int64(40))
	test.AssertEquals(t, l[firstEntryKey].Count, int64(40))
	test.AssertEquals(t, l[firstEntryKey].Period.Duration, time.Second)
	test.AssertEquals(t, l[secondEntryKey].Burst, int64(50))
	test.AssertEquals(t, l[secondEntryKey].Count, int64(50))
	test.AssertEquals(t, l[secondEntryKey].Period.Duration, time.Second*2)
	test.AssertEquals(t, l[thirdEntryKey].Burst, int64(60))
	test.AssertEquals(t, l[thirdEntryKey].Count, int64(60))
	test.AssertEquals(t, l[thirdEntryKey].Period.Duration, time.Second*3)

	// Path is empty string.
	_, err = loadAndParseOverrideLimits("")
	test.AssertError(t, err, "path is empty string")
	test.Assert(t, os.IsNotExist(err), "path is empty string")

	// Path to file which does not exist.
	_, err = loadAndParseOverrideLimits("testdata/file_does_not_exist.yml")
	test.AssertError(t, err, "a file that does not exist ")
	test.Assert(t, os.IsNotExist(err), "test file should not exist")

	// Burst cannot be 0.
	_, err = loadAndParseOverrideLimits("testdata/busted_override_burst_0.yml")
	test.AssertError(t, err, "single override limit with burst=0")
	test.Assert(t, !os.IsNotExist(err), "test file should exist")

	// Id cannot be empty.
	_, err = loadAndParseOverrideLimits("testdata/busted_override_empty_id.yml")
	test.AssertError(t, err, "single override limit with empty id")
	test.Assert(t, !os.IsNotExist(err), "test file should exist")

	// Name cannot be empty.
	_, err = loadAndParseOverrideLimits("testdata/busted_override_empty_name.yml")
	test.AssertError(t, err, "single override limit with empty name")
	test.Assert(t, !os.IsNotExist(err), "test file should exist")

	// Name must be a string representation of a valid Name enumeration.
	_, err = loadAndParseOverrideLimits("testdata/busted_override_invalid_name.yml")
	test.AssertError(t, err, "single override limit with invalid name")
	test.Assert(t, !os.IsNotExist(err), "test file should exist")

	// Multiple entries, second entry has a bad name.
	_, err = loadAndParseOverrideLimits("testdata/busted_overrides_second_entry_bad_name.yml")
	test.AssertError(t, err, "multiple override limits, second entry is bad")
	test.Assert(t, !os.IsNotExist(err), "test file should exist")

	// Multiple entries, third entry has id of "lol", instead of an IPv4 address.
	_, err = loadAndParseOverrideLimits("testdata/busted_overrides_third_entry_bad_id.yml")
	test.AssertError(t, err, "multiple override limits, third entry has bad Id value")
	test.Assert(t, !os.IsNotExist(err), "test file should exist")
}

func TestLoadAndParseDefaultLimits(t *testing.T) {
	// Load a single valid default limit.
	l, err := loadAndParseDefaultLimits("testdata/working_default.yml")
	test.AssertNotError(t, err, "valid single default limit")
	test.AssertEquals(t, l[NewRegistrationsPerIPAddress.EnumString()].Burst, int64(20))
	test.AssertEquals(t, l[NewRegistrationsPerIPAddress.EnumString()].Count, int64(20))
	test.AssertEquals(t, l[NewRegistrationsPerIPAddress.EnumString()].Period.Duration, time.Second)

	// Load multiple valid default limits.
	l, err = loadAndParseDefaultLimits("testdata/working_defaults.yml")
	test.AssertNotError(t, err, "multiple valid default limits")
	test.AssertEquals(t, l[NewRegistrationsPerIPAddress.EnumString()].Burst, int64(20))
	test.AssertEquals(t, l[NewRegistrationsPerIPAddress.EnumString()].Count, int64(20))
	test.AssertEquals(t, l[NewRegistrationsPerIPAddress.EnumString()].Period.Duration, time.Second)
	test.AssertEquals(t, l[NewRegistrationsPerIPv6Range.EnumString()].Burst, int64(30))
	test.AssertEquals(t, l[NewRegistrationsPerIPv6Range.EnumString()].Count, int64(30))
	test.AssertEquals(t, l[NewRegistrationsPerIPv6Range.EnumString()].Period.Duration, time.Second*2)

	// Path is empty string.
	_, err = loadAndParseDefaultLimits("")
	test.AssertError(t, err, "path is empty string")
	test.Assert(t, os.IsNotExist(err), "path is empty string")

	// Path to file which does not exist.
	_, err = loadAndParseDefaultLimits("testdata/file_does_not_exist.yml")
	test.AssertError(t, err, "a file that does not exist")
	test.Assert(t, os.IsNotExist(err), "test file should not exist")

	// Burst cannot be 0.
	_, err = loadAndParseDefaultLimits("testdata/busted_default_burst_0.yml")
	test.AssertError(t, err, "single default limit with burst=0")
	test.Assert(t, !os.IsNotExist(err), "test file should exist")

	// Name cannot be empty.
	_, err = loadAndParseDefaultLimits("testdata/busted_default_empty_name.yml")
	test.AssertError(t, err, "single default limit with empty name")
	test.Assert(t, !os.IsNotExist(err), "test file should exist")

	// Name must be a string representation of a valid Name enumeration.
	_, err = loadAndParseDefaultLimits("testdata/busted_default_invalid_name.yml")
	test.AssertError(t, err, "single default limit with invalid name")
	test.Assert(t, !os.IsNotExist(err), "test file should exist")

	// Multiple entries, second entry has a bad name.
	_, err = loadAndParseDefaultLimits("testdata/busted_defaults_second_entry_bad_name.yml")
	test.AssertError(t, err, "multiple default limits, one is bad")
	test.Assert(t, !os.IsNotExist(err), "test file should exist")
}
