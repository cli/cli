package policy

import (
	"fmt"
	"net/netip"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/letsencrypt/boulder/core"
	berrors "github.com/letsencrypt/boulder/errors"
	"github.com/letsencrypt/boulder/features"
	"github.com/letsencrypt/boulder/identifier"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/test"
)

func paImpl(t *testing.T) *AuthorityImpl {
	enabledChallenges := map[core.AcmeChallenge]bool{
		core.ChallengeTypeHTTP01:    true,
		core.ChallengeTypeDNS01:     true,
		core.ChallengeTypeTLSALPN01: true,
	}

	enabledIdentifiers := map[identifier.IdentifierType]bool{
		identifier.TypeDNS: true,
		identifier.TypeIP:  true,
	}

	pa, err := New(enabledIdentifiers, enabledChallenges, blog.NewMock())
	if err != nil {
		t.Fatalf("Couldn't create policy implementation: %s", err)
	}
	return pa
}

func TestWellFormedIdentifiers(t *testing.T) {
	testCases := []struct {
		ident identifier.ACMEIdentifier
		err   error
	}{
		// Invalid identifier types
		{identifier.ACMEIdentifier{}, errUnsupportedIdent}, // Empty identifier type
		{identifier.ACMEIdentifier{Type: "fnord", Value: "uh-oh, Spaghetti-Os[tm]"}, errUnsupportedIdent},

		// Empty identifier values
		{identifier.NewDNS(``), errEmptyIdentifier},                 // Empty DNS identifier
		{identifier.ACMEIdentifier{Type: "ip"}, errEmptyIdentifier}, // Empty IP identifier

		// DNS follies

		{identifier.NewDNS(`zomb!.com`), errInvalidDNSCharacter}, // ASCII character out of range
		{identifier.NewDNS(`emailaddress@myseriously.present.com`), errInvalidDNSCharacter},
		{identifier.NewDNS(`user:pass@myseriously.present.com`), errInvalidDNSCharacter},
		{identifier.NewDNS(`zömbo.com`), errInvalidDNSCharacter},                              // non-ASCII character
		{identifier.NewDNS(`127.0.0.1`), errIPAddressInDNS},                                   // IPv4 address
		{identifier.NewDNS(`fe80::1:1`), errInvalidDNSCharacter},                              // IPv6 address
		{identifier.NewDNS(`[2001:db8:85a3:8d3:1319:8a2e:370:7348]`), errInvalidDNSCharacter}, // unexpected IPv6 variants
		{identifier.NewDNS(`[2001:db8:85a3:8d3:1319:8a2e:370:7348]:443`), errInvalidDNSCharacter},
		{identifier.NewDNS(`2001:db8::/32`), errInvalidDNSCharacter},
		{identifier.NewDNS(`a.b.c.d.e.f.g.h.i.j.k`), errTooManyLabels}, // Too many labels (>10)

		{identifier.NewDNS(`www.0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef012345.com`), errNameTooLong}, // Too long (254 characters)

		{identifier.NewDNS(`www.ef0123456789abcdef013456789abcdef012345.789abcdef012345679abcdef0123456789abcdef01234.6789abcdef0123456789abcdef0.23456789abcdef0123456789a.cdef0123456789abcdef0123456789ab.def0123456789abcdef0123456789.bcdef0123456789abcdef012345.com`), nil}, // OK, not too long (240 characters)

		{identifier.NewDNS(`www.abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz.com`), errLabelTooLong}, // Label too long (>63 characters)

		{identifier.NewDNS(`www.-ombo.com`), errInvalidDNSCharacter}, // Label starts with '-'
		{identifier.NewDNS(`www.zomb-.com`), errInvalidDNSCharacter}, // Label ends with '-'
		{identifier.NewDNS(`xn--.net`), errInvalidDNSCharacter},      // Label ends with '-'
		{identifier.NewDNS(`-0b.net`), errInvalidDNSCharacter},       // First label begins with '-'
		{identifier.NewDNS(`-0.net`), errInvalidDNSCharacter},        // First label begins with '-'
		{identifier.NewDNS(`-.net`), errInvalidDNSCharacter},         // First label is only '-'
		{identifier.NewDNS(`---.net`), errInvalidDNSCharacter},       // First label is only hyphens
		{identifier.NewDNS(`0`), errTooFewLabels},
		{identifier.NewDNS(`1`), errTooFewLabels},
		{identifier.NewDNS(`*`), errMalformedWildcard},
		{identifier.NewDNS(`**`), errTooManyWildcards},
		{identifier.NewDNS(`*.*`), errTooManyWildcards},
		{identifier.NewDNS(`zombo*com`), errMalformedWildcard},
		{identifier.NewDNS(`*.com`), errICANNTLDWildcard},
		{identifier.NewDNS(`..a`), errLabelTooShort},
		{identifier.NewDNS(`a..a`), errLabelTooShort},
		{identifier.NewDNS(`.a..a`), errLabelTooShort},
		{identifier.NewDNS(`..foo.com`), errLabelTooShort},
		{identifier.NewDNS(`.`), errNameEndsInDot},
		{identifier.NewDNS(`..`), errNameEndsInDot},
		{identifier.NewDNS(`a..`), errNameEndsInDot},
		{identifier.NewDNS(`.....`), errNameEndsInDot},
		{identifier.NewDNS(`.a.`), errNameEndsInDot},
		{identifier.NewDNS(`www.zombo.com.`), errNameEndsInDot},
		{identifier.NewDNS(`www.zombo_com.com`), errInvalidDNSCharacter},
		{identifier.NewDNS(`\uFEFF`), errInvalidDNSCharacter}, // Byte order mark
		{identifier.NewDNS(`\uFEFFwww.zombo.com`), errInvalidDNSCharacter},
		{identifier.NewDNS(`www.zom\u202Ebo.com`), errInvalidDNSCharacter}, // Right-to-Left Override
		{identifier.NewDNS(`\u202Ewww.zombo.com`), errInvalidDNSCharacter},
		{identifier.NewDNS(`www.zom\u200Fbo.com`), errInvalidDNSCharacter}, // Right-to-Left Mark
		{identifier.NewDNS(`\u200Fwww.zombo.com`), errInvalidDNSCharacter},
		// Underscores are technically disallowed in DNS. Some DNS
		// implementations accept them but we will be conservative.
		{identifier.NewDNS(`www.zom_bo.com`), errInvalidDNSCharacter},
		{identifier.NewDNS(`zombocom`), errTooFewLabels},
		{identifier.NewDNS(`localhost`), errTooFewLabels},
		{identifier.NewDNS(`mail`), errTooFewLabels},

		// disallow capitalized letters for #927
		{identifier.NewDNS(`CapitalizedLetters.com`), errInvalidDNSCharacter},

		{identifier.NewDNS(`example.acting`), errNonPublic},
		{identifier.NewDNS(`example.internal`), errNonPublic},
		// All-numeric final label not okay.
		{identifier.NewDNS(`www.zombo.163`), errNonPublic},
		{identifier.NewDNS(`xn--109-3veba6djs1bfxlfmx6c9g.xn--f1awi.xn--p1ai`), errMalformedIDN}, // Not in Unicode NFC
		{identifier.NewDNS(`bq--abwhky3f6fxq.jakacomo.com`), errInvalidRLDH},
		// Three hyphens starting at third second char of first label.
		{identifier.NewDNS(`bq---abwhky3f6fxq.jakacomo.com`), errInvalidRLDH},
		// Three hyphens starting at second char of first label.
		{identifier.NewDNS(`h---test.hk2yz.org`), errInvalidRLDH},
		{identifier.NewDNS(`co.uk`), errICANNTLD},
		{identifier.NewDNS(`foo.bd`), errICANNTLD},

		// IP oopsies

		{identifier.ACMEIdentifier{Type: "ip", Value: `zombo.com`}, errIPInvalid}, // That's DNS!

		// Unexpected IPv4 variants
		{identifier.ACMEIdentifier{Type: "ip", Value: `192.168.1.1.1`}, errIPInvalid},            // extra octet
		{identifier.ACMEIdentifier{Type: "ip", Value: `192.168.1.256`}, errIPInvalid},            // octet out of range
		{identifier.ACMEIdentifier{Type: "ip", Value: `192.168.1.a1`}, errIPInvalid},             // character out of range
		{identifier.ACMEIdentifier{Type: "ip", Value: `192.168.1.0/24`}, errIPInvalid},           // with CIDR
		{identifier.ACMEIdentifier{Type: "ip", Value: `192.168.1.1:443`}, errIPInvalid},          // with port
		{identifier.ACMEIdentifier{Type: "ip", Value: `0xc0a80101`}, errIPInvalid},               // as hex
		{identifier.ACMEIdentifier{Type: "ip", Value: `1.1.168.192.in-addr.arpa`}, errIPInvalid}, // reverse DNS

		// Unexpected IPv6 variants
		{identifier.ACMEIdentifier{Type: "ip", Value: `3fff:aaa:a:c0ff:ee:a:bad:deed:ffff`}, errIPInvalid},                                        // extra octet
		{identifier.ACMEIdentifier{Type: "ip", Value: `3fff:aaa:a:c0ff:ee:a:bad:mead`}, errIPInvalid},                                             // character out of range
		{identifier.ACMEIdentifier{Type: "ip", Value: `2001:db8::/32`}, errIPInvalid},                                                             // with CIDR
		{identifier.ACMEIdentifier{Type: "ip", Value: `[3fff:aaa:a:c0ff:ee:a:bad:deed]`}, errIPInvalid},                                           // in brackets
		{identifier.ACMEIdentifier{Type: "ip", Value: `[3fff:aaa:a:c0ff:ee:a:bad:deed]:443`}, errIPInvalid},                                       // in brackets, with port
		{identifier.ACMEIdentifier{Type: "ip", Value: `0x3fff0aaa000ac0ff00ee000a0baddeed`}, errIPInvalid},                                        // as hex
		{identifier.ACMEIdentifier{Type: "ip", Value: `d.e.e.d.d.a.b.0.a.0.0.0.e.e.0.0.f.f.0.c.a.0.0.0.a.a.a.0.f.f.f.3.ip6.arpa`}, errIPInvalid},  // reverse DNS
		{identifier.ACMEIdentifier{Type: "ip", Value: `3fff:0aaa:a:c0ff:ee:a:bad:deed`}, errIPInvalid},                                            // leading 0 in 2nd octet (RFC 5952, Sec. 4.1)
		{identifier.ACMEIdentifier{Type: "ip", Value: `3fff:aaa:0:0:0:a:bad:deed`}, errIPInvalid},                                                 // lone 0s in 3rd-5th octets, :: not used (RFC 5952, Sec. 4.2.1)
		{identifier.ACMEIdentifier{Type: "ip", Value: `3fff:aaa::c0ff:ee:a:bad:deed`}, errIPInvalid},                                              // :: used for just one empty octet (RFC 5952, Sec. 4.2.2)
		{identifier.ACMEIdentifier{Type: "ip", Value: `3fff:aaa::ee:0:0:0`}, errIPInvalid},                                                        // :: used for the shorter of two possible collapses (RFC 5952, Sec. 4.2.3)
		{identifier.ACMEIdentifier{Type: "ip", Value: `fe80:0:0:0:a::`}, errIPInvalid},                                                            // :: used for the last of two possible equal-length collapses (RFC 5952, Sec. 4.2.3)
		{identifier.ACMEIdentifier{Type: "ip", Value: `3fff:aaa:a:C0FF:EE:a:bad:deed`}, errIPInvalid},                                             // alpha characters capitalized (RFC 5952, Sec. 4.3)
		{identifier.ACMEIdentifier{Type: "ip", Value: `::ffff:192.168.1.1`}, berrors.MalformedError("IP address is in a reserved address block")}, // IPv6-encapsulated IPv4

		// IANA special-purpose address blocks
		{identifier.NewIP(netip.MustParseAddr("192.0.2.129")), berrors.MalformedError("IP address is in a reserved address block")},                        // Documentation (TEST-NET-1)
		{identifier.NewIP(netip.MustParseAddr("2001:db8:eee:eeee:eeee:eeee:d01:f1")), berrors.MalformedError("IP address is in a reserved address block")}, // Documentation
	}

	// Test syntax errors
	for _, tc := range testCases {
		err := WellFormedIdentifiers(identifier.ACMEIdentifiers{tc.ident})
		if tc.err == nil {
			test.AssertNil(t, err, fmt.Sprintf("Unexpected error for %q identifier %q, got %s", tc.ident.Type, tc.ident.Value, err))
		} else {
			test.AssertError(t, err, fmt.Sprintf("Expected error for %q identifier %q, but got none", tc.ident.Type, tc.ident.Value))
			var berr *berrors.BoulderError
			test.AssertErrorWraps(t, err, &berr)
			test.AssertContains(t, berr.Error(), tc.err.Error())
		}
	}
}

