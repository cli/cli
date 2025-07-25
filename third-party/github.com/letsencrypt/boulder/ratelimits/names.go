package ratelimits

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/letsencrypt/boulder/iana"
	"github.com/letsencrypt/boulder/policy"
)

// Name is an enumeration of all rate limit names. It is used to intern rate
// limit names as strings and to provide a type-safe way to refer to rate
// limits.
//
// IMPORTANT: If you add or remove a limit Name, you MUST update:
//   - the string representation of the Name in nameToString,
//   - the validators for that name in validateIdForName(),
//   - the transaction constructors for that name in bucket.go, and
//   - the Subscriber facing error message in ErrForDecision().
type Name int

const (
	// Unknown is the zero value of Name and is used to indicate an unknown
	// limit name.
	Unknown Name = iota

	// NewRegistrationsPerIPAddress uses bucket key 'enum:ipAddress'.
	NewRegistrationsPerIPAddress

	// NewRegistrationsPerIPv6Range uses bucket key 'enum:ipv6rangeCIDR'. The
	// address range must be a /48. RFC 3177, which was published in 2001,
	// advised operators to allocate a /48 block of IPv6 addresses for most end
	// sites. RFC 6177, which was published in 2011 and obsoletes RFC 3177,
	// advises allocating a smaller /56 block. We've chosen to use the larger
	// /48 block for our IPv6 rate limiting. See:
	//   1. https://tools.ietf.org/html/rfc3177#section-3
	//   2. https://datatracker.ietf.org/doc/html/rfc6177#section-2
	NewRegistrationsPerIPv6Range

	// NewOrdersPerAccount uses bucket key 'enum:regId'.
	NewOrdersPerAccount

	// FailedAuthorizationsPerDomainPerAccount uses two different bucket keys
	// depending on the context:
	//  - When referenced in an overrides file: uses bucket key 'enum:regId',
	//    where regId is the ACME registration Id of the account.
	//  - When referenced in a transaction: uses bucket key
	//    'enum:regId:identValue', where regId is the ACME registration Id of
	//    the account and identValue is the value of an identifier in the
	//    certificate.
	FailedAuthorizationsPerDomainPerAccount

	// CertificatesPerDomain uses bucket key 'enum:domainOrCIDR', where
	// domainOrCIDR is a domain name or IP address in the certificate. It uses
	// two different IP address formats depending on the context:
	//  - When referenced in an overrides file: uses a single IP address.
	//  - When referenced in a transaction: uses an IP address prefix in CIDR
	//    notation. IPv4 prefixes must be /32, and IPv6 prefixes must be /64.
	// In both cases, IPv6 addresses must be the lowest address in their /64;
	// i.e. their last 64 bits must be zero.
	CertificatesPerDomain

	// CertificatesPerDomainPerAccount is only used for per-account overrides to
	// the CertificatesPerDomain rate limit. If this limit is referenced in the
	// default limits file, it will be ignored. It uses two different bucket
	// keys depending on the context:
	//  - When referenced in an overrides file: uses bucket key 'enum:regId',
	//    where regId is the ACME registration Id of the account.
	//  - When referenced in a transaction: uses bucket key
	//   'enum:regId:domainOrCIDR', where regId is the ACME registration Id of
	//    the account and domainOrCIDR is either a domain name in the
	//    certificate or an IP prefix in CIDR notation.
	//     - IP address formats vary by context, as for CertificatesPerDomain.
	//
	// When overrides to the CertificatesPerDomainPerAccount are configured for a
	// subscriber, the cost:
	//   - MUST be consumed from each CertificatesPerDomainPerAccount bucket and
	//   - SHOULD be consumed from each CertificatesPerDomain bucket, if possible.
	CertificatesPerDomainPerAccount

	// CertificatesPerFQDNSet uses bucket key 'enum:fqdnSet', where fqdnSet is a
	// hashed set of unique identifier values in the certificate.
	//
	// Note: When this is referenced in an overrides file, the fqdnSet MUST be
	// passed as a comma-separated list of identifier values.
	CertificatesPerFQDNSet

	// FailedAuthorizationsForPausingPerDomainPerAccount is similar to
	// FailedAuthorizationsPerDomainPerAccount in that it uses two different
	// bucket keys depending on the context:
	//  - When referenced in an overrides file: uses bucket key 'enum:regId',
	//    where regId is the ACME registration Id of the account.
	//  - When referenced in a transaction: uses bucket key
	//    'enum:regId:identValue', where regId is the ACME registration Id of
	//    the account and identValue is the value of an identifier in the
	//    certificate.
	FailedAuthorizationsForPausingPerDomainPerAccount
)

