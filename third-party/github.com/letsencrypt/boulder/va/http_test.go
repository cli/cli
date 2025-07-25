package va

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	mrand "math/rand/v2"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/miekg/dns"

	"github.com/letsencrypt/boulder/bdns"
	"github.com/letsencrypt/boulder/core"
	berrors "github.com/letsencrypt/boulder/errors"
	"github.com/letsencrypt/boulder/identifier"
	"github.com/letsencrypt/boulder/must"
	"github.com/letsencrypt/boulder/probs"
	"github.com/letsencrypt/boulder/test"

	"testing"
)

// TestDialerMismatchError tests that using a preresolvedDialer for one host for
// a dial to another host produces the expected dialerMismatchError.
func TestDialerMismatchError(t *testing.T) {
	d := preresolvedDialer{
		ip:       netip.MustParseAddr("127.0.0.1"),
		port:     1337,
		hostname: "letsencrypt.org",
	}

	expectedErr := dialerMismatchError{
		dialerHost: d.hostname,
		dialerIP:   d.ip.String(),
		dialerPort: d.port,
		host:       "lettuceencrypt.org",
	}

	_, err := d.DialContext(
		context.Background(),
		"tincan-and-string",
		"lettuceencrypt.org:80")
	test.AssertEquals(t, err.Error(), expectedErr.Error())
}

// dnsMockReturnsUnroutable is a DNSClient mock that always returns an
// unroutable address for LookupHost. This is useful in testing connect
// timeouts.
type dnsMockReturnsUnroutable struct {
	*bdns.MockClient
}

func (mock dnsMockReturnsUnroutable) LookupHost(_ context.Context, hostname string) ([]netip.Addr, bdns.ResolverAddrs, error) {
	return []netip.Addr{netip.MustParseAddr("64.112.117.254")}, bdns.ResolverAddrs{"dnsMockReturnsUnroutable"}, nil
}

// TestDialerTimeout tests that the preresolvedDialer's DialContext
// will timeout after the expected singleDialTimeout. This ensures timeouts at
// the TCP level are handled correctly. It also ensures that we show the client
// the appropriate "Timeout during connect" error message, which helps clients
// distinguish between firewall problems and server problems.
func TestDialerTimeout(t *testing.T) {
	va, _ := setup(nil, "", nil, nil)
	// Timeouts below 50ms tend to be flaky.
	va.singleDialTimeout = 50 * time.Millisecond

	// The context timeout needs to be larger than the singleDialTimeout
	ctxTimeout := 500 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), ctxTimeout)
	defer cancel()

	va.dnsClient = dnsMockReturnsUnroutable{&bdns.MockClient{}}
	// NOTE(@jsha): The only method I've found so far to trigger a connect timeout
	// is to connect to an unrouteable IP address. This usually generates
	// a connection timeout, but will rarely return "Network unreachable" instead.
	// If we get that, just retry until we get something other than "Network unreachable".
	var err error
	var took time.Duration
	for range 20 {
		started := time.Now()
		_, _, err = va.processHTTPValidation(ctx, identifier.NewDNS("unroutable.invalid"), "/.well-known/acme-challenge/whatever")
		took = time.Since(started)
		if err != nil && strings.Contains(err.Error(), "network is unreachable") {
			continue
		} else {
			break
		}
	}
	if err == nil {
		t.Fatalf("Connection should've timed out")
	}

	// Check that the HTTP connection doesn't return too fast, and times
	// out after the expected time
	if took < va.singleDialTimeout {
		t.Fatalf("fetch returned before %s (took: %s) with %q", va.singleDialTimeout, took, err.Error())
	}
	if took > 2*va.singleDialTimeout {
		t.Fatalf("fetch didn't timeout after %s (took: %s)", va.singleDialTimeout, took)
	}
	prob := detailedError(err)
	test.AssertEquals(t, prob.Type, probs.ConnectionProblem)
	test.AssertContains(t, prob.Detail, "Timeout during connect (likely firewall problem)")
}

func TestHTTPTransport(t *testing.T) {
	dummyDialerFunc := func(_ context.Context, _, _ string) (net.Conn, error) {
		return nil, nil
	}
	transport := httpTransport(dummyDialerFunc)
	// The HTTP Transport should have a TLS config that skips verifying
	// certificates.
	test.AssertEquals(t, transport.TLSClientConfig.InsecureSkipVerify, true)
	// Keep alives should be disabled
	test.AssertEquals(t, transport.DisableKeepAlives, true)
	test.AssertEquals(t, transport.MaxIdleConns, 1)
	test.AssertEquals(t, transport.IdleConnTimeout.String(), "1s")
	test.AssertEquals(t, transport.TLSHandshakeTimeout.String(), "10s")
}

func TestHTTPValidationTarget(t *testing.T) {
	// NOTE(@cpu): See `bdns/mocks.go` and the mock `LookupHost` function for the
	// hostnames used in this test.
	testCases := []struct {
		Name          string
		Ident         identifier.ACMEIdentifier
		ExpectedError error
		ExpectedIPs   []string
	}{
		{
			Name:          "No IPs for DNS identifier",
			Ident:         identifier.NewDNS("always.invalid"),
			ExpectedError: berrors.DNSError("No valid IP addresses found for always.invalid"),
		},
		{
			Name:        "Only IPv4 addrs for DNS identifier",
			Ident:       identifier.NewDNS("some.example.com"),
			ExpectedIPs: []string{"127.0.0.1"},
		},
		{
			Name:        "Only IPv6 addrs for DNS identifier",
			Ident:       identifier.NewDNS("ipv6.localhost"),
			ExpectedIPs: []string{"::1"},
		},
		{
			Name:  "Both IPv6 and IPv4 addrs for DNS identifier",
			Ident: identifier.NewDNS("ipv4.and.ipv6.localhost"),
			// In this case we expect 1 IPv6 address first, and then 1 IPv4 address
			ExpectedIPs: []string{"::1", "127.0.0.1"},
		},
		{
			Name:        "IPv4 IP address identifier",
			Ident:       identifier.NewIP(netip.MustParseAddr("127.0.0.1")),
			ExpectedIPs: []string{"127.0.0.1"},
		},
		{
			Name:        "IPv6 IP address identifier",
			Ident:       identifier.NewIP(netip.MustParseAddr("::1")),
			ExpectedIPs: []string{"::1"},
		},
	}

	const (
		examplePort  = 1234
		examplePath  = "/.well-known/path/i/took"
		exampleQuery = "my-path=was&my=own"
	)

	va, _ := setup(nil, "", nil, nil)
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			target, err := va.newHTTPValidationTarget(
				context.Background(),
				tc.Ident,
				examplePort,
				examplePath,
				exampleQuery)
			if err != nil && tc.ExpectedError == nil {
				t.Fatalf("Unexpected error from NewHTTPValidationTarget: %v", err)
			} else if err != nil && tc.ExpectedError != nil {
				test.AssertMarshaledEquals(t, err, tc.ExpectedError)
			} else if err == nil {
				// The target should be populated.
				test.AssertNotEquals(t, target.host, "")
				test.AssertNotEquals(t, target.port, 0)
				test.AssertNotEquals(t, target.path, "")
				// Calling ip() on the target should give the expected IPs in the right
				// order.
				for i, expectedIP := range tc.ExpectedIPs {
					gotIP := target.cur
					if (gotIP == netip.Addr{}) {
						t.Errorf("Expected IP %d to be %s got nil", i, expectedIP)
					} else {
						test.AssertEquals(t, gotIP.String(), expectedIP)
					}
					// Advance to the next IP
					_ = target.nextIP()
				}
			}
		})
	}
}