func TestWillingToIssue(t *testing.T) {
	shouldBeBlocked := identifier.ACMEIdentifiers{
		identifier.NewDNS(`highvalue.website1.org`),
		identifier.NewDNS(`website2.co.uk`),
		identifier.NewDNS(`www.website3.com`),
		identifier.NewDNS(`lots.of.labels.website4.com`),
		identifier.NewDNS(`banned.in.dc.com`),
		identifier.NewDNS(`bad.brains.banned.in.dc.com`),
	}
	blocklistContents := []string{
		`website2.com`,
		`website2.org`,
		`website2.co.uk`,
		`website3.com`,
		`website4.com`,
	}
	exactBlocklistContents := []string{
		`www.website1.org`,
		`highvalue.website1.org`,
		`dl.website1.org`,
	}
	adminBlockedContents := []string{
		`banned.in.dc.com`,
	}

	shouldBeAccepted := identifier.ACMEIdentifiers{
		identifier.NewDNS(`lowvalue.website1.org`),
		identifier.NewDNS(`website4.sucks`),
		identifier.NewDNS(`www.unrelated.com`),
		identifier.NewDNS(`unrelated.com`),
		identifier.NewDNS(`www.8675309.com`),
		identifier.NewDNS(`8675309.com`),
		identifier.NewDNS(`web5ite2.com`),
		identifier.NewDNS(`www.web-site2.com`),
		identifier.NewIP(netip.MustParseAddr(`9.9.9.9`)),
		identifier.NewIP(netip.MustParseAddr(`2620:fe::fe`)),
	}

	policy := blockedNamesPolicy{
		HighRiskBlockedNames: blocklistContents,
		ExactBlockedNames:    exactBlocklistContents,
		AdminBlockedNames:    adminBlockedContents,
	}

	yamlPolicyBytes, err := yaml.Marshal(policy)
	test.AssertNotError(t, err, "Couldn't YAML serialize blocklist")
	yamlPolicyFile, _ := os.CreateTemp("", "test-blocklist.*.yaml")
	defer os.Remove(yamlPolicyFile.Name())
	err = os.WriteFile(yamlPolicyFile.Name(), yamlPolicyBytes, 0640)
	test.AssertNotError(t, err, "Couldn't write YAML blocklist")

	pa := paImpl(t)

	err = pa.LoadHostnamePolicyFile(yamlPolicyFile.Name())
	test.AssertNotError(t, err, "Couldn't load rules")

	// Invalid encoding
	err = pa.WillingToIssue(identifier.ACMEIdentifiers{identifier.NewDNS("www.xn--m.com")})
	test.AssertError(t, err, "WillingToIssue didn't fail on a malformed IDN")
	// Invalid identifier type
	err = pa.WillingToIssue(identifier.ACMEIdentifiers{identifier.ACMEIdentifier{Type: "fnord", Value: "uh-oh, Spaghetti-Os[tm]"}})
	test.AssertError(t, err, "WillingToIssue didn't fail on an invalid identifier type")
	// Valid encoding
	err = pa.WillingToIssue(identifier.ACMEIdentifiers{identifier.NewDNS("www.xn--mnich-kva.com")})
	test.AssertNotError(t, err, "WillingToIssue failed on a properly formed IDN")
	// IDN TLD
	err = pa.WillingToIssue(identifier.ACMEIdentifiers{identifier.NewDNS("xn--example--3bhk5a.xn--p1ai")})
	test.AssertNotError(t, err, "WillingToIssue failed on a properly formed domain with IDN TLD")
	features.Reset()

	// Test expected blocked domains
	for _, ident := range shouldBeBlocked {
		err := pa.WillingToIssue(identifier.ACMEIdentifiers{ident})
		test.AssertError(t, err, "identifier was not correctly forbidden")
		var berr *berrors.BoulderError
		test.AssertErrorWraps(t, err, &berr)
		test.AssertContains(t, berr.Detail, errPolicyForbidden.Error())
	}

	// Test acceptance of good names
	for _, ident := range shouldBeAccepted {
		err := pa.WillingToIssue(identifier.ACMEIdentifiers{ident})
		test.AssertNotError(t, err, "identifier was incorrectly forbidden")
	}
}

