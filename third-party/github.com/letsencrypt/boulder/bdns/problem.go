package bdns

import (
	"context"
	"fmt"
	"net"

	"github.com/miekg/dns"
)

// Error wraps a DNS error with various relevant information
type Error struct {
	recordType uint16
	hostname   string
	// Exactly one of rCode or underlying should be set.
	underlying error
	rCode      int

	// Optional: If the resolver returned extended error information, it will be stored here.
	// https://www.rfc-editor.org/rfc/rfc8914
	extended *dns.EDNS0_EDE
}

// extendedDNSError returns non-nil if the input message contained an OPT RR
// with an EDE option. https://www.rfc-editor.org/rfc/rfc8914.
func extendedDNSError(msg *dns.Msg) *dns.EDNS0_EDE {
	opt := msg.IsEdns0()
	if opt != nil {
		for _, opt := range opt.Option {
			ede, ok := opt.(*dns.EDNS0_EDE)
			if !ok {
				continue
			}
			return ede
		}
	}
	return nil
}

// wrapErr returns a non-nil error if err is non-nil or if resp.Rcode is not dns.RcodeSuccess.
// The error includes appropriate details about the DNS query that failed.
func wrapErr(queryType uint16, hostname string, resp *dns.Msg, err error) error {
	if err != nil {
		return Error{
			recordType: queryType,
			hostname:   hostname,
			underlying: err,
			extended:   nil,
		}
	}
	if resp.Rcode != dns.RcodeSuccess {
		return Error{
			recordType: queryType,
			hostname:   hostname,
			rCode:      resp.Rcode,
			underlying: nil,
			extended:   extendedDNSError(resp),
		}
	}
	return nil
}

// A copy of miekg/dns's mapping of error codes to strings. We tweak it slightly so all DNSSEC-related
// errors say "DNSSEC" at the beginning.
// https://pkg.go.dev/github.com/miekg/dns#ExtendedErrorCodeToString
// Also note that not all of these codes can currently be emitted by Unbound. See Unbound's
// announcement post for EDE: https://blog.nlnetlabs.nl/extended-dns-error-support-for-unbound/
var extendedErrorCodeToString = map[uint16]string{
	dns.ExtendedErrorCodeOther:                      "Other",
	dns.ExtendedErrorCodeUnsupportedDNSKEYAlgorithm: "DNSSEC: Unsupported DNSKEY Algorithm",
	dns.ExtendedErrorCodeUnsupportedDSDigestType:    "DNSSEC: Unsupported DS Digest Type",
	dns.ExtendedErrorCodeStaleAnswer:                "Stale Answer",
	dns.ExtendedErrorCodeForgedAnswer:               "Forged Answer",
	dns.ExtendedErrorCodeDNSSECIndeterminate:        "DNSSEC: Indeterminate",
	dns.ExtendedErrorCodeDNSBogus:                   "DNSSEC: Bogus",
	dns.ExtendedErrorCodeSignatureExpired:           "DNSSEC: Signature Expired",
	dns.ExtendedErrorCodeSignatureNotYetValid:       "DNSSEC: Signature Not Yet Valid",
	dns.ExtendedErrorCodeDNSKEYMissing:              "DNSSEC: DNSKEY Missing",
	dns.ExtendedErrorCodeRRSIGsMissing:              "DNSSEC: RRSIGs Missing",
	dns.ExtendedErrorCodeNoZoneKeyBitSet:            "DNSSEC: No Zone Key Bit Set",
	dns.ExtendedErrorCodeNSECMissing:                "DNSSEC: NSEC Missing",
	dns.ExtendedErrorCodeCachedError:                "Cached Error",
	dns.ExtendedErrorCodeNotReady:                   "Not Ready",
	dns.ExtendedErrorCodeBlocked:                    "Blocked",
	dns.ExtendedErrorCodeCensored:                   "Censored",
	dns.ExtendedErrorCodeFiltered:                   "Filtered",
	dns.ExtendedErrorCodeProhibited:                 "Prohibited",
	dns.ExtendedErrorCodeStaleNXDOMAINAnswer:        "Stale NXDOMAIN Answer",
	dns.ExtendedErrorCodeNotAuthoritative:           "Not Authoritative",
	dns.ExtendedErrorCodeNotSupported:               "Not Supported",
	dns.ExtendedErrorCodeNoReachableAuthority:       "No Reachable Authority",
	dns.ExtendedErrorCodeNetworkError:               "Network Error between Resolver and Authority",
	dns.ExtendedErrorCodeInvalidData:                "Invalid Data",
}

func (d Error) Error() string {
	var detail, additional string
	if d.underlying != nil {
		if netErr, ok := d.underlying.(*net.OpError); ok {
			if netErr.Timeout() {
				detail = detailDNSTimeout
			} else {
				detail = detailDNSNetFailure
			}
			// Note: we check d.underlying here even though `Timeout()` does this because the call to `netErr.Timeout()` above only
			// happens for `*net.OpError` underlying types!
		} else if d.underlying == context.DeadlineExceeded {
			detail = detailDNSTimeout
		} else if d.underlying == context.Canceled {
			detail = detailCanceled
		} else {
			detail = detailServerFailure
		}
	} else if d.rCode != dns.RcodeSuccess {
		detail = dns.RcodeToString[d.rCode]
		if explanation, ok := rcodeExplanations[d.rCode]; ok {
			additional = " - " + explanation
		}
	} else {
		detail = detailServerFailure
	}

	if d.extended == nil {
		return fmt.Sprintf("DNS problem: %s looking up %s for %s%s", detail,
			dns.TypeToString[d.recordType], d.hostname, additional)
	}

	summary := extendedErrorCodeToString[d.extended.InfoCode]
	if summary == "" {
		summary = fmt.Sprintf("Unknown Extended DNS Error code %d", d.extended.InfoCode)
	}
	result := fmt.Sprintf("DNS problem: looking up %s for %s: %s",
		dns.TypeToString[d.recordType], d.hostname, summary)
	if d.extended.ExtraText != "" {
		result = result + ": " + d.extended.ExtraText
	}
	return result
}

const detailDNSTimeout = "query timed out"
const detailCanceled = "query timed out (and was canceled)"
const detailDNSNetFailure = "networking error"
const detailServerFailure = "server failure at resolver"

// rcodeExplanations provide additional friendly explanatory text to be included in DNS
// error messages, for select inscrutable RCODEs.
var rcodeExplanations = map[int]string{
	dns.RcodeNameError:     "check that a DNS record exists for this domain",
	dns.RcodeServerFailure: "the domain's nameservers may be malfunctioning",
}
