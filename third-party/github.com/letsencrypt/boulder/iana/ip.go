package iana

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"regexp"
	"slices"
	"strings"

	_ "embed"
)

type reservedPrefix struct {
	// addressFamily is "IPv4" or "IPv6".
	addressFamily string
	// The other fields are defined in:
	// https://www.iana.org/assignments/iana-ipv4-special-registry/iana-ipv4-special-registry.xhtml
	// https://www.iana.org/assignments/iana-ipv6-special-registry/iana-ipv6-special-registry.xhtml
	addressBlock netip.Prefix
	name         string
	rfc          string
	// The BRs' requirement that we not issue for Reserved IP Addresses only
	// cares about presence in one of these registries, not any of the other
	// metadata fields tracked by the registries. Therefore, we ignore the
	// Allocation Date, Termination Date, Source, Destination, Forwardable,
	// Globally Reachable, and Reserved By Protocol columns.
}

var (
	reservedPrefixes []reservedPrefix

	// https://www.iana.org/assignments/iana-ipv4-special-registry/iana-ipv4-special-registry.xhtml
	//go:embed data/iana-ipv4-special-registry-1.csv
	ipv4Registry []byte
	// https://www.iana.org/assignments/iana-ipv6-special-registry/iana-ipv6-special-registry.xhtml
	//go:embed data/iana-ipv6-special-registry-1.csv
	ipv6Registry []byte
)

// init parses and loads the embedded IANA special-purpose address registry CSV
// files for all address families, panicking if any one fails.
func init() {
	ipv4Prefixes, err := parseReservedPrefixFile(ipv4Registry, "IPv4")
	if err != nil {
		panic(err)
	}

	ipv6Prefixes, err := parseReservedPrefixFile(ipv6Registry, "IPv6")
	if err != nil {
		panic(err)
	}

	// Add multicast addresses, which aren't in the IANA registries.
	//
	// TODO(#8237): Move these entries to IP address blocklists once they're
	// implemented.
	additionalPrefixes := []reservedPrefix{
		{
			addressFamily: "IPv4",
			addressBlock:  netip.MustParsePrefix("224.0.0.0/4"),
			name:          "Multicast Addresses",
			rfc:           "[RFC3171]",
		},
		{
			addressFamily: "IPv6",
			addressBlock:  netip.MustParsePrefix("ff00::/8"),
			name:          "Multicast Addresses",
			rfc:           "[RFC4291]",
		},
	}

	reservedPrefixes = slices.Concat(ipv4Prefixes, ipv6Prefixes, additionalPrefixes)

	// Sort the list of reserved prefixes in descending order of prefix size, so
	// that checks will match the most-specific reserved prefix first.
	slices.SortFunc(reservedPrefixes, func(a, b reservedPrefix) int {
		if a.addressBlock.Bits() == b.addressBlock.Bits() {
			return 0
		}
		if a.addressBlock.Bits() > b.addressBlock.Bits() {
			return -1
		}
		return 1
	})
}

// Define regexps we'll use to clean up poorly formatted registry entries.
var (
	// 2+ sequential whitespace characters. The csv package takes care of
	// newlines automatically.
	ianaWhitespacesRE = regexp.MustCompile(`\s{2,}`)
	// Footnotes at the end, like `[2]`.
	ianaFootnotesRE = regexp.MustCompile(`\[\d+\]$`)
)

// parseReservedPrefixFile parses and returns the IANA special-purpose address
// registry CSV data for a single address family, or returns an error if parsing
// fails.
func parseReservedPrefixFile(registryData []byte, addressFamily string) ([]reservedPrefix, error) {
	if addressFamily != "IPv4" && addressFamily != "IPv6" {
		return nil, fmt.Errorf("failed to parse reserved address registry: invalid address family %q", addressFamily)
	}
	if registryData == nil {
		return nil, fmt.Errorf("failed to parse reserved %s address registry: empty", addressFamily)
	}

	reader := csv.NewReader(bytes.NewReader(registryData))

	// Parse the header row.
	record, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to parse reserved %s address registry header: %w", addressFamily, err)
	}
	if record[0] != "Address Block" || record[1] != "Name" || record[2] != "RFC" {
		return nil, fmt.Errorf("failed to parse reserved %s address registry header: must begin with \"Address Block\", \"Name\" and \"RFC\"", addressFamily)
	}

	// Parse the records.
	var prefixes []reservedPrefix
	for {
		row, err := reader.Read()
		if errors.Is(err, io.EOF) {
			// Finished parsing the file.
			if len(prefixes) < 1 {
				return nil, fmt.Errorf("failed to parse reserved %s address registry: no rows after header", addressFamily)
			}
			break
		} else if err != nil {
			return nil, err
		} else if len(row) < 3 {
			return nil, fmt.Errorf("failed to parse reserved %s address registry: incomplete row", addressFamily)
		}

		// Remove any footnotes, then handle each comma-separated prefix.
		for _, prefixStr := range strings.Split(ianaFootnotesRE.ReplaceAllLiteralString(row[0], ""), ",") {
			prefix, err := netip.ParsePrefix(strings.TrimSpace(prefixStr))
			if err != nil {
				return nil, fmt.Errorf("failed to parse reserved %s address registry: couldn't parse entry %q as an IP address prefix: %s", addressFamily, prefixStr, err)
			}

			prefixes = append(prefixes, reservedPrefix{
				addressFamily: addressFamily,
				addressBlock:  prefix,
				name:          row[1],
				// Replace any whitespace sequences with a single space.
				rfc: ianaWhitespacesRE.ReplaceAllLiteralString(row[2], " "),
			})
		}
	}

	return prefixes, nil
}

// IsReservedAddr returns an error if an IP address is part of a reserved range.
func IsReservedAddr(ip netip.Addr) error {
	for _, rpx := range reservedPrefixes {
		if rpx.addressBlock.Contains(ip) {
			return fmt.Errorf("IP address is in a reserved address block: %s: %s", rpx.rfc, rpx.name)
		}
	}

	return nil
}

// IsReservedPrefix returns an error if an IP address prefix overlaps with a
// reserved range.
func IsReservedPrefix(prefix netip.Prefix) error {
	for _, rpx := range reservedPrefixes {
		if rpx.addressBlock.Overlaps(prefix) {
			return fmt.Errorf("IP address is in a reserved address block: %s: %s", rpx.rfc, rpx.name)
		}
	}

	return nil
}