func TestExtractRequestTarget(t *testing.T) {
	mustURL := func(rawURL string) *url.URL {
		return must.Do(url.Parse(rawURL))
	}

	testCases := []struct {
		Name          string
		Req           *http.Request
		ExpectedError error
		ExpectedIdent identifier.ACMEIdentifier
		ExpectedPort  int
	}{
		{
			Name:          "nil input req",
			ExpectedError: fmt.Errorf("redirect HTTP request was nil"),
		},
		{
			Name: "invalid protocol scheme",
			Req: &http.Request{
				URL: mustURL("gopher://letsencrypt.org"),
			},
			ExpectedError: fmt.Errorf("Invalid protocol scheme in redirect target. " +
				`Only "http" and "https" protocol schemes are supported, ` +
				`not "gopher"`),
		},
		{
			Name: "invalid explicit port",
			Req: &http.Request{
				URL: mustURL("https://weird.port.letsencrypt.org:9999"),
			},
			ExpectedError: fmt.Errorf("Invalid port in redirect target. Only ports 80 " +
				"and 443 are supported, not 9999"),
		},
		{
			Name: "invalid empty host",
			Req: &http.Request{
				URL: mustURL("https:///who/needs/a/hostname?not=me"),
			},
			ExpectedError: errors.New("Invalid empty host in redirect target"),
		},
		{
			Name: "invalid .well-known hostname",
			Req: &http.Request{
				URL: mustURL("https://my.webserver.is.misconfigured.well-known/acme-challenge/xxx"),
			},
			ExpectedError: errors.New(`Invalid host in redirect target "my.webserver.is.misconfigured.well-known". Check webserver config for missing '/' in redirect target.`),
		},
		{
			Name: "invalid non-iana hostname",
			Req: &http.Request{
				URL: mustURL("https://my.tld.is.cpu/pretty/cool/right?yeah=Ithoughtsotoo"),
			},
			ExpectedError: errors.New("Invalid host in redirect target, must end in IANA registered TLD"),
		},
		{
			Name: "malformed wildcard-ish IPv4 address",
			Req: &http.Request{
				URL: mustURL("https://10.10.10.*"),
			},
			ExpectedError: errors.New("Invalid host in redirect target, must end in IANA registered TLD"),
		},
		{
			Name: "malformed too-long IPv6 address",
			Req: &http.Request{
				URL: mustURL("https://[a:b:c:d:e:f:b:a:d]"),
			},
			ExpectedError: errors.New("Invalid host in redirect target, must end in IANA registered TLD"),
		},
		{
			Name: "bare IPv4, implicit port",
			Req: &http.Request{
				URL: mustURL("http://127.0.0.1"),
			},
			ExpectedIdent: identifier.NewIP(netip.MustParseAddr("127.0.0.1")),
			ExpectedPort:  80,
		},
		{
			Name: "bare IPv4, explicit valid port",
			Req: &http.Request{
				URL: mustURL("http://127.0.0.1:80"),
			},
			ExpectedIdent: identifier.NewIP(netip.MustParseAddr("127.0.0.1")),
			ExpectedPort:  80,
		},
		{
			Name: "bare IPv4, explicit invalid port",
			Req: &http.Request{
				URL: mustURL("http://127.0.0.1:9999"),
			},
			ExpectedError: fmt.Errorf("Invalid port in redirect target. Only ports 80 " +
				"and 443 are supported, not 9999"),
		},
		{
			Name: "bare IPv4, HTTPS",
			Req: &http.Request{
				URL: mustURL("https://127.0.0.1"),
			},
			ExpectedIdent: identifier.NewIP(netip.MustParseAddr("127.0.0.1")),
			ExpectedPort:  443,
		},
		{
			Name: "bare IPv4, reserved IP address",
			Req: &http.Request{
				URL: mustURL("http://10.10.10.10"),
			},
			ExpectedError: fmt.Errorf("Invalid host in redirect target: " +
				"IP address is in a reserved address block: [RFC1918]: Private-Use"),
		},
		{
			Name: "bare IPv6, implicit port",
			Req: &http.Request{
				URL: mustURL("http://[::1]"),
			},
			ExpectedIdent: identifier.NewIP(netip.MustParseAddr("::1")),
			ExpectedPort:  80,
		},
		{
			Name: "bare IPv6, explicit valid port",
			Req: &http.Request{
				URL: mustURL("http://[::1]:80"),
			},
			ExpectedIdent: identifier.NewIP(netip.MustParseAddr("::1")),
			ExpectedPort:  80,
		},
		{
			Name: "bare IPv6, explicit invalid port",
			Req: &http.Request{
				URL: mustURL("http://[::1]:9999"),
			},
			ExpectedError: fmt.Errorf("Invalid port in redirect target. Only ports 80 " +
				"and 443 are supported, not 9999"),
		},
		{
			Name: "bare IPv6, HTTPS",
			Req: &http.Request{
				URL: mustURL("https://[::1]"),
			},
			ExpectedIdent: identifier.NewIP(netip.MustParseAddr("::1")),
			ExpectedPort:  443,
		},
		{
			Name: "bare IPv6, reserved IP address",
			Req: &http.Request{
				URL: mustURL("http://[3fff:aaa:aaaa:aaaa:abad:0ff1:cec0:ffee]"),
			},
			ExpectedError: fmt.Errorf("Invalid host in redirect target: " +
				"IP address is in a reserved address block: [RFC9637]: Documentation"),
		},
		{
			Name: "valid HTTP redirect, explicit port",
			Req: &http.Request{
				URL: mustURL("http://cpu.letsencrypt.org:80"),
			},
			ExpectedIdent: identifier.NewDNS("cpu.letsencrypt.org"),
			ExpectedPort:  80,
		},
		{
			Name: "valid HTTP redirect, implicit port",
			Req: &http.Request{
				URL: mustURL("http://cpu.letsencrypt.org"),
			},
			ExpectedIdent: identifier.NewDNS("cpu.letsencrypt.org"),
			ExpectedPort:  80,
		},
		{
			Name: "valid HTTPS redirect, explicit port",
			Req: &http.Request{
				URL: mustURL("https://cpu.letsencrypt.org:443/hello.world"),
			},
			ExpectedIdent: identifier.NewDNS("cpu.letsencrypt.org"),
			ExpectedPort:  443,
		},
		{
			Name: "valid HTTPS redirect, implicit port",
			Req: &http.Request{
				URL: mustURL("https://cpu.letsencrypt.org/hello.world"),
			},
			ExpectedIdent: identifier.NewDNS("cpu.letsencrypt.org"),
			ExpectedPort:  443,
		},
	}

	va, _ := setup(nil, "", nil, nil)
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			host, port, err := va.extractRequestTarget(tc.Req)
			if err != nil && tc.ExpectedError == nil {
				t.Errorf("Expected nil err got %v", err)
			} else if err != nil && tc.ExpectedError != nil {
				test.AssertEquals(t, err.Error(), tc.ExpectedError.Error())
			} else if err == nil && tc.ExpectedError != nil {
				t.Errorf("Expected err %v, got nil", tc.ExpectedError)
			} else {
				test.AssertEquals(t, host, tc.ExpectedIdent)
				test.AssertEquals(t, port, tc.ExpectedPort)
			}
		})
	}
}

// TestHTTPValidationDNSError attempts validation for a domain name that always
// generates a DNS error, and checks that a log line with the detailed error is
// generated.
func TestHTTPValidationDNSError(t *testing.T) {
	va, mockLog := setup(nil, "", nil, nil)

	_, _, prob := va.processHTTPValidation(ctx, identifier.NewDNS("always.error"), "/.well-known/acme-challenge/whatever")
	test.AssertError(t, prob, "Expected validation fetch to fail")
	matchingLines := mockLog.GetAllMatching(`read udp: some net error`)
	if len(matchingLines) != 1 {
		t.Errorf("Didn't see expected DNS error logged. Instead, got:\n%s",
			strings.Join(mockLog.GetAllMatching(`.*`), "\n"))
	}
}

// TestHTTPValidationDNSIdMismatchError tests that performing an HTTP-01
// challenge with a domain name that always returns a DNS ID mismatch error from
// the mock resolver results in valid query/response data being logged in
// a format we can decode successfully.
func TestHTTPValidationDNSIdMismatchError(t *testing.T) {
	va, mockLog := setup(nil, "", nil, nil)

	_, _, prob := va.processHTTPValidation(ctx, identifier.NewDNS("id.mismatch"), "/.well-known/acme-challenge/whatever")
	test.AssertError(t, prob, "Expected validation fetch to fail")
	matchingLines := mockLog.GetAllMatching(`logDNSError ID mismatch`)
	if len(matchingLines) != 1 {
		t.Errorf("Didn't see expected DNS error logged. Instead, got:\n%s",
			strings.Join(mockLog.GetAllMatching(`.*`), "\n"))
	}
	expectedRegex := regexp.MustCompile(
		`INFO: logDNSError ID mismatch ` +
			`chosenServer=\[mock.server\] ` +
			`hostname=\[id\.mismatch\] ` +
			`respHostname=\[id\.mismatch\.\] ` +
			`queryType=\[A\] ` +
			`msg=\[([A-Za-z0-9+=/\=]+)\] ` +
			`resp=\[([A-Za-z0-9+=/\=]+)\] ` +
			`err\=\[dns: id mismatch\]`,
	)

	matches := expectedRegex.FindAllStringSubmatch(matchingLines[0], -1)
	test.AssertEquals(t, len(matches), 1)
	submatches := matches[0]
	test.AssertEquals(t, len(submatches), 3)

	msgBytes, err := base64.StdEncoding.DecodeString(submatches[1])
	test.AssertNotError(t, err, "bad base64 encoded query msg")
	msg := new(dns.Msg)
	err = msg.Unpack(msgBytes)
	test.AssertNotError(t, err, "bad packed query msg")

	respBytes, err := base64.StdEncoding.DecodeString(submatches[2])
	test.AssertNotError(t, err, "bad base64 encoded resp msg")
	resp := new(dns.Msg)
	err = resp.Unpack(respBytes)
	test.AssertNotError(t, err, "bad packed response msg")
}

