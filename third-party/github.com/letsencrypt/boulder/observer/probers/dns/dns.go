package probers

import (
	"fmt"
	"time"

	"github.com/miekg/dns"
)

// DNSProbe is the exported 'Prober' object for monitors configured to
// perform DNS requests.
type DNSProbe struct {
	proto   string
	server  string
	recurse bool
	qname   string
	qtype   uint16
}

// Name returns a string that uniquely identifies the monitor.
func (p DNSProbe) Name() string {
	recursion := func() string {
		if p.recurse {
			return "recurse"
		}
		return "no-recurse"
	}()
	return fmt.Sprintf(
		"%s-%s-%s-%s-%s", p.server, p.proto, recursion, dns.TypeToString[p.qtype], p.qname)
}

// Kind returns a name that uniquely identifies the `Kind` of `Prober`.
func (p DNSProbe) Kind() string {
	return "DNS"
}

// Probe performs the configured DNS query.
func (p DNSProbe) Probe(timeout time.Duration) (bool, time.Duration) {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(p.qname), p.qtype)
	m.RecursionDesired = p.recurse
	c := dns.Client{Timeout: timeout, Net: p.proto}
	start := time.Now()
	r, _, err := c.Exchange(m, p.server)
	if err != nil {
		return false, time.Since(start)
	}
	if r == nil {
		return false, time.Since(start)
	}
	if r.Rcode != dns.RcodeSuccess {
		return false, time.Since(start)
	}
	return true, time.Since(start)
}
