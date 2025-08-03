package bdns

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jmhodges/clock"
	"github.com/miekg/dns"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/letsencrypt/boulder/iana"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/metrics"
)

// ResolverAddrs contains DNS resolver(s) that were chosen to perform a
// validation request or CAA recheck. A ResolverAddr will be in the form of
// host:port, A:host:port, or AAAA:host:port depending on which type of lookup
// was done.
type ResolverAddrs []string

// Client queries for DNS records
type Client interface {
	LookupTXT(context.Context, string) (txts []string, resolver ResolverAddrs, err error)
	LookupHost(context.Context, string) ([]netip.Addr, ResolverAddrs, error)
	LookupCAA(context.Context, string) ([]*dns.CAA, string, ResolverAddrs, error)
}

// impl represents a client that talks to an external resolver
type impl struct {
	dnsClient                exchanger
	servers                  ServerProvider
	allowRestrictedAddresses bool
	maxTries                 int
	clk                      clock.Clock
	log                      blog.Logger

	queryTime         *prometheus.HistogramVec
	totalLookupTime   *prometheus.HistogramVec
	timeoutCounter    *prometheus.CounterVec
	idMismatchCounter *prometheus.CounterVec
}

var _ Client = &impl{}

type exchanger interface {
	Exchange(m *dns.Msg, a string) (*dns.Msg, time.Duration, error)
}

// New constructs a new DNS resolver object that utilizes the
// provided list of DNS servers for resolution.
//
// `tlsConfig` is the configuration used for outbound DoH queries,
// if applicable.
func New(
	readTimeout time.Duration,
	servers ServerProvider,
	stats prometheus.Registerer,
	clk clock.Clock,
	maxTries int,
	userAgent string,
	log blog.Logger,
	tlsConfig *tls.Config,
) Client {
	var client exchanger

	// Clone the default transport because it comes with various settings
	// that we like, which are different from the zero value of an
	// `http.Transport`.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	// The default transport already sets this field, but it isn't
	// documented that it will always be set. Set it again to be sure,
	// because Unbound will reject non-HTTP/2 DoH requests.
	transport.ForceAttemptHTTP2 = true
	client = &dohExchanger{
		clk: clk,
		hc: http.Client{
			Timeout:   readTimeout,
			Transport: transport,
		},
		userAgent: userAgent,
	}

	queryTime := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "dns_query_time",
			Help:    "Time taken to perform a DNS query",
			Buckets: metrics.InternetFacingBuckets,
		},
		[]string{"qtype", "result", "resolver"},
	)
	totalLookupTime := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "dns_total_lookup_time",
			Help:    "Time taken to perform a DNS lookup, including all retried queries",
			Buckets: metrics.InternetFacingBuckets,
		},
		[]string{"qtype", "result", "retries", "resolver"},
	)
	timeoutCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dns_timeout",
			Help: "Counter of various types of DNS query timeouts",
		},
		[]string{"qtype", "type", "resolver", "isTLD"},
	)
	idMismatchCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dns_id_mismatch",
			Help: "Counter of DNS ErrId errors sliced by query type and resolver",
		},
		[]string{"qtype", "resolver"},
	)
	stats.MustRegister(queryTime, totalLookupTime, timeoutCounter, idMismatchCounter)
	return &impl{
		dnsClient:                client,
		servers:                  servers,
		allowRestrictedAddresses: false,
		maxTries:                 maxTries,
		clk:                      clk,
		queryTime:                queryTime,
		totalLookupTime:          totalLookupTime,
		timeoutCounter:           timeoutCounter,
		idMismatchCounter:        idMismatchCounter,
		log:                      log,
	}
}

// NewTest constructs a new DNS resolver object that utilizes the
// provided list of DNS servers for resolution and will allow loopback addresses.
// This constructor should *only* be called from tests (unit or integration).
func NewTest(
	readTimeout time.Duration,
	servers ServerProvider,
	stats prometheus.Registerer,
	clk clock.Clock,
	maxTries int,
	userAgent string,
	log blog.Logger,
	tlsConfig *tls.Config,
) Client {
	resolver := New(readTimeout, servers, stats, clk, maxTries, userAgent, log, tlsConfig)
	resolver.(*impl).allowRestrictedAddresses = true
	return resolver
}