func TestWillingToIssue_Wildcards(t *testing.T) {
	bannedDomains := []string{
		"zombo.gov.us",
	}
	exactBannedDomains := []string{
		"highvalue.letsdecrypt.org",
	}
	pa := paImpl(t)

	bannedBytes, err := yaml.Marshal(blockedNamesPolicy{
		HighRiskBlockedNames: bannedDomains,
		ExactBlockedNames:    exactBannedDomains,
	})
	test.AssertNotError(t, err, "Couldn't serialize banned list")
	f, _ := os.CreateTemp("", "test-wildcard-banlist.*.yaml")
	defer os.Remove(f.Name())
	err = os.WriteFile(f.Name(), bannedBytes, 0640)
	test.AssertNotError(t, err, "Couldn't write serialized banned list to file")
	err = pa.LoadHostnamePolicyFile(f.Name())
	test.AssertNotError(t, err, "Couldn't load policy contents from file")

	testCases := []struct {
		Name        string
		Domain      string
		ExpectedErr error
	}{
		{
			Name:        "Too many wildcards",
			Domain:      "ok.*.whatever.*.example.com",
			ExpectedErr: errTooManyWildcards,
		},
		{
			Name:        "Misplaced wildcard",
			Domain:      "ok.*.whatever.example.com",
			ExpectedErr: errMalformedWildcard,
		},
		{
			Name:        "Missing ICANN TLD",
			Domain:      "*.ok.madeup",
			ExpectedErr: errNonPublic,
		},
		{
			Name:        "Wildcard for ICANN TLD",
			Domain:      "*.com",
			ExpectedErr: errICANNTLDWildcard,
		},
		{
			Name:        "Forbidden base domain",
			Domain:      "*.zombo.gov.us",
			ExpectedErr: errPolicyForbidden,
		},
		// We should not allow getting a wildcard for that would cover an exact
		// blocklist domain
		{
			Name:        "Wildcard for ExactBlocklist base domain",
			Domain:      "*.letsdecrypt.org",
			ExpectedErr: errPolicyForbidden,
		},
		// We should allow a wildcard for a domain that doesn't match the exact
		// blocklist domain
		{
			Name:        "Wildcard for non-matching subdomain of ExactBlocklist domain",
			Domain:      "*.lowvalue.letsdecrypt.org",
			ExpectedErr: nil,
		},
		// We should allow getting a wildcard for an exact blocklist domain since it
		// only covers subdomains, not the exact name.
		{
			Name:        "Wildcard for ExactBlocklist domain",
			Domain:      "*.highvalue.letsdecrypt.org",
			ExpectedErr: nil,
		},
		{
			Name:        "Valid wildcard domain",
			Domain:      "*.everything.is.possible.at.zombo.com",
			ExpectedErr: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			err := pa.WillingToIssue(identifier.ACMEIdentifiers{identifier.NewDNS(tc.Domain)})
			if tc.ExpectedErr == nil {
				test.AssertNil(t, err, fmt.Sprintf("Unexpected error for domain %q, got %s", tc.Domain, err))
			} else {
				test.AssertError(t, err, fmt.Sprintf("Expected error for domain %q, but got none", tc.Domain))
				var berr *berrors.BoulderError
				test.AssertErrorWraps(t, err, &berr)
				test.AssertContains(t, berr.Error(), tc.ExpectedErr.Error())
			}
		})
	}
}

