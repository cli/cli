package ratelimits

import (
	"fmt"
	"testing"

	"github.com/letsencrypt/boulder/test"
)

func TestNameIsValid(t *testing.T) {
	t.Parallel()
	type args struct {
		name Name
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{name: "Unknown", args: args{name: Unknown}, want: false},
		{name: "9001", args: args{name: 9001}, want: false},
		{name: "NewRegistrationsPerIPAddress", args: args{name: NewRegistrationsPerIPAddress}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.args.name.isValid()
			test.AssertEquals(t, tt.want, got)
		})
	}
}

func TestValidateIdForName(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		limit Name
		desc  string
		id    string
		err   string
	}{
		{
			limit: NewRegistrationsPerIPAddress,
			desc:  "valid IPv4 address",
			id:    "64.112.117.1",
		},
		{
			limit: NewRegistrationsPerIPAddress,
			desc:  "reserved IPv4 address",
			id:    "10.0.0.1",
			err:   "in a reserved address block",
		},
		{
			limit: NewRegistrationsPerIPAddress,
			desc:  "valid IPv6 address",
			id:    "2602:80a:6000::42:42",
		},
		{
			limit: NewRegistrationsPerIPAddress,
			desc:  "IPv6 address in non-canonical form",
			id:    "2001:0db8:85a3:0000:0000:8a2e:0370:7334",
			err:   "must be in canonical form",
		},
		{
			limit: NewRegistrationsPerIPAddress,
			desc:  "empty string",
			id:    "",
			err:   "must be an IP address",
		},
		{
			limit: NewRegistrationsPerIPAddress,
			desc:  "one space",
			id:    " ",
			err:   "must be an IP address",
		},
		{
			limit: NewRegistrationsPerIPAddress,
			desc:  "invalid IPv4 address",
			id:    "10.0.0.9000",
			err:   "must be an IP address",
		},
		{
			limit: NewRegistrationsPerIPAddress,
			desc:  "invalid IPv6 address",
			id:    "2001:0db8:85a3:0000:0000:8a2e:0370:7334:9000",
			err:   "must be an IP address",
		},
		{
			limit: NewRegistrationsPerIPv6Range,
			desc:  "valid IPv6 address range",
			id:    "2602:80a:6000::/48",
		},
		{
			limit: NewRegistrationsPerIPv6Range,
			desc:  "IPv6 address range in non-canonical form",
			id:    "2602:080a:6000::/48",
			err:   "must be in canonical form",
		},
		{
			limit: NewRegistrationsPerIPv6Range,
			desc:  "IPv6 address range with low bits set",
			id:    "2602:080a:6000::1/48",
			err:   "must be in canonical form",
		},
		{
			limit: NewRegistrationsPerIPv6Range,
			desc:  "invalid IPv6 CIDR range",
			id:    "2001:0db8:0000::/128",
			err:   "must be /48",
		},
		{
			limit: NewRegistrationsPerIPv6Range,
			desc:  "invalid IPv6 CIDR",
			id:    "2001:0db8:0000::/48/48",
			err:   "must be an IPv6 CIDR range",
		},
		{
			limit: NewRegistrationsPerIPv6Range,
			desc:  "IPv4 CIDR when we expect IPv6 CIDR range",
			id:    "10.0.0.0/16",
			err:   "must be /48",
		},
		{
			limit: NewRegistrationsPerIPv6Range,
			desc:  "IPv4 CIDR with invalid long mask",
			id:    "10.0.0.0/48",
			err:   "must be an IPv6 CIDR range",
		},
		{
			limit: NewOrdersPerAccount,
			desc:  "valid regId",
			id:    "1234567890",
		},
		{
			limit: NewOrdersPerAccount,
			desc:  "invalid regId",
			id:    "lol",
			err:   "must be an ACME registration Id",
		},
		{
			limit: FailedAuthorizationsPerDomainPerAccount,
			desc:  "transaction: valid regId and domain",
			id:    "12345:example.com",
		},
		{
			limit: FailedAuthorizationsPerDomainPerAccount,
			desc:  "transaction: invalid regId",
			id:    "12ea5:example.com",
			err:   "invalid regId",
		},
		{
			limit: FailedAuthorizationsPerDomainPerAccount,
			desc:  "transaction: invalid domain",
			id:    "12345:examplecom",
			err:   "name needs at least one dot",
		},
		{
			limit: FailedAuthorizationsPerDomainPerAccount,
			desc:  "override: valid regId",
			id:    "12345",
		},
		{
			limit: FailedAuthorizationsPerDomainPerAccount,
			desc:  "override: invalid regId",
			id:    "12ea5",
			err:   "invalid regId",
		},
		{
			limit: FailedAuthorizationsForPausingPerDomainPerAccount,
			desc:  "transaction: valid regId and domain",
			id:    "12345:example.com",
		},
		{
			limit: FailedAuthorizationsForPausingPerDomainPerAccount,
			desc:  "transaction: invalid regId",
			id:    "12ea5:example.com",
			err:   "invalid regId",
		},
		{
			limit: FailedAuthorizationsForPausingPerDomainPerAccount,
			desc:  "transaction: invalid domain",
			id:    "12345:examplecom",
			err:   "name needs at least one dot",
		},
		{
			limit: FailedAuthorizationsForPausingPerDomainPerAccount,
			desc:  "override: valid regId",
			id:    "12345",
		},
		{
			limit: FailedAuthorizationsForPausingPerDomainPerAccount,
			desc:  "override: invalid regId",
			id:    "12ea5",
			err:   "invalid regId",
		},
		{
			limit: CertificatesPerDomainPerAccount,
			desc:  "transaction: valid regId and domain",
			id:    "12345:example.com",
		},
		{
			limit: CertificatesPerDomainPerAccount,
			desc:  "transaction: invalid regId",
			id:    "12ea5:example.com",
			err:   "invalid regId",
		},
		{
			limit: CertificatesPerDomainPerAccount,
			desc:  "transaction: invalid domain",
			id:    "12345:examplecom",
			err:   "name needs at least one dot",
		},
		{
			limit: CertificatesPerDomainPerAccount,
			desc:  "override: valid regId",
			id:    "12345",
		},
		{
			limit: CertificatesPerDomainPerAccount,
			desc:  "override: invalid regId",
			id:    "12ea5",
			err:   "invalid regId",
		},
		{
			limit: CertificatesPerDomain,
			desc:  "valid domain",
			id:    "example.com",
		},
		{
			limit: CertificatesPerDomain,
			desc:  "valid IPv4 address",
			id:    "64.112.117.1",
		},
		{
			limit: CertificatesPerDomain,
			desc:  "valid IPv6 address",
			id:    "2602:80a:6000::",
		},
		{
			limit: CertificatesPerDomain,
			desc:  "IPv6 address with subnet",
			id:    "2602:80a:6000::/64",
			err:   "nor an IP address",
		},
		{
			limit: CertificatesPerDomain,
			desc:  "malformed domain",
			id:    "example:.com",
			err:   "name contains an invalid character",
		},
		{
			limit: CertificatesPerDomain,
			desc:  "empty domain",
			id:    "",
			err:   "Identifier value (name) is empty",
		},
		{
			limit: CertificatesPerFQDNSet,
			desc:  "valid fqdnSet containing a single domain",
			id:    "example.com",
		},
		{
			limit: CertificatesPerFQDNSet,
			desc:  "valid fqdnSet containing a single IPv4 address",
			id:    "64.112.117.1",
		},
		{
			limit: CertificatesPerFQDNSet,
			desc:  "valid fqdnSet containing a single IPv6 address",
			id:    "2602:80a:6000::1",
		},
		{
			limit: CertificatesPerFQDNSet,
			desc:  "valid fqdnSet containing multiple domains",
			id:    "example.com,example.org",
		},
		{
			limit: CertificatesPerFQDNSet,
			desc:  "valid fqdnSet containing multiple domains and IPs",
			id:    "2602:80a:6000::1,64.112.117.1,example.com,example.org",
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s/%s", tc.limit, tc.desc), func(t *testing.T) {
			t.Parallel()
			err := validateIdForName(tc.limit, tc.id)
			if tc.err != "" {
				test.AssertError(t, err, "should have failed")
				test.AssertContains(t, err.Error(), tc.err)
			} else {
				test.AssertNotError(t, err, "should have succeeded")
			}
		})
	}
}