func TestSetupHTTPValidation(t *testing.T) {
	va, _ := setup(nil, "", nil, nil)

	mustTarget := func(t *testing.T, host string, port int, path string) *httpValidationTarget {
		target, err := va.newHTTPValidationTarget(
			context.Background(),
			identifier.NewDNS(host),
			port,
			path,
			"")
		if err != nil {
			t.Fatalf("Failed to construct httpValidationTarget for %q", host)
			return nil
		}
		return target
	}

	httpInputURL := "http://ipv4.and.ipv6.localhost/yellow/brick/road"
	httpsInputURL := "https://ipv4.and.ipv6.localhost/yellow/brick/road"

	testCases := []struct {
		Name           string
		InputURL       string
		InputTarget    *httpValidationTarget
		ExpectedRecord core.ValidationRecord
		ExpectedDialer *preresolvedDialer
		ExpectedError  error
	}{
		{
			Name:          "nil target",
			InputURL:      httpInputURL,
			ExpectedError: fmt.Errorf("httpValidationTarget can not be nil"),
		},
		{
			Name:          "empty input URL",
			InputTarget:   &httpValidationTarget{},
			ExpectedError: fmt.Errorf("reqURL can not be nil"),
		},
		{
			Name:     "target with no IPs",
			InputURL: httpInputURL,
			InputTarget: &httpValidationTarget{
				host: "ipv4.and.ipv6.localhost",
				port: va.httpPort,
				path: "idk",
			},
			ExpectedRecord: core.ValidationRecord{
				URL:      "http://ipv4.and.ipv6.localhost/yellow/brick/road",
				Hostname: "ipv4.and.ipv6.localhost",
				Port:     strconv.Itoa(va.httpPort),
			},
			ExpectedError: fmt.Errorf(`host "ipv4.and.ipv6.localhost" has no IP addresses remaining to use`),
		},
		{
			Name:        "HTTP input req",
			InputTarget: mustTarget(t, "ipv4.and.ipv6.localhost", va.httpPort, "/yellow/brick/road"),
			InputURL:    httpInputURL,
			ExpectedRecord: core.ValidationRecord{
				Hostname:          "ipv4.and.ipv6.localhost",
				Port:              strconv.Itoa(va.httpPort),
				URL:               "http://ipv4.and.ipv6.localhost/yellow/brick/road",
				AddressesResolved: []netip.Addr{netip.MustParseAddr("::1"), netip.MustParseAddr("127.0.0.1")},
				AddressUsed:       netip.MustParseAddr("::1"),
				ResolverAddrs:     []string{"MockClient"},
			},
			ExpectedDialer: &preresolvedDialer{
				ip:      netip.MustParseAddr("::1"),
				port:    va.httpPort,
				timeout: va.singleDialTimeout,
			},
		},
		{
			Name:        "HTTPS input req",
			InputTarget: mustTarget(t, "ipv4.and.ipv6.localhost", va.httpsPort, "/yellow/brick/road"),
			InputURL:    httpsInputURL,
			ExpectedRecord: core.ValidationRecord{
				Hostname:          "ipv4.and.ipv6.localhost",
				Port:              strconv.Itoa(va.httpsPort),
				URL:               "https://ipv4.and.ipv6.localhost/yellow/brick/road",
				AddressesResolved: []netip.Addr{netip.MustParseAddr("::1"), netip.MustParseAddr("127.0.0.1")},
				AddressUsed:       netip.MustParseAddr("::1"),
				ResolverAddrs:     []string{"MockClient"},
			},
			ExpectedDialer: &preresolvedDialer{
				ip:      netip.MustParseAddr("::1"),
				port:    va.httpsPort,
				timeout: va.singleDialTimeout,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			outDialer, outRecord, err := va.setupHTTPValidation(tc.InputURL, tc.InputTarget)
			if err != nil && tc.ExpectedError == nil {
				t.Errorf("Expected nil error, got %v", err)
			} else if err == nil && tc.ExpectedError != nil {
				t.Errorf("Expected %v error, got nil", tc.ExpectedError)
			} else if err != nil && tc.ExpectedError != nil {
				test.AssertEquals(t, err.Error(), tc.ExpectedError.Error())
			}
			if tc.ExpectedDialer == nil && outDialer != nil {
				t.Errorf("Expected nil dialer, got %v", outDialer)
			} else if tc.ExpectedDialer != nil {
				test.AssertMarshaledEquals(t, outDialer, tc.ExpectedDialer)
			}
			// In all cases we expect there to have been a validation record
			test.AssertMarshaledEquals(t, outRecord, tc.ExpectedRecord)
		})
	}
}

// A more concise version of httpSrv() that supports http.go tests
func httpTestSrv(t *testing.T, ipv6 bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	server := httptest.NewUnstartedServer(mux)

	if ipv6 {
		l, err := net.Listen("tcp", "[::1]:0")
		if err != nil {
			panic(fmt.Sprintf("httptest: failed to listen on a port: %v", err))
		}
		server.Listener = l
	}

	server.Start()
	httpPort := getPort(server)

	// A path that always returns an OK response
	mux.HandleFunc("/ok", func(resp http.ResponseWriter, req *http.Request) {
		resp.WriteHeader(http.StatusOK)
		fmt.Fprint(resp, "ok")
	})

	// A path that always times out by sleeping longer than the validation context
	// allows
	mux.HandleFunc("/timeout", func(resp http.ResponseWriter, req *http.Request) {
		time.Sleep(time.Second)
		resp.WriteHeader(http.StatusOK)
		fmt.Fprint(resp, "sorry, I'm a slow server")
	})

	// A path that always redirects to itself, creating a loop that will terminate
	// when detected.
	mux.HandleFunc("/loop", func(resp http.ResponseWriter, req *http.Request) {
		http.Redirect(
			resp,
			req,
			fmt.Sprintf("http://example.com:%d/loop", httpPort),
			http.StatusMovedPermanently)
	})

	// A path that sequentially redirects, creating an incrementing redirect
	// that will terminate when the redirect limit is reached and ensures each
	// URL is different than the last.
	for i := range maxRedirect + 2 {
		mux.HandleFunc(fmt.Sprintf("/max-redirect/%d", i),
			func(resp http.ResponseWriter, req *http.Request) {
				http.Redirect(
					resp,
					req,
					fmt.Sprintf("http://example.com:%d/max-redirect/%d", httpPort, i+1),
					http.StatusMovedPermanently,
				)
			})
	}

	// A path that always redirects to a URL with a non-HTTP/HTTPs protocol scheme
	mux.HandleFunc("/redir-bad-proto", func(resp http.ResponseWriter, req *http.Request) {
		http.Redirect(
			resp,
			req,
			"gopher://example.com",
			http.StatusMovedPermanently,
		)
	})

	// A path that always redirects to a URL with a port other than the configured
	// HTTP/HTTPS port
	mux.HandleFunc("/redir-bad-port", func(resp http.ResponseWriter, req *http.Request) {
		http.Redirect(
			resp,
			req,
			"https://example.com:1987",
			http.StatusMovedPermanently,
		)
	})

	// A path that always redirects to a URL with a bare IP address
	mux.HandleFunc("/redir-bare-ipv4", func(resp http.ResponseWriter, req *http.Request) {
		http.Redirect(
			resp,
			req,
			"http://127.0.0.1/ok",
			http.StatusMovedPermanently,
		)
	})

	mux.HandleFunc("/redir-bare-ipv6", func(resp http.ResponseWriter, req *http.Request) {
		http.Redirect(
			resp,
			req,
			"http://[::1]/ok",
			http.StatusMovedPermanently,
		)
	})

	mux.HandleFunc("/bad-status-code", func(resp http.ResponseWriter, req *http.Request) {
		resp.WriteHeader(http.StatusGone)
		fmt.Fprint(resp, "sorry, I'm gone")
	})

	// A path that always responds with a 303 redirect
	mux.HandleFunc("/303-see-other", func(resp http.ResponseWriter, req *http.Request) {
		http.Redirect(
			resp,
			req,
			"http://example.org/303-see-other",
			http.StatusSeeOther,
		)
	})

	tooLargeBuf := bytes.NewBuffer([]byte{})
	for range maxResponseSize + 10 {
		tooLargeBuf.WriteByte(byte(97))
	}
	mux.HandleFunc("/resp-too-big", func(resp http.ResponseWriter, req *http.Request) {
		resp.WriteHeader(http.StatusOK)
		fmt.Fprint(resp, tooLargeBuf)
	})

	// Create a buffer that starts with invalid UTF8 and is bigger than
	// maxResponseSize
	tooLargeInvalidUTF8 := bytes.NewBuffer([]byte{})
	tooLargeInvalidUTF8.WriteString("f\xffoo")
	tooLargeInvalidUTF8.Write(tooLargeBuf.Bytes())
	// invalid-utf8-body Responds with body that is larger than
	// maxResponseSize and starts with an invalid UTF8 string. This is to
	// test the codepath where invalid UTF8 is converted to valid UTF8
	// that can be passed as an error message via grpc.
	mux.HandleFunc("/invalid-utf8-body", func(resp http.ResponseWriter, req *http.Request) {
		resp.WriteHeader(http.StatusOK)
		fmt.Fprint(resp, tooLargeInvalidUTF8)
	})

	mux.HandleFunc("/redir-path-too-long", func(resp http.ResponseWriter, req *http.Request) {
		http.Redirect(
			resp,
			req,
			"https://example.com/this-is-too-long-01234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789",
			http.StatusMovedPermanently)
	})

	// A path that redirects to an uppercase public suffix (#4215)
	mux.HandleFunc("/redir-uppercase-publicsuffix", func(resp http.ResponseWriter, req *http.Request) {
		http.Redirect(
			resp,
			req,
			"http://example.COM/ok",
			http.StatusMovedPermanently)
	})

	// A path that returns a body containing printf formatting verbs
	mux.HandleFunc("/printf-verbs", func(resp http.ResponseWriter, req *http.Request) {
		resp.WriteHeader(http.StatusOK)
		fmt.Fprint(resp, "%"+"2F.well-known%"+"2F"+tooLargeBuf.String())
	})

	return server
}