// TestWillingToIssue_SubErrors tests that more than one rejected identifier
// results in an error with suberrors.
func TestWillingToIssue_SubErrors(t *testing.T) {
	banned := []string{
		"letsdecrypt.org",
		"example.com",
	}
	pa := paImpl(t)

	bannedBytes, err := yaml.Marshal(blockedNamesPolicy{
		HighRiskBlockedNames: banned,
		ExactBlockedNames:    banned,
	})
	test.AssertNotError(t, err, "Couldn't serialize banned list")
	f, _ := os.CreateTemp("", "test-wildcard-banlist.*.yaml")
	defer os.Remove(f.Name())
	err = os.WriteFile(f.Name(), bannedBytes, 0640)
	test.AssertNotError(t, err, "Couldn't write serialized banned list to file")
	err = pa.LoadHostnamePolicyFile(f.Name())
	test.AssertNotError(t, err, "Couldn't load policy contents from file")

	// Test multiple malformed domains and one banned domain; only the malformed ones will generate errors
	err = pa.WillingToIssue(identifier.ACMEIdentifiers{
		identifier.NewDNS("perfectly-fine.com"),      // fine
		identifier.NewDNS("letsdecrypt_org"),         // malformed
		identifier.NewDNS("example.comm"),            // malformed
		identifier.NewDNS("letsdecrypt.org"),         // banned
		identifier.NewDNS("also-perfectly-fine.com"), // fine
	})
	test.AssertDeepEquals(t, err,
		&berrors.BoulderError{
			Type:   berrors.RejectedIdentifier,
			Detail: "Cannot issue for \"letsdecrypt_org\": Domain name contains an invalid character (and 1 more problems. Refer to sub-problems for more information.)",
			SubErrors: []berrors.SubBoulderError{
				{
					BoulderError: &berrors.BoulderError{
						Type:   berrors.Malformed,
						Detail: "Domain name contains an invalid character",
					},
					Identifier: identifier.NewDNS("letsdecrypt_org"),
				},
				{
					BoulderError: &berrors.BoulderError{
						Type:   berrors.Malformed,
						Detail: "Domain name does not end with a valid public suffix (TLD)",
					},
					Identifier: identifier.NewDNS("example.comm"),
				},
			},
		})

	// Test multiple banned domains.
	err = pa.WillingToIssue(identifier.ACMEIdentifiers{
		identifier.NewDNS("perfectly-fine.com"),      // fine
		identifier.NewDNS("letsdecrypt.org"),         // banned
		identifier.NewDNS("example.com"),             // banned
		identifier.NewDNS("also-perfectly-fine.com"), // fine
	})
	test.AssertError(t, err, "Expected err from WillingToIssueWildcards")

	test.AssertDeepEquals(t, err,
		&berrors.BoulderError{
			Type:   berrors.RejectedIdentifier,
			Detail: "Cannot issue for \"letsdecrypt.org\": The ACME server refuses to issue a certificate for this domain name, because it is forbidden by policy (and 1 more problems. Refer to sub-problems for more information.)",
			SubErrors: []berrors.SubBoulderError{
				{
					BoulderError: &berrors.BoulderError{
						Type:   berrors.RejectedIdentifier,
						Detail: "The ACME server refuses to issue a certificate for this domain name, because it is forbidden by policy",
					},
					Identifier: identifier.NewDNS("letsdecrypt.org"),
				},
				{
					BoulderError: &berrors.BoulderError{
						Type:   berrors.RejectedIdentifier,
						Detail: "The ACME server refuses to issue a certificate for this domain name, because it is forbidden by policy",
					},
					Identifier: identifier.NewDNS("example.com"),
				},
			},
		})

	// Test willing to issue with only *one* bad identifier.
	err = pa.WillingToIssue(identifier.ACMEIdentifiers{identifier.NewDNS("letsdecrypt.org")})
	test.AssertDeepEquals(t, err,
		&berrors.BoulderError{
			Type:   berrors.RejectedIdentifier,
			Detail: "Cannot issue for \"letsdecrypt.org\": The ACME server refuses to issue a certificate for this domain name, because it is forbidden by policy",
		})
}

