package iana

import (
	"net/netip"
	"strings"
	"testing"
)

func TestIsReservedAddr(t *testing.T) {
	t.Parallel()

	cases := []struct {
		ip   string
		want string
	}{
		{"127.0.0.1", "Loopback"},          // second-lowest IP in a reserved /8, common mistaken request
		{"128.0.0.1", ""},                  // second-lowest IP just above a reserved /8
		{"192.168.254.254", "Private-Use"}, // highest IP in a reserved /16
		{"192.169.255.255", ""},            // highest IP in the /16 above a reserved /16

		{"::", "Unspecified Address"}, // lowest possible IPv6 address, reserved, possible parsing edge case
		{"::1", "Loopback Address"},   // reserved, common mistaken request
		{"::2", ""},                   // surprisingly unreserved

		{"fe80::1", "Link-Local Unicast"},                                 // second-lowest IP in a reserved /10
		{"febf:ffff:ffff:ffff:ffff:ffff:ffff:ffff", "Link-Local Unicast"}, // highest IP in a reserved /10
		{"fec0::1", ""}, // second-lowest IP just above a reserved /10

		{"192.0.0.170", "NAT64/DNS64 Discovery"},            // first of two reserved IPs that are comma-split in IANA's CSV; also a more-specific of a larger reserved block that comes first
		{"192.0.0.171", "NAT64/DNS64 Discovery"},            // second of two reserved IPs that are comma-split in IANA's CSV; also a more-specific of a larger reserved block that comes first
		{"2001:1::1", "Port Control Protocol Anycast"},      // reserved IP that comes after a line with a line break in IANA's CSV; also a more-specific of a larger reserved block that comes first
		{"2002::", "6to4"},                                  // lowest IP in a reserved /16 that has a footnote in IANA's CSV
		{"2002:ffff:ffff:ffff:ffff:ffff:ffff:ffff", "6to4"}, // highest IP in a reserved /16 that has a footnote in IANA's CSV

		{"0100::", "Discard-Only Address Block"},                         // part of a reserved block in a non-canonical IPv6 format
		{"0100::0000:ffff:ffff:ffff:ffff", "Discard-Only Address Block"}, // part of a reserved block in a non-canonical IPv6 format
		{"0100::0002:0000:0000:0000:0000", ""},                           // non-reserved but in a non-canonical IPv6 format

		// TODO(#8237): Move these entries to IP address blocklists once they're
		// implemented.
		{"ff00::1", "Multicast Addresses"},                                 // second-lowest IP in a reserved /8 we hardcode
		{"ff10::1", "Multicast Addresses"},                                 // in the middle of a reserved /8 we hardcode
		{"ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", "Multicast Addresses"}, // highest IP in a reserved /8 we hardcode
	}

	for _, tc := range cases {
		t.Run(tc.ip, func(t *testing.T) {
			t.Parallel()
			err := IsReservedAddr(netip.MustParseAddr(tc.ip))
			if err == nil && tc.want != "" {
				t.Errorf("Got success, wanted error for %#v", tc.ip)
			}
			if err != nil && !strings.Contains(err.Error(), tc.want) {
				t.Errorf("%#v: got %q, want %q", tc.ip, err.Error(), tc.want)
			}
		})
	}
}

func TestIsReservedPrefix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		cidr string
		want bool
	}{
		{"172.16.0.0/12", true},
		{"172.16.0.0/32", true},
		{"172.16.0.1/32", true},
		{"172.31.255.0/24", true},
		{"172.31.255.255/24", true},
		{"172.31.255.255/32", true},
		{"172.32.0.0/24", false},
		{"172.32.0.1/32", false},

		{"100::/64", true},
		{"100::/128", true},
		{"100::1/128", true},
		{"100::1:ffff:ffff:ffff:ffff/128", true},
		{"100:0:0:2::/64", false},
		{"100:0:0:2::1/128", false},
	}

	for _, tc := range cases {
		t.Run(tc.cidr, func(t *testing.T) {
			t.Parallel()
			err := IsReservedPrefix(netip.MustParsePrefix(tc.cidr))
			if err != nil && !tc.want {
				t.Error(err)
			}
			if err == nil && tc.want {
				t.Errorf("Wanted error for %#v, got success", tc.cidr)
			}
		})
	}
}
