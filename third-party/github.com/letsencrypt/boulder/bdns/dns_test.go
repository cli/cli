package bdns

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmhodges/clock"
	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"

	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/test"
)

const dnsLoopbackAddr = "127.0.0.1:4053"

func mockDNSQuery(w http.ResponseWriter, httpReq *http.Request) {
	if httpReq.Header.Get("Content-Type") != "application/dns-message" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "client didn't send Content-Type: application/dns-message")
	}
	if httpReq.Header.Get("Accept") != "application/dns-message" {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "client didn't accept Content-Type: application/dns-message")
	}

	requestBody, err := io.ReadAll(httpReq.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "reading body: %s", err)
	}
	httpReq.Body.Close()

	r := new(dns.Msg)
	err = r.Unpack(requestBody)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "unpacking request: %s", err)
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Compress = false

	appendAnswer := func(rr dns.RR) {
		m.Answer = append(m.Answer, rr)
	}
	for _, q := range r.Question {
		q.Name = strings.ToLower(q.Name)
		if q.Name == "servfail.com." || q.Name == "servfailexception.example.com" {
			m.Rcode = dns.RcodeServerFailure
			break
		}
		switch q.Qtype {
		case dns.TypeSOA:
			record := new(dns.SOA)
			record.Hdr = dns.RR_Header{Name: "letsencrypt.org.", Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: 0}
			record.Ns = "ns.letsencrypt.org."
			record.Mbox = "master.letsencrypt.org."
			record.Serial = 1
			record.Refresh = 1
			record.Retry = 1
			record.Expire = 1
			record.Minttl = 1
			appendAnswer(record)
		case dns.TypeAAAA:
			if q.Name == "v6.letsencrypt.org." {
				record := new(dns.AAAA)
				record.Hdr = dns.RR_Header{Name: "v6.letsencrypt.org.", Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 0}
				record.AAAA = net.ParseIP("2602:80a:6000:abad:cafe::1")
				appendAnswer(record)
			}
			if q.Name == "dualstack.letsencrypt.org." {
				record := new(dns.AAAA)
				record.Hdr = dns.RR_Header{Name: "dualstack.letsencrypt.org.", Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 0}
				record.AAAA = net.ParseIP("2602:80a:6000:abad:cafe::1")
				appendAnswer(record)
			}
			if q.Name == "v4error.letsencrypt.org." {
				record := new(dns.AAAA)
				record.Hdr = dns.RR_Header{Name: "v4error.letsencrypt.org.", Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 0}
				record.AAAA = net.ParseIP("2602:80a:6000:abad:cafe::1")
				appendAnswer(record)
			}
			if q.Name == "v6error.letsencrypt.org." {
				m.SetRcode(r, dns.RcodeNotImplemented)
			}
			if q.Name == "nxdomain.letsencrypt.org." {
				m.SetRcode(r, dns.RcodeNameError)
			}
			if q.Name == "dualstackerror.letsencrypt.org." {
				m.SetRcode(r, dns.RcodeNotImplemented)
			}
		case dns.TypeA:
			if q.Name == "cps.letsencrypt.org." {
				record := new(dns.A)
				record.Hdr = dns.RR_Header{Name: "cps.letsencrypt.org.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 0}
				record.A = net.ParseIP("64.112.117.1")
				appendAnswer(record)
			}
			if q.Name == "dualstack.letsencrypt.org." {
				record := new(dns.A)
				record.Hdr = dns.RR_Header{Name: "dualstack.letsencrypt.org.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 0}
				record.A = net.ParseIP("64.112.117.1")
				appendAnswer(record)
			}
			if q.Name == "v6error.letsencrypt.org." {
				record := new(dns.A)
				record.Hdr = dns.RR_Header{Name: "dualstack.letsencrypt.org.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 0}
				record.A = net.ParseIP("64.112.117.1")
				appendAnswer(record)
			}
			if q.Name == "v4error.letsencrypt.org." {
				m.SetRcode(r, dns.RcodeNotImplemented)
			}
			if q.Name == "nxdomain.letsencrypt.org." {
				m.SetRcode(r, dns.RcodeNameError)
			}
			if q.Name == "dualstackerror.letsencrypt.org." {
				m.SetRcode(r, dns.RcodeRefused)
			}
		case dns.TypeCNAME:
			if q.Name == "cname.letsencrypt.org." {
				record := new(dns.CNAME)
				record.Hdr = dns.RR_Header{Name: "cname.letsencrypt.org.", Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 30}
				record.Target = "cps.letsencrypt.org."
				appendAnswer(record)
			}
			if q.Name == "cname.example.com." {
				record := new(dns.CNAME)
				record.Hdr = dns.RR_Header{Name: "cname.example.com.", Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 30}
				record.Target = "CAA.example.com."
				appendAnswer(record)
			}
		case dns.TypeDNAME:
			if q.Name == "dname.letsencrypt.org." {
				record := new(dns.DNAME)
				record.Hdr = dns.RR_Header{Name: "dname.letsencrypt.org.", Rrtype: dns.TypeDNAME, Class: dns.ClassINET, Ttl: 30}
				record.Target = "cps.letsencrypt.org."
				appendAnswer(record)
			}
		case dns.TypeCAA:
			if q.Name == "bracewel.net." || q.Name == "caa.example.com." {
				record := new(dns.CAA)
				record.Hdr = dns.RR_Header{Name: q.Name, Rrtype: dns.TypeCAA, Class: dns.ClassINET, Ttl: 0}
				record.Tag = "issue"
				record.Value = "letsencrypt.org"
				record.Flag = 1
				appendAnswer(record)
			}
			if q.Name == "cname.example.com." {
				record := new(dns.CAA)
				record.Hdr = dns.RR_Header{Name: "caa.example.com.", Rrtype: dns.TypeCAA, Class: dns.ClassINET, Ttl: 0}
				record.Tag = "issue"
				record.Value = "letsencrypt.org"
				record.Flag = 1
				appendAnswer(record)
			}
			if q.Name == "gonetld." {
				m.SetRcode(r, dns.RcodeNameError)
			}
		case dns.TypeTXT:
			if q.Name == "split-txt.letsencrypt.org." {
				record := new(dns.TXT)
				record.Hdr = dns.RR_Header{Name: "split-txt.letsencrypt.org.", Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 0}
				record.Txt = []string{"a", "b", "c"}
				appendAnswer(record)
			} else {
				auth := new(dns.SOA)
				auth.Hdr = dns.RR_Header{Name: "letsencrypt.org.", Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: 0}
				auth.Ns = "ns.letsencrypt.org."
				auth.Mbox = "master.letsencrypt.org."
				auth.Serial = 1
				auth.Refresh = 1
				auth.Retry = 1
				auth.Expire = 1
				auth.Minttl = 1
				m.Ns = append(m.Ns, auth)
			}
			if q.Name == "nxdomain.letsencrypt.org." {
				m.SetRcode(r, dns.RcodeNameError)
			}
		}
	}

	body, err := m.Pack()
	if err != nil {
		fmt.Fprintf(os.Stderr, "packing reply: %s\n", err)
	}
	w.Header().Set("Content-Type", "application/dns-message")
	_, err = w.Write(body)
	if err != nil {
		panic(err) // running tests, so panic is OK
	}
}

func serveLoopResolver(stopChan chan bool) {
	m := http.NewServeMux()
	m.HandleFunc("/dns-query", mockDNSQuery)
	httpServer := &http.Server{
		Addr:         dnsLoopbackAddr,
		Handler:      m,
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
	}
	go func() {
		cert := "../test/certs/ipki/localhost/cert.pem"
		key := "../test/certs/ipki/localhost/key.pem"
		err := httpServer.ListenAndServeTLS(cert, key)
		if err != nil {
			fmt.Println(err)
		}
	}()
	go func() {
		<-stopChan
		err := httpServer.Shutdown(context.Background())
		if err != nil {
			log.Fatal(err)
		}
	}()
}

func pollServer() {
	backoff := 200 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(backoff)

	for {
		select {
		case <-ctx.Done():
			fmt.Fprintln(os.Stderr, "Timeout reached while testing for the dns server to come up")
			os.Exit(1)
		case <-ticker.C:
			conn, _ := dns.DialTimeout("udp", dnsLoopbackAddr, backoff)
			if conn != nil {
				_ = conn.Close()
				return
			}
		}
	}
}

// tlsConfig is used for the TLS config of client instances that talk to the
// DoH server set up in TestMain.
var tlsConfig *tls.Config

func TestMain(m *testing.M) {
	root, err := os.ReadFile("../test/certs/ipki/minica.pem")
	if err != nil {
		log.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(root)
	tlsConfig = &tls.Config{
		RootCAs: pool,
	}

	stop := make(chan bool, 1)
	serveLoopResolver(stop)
	pollServer()
	ret := m.Run()
	stop <- true
	os.Exit(ret)
}

func TestDNSNoServers(t *testing.T) {
	staticProvider, err := NewStaticProvider([]string{})
	test.AssertNotError(t, err, "Got error creating StaticProvider")

	obj := New(time.Hour, staticProvider, metrics.NoopRegisterer, clock.NewFake(), 1, "", blog.UseMock(), tlsConfig)

	_, resolvers, err := obj.LookupHost(context.Background(), "letsencrypt.org")
	test.AssertEquals(t, len(resolvers), 0)
	test.AssertError(t, err, "No servers")

	_, _, err = obj.LookupTXT(context.Background(), "letsencrypt.org")
	test.AssertError(t, err, "No servers")

	_, _, _, err = obj.LookupCAA(context.Background(), "letsencrypt.org")
	test.AssertError(t, err, "No servers")
}

func TestDNSOneServer(t *testing.T) {
	staticProvider, err := NewStaticProvider([]string{dnsLoopbackAddr})
	test.AssertNotError(t, err, "Got error creating StaticProvider")

	obj := New(time.Second*10, staticProvider, metrics.NoopRegisterer, clock.NewFake(), 1, "", blog.UseMock(), tlsConfig)

	_, resolvers, err := obj.LookupHost(context.Background(), "cps.letsencrypt.org")
	test.AssertEquals(t, len(resolvers), 2)
	slices.Sort(resolvers)
	test.AssertDeepEquals(t, resolvers, ResolverAddrs{"A:127.0.0.1:4053", "AAAA:127.0.0.1:4053"})
	test.AssertNotError(t, err, "No message")
}

func TestDNSDuplicateServers(t *testing.T) {
	staticProvider, err := NewStaticProvider([]string{dnsLoopbackAddr, dnsLoopbackAddr})
	test.AssertNotError(t, err, "Got error creating StaticProvider")

	obj := New(time.Second*10, staticProvider, metrics.NoopRegisterer, clock.NewFake(), 1, "", blog.UseMock(), tlsConfig)

	_, resolvers, err := obj.LookupHost(context.Background(), "cps.letsencrypt.org")
	test.AssertEquals(t, len(resolvers), 2)
	slices.Sort(resolvers)
	test.AssertDeepEquals(t, resolvers, ResolverAddrs{"A:127.0.0.1:4053", "AAAA:127.0.0.1:4053"})
	test.AssertNotError(t, err, "No message")
}

func TestDNSServFail(t *testing.T) {
	staticProvider, err := NewStaticProvider([]string{dnsLoopbackAddr})
	test.AssertNotError(t, err, "Got error creating StaticProvider")

	obj := New(time.Second*10, staticProvider, metrics.NoopRegisterer, clock.NewFake(), 1, "", blog.UseMock(), tlsConfig)
	bad := "servfail.com"

	_, _, err = obj.LookupTXT(context.Background(), bad)
	test.AssertError(t, err, "LookupTXT didn't return an error")

	_, _, err = obj.LookupHost(context.Background(), bad)
	test.AssertError(t, err, "LookupHost didn't return an error")

	emptyCaa, _, _, err := obj.LookupCAA(context.Background(), bad)
	test.Assert(t, len(emptyCaa) == 0, "Query returned non-empty list of CAA records")
	test.AssertError(t, err, "LookupCAA should have returned an error")
}

func TestDNSLookupTXT(t *testing.T) {
	staticProvider, err := NewStaticProvider([]string{dnsLoopbackAddr})
	test.AssertNotError(t, err, "Got error creating StaticProvider")

	obj := New(time.Second*10, staticProvider, metrics.NoopRegisterer, clock.NewFake(), 1, "", blog.UseMock(), tlsConfig)

	a, _, err := obj.LookupTXT(context.Background(), "letsencrypt.org")
	t.Logf("A: %v", a)
	test.AssertNotError(t, err, "No message")

	a, _, err = obj.LookupTXT(context.Background(), "split-txt.letsencrypt.org")
	t.Logf("A: %v ", a)
	test.AssertNotError(t, err, "No message")
	test.AssertEquals(t, len(a), 1)
	test.AssertEquals(t, a[0], "abc")
}

// TODO(#8213): Convert this to a table test.
func TestDNSLookupHost(t *testing.T) {
	staticProvider, err := NewStaticProvider([]string{dnsLoopbackAddr})
	test.AssertNotError(t, err, "Got error creating StaticProvider")

	obj := New(time.Second*10, staticProvider, metrics.NoopRegisterer, clock.NewFake(), 1, "", blog.UseMock(), tlsConfig)

	ip, resolvers, err := obj.LookupHost(context.Background(), "servfail.com")
	t.Logf("servfail.com - IP: %s, Err: %s", ip, err)
	test.AssertError(t, err, "Server failure")
	test.Assert(t, len(ip) == 0, "Should not have IPs")
	slices.Sort(resolvers)
	test.AssertDeepEquals(t, resolvers, ResolverAddrs{"A:127.0.0.1:4053", "AAAA:127.0.0.1:4053"})

	ip, resolvers, err = obj.LookupHost(context.Background(), "nonexistent.letsencrypt.org")
	t.Logf("nonexistent.letsencrypt.org - IP: %s, Err: %s", ip, err)
	test.AssertError(t, err, "No valid A or AAAA records should error")
	test.Assert(t, len(ip) == 0, "Should not have IPs")
	slices.Sort(resolvers)
	test.AssertDeepEquals(t, resolvers, ResolverAddrs{"A:127.0.0.1:4053", "AAAA:127.0.0.1:4053"})

	// Single IPv4 address
	ip, resolvers, err = obj.LookupHost(context.Background(), "cps.letsencrypt.org")
	t.Logf("cps.letsencrypt.org - IP: %s, Err: %s", ip, err)
	test.AssertNotError(t, err, "Not an error to exist")
	test.Assert(t, len(ip) == 1, "Should have IP")
	slices.Sort(resolvers)
	test.AssertDeepEquals(t, resolvers, ResolverAddrs{"A:127.0.0.1:4053", "AAAA:127.0.0.1:4053"})
	ip, resolvers, err = obj.LookupHost(context.Background(), "cps.letsencrypt.org")
	t.Logf("cps.letsencrypt.org - IP: %s, Err: %s", ip, err)
	test.AssertNotError(t, err, "Not an error to exist")
	test.Assert(t, len(ip) == 1, "Should have IP")
	slices.Sort(resolvers)
	test.AssertDeepEquals(t, resolvers, ResolverAddrs{"A:127.0.0.1:4053", "AAAA:127.0.0.1:4053"})

	// Single IPv6 address
	ip, resolvers, err = obj.LookupHost(context.Background(), "v6.letsencrypt.org")
	t.Logf("v6.letsencrypt.org - IP: %s, Err: %s", ip, err)
	test.AssertNotError(t, err, "Not an error to exist")
	test.Assert(t, len(ip) == 1, "Should not have IPs")
	slices.Sort(resolvers)
	test.AssertDeepEquals(t, resolvers, ResolverAddrs{"A:127.0.0.1:4053", "AAAA:127.0.0.1:4053"})

	// Both IPv6 and IPv4 address
	ip, resolvers, err = obj.LookupHost(context.Background(), "dualstack.letsencrypt.org")
	t.Logf("dualstack.letsencrypt.org - IP: %s, Err: %s", ip, err)
	test.AssertNotError(t, err, "Not an error to exist")
	test.Assert(t, len(ip) == 2, "Should have 2 IPs")
	expected := netip.MustParseAddr("64.112.117.1")
	test.Assert(t, ip[0] == expected, "wrong ipv4 address")
	expected = netip.MustParseAddr("2602:80a:6000:abad:cafe::1")
	test.Assert(t, ip[1] == expected, "wrong ipv6 address")
	slices.Sort(resolvers)
	test.AssertDeepEquals(t, resolvers, ResolverAddrs{"A:127.0.0.1:4053", "AAAA:127.0.0.1:4053"})

	// IPv6 error, IPv4 success
	ip, resolvers, err = obj.LookupHost(context.Background(), "v6error.letsencrypt.org")
	t.Logf("v6error.letsencrypt.org - IP: %s, Err: %s", ip, err)
	test.AssertNotError(t, err, "Not an error to exist")
	test.Assert(t, len(ip) == 1, "Should have 1 IP")
	expected = netip.MustParseAddr("64.112.117.1")
	test.Assert(t, ip[0] == expected, "wrong ipv4 address")
	slices.Sort(resolvers)
	test.AssertDeepEquals(t, resolvers, ResolverAddrs{"A:127.0.0.1:4053", "AAAA:127.0.0.1:4053"})

	// IPv6 success, IPv4 error
	ip, resolvers, err = obj.LookupHost(context.Background(), "v4error.letsencrypt.org")
	t.Logf("v4error.letsencrypt.org - IP: %s, Err: %s", ip, err)
	test.AssertNotError(t, err, "Not an error to exist")
	test.Assert(t, len(ip) == 1, "Should have 1 IP")
	expected = netip.MustParseAddr("2602:80a:6000:abad:cafe::1")
	test.Assert(t, ip[0] == expected, "wrong ipv6 address")
	slices.Sort(resolvers)
	test.AssertDeepEquals(t, resolvers, ResolverAddrs{"A:127.0.0.1:4053", "AAAA:127.0.0.1:4053"})

	// IPv6 error, IPv4 error
	// Should return both the IPv4 error (Refused) and the IPv6 error (NotImplemented)
	hostname := "dualstackerror.letsencrypt.org"
	ip, resolvers, err = obj.LookupHost(context.Background(), hostname)
	t.Logf("%s - IP: %s, Err: %s", hostname, ip, err)
	test.AssertError(t, err, "Should be an error")
	test.AssertContains(t, err.Error(), "REFUSED looking up A for")
	test.AssertContains(t, err.Error(), "NOTIMP looking up AAAA for")
	slices.Sort(resolvers)
	test.AssertDeepEquals(t, resolvers, ResolverAddrs{"A:127.0.0.1:4053", "AAAA:127.0.0.1:4053"})
}

func TestDNSNXDOMAIN(t *testing.T) {
	staticProvider, err := NewStaticProvider([]string{dnsLoopbackAddr})
	test.AssertNotError(t, err, "Got error creating StaticProvider")

	obj := New(time.Second*10, staticProvider, metrics.NoopRegisterer, clock.NewFake(), 1, "", blog.UseMock(), tlsConfig)

	hostname := "nxdomain.letsencrypt.org"
	_, _, err = obj.LookupHost(context.Background(), hostname)
	test.AssertContains(t, err.Error(), "NXDOMAIN looking up A for")
	test.AssertContains(t, err.Error(), "NXDOMAIN looking up AAAA for")

	_, _, err = obj.LookupTXT(context.Background(), hostname)
	expected := Error{dns.TypeTXT, hostname, nil, dns.RcodeNameError, nil}
	test.AssertDeepEquals(t, err, expected)
}

func TestDNSLookupCAA(t *testing.T) {
	staticProvider, err := NewStaticProvider([]string{dnsLoopbackAddr})
	test.AssertNotError(t, err, "Got error creating StaticProvider")

	obj := New(time.Second*10, staticProvider, metrics.NoopRegisterer, clock.NewFake(), 1, "", blog.UseMock(), tlsConfig)
	removeIDExp := regexp.MustCompile(" id: [[:digit:]]+")

	caas, resp, resolvers, err := obj.LookupCAA(context.Background(), "bracewel.net")
	test.AssertNotError(t, err, "CAA lookup failed")
	test.Assert(t, len(caas) > 0, "Should have CAA records")
	test.AssertEquals(t, len(resolvers), 1)
	test.AssertDeepEquals(t, resolvers, ResolverAddrs{"127.0.0.1:4053"})
	expectedResp := `;; opcode: QUERY, status: NOERROR, id: XXXX
;; flags: qr rd; QUERY: 1, ANSWER: 1, AUTHORITY: 0, ADDITIONAL: 0

;; QUESTION SECTION:
;bracewel.net.	IN	 CAA

;; ANSWER SECTION:
bracewel.net.	0	IN	CAA	1 issue "letsencrypt.org"
`
	test.AssertEquals(t, removeIDExp.ReplaceAllString(resp, " id: XXXX"), expectedResp)

	caas, resp, resolvers, err = obj.LookupCAA(context.Background(), "nonexistent.letsencrypt.org")
	test.AssertNotError(t, err, "CAA lookup failed")
	test.Assert(t, len(caas) == 0, "Shouldn't have CAA records")
	test.AssertEquals(t, resolvers[0], "127.0.0.1:4053")
	expectedResp = ""
	test.AssertEquals(t, resp, expectedResp)

	caas, resp, resolvers, err = obj.LookupCAA(context.Background(), "nxdomain.letsencrypt.org")
	slices.Sort(resolvers)
	test.AssertNotError(t, err, "CAA lookup failed")
	test.Assert(t, len(caas) == 0, "Shouldn't have CAA records")
	test.AssertEquals(t, resolvers[0], "127.0.0.1:4053")
	expectedResp = ""
	test.AssertEquals(t, resp, expectedResp)

	caas, resp, resolvers, err = obj.LookupCAA(context.Background(), "cname.example.com")
	test.AssertNotError(t, err, "CAA lookup failed")
	test.Assert(t, len(caas) > 0, "Should follow CNAME to find CAA")
	test.AssertEquals(t, resolvers[0], "127.0.0.1:4053")
	expectedResp = `;; opcode: QUERY, status: NOERROR, id: XXXX
;; flags: qr rd; QUERY: 1, ANSWER: 1, AUTHORITY: 0, ADDITIONAL: 0

;; QUESTION SECTION:
;cname.example.com.	IN	 CAA

;; ANSWER SECTION:
caa.example.com.	0	IN	CAA	1 issue "letsencrypt.org"
`
	test.AssertEquals(t, removeIDExp.ReplaceAllString(resp, " id: XXXX"), expectedResp)

	_, _, resolvers, err = obj.LookupCAA(context.Background(), "gonetld")
	test.AssertError(t, err, "should fail for TLD NXDOMAIN")
	test.AssertContains(t, err.Error(), "NXDOMAIN")
	test.AssertEquals(t, resolvers[0], "127.0.0.1:4053")
}

type testExchanger struct {
	sync.Mutex
	count int
	errs  []error
}

var errTooManyRequests = errors.New("too many requests")

func (te *testExchanger) Exchange(m *dns.Msg, a string) (*dns.Msg, time.Duration, error) {
	te.Lock()
	defer te.Unlock()
	msg := &dns.Msg{
		MsgHdr: dns.MsgHdr{Rcode: dns.RcodeSuccess},
	}
	if len(te.errs) <= te.count {
		return nil, 0, errTooManyRequests
	}
	err := te.errs[te.count]
	te.count++

	return msg, 2 * time.Millisecond, err
}

func TestRetry(t *testing.T) {
	isTempErr := &url.Error{Op: "read", Err: tempError(true)}
	nonTempErr := &url.Error{Op: "read", Err: tempError(false)}
	servFailError := errors.New("DNS problem: server failure at resolver looking up TXT for example.com")
	type testCase struct {
		name              string
		maxTries          int
		te                *testExchanger
		expected          error
		expectedCount     int
		metricsAllRetries float64
	}
	tests := []*testCase{
		// The success on first try case
		{
			name:     "success",
			maxTries: 3,
			te: &testExchanger{
				errs: []error{nil},
			},
			expected:      nil,
			expectedCount: 1,
		},
		// Immediate non-OpError, error returns immediately
		{
			name:     "non-operror",
			maxTries: 3,
			te: &testExchanger{
				errs: []error{errors.New("nope")},
			},
			expected:      servFailError,
			expectedCount: 1,
		},
		// Temporary err, then non-OpError stops at two tries
		{
			name:     "err-then-non-operror",
			maxTries: 3,
			te: &testExchanger{
				errs: []error{isTempErr, errors.New("nope")},
			},
			expected:      servFailError,
			expectedCount: 2,
		},
		// Temporary error given always
		{
			name:     "persistent-temp-error",
			maxTries: 3,
			te: &testExchanger{
				errs: []error{
					isTempErr,
					isTempErr,
					isTempErr,
				},
			},
			expected:          servFailError,
			expectedCount:     3,
			metricsAllRetries: 1,
		},
		// Even with maxTries at 0, we should still let a single request go
		// through
		{
			name:     "zero-maxtries",
			maxTries: 0,
			te: &testExchanger{
				errs: []error{nil},
			},
			expected:      nil,
			expectedCount: 1,
		},
		// Temporary error given just once causes two tries
		{
			name:     "single-temp-error",
			maxTries: 3,
			te: &testExchanger{
				errs: []error{
					isTempErr,
					nil,
				},
			},
			expected:      nil,
			expectedCount: 2,
		},
		// Temporary error given twice causes three tries
		{
			name:     "double-temp-error",
			maxTries: 3,
			te: &testExchanger{
				errs: []error{
					isTempErr,
					isTempErr,
					nil,
				},
			},
			expected:      nil,
			expectedCount: 3,
		},
		// Temporary error given thrice causes three tries and fails
		{
			name:     "triple-temp-error",
			maxTries: 3,
			te: &testExchanger{
				errs: []error{
					isTempErr,
					isTempErr,
					isTempErr,
				},
			},
			expected:          servFailError,
			expectedCount:     3,
			metricsAllRetries: 1,
		},
		// temporary then non-Temporary error causes two retries
		{
			name:     "temp-nontemp-error",
			maxTries: 3,
			te: &testExchanger{
				errs: []error{
					isTempErr,
					nonTempErr,
				},
			},
			expected:      servFailError,
			expectedCount: 2,
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			staticProvider, err := NewStaticProvider([]string{dnsLoopbackAddr})
			test.AssertNotError(t, err, "Got error creating StaticProvider")

			testClient := New(time.Second*10, staticProvider, metrics.NoopRegisterer, clock.NewFake(), tc.maxTries, "", blog.UseMock(), tlsConfig)
			dr := testClient.(*impl)
			dr.dnsClient = tc.te
			_, _, err = dr.LookupTXT(context.Background(), "example.com")
			if err == errTooManyRequests {
				t.Errorf("#%d, sent more requests than the test case handles", i)
			}
			expectedErr := tc.expected
			if (expectedErr == nil && err != nil) ||
				(expectedErr != nil && err == nil) ||
				(expectedErr != nil && expectedErr.Error() != err.Error()) {
				t.Errorf("#%d, error, expected %v, got %v", i, expectedErr, err)
			}
			if tc.expectedCount != tc.te.count {
				t.Errorf("#%d, error, expectedCount %v, got %v", i, tc.expectedCount, tc.te.count)
			}
			if tc.metricsAllRetries > 0 {
				test.AssertMetricWithLabelsEquals(
					t, dr.timeoutCounter, prometheus.Labels{
						"qtype":    "TXT",
						"type":     "out of retries",
						"resolver": "127.0.0.1",
						"isTLD":    "false",
					}, tc.metricsAllRetries)
			}
		})
	}

	staticProvider, err := NewStaticProvider([]string{dnsLoopbackAddr})
	test.AssertNotError(t, err, "Got error creating StaticProvider")

	testClient := New(time.Second*10, staticProvider, metrics.NoopRegisterer, clock.NewFake(), 3, "", blog.UseMock(), tlsConfig)
	dr := testClient.(*impl)
	dr.dnsClient = &testExchanger{errs: []error{isTempErr, isTempErr, nil}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = dr.LookupTXT(ctx, "example.com")
	if err == nil ||
		err.Error() != "DNS problem: query timed out (and was canceled) looking up TXT for example.com" {
		t.Errorf("expected %s, got %s", context.Canceled, err)
	}

	dr.dnsClient = &testExchanger{errs: []error{isTempErr, isTempErr, nil}}
	ctx, cancel = context.WithTimeout(context.Background(), -10*time.Hour)
	defer cancel()
	_, _, err = dr.LookupTXT(ctx, "example.com")
	if err == nil ||
		err.Error() != "DNS problem: query timed out looking up TXT for example.com" {
		t.Errorf("expected %s, got %s", context.DeadlineExceeded, err)
	}

	dr.dnsClient = &testExchanger{errs: []error{isTempErr, isTempErr, nil}}
	ctx, deadlineCancel := context.WithTimeout(context.Background(), -10*time.Hour)
	deadlineCancel()
	_, _, err = dr.LookupTXT(ctx, "example.com")
	if err == nil ||
		err.Error() != "DNS problem: query timed out looking up TXT for example.com" {
		t.Errorf("expected %s, got %s", context.DeadlineExceeded, err)
	}

	test.AssertMetricWithLabelsEquals(
		t, dr.timeoutCounter, prometheus.Labels{
			"qtype":    "TXT",
			"type":     "canceled",
			"resolver": "127.0.0.1",
		}, 1)

	test.AssertMetricWithLabelsEquals(
		t, dr.timeoutCounter, prometheus.Labels{
			"qtype":    "TXT",
			"type":     "deadline exceeded",
			"resolver": "127.0.0.1",
		}, 2)
}

func TestIsTLD(t *testing.T) {
	if isTLD("com") != "true" {
		t.Errorf("expected 'com' to be a TLD, got %q", isTLD("com"))
	}
	if isTLD("example.com") != "false" {
		t.Errorf("expected 'example.com' to not a TLD, got %q", isTLD("example.com"))
	}
}

type tempError bool

func (t tempError) Temporary() bool { return bool(t) }
func (t tempError) Error() string   { return fmt.Sprintf("Temporary: %t", t) }

// rotateFailureExchanger is a dns.Exchange implementation that tracks a count
// of the number of calls to `Exchange` for a given address in the `lookups`
// map. For all addresses in the `brokenAddresses` map, a retryable error is
// returned from `Exchange`. This mock is used by `TestRotateServerOnErr`.
type rotateFailureExchanger struct {
	sync.Mutex
	lookups         map[string]int
	brokenAddresses map[string]bool
}

// Exchange for rotateFailureExchanger tracks the `a` argument in `lookups` and
// if present in `brokenAddresses`, returns a temporary error.
func (e *rotateFailureExchanger) Exchange(m *dns.Msg, a string) (*dns.Msg, time.Duration, error) {
	e.Lock()
	defer e.Unlock()

	// Track that exchange was called for the given server
	e.lookups[a]++

	// If its a broken server, return a retryable error
	if e.brokenAddresses[a] {
		isTempErr := &url.Error{Op: "read", Err: tempError(true)}
		return nil, 2 * time.Millisecond, isTempErr
	}

	return m, 2 * time.Millisecond, nil
}

// TestRotateServerOnErr ensures that a retryable error returned from a DNS
// server will result in the retry being performed against the next server in
// the list.
func TestRotateServerOnErr(t *testing.T) {
	// Configure three DNS servers
	dnsServers := []string{
		"a:53", "b:53", "[2606:4700:4700::1111]:53",
	}

	// Set up a DNS client using these servers that will retry queries up to
	// a maximum of 5 times. It's important to choose a maxTries value >= the
	// number of dnsServers to ensure we always get around to trying the one
	// working server
	staticProvider, err := NewStaticProvider(dnsServers)
	test.AssertNotError(t, err, "Got error creating StaticProvider")

	maxTries := 5
	client := New(time.Second*10, staticProvider, metrics.NoopRegisterer, clock.NewFake(), maxTries, "", blog.UseMock(), tlsConfig)

	// Configure a mock exchanger that will always return a retryable error for
	// servers A and B. This will force server "[2606:4700:4700::1111]:53" to do
	// all the work once retries reach it.
	mock := &rotateFailureExchanger{
		brokenAddresses: map[string]bool{
			"a:53": true,
			"b:53": true,
		},
		lookups: make(map[string]int),
	}
	client.(*impl).dnsClient = mock

	// Perform a bunch of lookups. We choose the initial server randomly. Any time
	// A or B is chosen there should be an error and a retry using the next server
	// in the list. Since we configured maxTries to be larger than the number of
	// servers *all* queries should eventually succeed by being retried against
	// server "[2606:4700:4700::1111]:53".
	for range maxTries * 2 {
		_, resolvers, err := client.LookupTXT(context.Background(), "example.com")
		test.AssertEquals(t, len(resolvers), 1)
		test.AssertEquals(t, resolvers[0], "[2606:4700:4700::1111]:53")
		// Any errors are unexpected - server "[2606:4700:4700::1111]:53" should
		// have responded without error.
		test.AssertNotError(t, err, "Expected no error from eventual retry with functional server")
	}

	// We expect that the A and B servers had a non-zero number of lookups
	// attempted.
	test.Assert(t, mock.lookups["a:53"] > 0, "Expected A server to have non-zero lookup attempts")
	test.Assert(t, mock.lookups["b:53"] > 0, "Expected B server to have non-zero lookup attempts")

	// We expect that the server "[2606:4700:4700::1111]:53" eventually served
	// all of the lookups attempted.
	test.AssertEquals(t, mock.lookups["[2606:4700:4700::1111]:53"], maxTries*2)

}

type mockTempURLError struct{}

func (m *mockTempURLError) Error() string   { return "whoops, oh gosh" }
func (m *mockTempURLError) Timeout() bool   { return false }
func (m *mockTempURLError) Temporary() bool { return true }

type dohAlwaysRetryExchanger struct {
	sync.Mutex
	err error
}

func (dohE *dohAlwaysRetryExchanger) Exchange(m *dns.Msg, a string) (*dns.Msg, time.Duration, error) {
	dohE.Lock()
	defer dohE.Unlock()

	tempURLerror := &url.Error{
		Op:  "GET",
		URL: "https://example.com",
		Err: &mockTempURLError{},
	}

	return nil, time.Second, tempURLerror
}

func TestDOHMetric(t *testing.T) {
	staticProvider, err := NewStaticProvider([]string{dnsLoopbackAddr})
	test.AssertNotError(t, err, "Got error creating StaticProvider")

	testClient := New(time.Second*11, staticProvider, metrics.NoopRegisterer, clock.NewFake(), 0, "", blog.UseMock(), tlsConfig)
	resolver := testClient.(*impl)
	resolver.dnsClient = &dohAlwaysRetryExchanger{err: &url.Error{Op: "read", Err: tempError(true)}}

	// Starting out, we should count 0 "out of retries" errors.
	test.AssertMetricWithLabelsEquals(t, resolver.timeoutCounter, prometheus.Labels{"qtype": "None", "type": "out of retries", "resolver": "127.0.0.1", "isTLD": "false"}, 0)

	// Trigger the error.
	_, _, _ = resolver.exchangeOne(context.Background(), "example.com", 0)

	// Now, we should count 1 "out of retries" errors.
	test.AssertMetricWithLabelsEquals(t, resolver.timeoutCounter, prometheus.Labels{"qtype": "None", "type": "out of retries", "resolver": "127.0.0.1", "isTLD": "false"}, 1)
}
