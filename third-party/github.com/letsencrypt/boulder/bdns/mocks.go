package bdns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/miekg/dns"

	blog "github.com/letsencrypt/boulder/log"
)

// MockClient is a mock
type MockClient struct {
	Log blog.Logger
}

// LookupTXT is a mock
func (mock *MockClient) LookupTXT(_ context.Context, hostname string) ([]string, ResolverAddrs, error) {
	if hostname == "_acme-challenge.servfail.com" {
		return nil, ResolverAddrs{"MockClient"}, fmt.Errorf("SERVFAIL")
	}
	if hostname == "_acme-challenge.good-dns01.com" {
		// base64(sha256("LoqXcYV8q5ONbJQxbmR7SCTNo3tiAXDfowyjxAjEuX0"
		//               + "." + "9jg46WB3rR_AHD-EBXdN7cBkH1WOu0tA3M9fm21mqTI"))
		// expected token + test account jwk thumbprint
		return []string{"LPsIwTo7o8BoG0-vjCyGQGBWSVIPxI-i_X336eUOQZo"}, ResolverAddrs{"MockClient"}, nil
	}
	if hostname == "_acme-challenge.wrong-dns01.com" {
		return []string{"a"}, ResolverAddrs{"MockClient"}, nil
	}
	if hostname == "_acme-challenge.wrong-many-dns01.com" {
		return []string{"a", "b", "c", "d", "e"}, ResolverAddrs{"MockClient"}, nil
	}
	if hostname == "_acme-challenge.long-dns01.com" {
		return []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, ResolverAddrs{"MockClient"}, nil
	}
	if hostname == "_acme-challenge.no-authority-dns01.com" {
		// base64(sha256("LoqXcYV8q5ONbJQxbmR7SCTNo3tiAXDfowyjxAjEuX0"
		//               + "." + "9jg46WB3rR_AHD-EBXdN7cBkH1WOu0tA3M9fm21mqTI"))
		// expected token + test account jwk thumbprint
		return []string{"LPsIwTo7o8BoG0-vjCyGQGBWSVIPxI-i_X336eUOQZo"}, ResolverAddrs{"MockClient"}, nil
	}
	// empty-txts.com always returns zero TXT records
	if hostname == "_acme-challenge.empty-txts.com" {
		return []string{}, ResolverAddrs{"MockClient"}, nil
	}
	return []string{"hostname"}, ResolverAddrs{"MockClient"}, nil
}

// makeTimeoutError returns a a net.OpError for which Timeout() returns true.
func makeTimeoutError() *net.OpError {
	return &net.OpError{
		Err: os.NewSyscallError("ugh timeout", timeoutError{}),
	}
}

type timeoutError struct{}

func (t timeoutError) Error() string {
	return "so sloooow"
}
func (t timeoutError) Timeout() bool {
	return true
}

// LookupHost is a mock
func (mock *MockClient) LookupHost(_ context.Context, hostname string) ([]net.IP, ResolverAddrs, error) {
	if hostname == "always.invalid" ||
		hostname == "invalid.invalid" {
		return []net.IP{}, ResolverAddrs{"MockClient"}, nil
	}
	if hostname == "always.timeout" {
		return []net.IP{}, ResolverAddrs{"MockClient"}, &Error{dns.TypeA, "always.timeout", makeTimeoutError(), -1, nil}
	}
	if hostname == "always.error" {
		err := &net.OpError{
			Op:  "read",
			Net: "udp",
			Err: errors.New("some net error"),
		}
		m := new(dns.Msg)
		m.SetQuestion(dns.Fqdn(hostname), dns.TypeA)
		m.AuthenticatedData = true
		m.SetEdns0(4096, false)
		logDNSError(mock.Log, "mock.server", hostname, m, nil, err)
		return []net.IP{}, ResolverAddrs{"MockClient"}, &Error{dns.TypeA, hostname, err, -1, nil}
	}
	if hostname == "id.mismatch" {
		err := dns.ErrId
		m := new(dns.Msg)
		m.SetQuestion(dns.Fqdn(hostname), dns.TypeA)
		m.AuthenticatedData = true
		m.SetEdns0(4096, false)
		r := new(dns.Msg)
		record := new(dns.A)
		record.Hdr = dns.RR_Header{Name: dns.Fqdn(hostname), Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 0}
		record.A = net.ParseIP("127.0.0.1")
		r.Answer = append(r.Answer, record)
		logDNSError(mock.Log, "mock.server", hostname, m, r, err)
		return []net.IP{}, ResolverAddrs{"MockClient"}, &Error{dns.TypeA, hostname, err, -1, nil}
	}
	// dual-homed host with an IPv6 and an IPv4 address
	if hostname == "ipv4.and.ipv6.localhost" {
		return []net.IP{
			net.ParseIP("::1"),
			net.ParseIP("127.0.0.1"),
		}, ResolverAddrs{"MockClient"}, nil
	}
	if hostname == "ipv6.localhost" {
		return []net.IP{
			net.ParseIP("::1"),
		}, ResolverAddrs{"MockClient"}, nil
	}
	ip := net.ParseIP("127.0.0.1")
	return []net.IP{ip}, ResolverAddrs{"MockClient"}, nil
}

// LookupCAA returns mock records for use in tests.
func (mock *MockClient) LookupCAA(_ context.Context, domain string) ([]*dns.CAA, string, ResolverAddrs, error) {
	return nil, "", ResolverAddrs{"MockClient"}, nil
}
