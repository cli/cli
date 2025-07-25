package ratelimits

import (
	"net/netip"
	"slices"
	"testing"

	"github.com/letsencrypt/boulder/identifier"
)

func TestCoveringIdentifiers(t *testing.T) {
	cases := []struct {
		name    string
		idents  identifier.ACMEIdentifiers
		wantErr string
		want    []string
	}{
		{
			name: "empty string",
			idents: identifier.ACMEIdentifiers{
				identifier.NewDNS(""),
			},
			wantErr: "name is blank",
			want:    nil,
		},
		{
			name:   "two subdomains of same domain",
			idents: identifier.NewDNSSlice([]string{"www.example.com", "example.com"}),
			want:   []string{"example.com"},
		},
		{
			name:   "three subdomains across two domains",
			idents: identifier.NewDNSSlice([]string{"www.example.com", "example.com", "www.example.co.uk"}),
			want:   []string{"example.co.uk", "example.com"},
		},
		{
			name:   "three subdomains across two domains, plus a bare TLD",
			idents: identifier.NewDNSSlice([]string{"www.example.com", "example.com", "www.example.co.uk", "co.uk"}),
			want:   []string{"co.uk", "example.co.uk", "example.com"},
		},
		{
			name:   "two subdomains of same domain, one of them long",
			idents: identifier.NewDNSSlice([]string{"foo.bar.baz.www.example.com", "baz.example.com"}),
			want:   []string{"example.com"},
		},
		{
			name:   "a domain and two of its subdomains",
			idents: identifier.NewDNSSlice([]string{"github.io", "foo.github.io", "bar.github.io"}),
			want:   []string{"bar.github.io", "foo.github.io", "github.io"},
		},
		{
			name: "a domain and an IPv4 address",
			idents: identifier.ACMEIdentifiers{
				identifier.NewDNS("example.com"),
				identifier.NewIP(netip.MustParseAddr("127.0.0.1")),
			},
			want: []string{"127.0.0.1/32", "example.com"},
		},
		{
			name: "an IPv6 address",
			idents: identifier.ACMEIdentifiers{
				identifier.NewIP(netip.MustParseAddr("3fff:aaa:aaaa:aaaa:abad:0ff1:cec0:ffee")),
			},
			want: []string{"3fff:aaa:aaaa:aaaa::/64"},
		},
		{
			name: "four IP addresses in three prefixes",
			idents: identifier.ACMEIdentifiers{
				identifier.NewIP(netip.MustParseAddr("127.0.0.1")),
				identifier.NewIP(netip.MustParseAddr("127.0.0.254")),
				identifier.NewIP(netip.MustParseAddr("3fff:aaa:aaaa:aaaa:abad:0ff1:cec0:ffee")),
				identifier.NewIP(netip.MustParseAddr("3fff:aaa:aaaa:ffff:abad:0ff1:cec0:ffee")),
			},
			want: []string{"127.0.0.1/32", "127.0.0.254/32", "3fff:aaa:aaaa:aaaa::/64", "3fff:aaa:aaaa:ffff::/64"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := coveringIdentifiers(tc.idents)
			if err != nil && err.Error() != tc.wantErr {
				t.Errorf("Got unwanted error %#v", err.Error())
			}
			if err == nil && tc.wantErr != "" {
				t.Errorf("Got no error, wanted %#v", tc.wantErr)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("Got %#v, but want %#v", got, tc.want)
			}
		})
	}
}