func TestChallengeTypesFor(t *testing.T) {
	t.Parallel()
	pa := paImpl(t)

	testCases := []struct {
		name       string
		ident      identifier.ACMEIdentifier
		wantChalls []core.AcmeChallenge
		wantErr    string
	}{
		{
			name:  "dns",
			ident: identifier.NewDNS("example.com"),
			wantChalls: []core.AcmeChallenge{
				core.ChallengeTypeHTTP01, core.ChallengeTypeDNS01, core.ChallengeTypeTLSALPN01,
			},
		},
		{
			name:  "dns wildcard",
			ident: identifier.NewDNS("*.example.com"),
			wantChalls: []core.AcmeChallenge{
				core.ChallengeTypeDNS01,
			},
		},
		{
			name:  "ip",
			ident: identifier.NewIP(netip.MustParseAddr("1.2.3.4")),
			wantChalls: []core.AcmeChallenge{
				core.ChallengeTypeHTTP01, core.ChallengeTypeTLSALPN01,
			},
		},
		{
			name:    "invalid",
			ident:   identifier.ACMEIdentifier{Type: "fnord", Value: "uh-oh, Spaghetti-Os[tm]"},
			wantErr: "unrecognized identifier type",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			challs, err := pa.ChallengeTypesFor(tc.ident)

			if len(tc.wantChalls) != 0 {
				test.AssertNotError(t, err, "should have succeeded")
				test.AssertDeepEquals(t, challs, tc.wantChalls)
			}

			if tc.wantErr != "" {
				test.AssertError(t, err, "should have errored")
				test.AssertContains(t, err.Error(), tc.wantErr)
			}
		})
	}
}

