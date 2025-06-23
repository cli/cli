package ratelimits

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/letsencrypt/boulder/policy"
)

// Name is an enumeration of all rate limit names. It is used to intern rate
// limit names as strings and to provide a type-safe way to refer to rate
// limits.
//
// IMPORTANT: If you add a new limit Name, you MUST add:
//   - it to the nameToString mapping,
//   - an entry for it in the validateIdForName(), and
//   - provide the appropriate constructors in bucket.go.
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
	//  - When referenced in a transaction: uses bucket key 'enum:regId:domain',
	//    where regId is the ACME registration Id of the account and domain is a
	//    domain name in the certificate.
	FailedAuthorizationsPerDomainPerAccount

	// CertificatesPerDomain uses bucket key 'enum:domain', where domain is a
	// domain name in the certificate.
	CertificatesPerDomain

	// CertificatesPerDomainPerAccount is only used for per-account overrides to
	// the CertificatesPerDomain rate limit. If this limit is referenced in the
	// default limits file, it will be ignored. It uses two different bucket
	// keys depending on the context:
	//  - When referenced in an overrides file: uses bucket key 'enum:regId',
	//    where regId is the ACME registration Id of the account.
	//  - When referenced in a transaction: uses bucket key 'enum:regId:domain',
	//    where regId is the ACME registration Id of the account and domain is a
	//    domain name in the certificate.
	//
	// When overrides to the CertificatesPerDomainPerAccount are configured for a
	// subscriber, the cost:
	//   - MUST be consumed from each CertificatesPerDomainPerAccount bucket and
	//   - SHOULD be consumed from each CertificatesPerDomain bucket, if possible.
	CertificatesPerDomainPerAccount

	// CertificatesPerFQDNSet uses bucket key 'enum:fqdnSet', where fqdnSet is a
	// hashed set of unique eTLD+1 domain names in the certificate.
	//
	// Note: When this is referenced in an overrides file, the fqdnSet MUST be
	// passed as a comma-separated list of domain names.
	CertificatesPerFQDNSet
)

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

// nameToString is a map of Name values to string names.
var nameToString = map[Name]string{
	Unknown:                                 "Unknown",
	NewRegistrationsPerIPAddress:            "NewRegistrationsPerIPAddress",
	NewRegistrationsPerIPv6Range:            "NewRegistrationsPerIPv6Range",
	NewOrdersPerAccount:                     "NewOrdersPerAccount",
	FailedAuthorizationsPerDomainPerAccount: "FailedAuthorizationsPerDomainPerAccount",
	CertificatesPerDomain:                   "CertificatesPerDomain",
	CertificatesPerDomainPerAccount:         "CertificatesPerDomainPerAccount",
	CertificatesPerFQDNSet:                  "CertificatesPerFQDNSet",
}

// validIPAddress validates that the provided string is a valid IP address.
func validIPAddress(id string) error {
	ip := net.ParseIP(id)
	if ip == nil {
		return fmt.Errorf("invalid IP address, %q must be an IP address", id)
	}
	return nil
}

// validIPv6RangeCIDR validates that the provided string is formatted is an IPv6
// CIDR range with a /48 mask.
func validIPv6RangeCIDR(id string) error {
	_, ipNet, err := net.ParseCIDR(id)
	if err != nil {
		return fmt.Errorf(
			"invalid CIDR, %q must be an IPv6 CIDR range", id)
	}
	ones, _ := ipNet.Mask.Size()
	if ones != 48 {
		// This also catches the case where the range is an IPv4 CIDR, since an
		// IPv4 CIDR can't have a /48 subnet mask - the maximum is /32.
		return fmt.Errorf(
			"invalid CIDR, %q must be /48", id)
	}
	return nil
}

// validateRegId validates that the provided string is a valid ACME regId.
func validateRegId(id string) error {
	_, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid regId, %q must be an ACME registration Id", id)
	}
	return nil
}

// validateDomain validates that the provided string is formatted 'domain',
// where domain is a domain name.
func validateDomain(id string) error {
	err := policy.ValidDomain(id)
	if err != nil {
		return fmt.Errorf("invalid domain, %q must be formatted 'domain': %w", id, err)
	}
	return nil
}

// validateRegIdDomain validates that the provided string is formatted
// 'regId:domain', where regId is an ACME registration Id and domain is a domain
// name.
func validateRegIdDomain(id string) error {
	regIdDomain := strings.Split(id, ":")
	if len(regIdDomain) != 2 {
		return fmt.Errorf(
			"invalid regId:domain, %q must be formatted 'regId:domain'", id)
	}
	err := validateRegId(regIdDomain[0])
	if err != nil {
		return fmt.Errorf(
			"invalid regId, %q must be formatted 'regId:domain'", id)
	}
	err = policy.ValidDomain(regIdDomain[1])
	if err != nil {
		return fmt.Errorf(
			"invalid domain, %q must be formatted 'regId:domain': %w", id, err)
	}
	return nil
}

// validateFQDNSet validates that the provided string is formatted 'fqdnSet',
// where fqdnSet is a comma-separated list of domain names.
func validateFQDNSet(id string) error {
	domains := strings.Split(id, ",")
	if len(domains) == 0 {
		return fmt.Errorf(
			"invalid fqdnSet, %q must be formatted 'fqdnSet'", id)
	}
	return policy.WellFormedDomainNames(domains)
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
			// 'enum:regId:domain' for transaction
			return validateRegIdDomain(id)
		} else {
			// 'enum:regId' for overrides
			return validateRegId(id)
		}

	case CertificatesPerDomainPerAccount:
		if strings.Contains(id, ":") {
			// 'enum:regId:domain' for transaction
			return validateRegIdDomain(id)
		} else {
			// 'enum:regId' for overrides
			return validateRegId(id)
		}

	case CertificatesPerDomain:
		// 'enum:domain'
		return validateDomain(id)

	case CertificatesPerFQDNSet:
		// 'enum:fqdnSet'
		return validateFQDNSet(id)

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
	names := make([]string, len(nameToString))
	for _, v := range nameToString {
		names = append(names, v)
	}
	return names
}()
