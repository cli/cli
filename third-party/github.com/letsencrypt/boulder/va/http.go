package va

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/letsencrypt/boulder/bdns"
	"github.com/letsencrypt/boulder/core"
	berrors "github.com/letsencrypt/boulder/errors"
	"github.com/letsencrypt/boulder/iana"
	"github.com/letsencrypt/boulder/identifier"
)

const (
	// maxRedirect is the maximum number of redirects the VA will follow
	// processing an HTTP-01 challenge.
	maxRedirect = 10
	// maxResponseSize holds the maximum number of bytes that will be read from an
	// HTTP-01 challenge response. The expected payload should be ~87 bytes. Since
	// it may be padded by whitespace which we previously allowed accept up to 128
	// bytes before rejecting a response (32 byte b64 encoded token + . + 32 byte
	// b64 encoded key fingerprint).
	maxResponseSize = 128
	// maxPathSize is the maximum number of bytes we will accept in the path of a
	// redirect URL.
	maxPathSize = 2000
)

// preresolvedDialer is a struct type that provides a DialContext function which
// will connect to the provided IP and port instead of letting DNS resolve
// The hostname of the preresolvedDialer is used to ensure the dial only completes
// using the pre-resolved IP/port when used for the correct host.
type preresolvedDialer struct {
	ip       netip.Addr
	port     int
	hostname string
	timeout  time.Duration
}

// a dialerMismatchError is produced when a preresolvedDialer is used to dial
// a host other than the dialer's specified hostname.
type dialerMismatchError struct {
	// The original dialer information
	dialerHost string
	dialerIP   string
	dialerPort int
	// The host that the dialer was incorrectly used with
	host string
}

func (e *dialerMismatchError) Error() string {
	return fmt.Sprintf(
		"preresolvedDialer mismatch: dialer is for %q (ip: %q port: %d) not %q",
		e.dialerHost, e.dialerIP, e.dialerPort, e.host)
}

// DialContext for a preresolvedDialer shaves 10ms off of the context it was
// given before calling the default transport DialContext using the pre-resolved
// IP and port as the host. If the original host being dialed by DialContext
// does not match the expected hostname in the preresolvedDialer an error will
// be returned instead. This helps prevents a bug that might use
// a preresolvedDialer for the wrong host.
//
// Shaving the context helps us be able to differentiate between timeouts during
// connect and timeouts after connect.
//
// Using preresolved information for the host argument given to the real
// transport dial lets us have fine grained control over IP address resolution for
// domain names.
func (d *preresolvedDialer) DialContext(
	ctx context.Context,
	network,
	origAddr string) (net.Conn, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		// Shouldn't happen: All requests should have a deadline by this point.
		deadline = time.Now().Add(100 * time.Second)
	} else {
		// Set the context deadline slightly shorter than the HTTP deadline, so we
		// get a useful error rather than a generic "deadline exceeded" error. This
		// lets us give a more specific error to the subscriber.
		deadline = deadline.Add(-10 * time.Millisecond)
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	// NOTE(@cpu): I don't capture and check the origPort here because using
	// `net.SplitHostPort` and also supporting the va's custom httpPort and
	// httpsPort is cumbersome. The initial origAddr may be "example.com:80"
	// if the URL used for the dial input was "http://example.com" without an
	// explicit port. Checking for equality here will fail unless we add
	// special case logic for converting 80/443 -> httpPort/httpsPort when
	// configured. This seems more likely to cause bugs than catch them so I'm
	// ignoring this for now. In the future if we remove the httpPort/httpsPort
	// (we should!) we can also easily enforce that the preresolved dialer port
	// matches expected here.
	origHost, _, err := net.SplitHostPort(origAddr)
	if err != nil {
		return nil, err
	}
	// If the hostname we're dialing isn't equal to the hostname the dialer was
	// constructed for then a bug has occurred where we've mismatched the
	// preresolved dialer.
	if origHost != d.hostname {
		return nil, &dialerMismatchError{
			dialerHost: d.hostname,
			dialerIP:   d.ip.String(),
			dialerPort: d.port,
			host:       origHost,
		}
	}

	// Make a new dial address using the pre-resolved IP and port.
	targetAddr := net.JoinHostPort(d.ip.String(), strconv.Itoa(d.port))

	// Create a throw-away dialer using default values and the dialer timeout
	// (populated from the VA singleDialTimeout).
	throwAwayDialer := &net.Dialer{
		Timeout: d.timeout,
		// Default KeepAlive - see Golang src/net/http/transport.go DefaultTransport
		KeepAlive: 30 * time.Second,
	}
	return throwAwayDialer.DialContext(ctx, network, targetAddr)
}