type testNetErr struct{}

func (e *testNetErr) Error() string {
	return "testNetErr"
}

func (e *testNetErr) Temporary() bool {
	return false
}

func (e *testNetErr) Timeout() bool {
	return false
}

func TestFallbackErr(t *testing.T) {
	untypedErr := errors.New("the least interesting kind of error")
	berr := berrors.InternalServerError("code violet: class neptune")
	netOpErr := &net.OpError{
		Op:  "siphon",
		Err: fmt.Errorf("port was clogged. please empty packets"),
	}
	netDialOpErr := &net.OpError{
		Op:  "dial",
		Err: fmt.Errorf("your call is important to us - please stay on the line"),
	}
	netErr := &testNetErr{}

	testCases := []struct {
		Name           string
		Err            error
		ExpectFallback bool
	}{
		{
			Name: "Nil error",
			Err:  nil,
		},
		{
			Name: "Standard untyped error",
			Err:  untypedErr,
		},
		{
			Name: "A Boulder error instance",
			Err:  berr,
		},
		{
			Name: "A non-dial net.OpError instance",
			Err:  netOpErr,
		},
		{
			Name:           "A dial net.OpError instance",
			Err:            netDialOpErr,
			ExpectFallback: true,
		},
		{
			Name: "A generic net.Error instance",
			Err:  netErr,
		},
		{
			Name: "A URL error wrapping a standard error",
			Err: &url.Error{
				Op:  "ivy",
				URL: "https://en.wikipedia.org/wiki/Operation_Ivy_(band)",
				Err: errors.New("take warning"),
			},
		},
		{
			Name: "A URL error wrapping a nil error",
			Err: &url.Error{
				Err: nil,
			},
		},
		{
			Name: "A URL error wrapping a Boulder error instance",
			Err: &url.Error{
				Err: berr,
			},
		},
		{
			Name: "A URL error wrapping a non-dial net OpError",
			Err: &url.Error{
				Err: netOpErr,
			},
		},
		{
			Name: "A URL error wrapping a dial net.OpError",
			Err: &url.Error{
				Err: netDialOpErr,
			},
			ExpectFallback: true,
		},
		{
			Name: "A URL error wrapping a generic net Error",
			Err: &url.Error{
				Err: netErr,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			if isFallback := fallbackErr(tc.Err); isFallback != tc.ExpectFallback {
				t.Errorf(
					"Expected fallbackErr for %t to be %v was %v\n",
					tc.Err, tc.ExpectFallback, isFallback)
			}
		})
	}
}