// TestMalformedExactBlocklist tests that loading a YAML policy file with an
// invalid exact blocklist entry will fail as expected.
func TestMalformedExactBlocklist(t *testing.T) {
	pa := paImpl(t)

	exactBannedDomains := []string{
		// Only one label - not valid
		"com",
	}
	bannedDomains := []string{
		"placeholder.domain.not.important.for.this.test.com",
	}

	// Create YAML for the exactBannedDomains
	bannedBytes, err := yaml.Marshal(blockedNamesPolicy{
		HighRiskBlockedNames: bannedDomains,
		ExactBlockedNames:    exactBannedDomains,
	})
	test.AssertNotError(t, err, "Couldn't serialize banned list")

	// Create a temp file for the YAML contents
	f, _ := os.CreateTemp("", "test-invalid-exactblocklist.*.yaml")
	defer os.Remove(f.Name())
	// Write the YAML to the temp file
	err = os.WriteFile(f.Name(), bannedBytes, 0640)
	test.AssertNotError(t, err, "Couldn't write serialized banned list to file")

	// Try to use the YAML tempfile as the hostname policy. It should produce an
	// error since the exact blocklist contents are malformed.
	err = pa.LoadHostnamePolicyFile(f.Name())
	test.AssertError(t, err, "Loaded invalid exact blocklist content without error")
	test.AssertEquals(t, err.Error(), "Malformed ExactBlockedNames entry, only one label: \"com\"")
}