// a dialerFunc meets the function signature requirements of
// a http.Transport.DialContext handler.
type dialerFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// httpTransport constructs a HTTP Transport with settings appropriate for
// HTTP-01 validation. The provided dialerFunc is used as the Transport's
// DialContext handler.
func httpTransport(df dialerFunc) *http.Transport {
	return &http.Transport{
		DialContext: df,
		// We are talking to a client that does not yet have a certificate,
		// so we accept a temporary, invalid one.
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		// We don't expect to make multiple requests to a client, so close
		// connection immediately.
		DisableKeepAlives: true,
		// We don't want idle connections, but 0 means "unlimited," so we pick 1.
		MaxIdleConns:        1,
		IdleConnTimeout:     time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
}

// httpValidationTarget bundles all of the information needed to make an HTTP-01
// validation request against a target.
type httpValidationTarget struct {
	// the host being validated
	host string
	// the port for the validation request
	port int
	// the path for the validation request
	path string
	// query data for validation request (potentially populated when
	// following redirects)
	query string
	// all of the IP addresses available for the host
	available []netip.Addr
	// the IP addresses that were tried for validation previously that were cycled
	// out of cur by calls to nextIP()
	tried []netip.Addr
	// the IP addresses that will be drawn from by calls to nextIP() to set curIP
	next []netip.Addr
	// the current IP address being used for validation (if any)
	cur netip.Addr
	// the DNS resolver(s) that will attempt to fulfill the validation request
	resolvers bdns.ResolverAddrs
}

// nextIP changes the cur IP by removing the first entry from the next slice and
// setting it to cur. If cur was previously set the value will be added to the
// tried slice to keep track of IPs that were previously used. If nextIP() is
// called but vt.next is empty an error is returned.
func (vt *httpValidationTarget) nextIP() error {
	if len(vt.next) == 0 {
		return fmt.Errorf(
			"host %q has no IP addresses remaining to use",
			vt.host)
	}
	vt.tried = append(vt.tried, vt.cur)
	vt.cur = vt.next[0]
	vt.next = vt.next[1:]
	return nil
}

// newHTTPValidationTarget creates a httpValidationTarget for the given host,
// port, and path. This involves querying DNS for the IP addresses for the host.
// An error is returned if there are no usable IP addresses or if the DNS
// lookups fail.
func (va *ValidationAuthorityImpl) newHTTPValidationTarget(
	ctx context.Context,
	ident identifier.ACMEIdentifier,
	port int,
	path string,
	query string) (*httpValidationTarget, error) {
	var addrs []netip.Addr
	var resolvers bdns.ResolverAddrs
	switch ident.Type {
	case identifier.TypeDNS:
		// Resolve IP addresses for the identifier
		dnsAddrs, dnsResolvers, err := va.getAddrs(ctx, ident.Value)
		if err != nil {
			return nil, err
		}
		addrs, resolvers = dnsAddrs, dnsResolvers
	case identifier.TypeIP:
		netIP, err := netip.ParseAddr(ident.Value)
		if err != nil {
			return nil, fmt.Errorf("can't parse IP address %q: %s", ident.Value, err)
		}
		addrs = []netip.Addr{netIP}
	default:
		return nil, fmt.Errorf("unknown identifier type: %s", ident.Type)
	}

	target := &httpValidationTarget{
		host:      ident.Value,
		port:      port,
		path:      path,
		query:     query,
		available: addrs,
		resolvers: resolvers,
	}

	// Separate the addresses into the available v4 and v6 addresses
	v4Addrs, v6Addrs := availableAddresses(addrs)
	hasV6Addrs := len(v6Addrs) > 0
	hasV4Addrs := len(v4Addrs) > 0

	if !hasV6Addrs && !hasV4Addrs {
		// If there are no v6 addrs and no v4addrs there was a bug with getAddrs or
		// availableAddresses and we need to return an error.
		return nil, fmt.Errorf("host %q has no IPv4 or IPv6 addresses", ident.Value)
	} else if !hasV6Addrs && hasV4Addrs {
		// If there are no v6 addrs and there are v4 addrs then use the first v4
		// address. There's no fallback address.
		target.next = []netip.Addr{v4Addrs[0]}
	} else if hasV6Addrs && hasV4Addrs {
		// If there are both v6 addrs and v4 addrs then use the first v6 address and
		// fallback with the first v4 address.
		target.next = []netip.Addr{v6Addrs[0], v4Addrs[0]}
	} else if hasV6Addrs && !hasV4Addrs {
		// If there are just v6 addrs then use the first v6 address. There's no
		// fallback address.
		target.next = []netip.Addr{v6Addrs[0]}
	}

	// Advance the target using nextIP to populate the cur IP before returning
	_ = target.nextIP()
	return target, nil
}

// extractRequestTarget extracts the host and port specified in the provided
// HTTP redirect request. If the request's URL's protocol schema is not HTTP or
// HTTPS an error is returned. If an explicit port is specified in the request's
// URL and it isn't the VA's HTTP or HTTPS port, an error is returned.
func (va *ValidationAuthorityImpl) extractRequestTarget(req *http.Request) (identifier.ACMEIdentifier, int, error) {
	// A nil request is certainly not a valid redirect and has no port to extract.
	if req == nil {
		return identifier.ACMEIdentifier{}, 0, fmt.Errorf("redirect HTTP request was nil")
	}

	reqScheme := req.URL.Scheme

	// The redirect request must use HTTP or HTTPs protocol schemes regardless of the port..
	if reqScheme != "http" && reqScheme != "https" {
		return identifier.ACMEIdentifier{}, 0, berrors.ConnectionFailureError(
			"Invalid protocol scheme in redirect target. "+
				`Only "http" and "https" protocol schemes are supported, not %q`, reqScheme)
	}

	// Try to parse an explicit port number from the request URL host. If there
	// is one, we need to make sure its a valid port. If there isn't one we need
	// to pick the port based on the reqScheme default port.
	reqHost := req.URL.Hostname()
	var reqPort int
	// URL.Port() will return "" for an invalid port, not just an empty port. To
	// reject invalid ports, we rely on the calling function having used
	// URL.Parse(), which does enforce validity.
	if req.URL.Port() != "" {
		parsedPort, err := strconv.Atoi(req.URL.Port())
		if err != nil {
			return identifier.ACMEIdentifier{}, 0, err
		}

		// The explicit port must match the VA's configured HTTP or HTTPS port.
		if parsedPort != va.httpPort && parsedPort != va.httpsPort {
			return identifier.ACMEIdentifier{}, 0, berrors.ConnectionFailureError(
				"Invalid port in redirect target. Only ports %d and %d are supported, not %d",
				va.httpPort, va.httpsPort, parsedPort)
		}

		reqPort = parsedPort
	} else if reqScheme == "http" {
		reqPort = va.httpPort
	} else if reqScheme == "https" {
		reqPort = va.httpsPort
	} else {
		// This shouldn't happen but defensively return an internal server error in
		// case it does.
		return identifier.ACMEIdentifier{}, 0, fmt.Errorf("unable to determine redirect HTTP request port")
	}

	if reqHost == "" {
		return identifier.ACMEIdentifier{}, 0, berrors.ConnectionFailureError("Invalid empty host in redirect target")
	}

	// Often folks will misconfigure their webserver to send an HTTP redirect
	// missing a `/' between the FQDN and the path. E.g. in Apache using:
	//   Redirect / https://bad-redirect.org
	// Instead of
	//   Redirect / https://bad-redirect.org/
	// Will produce an invalid HTTP-01 redirect target like:
	//   https://bad-redirect.org.well-known/acme-challenge/xxxx
	// This happens frequently enough we want to return a distinct error message
	// for this case by detecting the reqHost ending in ".well-known".
	if strings.HasSuffix(reqHost, ".well-known") {
		return identifier.ACMEIdentifier{}, 0, berrors.ConnectionFailureError(
			"Invalid host in redirect target %q. Check webserver config for missing '/' in redirect target.",
			reqHost,
		)
	}

	reqIP, err := netip.ParseAddr(reqHost)
	if err == nil {
		err := va.isReservedIPFunc(reqIP)
		if err != nil {
			return identifier.ACMEIdentifier{}, 0, berrors.ConnectionFailureError("Invalid host in redirect target: %s", err)
		}
		return identifier.NewIP(reqIP), reqPort, nil
	}

	if _, err := iana.ExtractSuffix(reqHost); err != nil {
		return identifier.ACMEIdentifier{}, 0, berrors.ConnectionFailureError("Invalid host in redirect target, must end in IANA registered TLD")
	}

	return identifier.NewDNS(reqHost), reqPort, nil
}

// setupHTTPValidation sets up a preresolvedDialer and a validation record for
// the given request URL and httpValidationTarget. If the req URL is empty, or
// the validation target is nil or has no available IP addresses, an error will
// be returned.
func (va *ValidationAuthorityImpl) setupHTTPValidation(
	reqURL string,
	target *httpValidationTarget) (*preresolvedDialer, core.ValidationRecord, error) {
	if reqURL == "" {
		return nil,
			core.ValidationRecord{},
			fmt.Errorf("reqURL can not be nil")
	}
	if target == nil {
		// This is the only case where returning an empty validation record makes
		// sense - we can't construct a better one, something has gone quite wrong.
		return nil,
			core.ValidationRecord{},
			fmt.Errorf("httpValidationTarget can not be nil")
	}

	// Construct a base validation record with the validation target's
	// information.
	record := core.ValidationRecord{
		Hostname:          target.host,
		Port:              strconv.Itoa(target.port),
		AddressesResolved: target.available,
		URL:               reqURL,
		ResolverAddrs:     target.resolvers,
	}

	// Get the target IP to build a preresolved dialer with
	targetIP := target.cur
	if (targetIP == netip.Addr{}) {
		return nil,
			record,
			fmt.Errorf(
				"host %q has no IP addresses remaining to use",
				target.host)
	}

	// This is a backstop check to avoid connecting to reserved IP addresses.
	// They should have been caught and excluded by `bdns.LookupHost`.
	err := va.isReservedIPFunc(targetIP)
	if err != nil {
		return nil, record, err
	}

	record.AddressUsed = targetIP

	dialer := &preresolvedDialer{
		ip:       targetIP,
		port:     target.port,
		hostname: target.host,
		timeout:  va.singleDialTimeout,
	}
	return dialer, record, nil
}

// fallbackErr returns true only for net.OpError instances where the op is equal
// to "dial", or url.Error instances wrapping such an error. fallbackErr returns
// false for all other errors. By policy, only dial errors (not read or write
// errors) are eligible for fallback from an IPv6 to an IPv4 address.
func fallbackErr(err error) bool {
	// Err shouldn't ever be nil if we're considering it for fallback
	if err == nil {
		return false
	}
	// Net OpErrors are fallback errs only if the operation was a "dial"
	// All other errs are not fallback errs
	var netOpError *net.OpError
	return errors.As(err, &netOpError) && netOpError.Op == "dial"
}

// processHTTPValidation performs an HTTP validation for the given host, port
// and path. If successful the body of the HTTP response is returned along with
// the validation records created during the validation. If not successful
// a non-nil error and potentially some ValidationRecords are returned.
func (va *ValidationAuthorityImpl) processHTTPValidation(
	ctx context.Context,
	ident identifier.ACMEIdentifier,
	path string) ([]byte, []core.ValidationRecord, error) {
	// Create a target for the host, port and path with no query parameters
	target, err := va.newHTTPValidationTarget(ctx, ident, va.httpPort, path, "")
	if err != nil {
		return nil, nil, err
	}

	// When constructing a URL, bare IPv6 addresses must be enclosed in square
	// brackets. Otherwise, a colon may be interpreted as a port separator.
	host := ident.Value
	if ident.Type == identifier.TypeIP {
		netipHost, err := netip.ParseAddr(host)
		if err != nil {
			return nil, nil, fmt.Errorf("couldn't parse IP address from identifier")
		}
		if !netipHost.Is4() {
			host = "[" + host + "]"
		}
	}

	// Create an initial GET Request
	initialURL := url.URL{
		Scheme: "http",
		Host:   host,
		Path:   path,
	}
	initialReq, err := http.NewRequest("GET", initialURL.String(), nil)
	if err != nil {
		return nil, nil, newIPError(target.cur, err)
	}

	// Add a context to the request. Shave some time from the
	// overall context deadline so that we are not racing with gRPC when the
	// HTTP server is timing out. This avoids returning ServerInternal
	// errors when we should be returning Connection errors. This may fix a flaky
	// integration test: https://github.com/letsencrypt/boulder/issues/4087
	// Note: The gRPC interceptor in grpc/interceptors.go already shaves some time
	// off RPCs, but this takes off additional time because HTTP-related timeouts
	// are so common (and because it might fix a flaky build).
	deadline, ok := ctx.Deadline()
	if !ok {
		return nil, nil, fmt.Errorf("processHTTPValidation had no deadline")
	} else {
		deadline = deadline.Add(-200 * time.Millisecond)
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	initialReq = initialReq.WithContext(ctx)
	if va.userAgent != "" {
		initialReq.Header.Set("User-Agent", va.userAgent)
	}
	// Some of our users use mod_security. Mod_security sees a lack of Accept
	// headers as bot behavior and rejects requests. While this is a bug in
	// mod_security's rules (given that the HTTP specs disagree with that
	// requirement), we add the Accept header now in order to fix our
	// mod_security users' mysterious breakages. See
	// <https://github.com/SpiderLabs/owasp-modsecurity-crs/issues/265> and
	// <https://github.com/letsencrypt/boulder/issues/1019>. This was done
	// because it's a one-line fix with no downside. We're not likely to want to
	// do many more things to satisfy misunderstandings around HTTP.
	initialReq.Header.Set("Accept", "*/*")

	// Set up the initial validation request and a base validation record
	dialer, baseRecord, err := va.setupHTTPValidation(initialReq.URL.String(), target)
	if err != nil {
		return nil, []core.ValidationRecord{}, newIPError(target.cur, err)
	}

	// Build a transport for this validation that will use the preresolvedDialer's
	// DialContext function
	transport := httpTransport(dialer.DialContext)

	va.log.AuditInfof("Attempting to validate HTTP-01 for %q with GET to %q",
		initialReq.Host, initialReq.URL.String())

	// Create a closure around records & numRedirects we can use with a HTTP
	// client to process redirects per our own policy (e.g. resolving IP
	// addresses explicitly, not following redirects to ports != [80,443], etc)
	records := []core.ValidationRecord{baseRecord}
	numRedirects := 0
	processRedirect := func(req *http.Request, via []*http.Request) error {
		va.log.Debugf("processing a HTTP redirect from the server to %q", req.URL.String())
		// Only process up to maxRedirect redirects
		if numRedirects > maxRedirect {
			return berrors.ConnectionFailureError("Too many redirects")
		}
		numRedirects++
		va.metrics.http01Redirects.Inc()

		if req.Response.TLS != nil && req.Response.TLS.Version < tls.VersionTLS12 {
			return berrors.ConnectionFailureError(
				"validation attempt was redirected to an HTTPS server that doesn't " +
					"support TLSv1.2 or better. See " +
					"https://community.letsencrypt.org/t/rejecting-sha-1-csrs-and-validation-using-tls-1-0-1-1-urls/175144")
		}

		// If the response contains an HTTP 303 or any other forbidden redirect,
		// do not follow it. The four allowed redirect status codes are defined
		// explicitly in BRs Section 3.2.2.4.19. Although the go stdlib currently
		// limits redirects to a set of status codes with only one additional
		// entry (303), we capture the full list of allowed codes here in case the
		// go stdlib expands the set of redirects it follows in the future.
		acceptableRedirects := map[int]struct{}{
			301: {}, 302: {}, 307: {}, 308: {},
		}
		if _, present := acceptableRedirects[req.Response.StatusCode]; !present {
			return berrors.ConnectionFailureError("received disallowed redirect status code")
		}

		// Lowercase the redirect host immediately, as the dialer and redirect
		// validation expect it to have been lowercased already.
		req.URL.Host = strings.ToLower(req.URL.Host)

		// Extract the redirect target's host and port. This will return an error if
		// the redirect request scheme, host or port is not acceptable.
		redirHost, redirPort, err := va.extractRequestTarget(req)
		if err != nil {
			return err
		}

		redirPath := req.URL.Path
		if len(redirPath) > maxPathSize {
			return berrors.ConnectionFailureError("Redirect target too long")
		}

		// If the redirect URL has query parameters we need to preserve
		// those in the redirect path
		redirQuery := ""
		if req.URL.RawQuery != "" {
			redirQuery = req.URL.RawQuery
		}

		// Check for a redirect loop. If any URL is found twice before the
		// redirect limit, return error.
		for _, record := range records {
			if req.URL.String() == record.URL {
				return berrors.ConnectionFailureError("Redirect loop detected")
			}
		}

		// Create a validation target for the redirect host. This will resolve IP
		// addresses for the host explicitly.
		redirTarget, err := va.newHTTPValidationTarget(ctx, redirHost, redirPort, redirPath, redirQuery)
		if err != nil {
			return err
		}

		// Setup validation for the target. This will produce a preresolved dialer we can
		// assign to the client transport in order to connect to the redirect target using
		// the IP address we selected.
		redirDialer, redirRecord, err := va.setupHTTPValidation(req.URL.String(), redirTarget)
		records = append(records, redirRecord)
		if err != nil {
			return err
		}

		va.log.Debugf("following redirect to host %q url %q", req.Host, req.URL.String())
		// Replace the transport's DialContext with the new preresolvedDialer for
		// the redirect.
		transport.DialContext = redirDialer.DialContext
		return nil
	}

	// Create a new HTTP client configured to use the customized transport and
	// to check HTTP redirects encountered with processRedirect
	client := http.Client{
		Transport:     transport,
		CheckRedirect: processRedirect,
	}

	// Make the initial validation request. This may result in redirects being
	// followed.
	httpResponse, err := client.Do(initialReq)
	// If there was an error and its a kind of error we consider a fallback error,
	// then try to fallback.
	if err != nil && fallbackErr(err) {
		// Try to advance to another IP. If there was an error advancing we don't
		// have a fallback address to use and must return the original error.
		advanceTargetIPErr := target.nextIP()
		if advanceTargetIPErr != nil {
			return nil, records, newIPError(records[len(records)-1].AddressUsed, err)
		}

		// setup another validation to retry the target with the new IP and append
		// the retry record.
		retryDialer, retryRecord, err := va.setupHTTPValidation(initialReq.URL.String(), target)
		if err != nil {
			return nil, records, newIPError(records[len(records)-1].AddressUsed, err)
		}

		records = append(records, retryRecord)
		va.metrics.http01Fallbacks.Inc()
		// Replace the transport's dialer with the preresolvedDialer for the retry
		// host.
		transport.DialContext = retryDialer.DialContext

		// Perform the retry
		httpResponse, err = client.Do(initialReq)
		// If the retry still failed there isn't anything more to do, return the
		// error immediately.
		if err != nil {
			return nil, records, newIPError(records[len(records)-1].AddressUsed, err)
		}
	} else if err != nil {
		// if the error was not a fallbackErr then return immediately.
		return nil, records, newIPError(records[len(records)-1].AddressUsed, err)
	}

	if httpResponse.StatusCode != 200 {
		return nil, records, newIPError(records[len(records)-1].AddressUsed, berrors.UnauthorizedError("Invalid response from %s: %d",
			records[len(records)-1].URL, httpResponse.StatusCode))
	}

	// At this point we've made a successful request (be it from a retry or
	// otherwise) and can read and process the response body.
	body, err := io.ReadAll(&io.LimitedReader{R: httpResponse.Body, N: maxResponseSize})
	closeErr := httpResponse.Body.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, records, newIPError(records[len(records)-1].AddressUsed, berrors.UnauthorizedError("Error reading HTTP response body: %v", err))
	}

	// io.LimitedReader will silently truncate a Reader so if the
	// resulting payload is the same size as maxResponseSize fail
	if len(body) >= maxResponseSize {
		return nil, records, newIPError(records[len(records)-1].AddressUsed, berrors.UnauthorizedError("Invalid response from %s: %q",
			records[len(records)-1].URL, body))
	}

	return body, records, nil
}

func (va *ValidationAuthorityImpl) validateHTTP01(ctx context.Context, ident identifier.ACMEIdentifier, token string, keyAuthorization string) ([]core.ValidationRecord, error) {
	if ident.Type != identifier.TypeDNS && ident.Type != identifier.TypeIP {
		va.log.Info(fmt.Sprintf("Identifier type for HTTP-01 challenge was not DNS or IP: %s", ident))
		return nil, berrors.MalformedError("Identifier type for HTTP-01 challenge was not DNS or IP")
	}

	// Perform the fetch
	path := fmt.Sprintf(".well-known/acme-challenge/%s", token)
	body, validationRecords, err := va.processHTTPValidation(ctx, ident, "/"+path)
	if err != nil {
		return validationRecords, err
	}
	payload := strings.TrimRightFunc(string(body), unicode.IsSpace)

	if payload != keyAuthorization {
		problem := berrors.UnauthorizedError("The key authorization file from the server did not match this challenge. Expected %q (got %q)",
			keyAuthorization, payload)
		va.log.Infof("%s for %s", problem, ident)
		return validationRecords, problem
	}

	return validationRecords, nil
}