func TestFetchHTTP(t *testing.T) {
	// Create test servers
	testSrvIPv4 := httpTestSrv(t, false)
	defer testSrvIPv4.Close()
	testSrvIPv6 := httpTestSrv(t, true)
	defer testSrvIPv6.Close()

	// Setup VAs. By providing the testSrv to setup the VA will use the testSrv's
	// randomly assigned port as its HTTP port.
	vaIPv4, _ := setup(testSrvIPv4, "", nil, nil)
	vaIPv6, _ := setup(testSrvIPv6, "", nil, nil)

	// We need to know the randomly assigned HTTP port for testcases as well
	httpPortIPv4 := getPort(testSrvIPv4)
	httpPortIPv6 := getPort(testSrvIPv6)

	// For the looped test case we expect one validation record per redirect
	// until boulder detects that a url has been used twice indicating a
	// redirect loop. Because it is hitting the /loop endpoint it will encounter
	// this scenario after the base url and fail on the second time hitting the
	// redirect with a port definition. On i=0 it will encounter the first
	// redirect to the url with a port definition and on i=1 it will encounter
	// the second redirect to the url with the port and get an expected error.
	expectedLoopRecords := []core.ValidationRecord{}
	for i := range 2 {
		// The first request will not have a port # in the URL.
		url := "http://example.com/loop"
		if i != 0 {
			url = fmt.Sprintf("http://example.com:%d/loop", httpPortIPv4)
		}
		expectedLoopRecords = append(expectedLoopRecords,
			core.ValidationRecord{
				Hostname:          "example.com",
				Port:              strconv.Itoa(httpPortIPv4),
				URL:               url,
				AddressesResolved: []netip.Addr{netip.MustParseAddr("127.0.0.1")},
				AddressUsed:       netip.MustParseAddr("127.0.0.1"),
				ResolverAddrs:     []string{"MockClient"},
			})
	}

	// For the too many redirect test case we expect one validation record per
	// redirect up to maxRedirect (inclusive). There is also +1 record for the
	// base lookup, giving a termination criteria of > maxRedirect+1
	expectedTooManyRedirRecords := []core.ValidationRecord{}
	for i := range maxRedirect + 2 {
		// The first request will not have a port # in the URL.
		url := "http://example.com/max-redirect/0"
		if i != 0 {
			url = fmt.Sprintf("http://example.com:%d/max-redirect/%d", httpPortIPv4, i)
		}
		expectedTooManyRedirRecords = append(expectedTooManyRedirRecords,
			core.ValidationRecord{
				Hostname:          "example.com",
				Port:              strconv.Itoa(httpPortIPv4),
				URL:               url,
				AddressesResolved: []netip.Addr{netip.MustParseAddr("127.0.0.1")},
				AddressUsed:       netip.MustParseAddr("127.0.0.1"),
				ResolverAddrs:     []string{"MockClient"},
			})
	}

	expectedTruncatedResp := bytes.NewBuffer([]byte{})
	for range maxResponseSize {
		expectedTruncatedResp.WriteByte(byte(97))
	}

	testCases := []struct {
		Name            string
		IPv6            bool
		Ident           identifier.ACMEIdentifier
		Path            string
		ExpectedBody    string
		ExpectedRecords []core.ValidationRecord
		ExpectedProblem *probs.ProblemDetails
	}{
		{
			Name:  "No IPs for host",
			Ident: identifier.NewDNS("always.invalid"),
			Path:  "/.well-known/whatever",
			ExpectedProblem: probs.DNS(
				"No valid IP addresses found for always.invalid"),
			// There are no validation records in this case because the base record
			// is only constructed once a URL is made.
			ExpectedRecords: nil,
		},
		{
			Name:  "Timeout for host with standard ACME allowed port",
			Ident: identifier.NewDNS("example.com"),
			Path:  "/timeout",
			ExpectedProblem: probs.Connection(
				"127.0.0.1: Fetching http://example.com/timeout: " +
					"Timeout after connect (your server may be slow or overloaded)"),
			ExpectedRecords: []core.ValidationRecord{
				{
					Hostname:          "example.com",
					Port:              strconv.Itoa(httpPortIPv4),
					URL:               "http://example.com/timeout",
					AddressesResolved: []netip.Addr{netip.MustParseAddr("127.0.0.1")},
					AddressUsed:       netip.MustParseAddr("127.0.0.1"),
					ResolverAddrs:     []string{"MockClient"},
				},
			},
		},
		{
			Name:  "Redirect loop",
			Ident: identifier.NewDNS("example.com"),
			Path:  "/loop",
			ExpectedProblem: probs.Connection(fmt.Sprintf(
				"127.0.0.1: Fetching http://example.com:%d/loop: Redirect loop detected", httpPortIPv4)),
			ExpectedRecords: expectedLoopRecords,
		},
		{
			Name:  "Too many redirects",
			Ident: identifier.NewDNS("example.com"),
			Path:  "/max-redirect/0",
			ExpectedProblem: probs.Connection(fmt.Sprintf(
				"127.0.0.1: Fetching http://example.com:%d/max-redirect/12: Too many redirects", httpPortIPv4)),
			ExpectedRecords: expectedTooManyRedirRecords,
		},
		{
			Name:  "Redirect to bad protocol",
			Ident: identifier.NewDNS("example.com"),
			Path:  "/redir-bad-proto",
			ExpectedProblem: probs.Connection(
				"127.0.0.1: Fetching gopher://example.com: Invalid protocol scheme in " +
					`redirect target. Only "http" and "https" protocol schemes ` +
					`are supported, not "gopher"`),
			ExpectedRecords: []core.ValidationRecord{
				{
					Hostname:          "example.com",
					Port:              strconv.Itoa(httpPortIPv4),
					URL:               "http://example.com/redir-bad-proto",
					AddressesResolved: []netip.Addr{netip.MustParseAddr("127.0.0.1")},
					AddressUsed:       netip.MustParseAddr("127.0.0.1"),
					ResolverAddrs:     []string{"MockClient"},
				},
			},
		},
		{
			Name:  "Redirect to bad port",
			Ident: identifier.NewDNS("example.com"),
			Path:  "/redir-bad-port",
			ExpectedProblem: probs.Connection(fmt.Sprintf(
				"127.0.0.1: Fetching https://example.com:1987: Invalid port in redirect target. "+
					"Only ports %d and 443 are supported, not 1987", httpPortIPv4)),
			ExpectedRecords: []core.ValidationRecord{
				{
					Hostname:          "example.com",
					Port:              strconv.Itoa(httpPortIPv4),
					URL:               "http://example.com/redir-bad-port",
					AddressesResolved: []netip.Addr{netip.MustParseAddr("127.0.0.1")},
					AddressUsed:       netip.MustParseAddr("127.0.0.1"),
					ResolverAddrs:     []string{"MockClient"},
				},
			},
		},
		{
			Name:         "Redirect to bare IPv4 address",
			Ident:        identifier.NewDNS("example.com"),
			Path:         "/redir-bare-ipv4",
			ExpectedBody: "ok",
			ExpectedRecords: []core.ValidationRecord{
				{
					Hostname:          "example.com",
					Port:              strconv.Itoa(httpPortIPv4),
					URL:               "http://example.com/redir-bare-ipv4",
					AddressesResolved: []netip.Addr{netip.MustParseAddr("127.0.0.1")},
					AddressUsed:       netip.MustParseAddr("127.0.0.1"),
					ResolverAddrs:     []string{"MockClient"},
				},
				{
					Hostname:          "127.0.0.1",
					Port:              strconv.Itoa(httpPortIPv4),
					URL:               "http://127.0.0.1/ok",
					AddressesResolved: []netip.Addr{netip.MustParseAddr("127.0.0.1")},
					AddressUsed:       netip.MustParseAddr("127.0.0.1"),
				},
			},
		}, {
			Name:         "Redirect to bare IPv6 address",
			IPv6:         true,
			Ident:        identifier.NewDNS("ipv6.localhost"),
			Path:         "/redir-bare-ipv6",
			ExpectedBody: "ok",
			ExpectedRecords: []core.ValidationRecord{
				{
					Hostname:          "ipv6.localhost",
					Port:              strconv.Itoa(httpPortIPv6),
					URL:               "http://ipv6.localhost/redir-bare-ipv6",
					AddressesResolved: []netip.Addr{netip.MustParseAddr("::1")},
					AddressUsed:       netip.MustParseAddr("::1"),
					ResolverAddrs:     []string{"MockClient"},
				},
				{
					Hostname:          "::1",
					Port:              strconv.Itoa(httpPortIPv6),
					URL:               "http://[::1]/ok",
					AddressesResolved: []netip.Addr{netip.MustParseAddr("::1")},
					AddressUsed:       netip.MustParseAddr("::1"),
				},
			},
		},
		{
			Name:  "Redirect to long path",
			Ident: identifier.NewDNS("example.com"),
			Path:  "/redir-path-too-long",
			ExpectedProblem: probs.Connection(
				"127.0.0.1: Fetching https://example.com/this-is-too-long-01234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789: Redirect target too long"),
			ExpectedRecords: []core.ValidationRecord{
				{
					Hostname:          "example.com",
					Port:              strconv.Itoa(httpPortIPv4),
					URL:               "http://example.com/redir-path-too-long",
					AddressesResolved: []netip.Addr{netip.MustParseAddr("127.0.0.1")},
					AddressUsed:       netip.MustParseAddr("127.0.0.1"),
					ResolverAddrs:     []string{"MockClient"},
				},
			},
		},
		{
			Name:  "Wrong HTTP status code",
			Ident: identifier.NewDNS("example.com"),
			Path:  "/bad-status-code",
			ExpectedProblem: probs.Unauthorized(
				"127.0.0.1: Invalid response from http://example.com/bad-status-code: 410"),
			ExpectedRecords: []core.ValidationRecord{
				{
					Hostname:          "example.com",
					Port:              strconv.Itoa(httpPortIPv4),
					URL:               "http://example.com/bad-status-code",
					AddressesResolved: []netip.Addr{netip.MustParseAddr("127.0.0.1")},
					AddressUsed:       netip.MustParseAddr("127.0.0.1"),
					ResolverAddrs:     []string{"MockClient"},
				},
			},
		},
		{
			Name:  "HTTP status code 303 redirect",
			Ident: identifier.NewDNS("example.com"),
			Path:  "/303-see-other",
			ExpectedProblem: probs.Connection(
				"127.0.0.1: Fetching http://example.org/303-see-other: received disallowed redirect status code"),
			ExpectedRecords: []core.ValidationRecord{
				{
					Hostname:          "example.com",
					Port:              strconv.Itoa(httpPortIPv4),
					URL:               "http://example.com/303-see-other",
					AddressesResolved: []netip.Addr{netip.MustParseAddr("127.0.0.1")},
					AddressUsed:       netip.MustParseAddr("127.0.0.1"),
					ResolverAddrs:     []string{"MockClient"},
				},
			},
		},
		{
			Name:  "Response too large",
			Ident: identifier.NewDNS("example.com"),
			Path:  "/resp-too-big",
			ExpectedProblem: probs.Unauthorized(fmt.Sprintf(
				"127.0.0.1: Invalid response from http://example.com/resp-too-big: %q", expectedTruncatedResp.String(),
			)),
			ExpectedRecords: []core.ValidationRecord{
				{
					Hostname:          "example.com",
					Port:              strconv.Itoa(httpPortIPv4),
					URL:               "http://example.com/resp-too-big",
					AddressesResolved: []netip.Addr{netip.MustParseAddr("127.0.0.1")},
					AddressUsed:       netip.MustParseAddr("127.0.0.1"),
					ResolverAddrs:     []string{"MockClient"},
				},
			},
		},
		{
			Name:  "Broken IPv6 only",
			Ident: identifier.NewDNS("ipv6.localhost"),
			Path:  "/ok",
			ExpectedProblem: probs.Connection(
				"::1: Fetching http://ipv6.localhost/ok: Connection refused"),
			ExpectedRecords: []core.ValidationRecord{
				{
					Hostname:          "ipv6.localhost",
					Port:              strconv.Itoa(httpPortIPv4),
					URL:               "http://ipv6.localhost/ok",
					AddressesResolved: []netip.Addr{netip.MustParseAddr("::1")},
					AddressUsed:       netip.MustParseAddr("::1"),
					ResolverAddrs:     []string{"MockClient"},
				},
			},
		},
		{
			Name:         "Dual homed w/ broken IPv6, working IPv4",
			Ident:        identifier.NewDNS("ipv4.and.ipv6.localhost"),
			Path:         "/ok",
			ExpectedBody: "ok",
			ExpectedRecords: []core.ValidationRecord{
				{
					Hostname:          "ipv4.and.ipv6.localhost",
					Port:              strconv.Itoa(httpPortIPv4),
					URL:               "http://ipv4.and.ipv6.localhost/ok",
					AddressesResolved: []netip.Addr{netip.MustParseAddr("::1"), netip.MustParseAddr("127.0.0.1")},
					// The first validation record should have used the IPv6 addr
					AddressUsed:   netip.MustParseAddr("::1"),
					ResolverAddrs: []string{"MockClient"},
				},
				{
					Hostname:          "ipv4.and.ipv6.localhost",
					Port:              strconv.Itoa(httpPortIPv4),
					URL:               "http://ipv4.and.ipv6.localhost/ok",
					AddressesResolved: []netip.Addr{netip.MustParseAddr("::1"), netip.MustParseAddr("127.0.0.1")},
					// The second validation record should have used the IPv4 addr as a fallback
					AddressUsed:   netip.MustParseAddr("127.0.0.1"),
					ResolverAddrs: []string{"MockClient"},
				},
			},
		},
		{
			Name:         "Working IPv4 only",
			Ident:        identifier.NewDNS("example.com"),
			Path:         "/ok",
			ExpectedBody: "ok",
			ExpectedRecords: []core.ValidationRecord{
				{
					Hostname:          "example.com",
					Port:              strconv.Itoa(httpPortIPv4),
					URL:               "http://example.com/ok",
					AddressesResolved: []netip.Addr{netip.MustParseAddr("127.0.0.1")},
					AddressUsed:       netip.MustParseAddr("127.0.0.1"),
					ResolverAddrs:     []string{"MockClient"},
				},
			},
		},
		{
			Name:         "Redirect to uppercase Public Suffix",
			Ident:        identifier.NewDNS("example.com"),
			Path:         "/redir-uppercase-publicsuffix",
			ExpectedBody: "ok",
			ExpectedRecords: []core.ValidationRecord{
				{
					Hostname:          "example.com",
					Port:              strconv.Itoa(httpPortIPv4),
					URL:               "http://example.com/redir-uppercase-publicsuffix",
					AddressesResolved: []netip.Addr{netip.MustParseAddr("127.0.0.1")},
					AddressUsed:       netip.MustParseAddr("127.0.0.1"),
					ResolverAddrs:     []string{"MockClient"},
				},
				{
					Hostname:          "example.com",
					Port:              strconv.Itoa(httpPortIPv4),
					URL:               "http://example.com/ok",
					AddressesResolved: []netip.Addr{netip.MustParseAddr("127.0.0.1")},
					AddressUsed:       netip.MustParseAddr("127.0.0.1"),
					ResolverAddrs:     []string{"MockClient"},
				},
			},
		},
		{
			Name:  "Reflected response body containing printf verbs",
			Ident: identifier.NewDNS("example.com"),
			Path:  "/printf-verbs",
			ExpectedProblem: &probs.ProblemDetails{
				Type: probs.UnauthorizedProblem,
				Detail: fmt.Sprintf("127.0.0.1: Invalid response from http://example.com/printf-verbs: %q",
					("%2F.well-known%2F" + expectedTruncatedResp.String())[:maxResponseSize]),
				HTTPStatus: http.StatusForbidden,
			},
			ExpectedRecords: []core.ValidationRecord{
				{
					Hostname:          "example.com",
					Port:              strconv.Itoa(httpPortIPv4),
					URL:               "http://example.com/printf-verbs",
					AddressesResolved: []netip.Addr{netip.MustParseAddr("127.0.0.1")},
					AddressUsed:       netip.MustParseAddr("127.0.0.1"),
					ResolverAddrs:     []string{"MockClient"},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*500)
			defer cancel()
			var body []byte
			var records []core.ValidationRecord
			var err error
			if tc.IPv6 {
				body, records, err = vaIPv6.processHTTPValidation(ctx, tc.Ident, tc.Path)
			} else {
				body, records, err = vaIPv4.processHTTPValidation(ctx, tc.Ident, tc.Path)
			}
			if tc.ExpectedProblem == nil {
				test.AssertNotError(t, err, "expected nil prob")
			} else {
				test.AssertError(t, err, "expected non-nil prob")
				prob := detailedError(err)
				test.AssertMarshaledEquals(t, prob, tc.ExpectedProblem)
			}
			if tc.ExpectedBody != "" {
				test.AssertEquals(t, string(body), tc.ExpectedBody)
			}
			// in all cases we expect validation records to be present and matching expected
			test.AssertMarshaledEquals(t, records, tc.ExpectedRecords)
		})
	}
}