// nameToString is a map of Name values to string names.
var nameToString = map[Name]string{
	Unknown:                                           "Unknown",
	NewRegistrationsPerIPAddress:                      "NewRegistrationsPerIPAddress",
	NewRegistrationsPerIPv6Range:                      "NewRegistrationsPerIPv6Range",
	NewOrdersPerAccount:                               "NewOrdersPerAccount",
	FailedAuthorizationsPerDomainPerAccount:           "FailedAuthorizationsPerDomainPerAccount",
	CertificatesPerDomain:                             "CertificatesPerDomain",
	CertificatesPerDomainPerAccount:                   "CertificatesPerDomainPerAccount",
	CertificatesPerFQDNSet:                            "CertificatesPerFQDNSet",
	FailedAuthorizationsForPausingPerDomainPerAccount: "FailedAuthorizationsForPausingPerDomainPerAccount",
}

// isValid returns true if the Name is a valid rate limit name.
func (n Name) isValid() bool {
	return n > Unknown && n < Name(len(nameToString))
}

// String returns the string representation of the Name. It allows Name to
// satisfy the fmt.Stringer interface.
func (n Name) String() string {
	if !n.isValid() {
		return nameToString[Unknown]
	}
	return nameToString[n]
}

// EnumString returns the string representation of the Name enumeration.
func (n Name) EnumString() string {
	if !n.isValid() {
		return nameToString[Unknown]
	}
	return strconv.Itoa(int(n))
}

// validIPAddress validates that the provided string is a valid IP address.
func validIPAddress(id string) error {
	ip, err := netip.ParseAddr(id)
	if err != nil {
		return fmt.Errorf("invalid IP address, %q must be an IP address", id)
	}
	canon := ip.String()
	if canon != id {
		return fmt.Errorf(
			"invalid IP address, %q must be in canonical form (%q)", id, canon)
	}
	return iana.IsReservedAddr(ip)
}

// validIPv6RangeCIDR validates that the provided string is formatted as an IPv6
// prefix in CIDR notation, with a /48 mask.
func validIPv6RangeCIDR(id string) error {
	prefix, err := netip.ParsePrefix(id)
	if err != nil {
		return fmt.Errorf(
			"invalid CIDR, %q must be an IPv6 CIDR range", id)
	}
	if prefix.Bits() != 48 {
		// This also catches the case where the range is an IPv4 CIDR, since an
		// IPv4 CIDR can't have a /48 subnet mask - the maximum is /32.
		return fmt.Errorf(
			"invalid CIDR, %q must be /48", id)
	}
	canon := prefix.Masked().String()
	if canon != id {
		return fmt.Errorf(
			"invalid CIDR, %q must be in canonical form (%q)", id, canon)
	}
	return iana.IsReservedPrefix(prefix)
}

// validateRegId validates that the provided string is a valid ACME regId.
func validateRegId(id string) error {
	_, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid regId, %q must be an ACME registration Id", id)
	}
	return nil
}

// validateRegIdIdentValue validates that the provided string is formatted
// 'regId:identValue', where regId is an ACME registration Id and identValue is
// a valid identifier value.
func validateRegIdIdentValue(id string) error {
	regIdIdentValue := strings.Split(id, ":")
	if len(regIdIdentValue) != 2 {
		return fmt.Errorf(
			"invalid regId:identValue, %q must be formatted 'regId:identValue'", id)
	}
	err := validateRegId(regIdIdentValue[0])
	if err != nil {
		return fmt.Errorf(
			"invalid regId, %q must be formatted 'regId:identValue'", id)
	}
	domainErr := policy.ValidDomain(regIdIdentValue[1])
	if domainErr != nil {
		ipErr := policy.ValidIP(regIdIdentValue[1])
		if ipErr != nil {
			return fmt.Errorf("invalid identValue, %q must be formatted 'regId:identValue': %w as domain, %w as IP", id, domainErr, ipErr)
		}
	}
	return nil
}