// exchangeOne performs a single DNS exchange with a randomly chosen server
// out of the server list, returning the response, time, and error (if any).
// We assume that the upstream resolver requests and validates DNSSEC records
// itself.
func (dnsClient *impl) exchangeOne(ctx context.Context, hostname string, qtype uint16) (resp *dns.Msg, resolver string, err error) {
	m := new(dns.Msg)
	// Set question type
	m.SetQuestion(dns.Fqdn(hostname), qtype)
	// Set the AD bit in the query header so that the resolver knows that
	// we are interested in this bit in the response header. If this isn't
	// set the AD bit in the response is useless (RFC 6840 Section 5.7).
	// This has no security implications, it simply allows us to gather
	// metrics about the percentage of responses that are secured with
	// DNSSEC.
	m.AuthenticatedData = true
	// Tell the resolver that we're willing to receive responses up to 4096 bytes.
	// This happens sometimes when there are a very large number of CAA records
	// present.
	m.SetEdns0(4096, false)

	servers, err := dnsClient.servers.Addrs()
	if err != nil {
		return nil, "", fmt.Errorf("failed to list DNS servers: %w", err)
	}
	chosenServerIndex := 0
	chosenServer := servers[chosenServerIndex]
	resolver = chosenServer

	// Strip off the IP address part of the server address because
	// we talk to the same server on multiple ports, and don't want
	// to blow up the cardinality.
	chosenServerIP, _, err := net.SplitHostPort(chosenServer)
	if err != nil {
		return
	}

	start := dnsClient.clk.Now()
	client := dnsClient.dnsClient
	qtypeStr := dns.TypeToString[qtype]
	tries := 1
	defer func() {
		result := "failed"
		if resp != nil {
			result = dns.RcodeToString[resp.Rcode]
		}
		dnsClient.totalLookupTime.With(prometheus.Labels{
			"qtype":    qtypeStr,
			"result":   result,
			"retries":  strconv.Itoa(tries),
			"resolver": chosenServerIP,
		}).Observe(dnsClient.clk.Since(start).Seconds())
	}()
	for {
		ch := make(chan dnsResp, 1)

		// Strip off the IP address part of the server address because
		// we talk to the same server on multiple ports, and don't want
		// to blow up the cardinality.
		// Note: validateServerAddress() has already checked net.SplitHostPort()
		// and ensures that chosenServer can't be a bare port, e.g. ":1337"
		chosenServerIP, _, err = net.SplitHostPort(chosenServer)
		if err != nil {
			return
		}

		go func() {
			rsp, rtt, err := client.Exchange(m, chosenServer)
			result := "failed"
			if rsp != nil {
				result = dns.RcodeToString[rsp.Rcode]
			}
			if err != nil {
				logDNSError(dnsClient.log, chosenServer, hostname, m, rsp, err)
				if err == dns.ErrId {
					dnsClient.idMismatchCounter.With(prometheus.Labels{
						"qtype":    qtypeStr,
						"resolver": chosenServerIP,
					}).Inc()
				}
			}
			dnsClient.queryTime.With(prometheus.Labels{
				"qtype":    qtypeStr,
				"result":   result,
				"resolver": chosenServerIP,
			}).Observe(rtt.Seconds())
			ch <- dnsResp{m: rsp, err: err}
		}()
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				dnsClient.timeoutCounter.With(prometheus.Labels{
					"qtype":    qtypeStr,
					"type":     "deadline exceeded",
					"resolver": chosenServerIP,
					"isTLD":    isTLD(hostname),
				}).Inc()
			} else if ctx.Err() == context.Canceled {
				dnsClient.timeoutCounter.With(prometheus.Labels{
					"qtype":    qtypeStr,
					"type":     "canceled",
					"resolver": chosenServerIP,
					"isTLD":    isTLD(hostname),
				}).Inc()
			} else {
				dnsClient.timeoutCounter.With(prometheus.Labels{
					"qtype":    qtypeStr,
					"type":     "unknown",
					"resolver": chosenServerIP,
				}).Inc()
			}
			err = ctx.Err()
			return
		case r := <-ch:
			if r.err != nil {
				var isRetryable bool
				// According to the http package documentation, retryable
				// errors emitted by the http package are of type *url.Error.
				var urlErr *url.Error
				isRetryable = errors.As(r.err, &urlErr) && urlErr.Temporary()
				hasRetriesLeft := tries < dnsClient.maxTries
				if isRetryable && hasRetriesLeft {
					tries++
					// Chose a new server to retry the query with by incrementing the
					// chosen server index modulo the number of servers. This ensures that
					// if one dns server isn't available we retry with the next in the
					// list.
					chosenServerIndex = (chosenServerIndex + 1) % len(servers)
					chosenServer = servers[chosenServerIndex]
					resolver = chosenServer
					continue
				} else if isRetryable && !hasRetriesLeft {
					dnsClient.timeoutCounter.With(prometheus.Labels{
						"qtype":    qtypeStr,
						"type":     "out of retries",
						"resolver": chosenServerIP,
						"isTLD":    isTLD(hostname),
					}).Inc()
				}
			}
			resp, err = r.m, r.err
			return
		}
	}
}