// All paths that get assigned to tokens MUST be valid tokens
const pathWrongToken = "i6lNAC4lOOLYCl-A08VJt9z_tKYvVk63Dumo8icsBjQ"
const path404 = "404"
const path500 = "500"
const pathFound = "GBq8SwWq3JsbREFdCamk5IX3KLsxW5ULeGs98Ajl_UM"
const pathMoved = "5J4FIMrWNfmvHZo-QpKZngmuhqZGwRm21-oEgUDstJM"
const pathRedirectInvalidPort = "port-redirect"
const pathWait = "wait"
const pathWaitLong = "wait-long"
const pathReLookup = "7e-P57coLM7D3woNTp_xbJrtlkDYy6PWf3mSSbLwCr4"
const pathReLookupInvalid = "re-lookup-invalid"
const pathRedirectToFailingURL = "re-to-failing-url"
const pathLooper = "looper"
const pathValid = "valid"
const rejectUserAgent = "rejectMe"

func httpSrv(t *testing.T, token string, ipv6 bool) *httptest.Server {
	m := http.NewServeMux()

	server := httptest.NewUnstartedServer(m)

	defaultToken := token
	currentToken := defaultToken

	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, path404) {
			t.Logf("HTTPSRV: Got a 404 req\n")
			http.NotFound(w, r)
		} else if strings.HasSuffix(r.URL.Path, path500) {
			t.Logf("HTTPSRV: Got a 500 req\n")
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		} else if strings.HasSuffix(r.URL.Path, pathMoved) {
			t.Logf("HTTPSRV: Got a http.StatusMovedPermanently redirect req\n")
			if currentToken == defaultToken {
				currentToken = pathMoved
			}
			http.Redirect(w, r, pathValid, http.StatusMovedPermanently)
		} else if strings.HasSuffix(r.URL.Path, pathFound) {
			t.Logf("HTTPSRV: Got a http.StatusFound redirect req\n")
			if currentToken == defaultToken {
				currentToken = pathFound
			}
			http.Redirect(w, r, pathMoved, http.StatusFound)
		} else if strings.HasSuffix(r.URL.Path, pathWait) {
			t.Logf("HTTPSRV: Got a wait req\n")
			time.Sleep(time.Second * 3)
		} else if strings.HasSuffix(r.URL.Path, pathWaitLong) {
			t.Logf("HTTPSRV: Got a wait-long req\n")
			time.Sleep(time.Second * 10)
		} else if strings.HasSuffix(r.URL.Path, pathReLookup) {
			t.Logf("HTTPSRV: Got a redirect req to a valid hostname\n")
			if currentToken == defaultToken {
				currentToken = pathReLookup
			}
			port := getPort(server)
			http.Redirect(w, r, fmt.Sprintf("http://other.valid.com:%d/path", port), http.StatusFound)
		} else if strings.HasSuffix(r.URL.Path, pathReLookupInvalid) {
			t.Logf("HTTPSRV: Got a redirect req to an invalid host\n")
			http.Redirect(w, r, "http://invalid.invalid/path", http.StatusFound)
		} else if strings.HasSuffix(r.URL.Path, pathRedirectToFailingURL) {
			t.Logf("HTTPSRV: Redirecting to a URL that will fail\n")
			port := getPort(server)
			http.Redirect(w, r, fmt.Sprintf("http://other.valid.com:%d/%s", port, path500), http.StatusMovedPermanently)
		} else if strings.HasSuffix(r.URL.Path, pathLooper) {
			t.Logf("HTTPSRV: Got a loop req\n")
			http.Redirect(w, r, r.URL.String(), http.StatusMovedPermanently)
		} else if strings.HasSuffix(r.URL.Path, pathRedirectInvalidPort) {
			t.Logf("HTTPSRV: Got a port redirect req\n")
			// Port 8080 is not the VA's httpPort or httpsPort and should be rejected
			http.Redirect(w, r, "http://other.valid.com:8080/path", http.StatusFound)
		} else if r.Header.Get("User-Agent") == rejectUserAgent {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("found trap User-Agent"))
		} else {
			t.Logf("HTTPSRV: Got a valid req\n")
			t.Logf("HTTPSRV: Path = %s\n", r.URL.Path)

			ch := core.Challenge{Token: currentToken}
			keyAuthz, _ := ch.ExpectedKeyAuthorization(accountKey)
			t.Logf("HTTPSRV: Key Authz = '%s%s'\n", keyAuthz, "\\n\\r \\t")

			fmt.Fprint(w, keyAuthz, "\n\r \t")
			currentToken = defaultToken
		}
	})

	if ipv6 {
		l, err := net.Listen("tcp", "[::1]:0")
		if err != nil {
			panic(fmt.Sprintf("httptest: failed to listen on a port: %v", err))
		}
		server.Listener = l
	}

	server.Start()
	return server
}