func TestValidEmailError(t *testing.T) {
	err := ValidEmail("(๑•́ ω •̀๑)")
	test.AssertEquals(t, err.Error(), "unable to parse email address")

	err = ValidEmail("john.smith@gmail.com #replace with real email")
	test.AssertEquals(t, err.Error(), "unable to parse email address")

	err = ValidEmail("example@example.com")
	test.AssertEquals(t, err.Error(), "contact email has forbidden domain \"example.com\"")

	err = ValidEmail("example@-foobar.com")
	test.AssertEquals(t, err.Error(), "contact email has invalid domain: Domain name contains an invalid character")
}

func TestCheckAuthzChallenges(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		authz   core.Authorization
		enabled map[core.AcmeChallenge]bool
		wantErr string
	}{
		{
			name: "unrecognized identifier",
			authz: core.Authorization{
				Identifier: identifier.ACMEIdentifier{Type: "oops", Value: "example.com"},
				Challenges: []core.Challenge{{Type: core.ChallengeTypeDNS01, Status: core.StatusValid}},
			},
			wantErr: "unrecognized identifier type",
		},
		{
			name: "no challenges",
			authz: core.Authorization{
				Identifier: identifier.NewDNS("example.com"),
				Challenges: []core.Challenge{},
			},
			wantErr: "has no challenges",
		},
		{
			name: "no valid challenges",
			authz: core.Authorization{
				Identifier: identifier.NewDNS("example.com"),
				Challenges: []core.Challenge{{Type: core.ChallengeTypeDNS01, Status: core.StatusPending}},
			},
			wantErr: "not solved by any challenge",
		},
		{
			name: "solved by disabled challenge",
			authz: core.Authorization{
				Identifier: identifier.NewDNS("example.com"),
				Challenges: []core.Challenge{{Type: core.ChallengeTypeDNS01, Status: core.StatusValid}},
			},
			enabled: map[core.AcmeChallenge]bool{core.ChallengeTypeHTTP01: true},
			wantErr: "disabled challenge type",
		},
		{
			name: "solved by wrong kind of challenge",
			authz: core.Authorization{
				Identifier: identifier.NewDNS("*.example.com"),
				Challenges: []core.Challenge{{Type: core.ChallengeTypeHTTP01, Status: core.StatusValid}},
			},
			wantErr: "inapplicable challenge type",
		},
		{
			name: "valid authz",
			authz: core.Authorization{
				Identifier: identifier.NewDNS("example.com"),
				Challenges: []core.Challenge{{Type: core.ChallengeTypeTLSALPN01, Status: core.StatusValid}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pa := paImpl(t)

			if tc.enabled != nil {
				pa.enabledChallenges = tc.enabled
			}

			err := pa.CheckAuthzChallenges(&tc.authz)

			if tc.wantErr == "" {
				test.AssertNotError(t, err, "should have succeeded")
			} else {
				test.AssertError(t, err, "should have errored")
				test.AssertContains(t, err.Error(), tc.wantErr)
			}
		})
	}
}