// isTLD returns a simplified view of whether something is a TLD: does it have
// any dots in it? This returns true or false as a string, and is meant solely
// for Prometheus metrics.
func isTLD(hostname string) string {
	if strings.Contains(hostname, ".") {
		return "false"
	} else {
		return "true"
	}
}

type dnsResp struct {
	m   *dns.Msg
	err error
}

// LookupTXT sends a DNS query to find all TXT records associated with
// the provided hostname which it returns along with the returned
// DNS authority section.
func (dnsClient *impl) LookupTXT(ctx context.Context, hostname string) ([]string, ResolverAddrs, error) {
	var txt []string
	dnsType := dns.TypeTXT
	r, resolver, err := dnsClient.exchangeOne(ctx, hostname, dnsType)
	errWrap := wrapErr(dnsType, hostname, r, err)
	if errWrap != nil {
		return nil, ResolverAddrs{resolver}, errWrap
	}

	for _, answer := range r.Answer {
		if answer.Header().Rrtype == dnsType {
			if txtRec, ok := answer.(*dns.TXT); ok {
				txt = append(txt, strings.Join(txtRec.Txt, ""))
			}
		}
	}

	return txt, ResolverAddrs{resolver}, err
}

func (dnsClient *impl) lookupIP(ctx context.Context, hostname string, ipType uint16) ([]dns.RR, string, error) {
	resp, resolver, err := dnsClient.exchangeOne(ctx, hostname, ipType)
	switch ipType {
	case dns.TypeA:
		if resolver != "" {
			resolver = "A:" + resolver
		}
	case dns.TypeAAAA:
		if resolver != "" {
			resolver = "AAAA:" + resolver
		}
	}
	errWrap := wrapErr(ipType, hostname, resp, err)
	if errWrap != nil {
		return nil, resolver, errWrap
	}
	return resp.Answer, resolver, nil
}

// LookupHost sends a DNS query to find all A and AAAA records associated with
// the provided hostname. This method assumes that the external resolver will
// chase CNAME/DNAME aliases and return relevant records. It will retry
// requests in the case of temporary network errors. It returns an error if
// both the A and AAAA lookups fail or are empty, but succeeds otherwise.
func (dnsClient *impl) LookupHost(ctx context.Context, hostname string) ([]netip.Addr, ResolverAddrs, error) {
	var recordsA, recordsAAAA []dns.RR
	var errA, errAAAA error
	var resolverA, resolverAAAA string
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		recordsA, resolverA, errA = dnsClient.lookupIP(ctx, hostname, dns.TypeA)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		recordsAAAA, resolverAAAA, errAAAA = dnsClient.lookupIP(ctx, hostname, dns.TypeAAAA)
	}()
	wg.Wait()

	resolvers := ResolverAddrs{resolverA, resolverAAAA}
	resolvers = slices.DeleteFunc(resolvers, func(a string) bool {
		return a == ""
	})

	var addrsA []netip.Addr
	if errA == nil {
		for _, answer := range recordsA {
			if answer.Header().Rrtype == dns.TypeA {
				a, ok := answer.(*dns.A)
				if ok && a.A.To4() != nil {
					netIP, ok := netip.AddrFromSlice(a.A)
					if ok && (iana.IsReservedAddr(netIP) == nil || dnsClient.allowRestrictedAddresses) {
						addrsA = append(addrsA, netIP)
					}
				}
			}
		}
		if len(addrsA) == 0 {
			errA = fmt.Errorf("no valid A records found for %s", hostname)
		}
	}

	var addrsAAAA []netip.Addr
	if errAAAA == nil {
		for _, answer := range recordsAAAA {
			if answer.Header().Rrtype == dns.TypeAAAA {
				aaaa, ok := answer.(*dns.AAAA)
				if ok && aaaa.AAAA.To16() != nil {
					netIP, ok := netip.AddrFromSlice(aaaa.AAAA)
					if ok && (iana.IsReservedAddr(netIP) == nil || dnsClient.allowRestrictedAddresses) {
						addrsAAAA = append(addrsAAAA, netIP)
					}
				}
			}
		}
		if len(addrsAAAA) == 0 {
			errAAAA = fmt.Errorf("no valid AAAA records found for %s", hostname)
		}
	}

	if errA != nil && errAAAA != nil {
		// Construct a new error from both underlying errors. We can only use %w for
		// one of them, because the go error unwrapping protocol doesn't support
		// branching. We don't use ProblemDetails and SubProblemDetails here, because
		// this error will get wrapped in a DNSError and further munged by higher
		// layers in the stack.
		return nil, resolvers, fmt.Errorf("%w; %s", errA, errAAAA)
	}

	return append(addrsA, addrsAAAA...), resolvers, nil
}