func TestHTTPBadPort(t *testing.T) {
	hs := httpSrv(t, expectedToken, false)
	defer hs.Close()

	va, _ := setup(hs, "", nil, nil)

	// Pick a random port between 40000 and 65000 - with great certainty we won't
	// have an HTTP server listening on this port and the test will fail as
	// intended
	badPort := 40000 + mrand.IntN(25000)
	va.httpPort = badPort

	_, err := va.validateHTTP01(ctx, identifier.NewDNS("localhost"), expectedToken, expectedKeyAuthorization)
	if err == nil {
		t.Fatalf("Server's down; expected refusal. Where did we connect?")
	}
	prob := detailedError(err)
	test.AssertEquals(t, prob.Type, probs.ConnectionProblem)
	if !strings.Contains(prob.Detail, "Connection refused") {
		t.Errorf("Expected a connection refused error, got %q", prob.Detail)
	}
}

func TestHTTPBadIdentifier(t *testing.T) {
	hs := httpSrv(t, expectedToken, false)
	defer hs.Close()

	va, _ := setup(hs, "", nil, nil)

	_, err := va.validateHTTP01(ctx, identifier.ACMEIdentifier{Type: "smime", Value: "dobber@bad.horse"}, expectedToken, expectedKeyAuthorization)
	if err == nil {
		t.Fatalf("Server accepted a hypothetical S/MIME identifier")
	}
	prob := detailedError(err)
	test.AssertEquals(t, prob.Type, probs.MalformedProblem)
	if !strings.Contains(prob.Detail, "Identifier type for HTTP-01 challenge was not DNS or IP") {
		t.Errorf("Expected an identifier type error, got %q", prob.Detail)
	}
}

func TestHTTPKeyAuthorizationFileMismatch(t *testing.T) {
	m := http.NewServeMux()
	hs := httptest.NewUnstartedServer(m)
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("\xef\xffAABBCC"))
	})
	hs.Start()

	va, _ := setup(hs, "", nil, nil)
	_, err := va.validateHTTP01(ctx, identifier.NewDNS("localhost.com"), expectedToken, expectedKeyAuthorization)

	if err == nil {
		t.Fatalf("Expected validation to fail when file mismatched.")
	}
	expected := fmt.Sprintf(`The key authorization file from the server did not match this challenge. Expected "%s" (got "\xef\xffAABBCC")`, expectedKeyAuthorization)
	if err.Error() != expected {
		t.Errorf("validation failed with %s, expected %s", err, expected)
	}
}

func TestHTTP(t *testing.T) {
	hs := httpSrv(t, expectedToken, false)
	defer hs.Close()

	va, log := setup(hs, "", nil, nil)

	_, err := va.validateHTTP01(ctx, identifier.NewDNS("localhost.com"), expectedToken, expectedKeyAuthorization)
	if err != nil {
		t.Errorf("Unexpected failure in HTTP validation for DNS: %s", err)
	}
	test.AssertEquals(t, len(log.GetAllMatching(`\[AUDIT\] `)), 1)

	log.Clear()
	_, err = va.validateHTTP01(ctx, identifier.NewIP(netip.MustParseAddr("127.0.0.1")), expectedToken, expectedKeyAuthorization)
	if err != nil {
		t.Errorf("Unexpected failure in HTTP validation for IPv4: %s", err)
	}
	test.AssertEquals(t, len(log.GetAllMatching(`\[AUDIT\] `)), 1)

	log.Clear()
	_, err = va.validateHTTP01(ctx, identifier.NewDNS("localhost.com"), path404, ka(path404))
	if err == nil {
		t.Fatalf("Should have found a 404 for the challenge.")
	}
	test.AssertErrorIs(t, err, berrors.Unauthorized)
	test.AssertEquals(t, len(log.GetAllMatching(`\[AUDIT\] `)), 1)

	log.Clear()
	// The "wrong token" will actually be the expectedToken.  It's wrong
	// because it doesn't match pathWrongToken.
	_, err = va.validateHTTP01(ctx, identifier.NewDNS("localhost.com"), pathWrongToken, ka(pathWrongToken))
	if err == nil {
		t.Fatalf("Should have found the wrong token value.")
	}
	prob := detailedError(err)
	test.AssertEquals(t, prob.Type, probs.UnauthorizedProblem)
	test.AssertEquals(t, len(log.GetAllMatching(`\[AUDIT\] `)), 1)

	log.Clear()
	_, err = va.validateHTTP01(ctx, identifier.NewDNS("localhost.com"), pathMoved, ka(pathMoved))
	if err != nil {
		t.Fatalf("Failed to follow http.StatusMovedPermanently redirect")
	}
	redirectValid := `following redirect to host "" url "http://localhost.com/.well-known/acme-challenge/` + pathValid + `"`
	matchedValidRedirect := log.GetAllMatching(redirectValid)
	test.AssertEquals(t, len(matchedValidRedirect), 1)

	log.Clear()
	_, err = va.validateHTTP01(ctx, identifier.NewDNS("localhost.com"), pathFound, ka(pathFound))
	if err != nil {
		t.Fatalf("Failed to follow http.StatusFound redirect")
	}
	redirectMoved := `following redirect to host "" url "http://localhost.com/.well-known/acme-challenge/` + pathMoved + `"`
	matchedMovedRedirect := log.GetAllMatching(redirectMoved)
	test.AssertEquals(t, len(matchedValidRedirect), 1)
	test.AssertEquals(t, len(matchedMovedRedirect), 1)

	_, err = va.validateHTTP01(ctx, identifier.NewDNS("always.invalid"), pathFound, ka(pathFound))
	if err == nil {
		t.Fatalf("Domain name is invalid.")
	}
	prob = detailedError(err)
	test.AssertEquals(t, prob.Type, probs.DNSProblem)
}

func TestHTTPIPv6(t *testing.T) {
	hs := httpSrv(t, expectedToken, true)
	defer hs.Close()

	va, log := setup(hs, "", nil, nil)

	_, err := va.validateHTTP01(ctx, identifier.NewIP(netip.MustParseAddr("::1")), expectedToken, expectedKeyAuthorization)
	if err != nil {
		t.Errorf("Unexpected failure in HTTP validation for IPv6: %s", err)
	}
	test.AssertEquals(t, len(log.GetAllMatching(`\[AUDIT\] `)), 1)
}

func TestHTTPTimeout(t *testing.T) {
	hs := httpSrv(t, expectedToken, false)
	defer hs.Close()

	va, _ := setup(hs, "", nil, nil)

	started := time.Now()
	timeout := 250 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_, err := va.validateHTTP01(ctx, identifier.NewDNS("localhost"), pathWaitLong, ka(pathWaitLong))
	if err == nil {
		t.Fatalf("Connection should've timed out")
	}

	took := time.Since(started)
	// Check that the HTTP connection doesn't return before a timeout, and times
	// out after the expected time
	if took < timeout-200*time.Millisecond {
		t.Fatalf("HTTP timed out before %s: %s with %s", timeout, took, err)
	}
	if took > 2*timeout {
		t.Fatalf("HTTP connection didn't timeout after %s", timeout)
	}
	prob := detailedError(err)
	test.AssertEquals(t, prob.Type, probs.ConnectionProblem)
	test.AssertEquals(t, prob.Detail, "127.0.0.1: Fetching http://localhost/.well-known/acme-challenge/wait-long: Timeout after connect (your server may be slow or overloaded)")
}

