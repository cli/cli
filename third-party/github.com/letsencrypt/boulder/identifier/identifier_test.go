package identifier

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"net"
	"net/netip"
	"reflect"
	"slices"
	"testing"
)

// TestFromX509 tests FromCert and FromCSR, which are fromX509's public
// wrappers.
func TestFromX509(t *testing.T) {
	cases := []struct {
		name        string
		subject     pkix.Name
		dnsNames    []string
		ipAddresses []net.IP
		want        ACMEIdentifiers
	}{
		{
			name:     "no explicit CN",
			dnsNames: []string{"a.com"},
			want:     ACMEIdentifiers{NewDNS("a.com")},
		},
		{
			name:     "explicit uppercase CN",
			subject:  pkix.Name{CommonName: "A.com"},
			dnsNames: []string{"a.com"},
			want:     ACMEIdentifiers{NewDNS("a.com")},
		},
		{
			name:     "no explicit CN, uppercase SAN",
			dnsNames: []string{"A.com"},
			want:     ACMEIdentifiers{NewDNS("a.com")},
		},
		{
			name:     "duplicate SANs",
			dnsNames: []string{"b.com", "b.com", "a.com", "a.com"},
			want:     ACMEIdentifiers{NewDNS("a.com"), NewDNS("b.com")},
		},
		{
			name:     "explicit CN not found in SANs",
			subject:  pkix.Name{CommonName: "a.com"},
			dnsNames: []string{"b.com"},
			want:     ACMEIdentifiers{NewDNS("a.com"), NewDNS("b.com")},
		},
		{
			name:        "mix of DNSNames and IPAddresses",
			dnsNames:    []string{"a.com"},
			ipAddresses: []net.IP{{192, 168, 1, 1}},
			want:        ACMEIdentifiers{NewDNS("a.com"), NewIP(netip.MustParseAddr("192.168.1.1"))},
		},
	}
	for _, tc := range cases {
		t.Run("cert/"+tc.name, func(t *testing.T) {
			t.Parallel()
			got := FromCert(&x509.Certificate{Subject: tc.subject, DNSNames: tc.dnsNames, IPAddresses: tc.ipAddresses})
			if !slices.Equal(got, tc.want) {
				t.Errorf("FromCert() got %#v, but want %#v", got, tc.want)
			}
		})
		t.Run("csr/"+tc.name, func(t *testing.T) {
			t.Parallel()
			got := FromCSR(&x509.CertificateRequest{Subject: tc.subject, DNSNames: tc.dnsNames, IPAddresses: tc.ipAddresses})
			if !slices.Equal(got, tc.want) {
				t.Errorf("FromCSR() got %#v, but want %#v", got, tc.want)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	cases := []struct {
		name   string
		idents ACMEIdentifiers
		want   ACMEIdentifiers
	}{
		{
			name: "convert to lowercase",
			idents: ACMEIdentifiers{
				{Type: TypeDNS, Value: "AlPha.example.coM"},
				{Type: TypeIP, Value: "fe80::CAFE"},
			},
			want: ACMEIdentifiers{
				{Type: TypeDNS, Value: "alpha.example.com"},
				{Type: TypeIP, Value: "fe80::cafe"},
			},
		},
		{
			name: "sort",
			idents: ACMEIdentifiers{
				{Type: TypeDNS, Value: "foobar.com"},
				{Type: TypeDNS, Value: "bar.com"},
				{Type: TypeDNS, Value: "baz.com"},
				{Type: TypeDNS, Value: "a.com"},
				{Type: TypeIP, Value: "fe80::cafe"},
				{Type: TypeIP, Value: "2001:db8::1dea"},
				{Type: TypeIP, Value: "192.168.1.1"},
			},
			want: ACMEIdentifiers{
				{Type: TypeDNS, Value: "a.com"},
				{Type: TypeDNS, Value: "bar.com"},
				{Type: TypeDNS, Value: "baz.com"},
				{Type: TypeDNS, Value: "foobar.com"},
				{Type: TypeIP, Value: "192.168.1.1"},
				{Type: TypeIP, Value: "2001:db8::1dea"},
				{Type: TypeIP, Value: "fe80::cafe"},
			},
		},
		{
			name: "de-duplicate",
			idents: ACMEIdentifiers{
				{Type: TypeDNS, Value: "AlPha.example.coM"},
				{Type: TypeIP, Value: "fe80::CAFE"},
				{Type: TypeDNS, Value: "alpha.example.com"},
				{Type: TypeIP, Value: "fe80::cafe"},
				NewIP(netip.MustParseAddr("fe80:0000:0000:0000:0000:0000:0000:cafe")),
			},
			want: ACMEIdentifiers{
				{Type: TypeDNS, Value: "alpha.example.com"},
				{Type: TypeIP, Value: "fe80::cafe"},
			},
		},
		{
			name: "DNS before IP",
			idents: ACMEIdentifiers{
				{Type: TypeIP, Value: "fe80::cafe"},
				{Type: TypeDNS, Value: "alpha.example.com"},
			},
			want: ACMEIdentifiers{
				{Type: TypeDNS, Value: "alpha.example.com"},
				{Type: TypeIP, Value: "fe80::cafe"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Normalize(tc.idents)
			if !slices.Equal(got, tc.want) {
				t.Errorf("Got %#v, but want %#v", got, tc.want)
			}
		})
	}
}

func TestToValues(t *testing.T) {
	cases := []struct {
		name            string
		idents          ACMEIdentifiers
		wantErr         string
		wantDnsNames    []string
		wantIpAddresses []net.IP
	}{
		{
			name: "DNS names and IP addresses",
			// These are deliberately out of alphabetical and type order, to
			// ensure ToValues doesn't do normalization, which ought to be done
			// explicitly.
			idents: ACMEIdentifiers{
				{Type: TypeDNS, Value: "beta.example.com"},
				{Type: TypeIP, Value: "fe80::cafe"},
				{Type: TypeDNS, Value: "alpha.example.com"},
				{Type: TypeIP, Value: "127.0.0.1"},
			},
			wantErr:         "",
			wantDnsNames:    []string{"beta.example.com", "alpha.example.com"},
			wantIpAddresses: []net.IP{net.ParseIP("fe80::cafe"), net.ParseIP("127.0.0.1")},
		},
		{
			name: "DNS names only",
			idents: ACMEIdentifiers{
				{Type: TypeDNS, Value: "alpha.example.com"},
				{Type: TypeDNS, Value: "beta.example.com"},
			},
			wantErr:         "",
			wantDnsNames:    []string{"alpha.example.com", "beta.example.com"},
			wantIpAddresses: nil,
		},
		{
			name: "IP addresses only",
			idents: ACMEIdentifiers{
				{Type: TypeIP, Value: "127.0.0.1"},
				{Type: TypeIP, Value: "fe80::cafe"},
			},
			wantErr:         "",
			wantDnsNames:    nil,
			wantIpAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("fe80::cafe")},
		},
		{
			name: "invalid IP address",
			idents: ACMEIdentifiers{
				{Type: TypeIP, Value: "fe80::c0ffee"},
			},
			wantErr:         "parsing IP address: fe80::c0ffee",
			wantDnsNames:    nil,
			wantIpAddresses: nil,
		},
		{
			name: "invalid identifier type",
			idents: ACMEIdentifiers{
				{Type: "fnord", Value: "panic.example.com"},
			},
			wantErr:         "evaluating identifier type: fnord for panic.example.com",
			wantDnsNames:    nil,
			wantIpAddresses: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotDnsNames, gotIpAddresses, gotErr := tc.idents.ToValues()
			if !slices.Equal(gotDnsNames, tc.wantDnsNames) {
				t.Errorf("Got DNS names %#v, but want %#v", gotDnsNames, tc.wantDnsNames)
			}
			if !reflect.DeepEqual(gotIpAddresses, tc.wantIpAddresses) {
				t.Errorf("Got IP addresses %#v, but want %#v", gotIpAddresses, tc.wantIpAddresses)
			}
			if tc.wantErr != "" && (gotErr.Error() != tc.wantErr) {
				t.Errorf("Got error %#v, but want %#v", gotErr.Error(), tc.wantErr)
			}
			if tc.wantErr == "" && gotErr != nil {
				t.Errorf("Got error %#v, but didn't want one", gotErr.Error())
			}
		})
	}
}