// LookupCAA sends a DNS query to find all CAA records associated with
// the provided hostname and the complete dig-style RR `response`. This
// response is quite verbose, however it's only populated when the CAA
// response is non-empty.
func (dnsClient *impl) LookupCAA(ctx context.Context, hostname string) ([]*dns.CAA, string, ResolverAddrs, error) {
	dnsType := dns.TypeCAA
	r, resolver, err := dnsClient.exchangeOne(ctx, hostname, dnsType)

	// Special case: when checking CAA for non-TLD names, treat NXDOMAIN as a
	// successful response containing an empty set of records. This can come up in
	// situations where records were provisioned for validation (e.g. TXT records
	// for DNS-01 challenge) and then removed after validation but before CAA
	// rechecking. But allow NXDOMAIN for TLDs to fall through to the error code
	// below, so we don't issue for gTLDs that have been removed by ICANN.
	if err == nil && r.Rcode == dns.RcodeNameError && strings.Contains(hostname, ".") {
		return nil, "", ResolverAddrs{resolver}, nil
	}

	errWrap := wrapErr(dnsType, hostname, r, err)
	if errWrap != nil {
		return nil, "", ResolverAddrs{resolver}, errWrap
	}

	var CAAs []*dns.CAA
	for _, answer := range r.Answer {
		if caaR, ok := answer.(*dns.CAA); ok {
			CAAs = append(CAAs, caaR)
		}
	}
	var response string
	if len(CAAs) > 0 {
		response = r.String()
	}
	return CAAs, response, ResolverAddrs{resolver}, nil
}

// logDNSError logs the provided err result from making a query for hostname to
// the chosenServer. If the err is a `dns.ErrId` instance then the Base64
// encoded bytes of the query (and if not-nil, the response) in wire format
// is logged as well. This function is called from exchangeOne only for the case
// where an error occurs querying a hostname that indicates a problem between
// the VA and the chosenServer.
func logDNSError(
	logger blog.Logger,
	chosenServer string,
	hostname string,
	msg, resp *dns.Msg,
	underlying error) {
	// We don't expect logDNSError to be called with a nil msg or err but
	// if it happens return early. We allow resp to be nil.
	if msg == nil || len(msg.Question) == 0 || underlying == nil {
		return
	}
	queryType := dns.TypeToString[msg.Question[0].Qtype]

	// If the error indicates there was a query/response ID mismatch then we want
	// to log more detail.
	if underlying == dns.ErrId {
		packedMsgBytes, err := msg.Pack()
		if err != nil {
			logger.Errf("logDNSError failed to pack msg: %v", err)
			return
		}
		encodedMsg := base64.StdEncoding.EncodeToString(packedMsgBytes)

		var encodedResp string
		var respQname string
		if resp != nil {
			packedRespBytes, err := resp.Pack()
			if err != nil {
				logger.Errf("logDNSError failed to pack resp: %v", err)
				return
			}
			encodedResp = base64.StdEncoding.EncodeToString(packedRespBytes)
			if len(resp.Answer) > 0 && resp.Answer[0].Header() != nil {
				respQname = resp.Answer[0].Header().Name
			}
		}

		logger.Infof(
			"logDNSError ID mismatch chosenServer=[%s] hostname=[%s] respHostname=[%s] queryType=[%s] msg=[%s] resp=[%s] err=[%s]",
			chosenServer,
			hostname,
			respQname,
			queryType,
			encodedMsg,
			encodedResp,
			underlying)
	} else {
		// Otherwise log a general DNS error
		logger.Infof("logDNSError chosenServer=[%s] hostname=[%s] queryType=[%s] err=[%s]",
			chosenServer,
			hostname,
			queryType,
			underlying)
	}
}

type dohExchanger struct {
	clk       clock.Clock
	hc        http.Client
	userAgent string
}

// Exchange sends a DoH query to the provided DoH server and returns the response.
func (d *dohExchanger) Exchange(query *dns.Msg, server string) (*dns.Msg, time.Duration, error) {
	q, err := query.Pack()
	if err != nil {
		return nil, 0, err
	}

	// The default Unbound URL template
	url := fmt.Sprintf("https://%s/dns-query", server)
	req, err := http.NewRequest("POST", url, strings.NewReader(string(q)))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")
	if len(d.userAgent) > 0 {
		req.Header.Set("User-Agent", d.userAgent)
	}

	start := d.clk.Now()
	resp, err := d.hc.Do(req)
	if err != nil {
		return nil, d.clk.Since(start), err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, d.clk.Since(start), fmt.Errorf("doh: http status %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, d.clk.Since(start), fmt.Errorf("doh: reading response body: %w", err)
	}

	response := new(dns.Msg)
	err = response.Unpack(b)
	if err != nil {
		return nil, d.clk.Since(start), fmt.Errorf("doh: unpacking response: %w", err)
	}

	return response, d.clk.Since(start), nil
}