func TestHTTPRedirectLookup(t *testing.T) {
	hs := httpSrv(t, expectedToken, false)
	defer hs.Close()
	va, log := setup(hs, "", nil, nil)

	_, err := va.validateHTTP01(ctx, identifier.NewDNS("localhost.com"), pathMoved, ka(pathMoved))
	if err != nil {
		t.Fatalf("Unexpected failure in redirect (%s): %s", pathMoved, err)
	}
	redirectValid := `following redirect to host "" url "http://localhost.com/.well-known/acme-challenge/` + pathValid + `"`
	matchedValidRedirect := log.GetAllMatching(redirectValid)
	test.AssertEquals(t, len(matchedValidRedirect), 1)
	test.AssertEquals(t, len(log.GetAllMatching(`Resolved addresses for localhost.com: \[127.0.0.1\]`)), 2)

	log.Clear()
	_, err = va.validateHTTP01(ctx, identifier.NewDNS("localhost.com"), pathFound, ka(pathFound))
	if err != nil {
		t.Fatalf("Unexpected failure in redirect (%s): %s", pathFound, err)
	}
	redirectMoved := `following redirect to host "" url "http://localhost.com/.well-known/acme-challenge/` + pathMoved + `"`
	matchedMovedRedirect := log.GetAllMatching(redirectMoved)
	test.AssertEquals(t, len(matchedMovedRedirect), 1)
	test.AssertEquals(t, len(log.GetAllMatching(`Resolved addresses for localhost.com: \[127.0.0.1\]`)), 3)

	log.Clear()
	_, err = va.validateHTTP01(ctx, identifier.NewDNS("localhost.com"), pathReLookupInvalid, ka(pathReLookupInvalid))
	test.AssertError(t, err, "error for pathReLookupInvalid should not be nil")
	test.AssertEquals(t, len(log.GetAllMatching(`Resolved addresses for localhost.com: \[127.0.0.1\]`)), 1)
	prob := detailedError(err)
	test.AssertDeepEquals(t, prob, probs.Connection(`127.0.0.1: Fetching http://invalid.invalid/path: Invalid host in redirect target, must end in IANA registered TLD`))

	log.Clear()
	_, err = va.validateHTTP01(ctx, identifier.NewDNS("localhost.com"), pathReLookup, ka(pathReLookup))
	if err != nil {
		t.Fatalf("Unexpected error in redirect (%s): %s", pathReLookup, err)
	}
	redirectPattern := `following redirect to host "" url "http://other.valid.com:\d+/path"`
	test.AssertEquals(t, len(log.GetAllMatching(redirectPattern)), 1)
	test.AssertEquals(t, len(log.GetAllMatching(`Resolved addresses for localhost.com: \[127.0.0.1\]`)), 1)
	test.AssertEquals(t, len(log.GetAllMatching(`Resolved addresses for other.valid.com: \[127.0.0.1\]`)), 1)

	log.Clear()
	_, err = va.validateHTTP01(ctx, identifier.NewDNS("localhost.com"), pathRedirectInvalidPort, ka(pathRedirectInvalidPort))
	test.AssertNotNil(t, err, "error for pathRedirectInvalidPort should not be nil")
	prob = detailedError(err)
	test.AssertEquals(t, prob.Detail, fmt.Sprintf(
		"127.0.0.1: Fetching http://other.valid.com:8080/path: Invalid port in redirect target. "+
			"Only ports %d and %d are supported, not 8080", va.httpPort, va.httpsPort))

	// This case will redirect from a valid host to a host that is throwing
	// HTTP 500 errors. The test case is ensuring that the connection error
	// is referencing the redirected to host, instead of the original host.
	log.Clear()
	_, err = va.validateHTTP01(ctx, identifier.NewDNS("localhost.com"), pathRedirectToFailingURL, ka(pathRedirectToFailingURL))
	test.AssertNotNil(t, err, "err should not be nil")
	prob = detailedError(err)
	test.AssertDeepEquals(t, prob,
		probs.Unauthorized(
			fmt.Sprintf("127.0.0.1: Invalid response from http://other.valid.com:%d/500: 500",
				va.httpPort)))
}

func TestHTTPRedirectLoop(t *testing.T) {
	hs := httpSrv(t, expectedToken, false)
	defer hs.Close()
	va, _ := setup(hs, "", nil, nil)

	_, prob := va.validateHTTP01(ctx, identifier.NewDNS("localhost"), "looper", ka("looper"))
	if prob == nil {
		t.Fatalf("Challenge should have failed for looper")
	}
}

func TestHTTPRedirectUserAgent(t *testing.T) {
	hs := httpSrv(t, expectedToken, false)
	defer hs.Close()
	va, _ := setup(hs, "", nil, nil)
	va.userAgent = rejectUserAgent

	_, prob := va.validateHTTP01(ctx, identifier.NewDNS("localhost"), pathMoved, ka(pathMoved))
	if prob == nil {
		t.Fatalf("Challenge with rejectUserAgent should have failed (%s).", pathMoved)
	}

	_, prob = va.validateHTTP01(ctx, identifier.NewDNS("localhost"), pathFound, ka(pathFound))
	if prob == nil {
		t.Fatalf("Challenge with rejectUserAgent should have failed (%s).", pathFound)
	}
}

func getPort(hs *httptest.Server) int {
	url, err := url.Parse(hs.URL)
	if err != nil {
		panic(fmt.Sprintf("Failed to parse hs URL: %q - %s", hs.URL, err.Error()))
	}
	_, portString, err := net.SplitHostPort(url.Host)
	if err != nil {
		panic(fmt.Sprintf("Failed to split hs URL host: %q - %s", url.Host, err.Error()))
	}
	port, err := strconv.ParseInt(portString, 10, 64)
	if err != nil {
		panic(fmt.Sprintf("Failed to parse hs URL port: %q - %s", portString, err.Error()))
	}
	return int(port)
}

func TestValidateHTTP(t *testing.T) {
	token := core.NewToken()

	hs := httpSrv(t, token, false)
	defer hs.Close()

	va, _ := setup(hs, "", nil, nil)

	_, prob := va.validateHTTP01(ctx, identifier.NewDNS("localhost"), token, ka(token))
	test.Assert(t, prob == nil, "validation failed")
}

func TestLimitedReader(t *testing.T) {
	token := core.NewToken()

	hs := httpSrv(t, "012345\xff67890123456789012345678901234567890123456789012345678901234567890123456789", false)
	va, _ := setup(hs, "", nil, nil)
	defer hs.Close()

	_, err := va.validateHTTP01(ctx, identifier.NewDNS("localhost"), token, ka(token))

	prob := detailedError(err)
	test.AssertEquals(t, prob.Type, probs.UnauthorizedProblem)
	test.Assert(t, strings.HasPrefix(prob.Detail, "127.0.0.1: Invalid response from "),
		"Expected failure due to truncation")

	if !utf8.ValidString(err.Error()) {
		t.Errorf("Problem Detail contained an invalid UTF-8 string")
	}
}

type hostHeaderHandler struct {
	host string
}

func (handler *hostHeaderHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	handler.host = req.Host
}

// TestHTTPHostHeader tests compliance with RFC 8555, Sec. 8.3 & RFC 8738, Sec.
// 5.
func TestHTTPHostHeader(t *testing.T) {
	testCases := []struct {
		Name  string
		Ident identifier.ACMEIdentifier
		IPv6  bool
		want  string
	}{
		{
			Name:  "DNS name",
			Ident: identifier.NewDNS("example.com"),
			want:  "example.com",
		},
		{
			Name:  "IPv4 address",
			Ident: identifier.NewIP(netip.MustParseAddr("127.0.0.1")),
			want:  "127.0.0.1",
		},
		{
			Name:  "IPv6 address",
			Ident: identifier.NewIP(netip.MustParseAddr("::1")),
			IPv6:  true,
			want:  "[::1]",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*500)
			defer cancel()

			handler := hostHeaderHandler{}
			testSrv := httptest.NewUnstartedServer(&handler)

			if tc.IPv6 {
				l, err := net.Listen("tcp", "[::1]:0")
				if err != nil {
					panic(fmt.Sprintf("httptest: failed to listen on a port: %v", err))
				}
				testSrv.Listener = l
			}

			testSrv.Start()
			defer testSrv.Close()

			// Setup VA. By providing the testSrv to setup the VA will use the
			// testSrv's randomly assigned port as its HTTP port.
			va, _ := setup(testSrv, "", nil, nil)

			var got string
			_, _, _ = va.processHTTPValidation(ctx, tc.Ident, "/ok")
			got = handler.host
			if got != tc.want {
				t.Errorf("Got host %#v, but want %#v", got, tc.want)
			}
		})
	}
}