// validateDomainOrCIDR validates that the provided string is either a domain
// name or an IP address. IPv6 addresses must be the lowest address in their
// /64, i.e. their last 64 bits must be zero.
func validateDomainOrCIDR(id string) error {
	domainErr := policy.ValidDomain(id)
	if domainErr == nil {
		// This is a valid domain.
		return nil
	}

	ip, ipErr := netip.ParseAddr(id)
	if ipErr != nil {
		return fmt.Errorf("%q is neither a domain (%w) nor an IP address (%w)", id, domainErr, ipErr)
	}

	if ip.String() != id {
		return fmt.Errorf("invalid IP address %q, must be in canonical form (%q)", id, ip.String())
	}

	prefix, prefixErr := coveringPrefix(ip)
	if prefixErr != nil {
		return fmt.Errorf("invalid IP address %q, couldn't determine prefix: %w", id, prefixErr)
	}
	if prefix.Addr() != ip {
		return fmt.Errorf("invalid IP address %q, must be the lowest address in its prefix (%q)", id, prefix.Addr().String())
	}

	return iana.IsReservedPrefix(prefix)
}

// validateRegIdDomainOrCIDR validates that the provided string is formatted
// 'regId:domainOrCIDR', where domainOrCIDR is either a domain name or an IP
// address. IPv6 addresses must be the lowest address in their /64, i.e. their
// last 64 bits must be zero.
func validateRegIdDomainOrCIDR(id string) error {
	regIdDomainOrCIDR := strings.Split(id, ":")
	if len(regIdDomainOrCIDR) != 2 {
		return fmt.Errorf(
			"invalid regId:domainOrCIDR, %q must be formatted 'regId:domainOrCIDR'", id)
	}
	err := validateRegId(regIdDomainOrCIDR[0])
	if err != nil {
		return fmt.Errorf(
			"invalid regId, %q must be formatted 'regId:domainOrCIDR'", id)
	}
	err = validateDomainOrCIDR(regIdDomainOrCIDR[1])
	if err != nil {
		return fmt.Errorf("invalid domainOrCIDR, %q must be formatted 'regId:domainOrCIDR': %w", id, err)
	}
	return nil
}

// validateFQDNSet validates that the provided string is formatted 'fqdnSet',
// where fqdnSet is a comma-separated list of identifier values.
func validateFQDNSet(id string) error {
	values := strings.Split(id, ",")
	if len(values) == 0 {
		return fmt.Errorf(
			"invalid fqdnSet, %q must be formatted 'fqdnSet'", id)
	}
	for _, value := range values {
		domainErr := policy.ValidDomain(value)
		if domainErr != nil {
			ipErr := policy.ValidIP(value)
			if ipErr != nil {
				return fmt.Errorf("invalid fqdnSet member %q: %w as domain, %w as IP", id, domainErr, ipErr)
			}
		}
	}
	return nil
}

func validateIdForName(name Name, id string) error {
	switch name {
	case NewRegistrationsPerIPAddress:
		// 'enum:ipaddress'
		return validIPAddress(id)

	case NewRegistrationsPerIPv6Range:
		// 'enum:ipv6rangeCIDR'
		return validIPv6RangeCIDR(id)

	case NewOrdersPerAccount:
		// 'enum:regId'
		return validateRegId(id)

	case FailedAuthorizationsPerDomainPerAccount:
		if strings.Contains(id, ":") {
			// 'enum:regId:identValue' for transaction
			return validateRegIdIdentValue(id)
		} else {
			// 'enum:regId' for overrides
			return validateRegId(id)
		}

	case CertificatesPerDomainPerAccount:
		if strings.Contains(id, ":") {
			// 'enum:regId:domainOrCIDR' for transaction
			return validateRegIdDomainOrCIDR(id)
		} else {
			// 'enum:regId' for overrides
			return validateRegId(id)
		}

	case CertificatesPerDomain:
		// 'enum:domainOrCIDR'
		return validateDomainOrCIDR(id)

	case CertificatesPerFQDNSet:
		// 'enum:fqdnSet'
		return validateFQDNSet(id)

	case FailedAuthorizationsForPausingPerDomainPerAccount:
		if strings.Contains(id, ":") {
			// 'enum:regId:identValue' for transaction
			return validateRegIdIdentValue(id)
		} else {
			// 'enum:regId' for overrides
			return validateRegId(id)
		}

	case Unknown:
		fallthrough

	default:
		// This should never happen.
		return fmt.Errorf("unknown limit enum %q", name)
	}
}

// stringToName is a map of string names to Name values.
var stringToName = func() map[string]Name {
	m := make(map[string]Name, len(nameToString))
	for k, v := range nameToString {
		m[v] = k
	}
	return m
}()

// limitNames is a slice of all rate limit names.
var limitNames = func() []string {
	names := make([]string, 0, len(nameToString))
	for _, v := range nameToString {
		names = append(names, v)
	}
	return names
}()