func TestWillingToIssue_IdentifierType(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		ident   identifier.ACMEIdentifier
		enabled map[identifier.IdentifierType]bool
		wantErr string
	}{
		{
			name:    "DNS identifier, none enabled",
			ident:   identifier.NewDNS("example.com"),
			enabled: nil,
			wantErr: "The ACME server has disabled this identifier type",
		},
		{
			name:    "DNS identifier, DNS enabled",
			ident:   identifier.NewDNS("example.com"),
			enabled: map[identifier.IdentifierType]bool{identifier.TypeDNS: true},
			wantErr: "",
		},
		{
			name:    "DNS identifier, DNS & IP enabled",
			ident:   identifier.NewDNS("example.com"),
			enabled: map[identifier.IdentifierType]bool{identifier.TypeDNS: true, identifier.TypeIP: true},
			wantErr: "",
		},
		{
			name:    "DNS identifier, IP enabled",
			ident:   identifier.NewDNS("example.com"),
			enabled: map[identifier.IdentifierType]bool{identifier.TypeIP: true},
			wantErr: "The ACME server has disabled this identifier type",
		},
		{
			name:    "IP identifier, none enabled",
			ident:   identifier.NewIP(netip.MustParseAddr("9.9.9.9")),
			enabled: nil,
			wantErr: "The ACME server has disabled this identifier type",
		},
		{
			name:    "IP identifier, DNS enabled",
			ident:   identifier.NewIP(netip.MustParseAddr("9.9.9.9")),
			enabled: map[identifier.IdentifierType]bool{identifier.TypeDNS: true},
			wantErr: "The ACME server has disabled this identifier type",
		},
		{
			name:    "IP identifier, DNS & IP enabled",
			ident:   identifier.NewIP(netip.MustParseAddr("9.9.9.9")),
			enabled: map[identifier.IdentifierType]bool{identifier.TypeDNS: true, identifier.TypeIP: true},
			wantErr: "",
		},
		{
			name:    "IP identifier, IP enabled",
			ident:   identifier.NewIP(netip.MustParseAddr("9.9.9.9")),
			enabled: map[identifier.IdentifierType]bool{identifier.TypeIP: true},
			wantErr: "",
		},
		{
			name:    "invalid identifier type",
			ident:   identifier.ACMEIdentifier{Type: "drywall", Value: "oh yeah!"},
			enabled: map[identifier.IdentifierType]bool{"drywall": true},
			wantErr: "Invalid identifier type",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			policy := blockedNamesPolicy{
				HighRiskBlockedNames: []string{"zombo.gov.us"},
				ExactBlockedNames:    []string{`highvalue.website1.org`},
				AdminBlockedNames:    []string{`banned.in.dc.com`},
			}

			yamlPolicyBytes, err := yaml.Marshal(policy)
			test.AssertNotError(t, err, "Couldn't YAML serialize blocklist")
			yamlPolicyFile, _ := os.CreateTemp("", "test-blocklist.*.yaml")
			defer os.Remove(yamlPolicyFile.Name())
			err = os.WriteFile(yamlPolicyFile.Name(), yamlPolicyBytes, 0640)
			test.AssertNotError(t, err, "Couldn't write YAML blocklist")

			pa := paImpl(t)

			err = pa.LoadHostnamePolicyFile(yamlPolicyFile.Name())
			test.AssertNotError(t, err, "Couldn't load rules")

			pa.enabledIdentifiers = tc.enabled

			err = pa.WillingToIssue(identifier.ACMEIdentifiers{tc.ident})

			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("should have succeeded, but got error: %s", err.Error())
				}
			} else {
				if err == nil {
					t.Errorf("should have failed")
				} else if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("wrong error; wanted '%s', but got '%s'", tc.wantErr, err.Error())
				}
			}
		})
	}
}
