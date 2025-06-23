package va

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/letsencrypt/boulder/bdns"
	"github.com/letsencrypt/boulder/canceled"
	"github.com/letsencrypt/boulder/core"
	berrors "github.com/letsencrypt/boulder/errors"
	bgrpc "github.com/letsencrypt/boulder/grpc"
	"github.com/letsencrypt/boulder/identifier"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/probs"
	vapb "github.com/letsencrypt/boulder/va/proto"
)

var (
	// badTLSHeader contains the string 'HTTP /' which is returned when
	// we try to talk TLS to a server that only talks HTTP
	badTLSHeader = []byte{0x48, 0x54, 0x54, 0x50, 0x2f}
	// h2SettingsFrameErrRegex is a regex against a net/http error indicating
	// a malformed HTTP response that matches the initial SETTINGS frame of an
	// HTTP/2 connection. This happens when a server configures HTTP/2 on port
	// :80, failing HTTP-01 challenges.
	//
	// The regex first matches the error string prefix and then matches the raw
	// bytes of an arbitrarily sized HTTP/2 SETTINGS frame:
	//   0x00 0x00 0x?? 0x04 0x00 0x00 0x00 0x00
	//
	// The third byte is variable and indicates the frame size. Typically
	// this will be 0x12.
	// The 0x04 in the fourth byte indicates that the frame is SETTINGS type.
	//
	// See:
	//   * https://tools.ietf.org/html/rfc7540#section-4.1
	//   * https://tools.ietf.org/html/rfc7540#section-6.5
	//
	// NOTE(@cpu): Using a regex is a hack but unfortunately for this case
	// http.Client.Do() will return a url.Error err that wraps
	// a errors.ErrorString instance. There isn't much else to do with one of
	// those except match the encoded byte string with a regex. :-X
	//
	// NOTE(@cpu): The first component of this regex is optional to avoid an
	// integration test flake. In some (fairly rare) conditions the malformed
	// response error will be returned simply as a http.badStringError without
	// the broken transport prefix. Most of the time the error is returned with
	// a transport connection error prefix.
	h2SettingsFrameErrRegex = regexp.MustCompile(`(?:net\/http\: HTTP\/1\.x transport connection broken: )?malformed HTTP response \"\\x00\\x00\\x[a-f0-9]{2}\\x04\\x00\\x00\\x00\\x00\\x00.*"`)
)

// RemoteClients wraps the vapb.VAClient and vapb.CAAClient interfaces to aid in
// mocking remote VAs for testing.
type RemoteClients struct {
	vapb.VAClient
	vapb.CAAClient
}

// RemoteVA embeds RemoteClients and adds a field containing the address of the
// remote gRPC server since the underlying gRPC client doesn't provide a way to
// extract this metadata which is useful for debugging gRPC connection issues.
type RemoteVA struct {
	RemoteClients
	Address string
}

type vaMetrics struct {
	validationTime                    *prometheus.HistogramVec
	localValidationTime               *prometheus.HistogramVec
	remoteValidationTime              *prometheus.HistogramVec
	remoteValidationFailures          prometheus.Counter
	caaCheckTime                      *prometheus.HistogramVec
	localCAACheckTime                 *prometheus.HistogramVec
	remoteCAACheckTime                *prometheus.HistogramVec
	remoteCAACheckFailures            prometheus.Counter
	prospectiveRemoteCAACheckFailures prometheus.Counter
	tlsALPNOIDCounter                 *prometheus.CounterVec
	http01Fallbacks                   prometheus.Counter
	http01Redirects                   prometheus.Counter
	caaCounter                        *prometheus.CounterVec
	ipv4FallbackCounter               prometheus.Counter
}

func initMetrics(stats prometheus.Registerer) *vaMetrics {
	validationTime := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "validation_time",
			Help:    "Total time taken to validate a challenge and aggregate results",
			Buckets: metrics.InternetFacingBuckets,
		},
		[]string{"type", "result", "problem_type"})
	stats.MustRegister(validationTime)
	localValidationTime := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "local_validation_time",
			Help:    "Time taken to locally validate a challenge",
			Buckets: metrics.InternetFacingBuckets,
		},
		[]string{"type", "result"})
	stats.MustRegister(localValidationTime)
	remoteValidationTime := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "remote_validation_time",
			Help:    "Time taken to remotely validate a challenge",
			Buckets: metrics.InternetFacingBuckets,
		},
		[]string{"type"})
	stats.MustRegister(remoteValidationTime)
	remoteValidationFailures := prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "remote_validation_failures",
			Help: "Number of validations failed due to remote VAs returning failure when consensus is enforced",
		})
	stats.MustRegister(remoteValidationFailures)
	caaCheckTime := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "caa_check_time",
			Help:    "Total time taken to check CAA records and aggregate results",
			Buckets: metrics.InternetFacingBuckets,
		},
		[]string{"result"})
	stats.MustRegister(caaCheckTime)
	localCAACheckTime := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "caa_check_time_local",
			Help:    "Time taken to locally check CAA records",
			Buckets: metrics.InternetFacingBuckets,
		},
		[]string{"result"})
	stats.MustRegister(localCAACheckTime)
	remoteCAACheckTime := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "caa_check_time_remote",
			Help:    "Time taken to remotely check CAA records",
			Buckets: metrics.InternetFacingBuckets,
		},
		[]string{"result"})
	stats.MustRegister(remoteCAACheckTime)
	remoteCAACheckFailures := prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "remote_caa_check_failures",
			Help: "Number of CAA checks failed due to remote VAs returning failure when consensus is enforced",
		})
	stats.MustRegister(remoteCAACheckFailures)
	prospectiveRemoteCAACheckFailures := prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "prospective_remote_caa_check_failures",
			Help: "Number of CAA rechecks that would have failed due to remote VAs returning failure if consesus were enforced",
		})
	stats.MustRegister(prospectiveRemoteCAACheckFailures)
	tlsALPNOIDCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tls_alpn_oid_usage",
			Help: "Number of TLS ALPN validations using either of the two OIDs",
		},
		[]string{"oid"},
	)
	stats.MustRegister(tlsALPNOIDCounter)
	http01Fallbacks := prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "http01_fallbacks",
			Help: "Number of IPv6 to IPv4 HTTP-01 fallback requests made",
		})
	stats.MustRegister(http01Fallbacks)
	http01Redirects := prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "http01_redirects",
			Help: "Number of HTTP-01 redirects followed",
		})
	stats.MustRegister(http01Redirects)
	caaCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "caa_sets_processed",
		Help: "A counter of CAA sets processed labelled by result",
	}, []string{"result"})
	stats.MustRegister(caaCounter)
	ipv4FallbackCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "tls_alpn_ipv4_fallback",
		Help: "A counter of IPv4 fallbacks during TLS ALPN validation",
	})
	stats.MustRegister(ipv4FallbackCounter)

	return &vaMetrics{
		validationTime:                    validationTime,
		remoteValidationTime:              remoteValidationTime,
		localValidationTime:               localValidationTime,
		remoteValidationFailures:          remoteValidationFailures,
		caaCheckTime:                      caaCheckTime,
		localCAACheckTime:                 localCAACheckTime,
		remoteCAACheckTime:                remoteCAACheckTime,
		remoteCAACheckFailures:            remoteCAACheckFailures,
		prospectiveRemoteCAACheckFailures: prospectiveRemoteCAACheckFailures,
		tlsALPNOIDCounter:                 tlsALPNOIDCounter,
		http01Fallbacks:                   http01Fallbacks,
		http01Redirects:                   http01Redirects,
		caaCounter:                        caaCounter,
		ipv4FallbackCounter:               ipv4FallbackCounter,
	}
}

// PortConfig specifies what ports the VA should call to on the remote
// host when performing its checks.
type portConfig struct {
	HTTPPort  int
	HTTPSPort int
	TLSPort   int
}

// newDefaultPortConfig is a constructor which returns a portConfig with default
// settings.
//
// CABF BRs section 1.6.1: Authorized Ports: One of the following ports: 80
// (http), 443 (https), 25 (smtp), 22 (ssh).
//
// RFC 8555 section 8.3: Dereference the URL using an HTTP GET request. This
// request MUST be sent to TCP port 80 on the HTTP server.
//
// RFC 8737 section 3: The ACME server initiates a TLS connection to the chosen
// IP address. This connection MUST use TCP port 443.
func newDefaultPortConfig() *portConfig {
	return &portConfig{
		HTTPPort:  80,
		HTTPSPort: 443,
		TLSPort:   443,
	}
}

// ValidationAuthorityImpl represents a VA
type ValidationAuthorityImpl struct {
	vapb.UnsafeVAServer
	vapb.UnsafeCAAServer
	log                blog.Logger
	dnsClient          bdns.Client
	issuerDomain       string
	httpPort           int
	httpsPort          int
	tlsPort            int
	userAgent          string
	clk                clock.Clock
	remoteVAs          []RemoteVA
	maxRemoteFailures  int
	accountURIPrefixes []string
	singleDialTimeout  time.Duration

	metrics *vaMetrics
}

var _ vapb.VAServer = (*ValidationAuthorityImpl)(nil)
var _ vapb.CAAServer = (*ValidationAuthorityImpl)(nil)

// NewValidationAuthorityImpl constructs a new VA
func NewValidationAuthorityImpl(
	resolver bdns.Client,
	remoteVAs []RemoteVA,
	maxRemoteFailures int,
	userAgent string,
	issuerDomain string,
	stats prometheus.Registerer,
	clk clock.Clock,
	logger blog.Logger,
	accountURIPrefixes []string,
) (*ValidationAuthorityImpl, error) {

	if len(accountURIPrefixes) == 0 {
		return nil, errors.New("no account URI prefixes configured")
	}

	pc := newDefaultPortConfig()

	va := &ValidationAuthorityImpl{
		log:                logger,
		dnsClient:          resolver,
		issuerDomain:       issuerDomain,
		httpPort:           pc.HTTPPort,
		httpsPort:          pc.HTTPSPort,
		tlsPort:            pc.TLSPort,
		userAgent:          userAgent,
		clk:                clk,
		metrics:            initMetrics(stats),
		remoteVAs:          remoteVAs,
		maxRemoteFailures:  maxRemoteFailures,
		accountURIPrefixes: accountURIPrefixes,
		// singleDialTimeout specifies how long an individual `DialContext` operation may take
		// before timing out. This timeout ignores the base RPC timeout and is strictly
		// used for the DialContext operations that take place during an
		// HTTP-01 challenge validation.
		singleDialTimeout: 10 * time.Second,
	}

	return va, nil
}

// Used for audit logging
type verificationRequestEvent struct {
	ID                string         `json:",omitempty"`
	Requester         int64          `json:",omitempty"`
	Hostname          string         `json:",omitempty"`
	Challenge         core.Challenge `json:",omitempty"`
	ValidationLatency float64
	UsedRSAKEX        bool   `json:",omitempty"`
	Error             string `json:",omitempty"`
	InternalError     string `json:",omitempty"`
}

// ipError is an error type used to pass though the IP address of the remote
// host when an error occurs during HTTP-01 and TLS-ALPN domain validation.
type ipError struct {
	ip  net.IP
	err error
}

// newIPError wraps an error and the IP of the remote host in an ipError so we
// can display the IP in the problem details returned to the client.
func newIPError(ip net.IP, err error) error {
	return ipError{ip: ip, err: err}
}

// Unwrap returns the underlying error.
func (i ipError) Unwrap() error {
	return i.err
}

// Error returns a string representation of the error.
func (i ipError) Error() string {
	return fmt.Sprintf("%s: %s", i.ip, i.err)
}

// detailedError returns a ProblemDetails corresponding to an error
// that occurred during HTTP-01 or TLS-ALPN domain validation. Specifically it
// tries to unwrap known Go error types and present something a little more
// meaningful. It additionally handles `berrors.ConnectionFailure` errors by
// passing through the detailed message.
func detailedError(err error) *probs.ProblemDetails {
	var ipErr ipError
	if errors.As(err, &ipErr) {
		detailedErr := detailedError(ipErr.err)
		if ipErr.ip == nil {
			// This should never happen.
			return detailedErr
		}
		// Prefix the error message with the IP address of the remote host.
		detailedErr.Detail = fmt.Sprintf("%s: %s", ipErr.ip, detailedErr.Detail)
		return detailedErr
	}
	// net/http wraps net.OpError in a url.Error. Unwrap them.
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		prob := detailedError(urlErr.Err)
		prob.Detail = fmt.Sprintf("Fetching %s: %s", urlErr.URL, prob.Detail)
		return prob
	}

	var tlsErr tls.RecordHeaderError
	if errors.As(err, &tlsErr) && bytes.Equal(tlsErr.RecordHeader[:], badTLSHeader) {
		return probs.Malformed("Server only speaks HTTP, not TLS")
	}

	var netOpErr *net.OpError
	if errors.As(err, &netOpErr) {
		if fmt.Sprintf("%T", netOpErr.Err) == "tls.alert" {
			// All the tls.alert error strings are reasonable to hand back to a
			// user. Confirmed against Go 1.8.
			return probs.TLS(netOpErr.Error())
		} else if netOpErr.Timeout() && netOpErr.Op == "dial" {
			return probs.Connection("Timeout during connect (likely firewall problem)")
		} else if netOpErr.Timeout() {
			return probs.Connection(fmt.Sprintf("Timeout during %s (your server may be slow or overloaded)", netOpErr.Op))
		}
	}
	var syscallErr *os.SyscallError
	if errors.As(err, &syscallErr) {
		switch syscallErr.Err {
		case syscall.ECONNREFUSED:
			return probs.Connection("Connection refused")
		case syscall.ENETUNREACH:
			return probs.Connection("Network unreachable")
		case syscall.ECONNRESET:
			return probs.Connection("Connection reset by peer")
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return probs.Connection("Timeout after connect (your server may be slow or overloaded)")
	}
	if errors.Is(err, berrors.ConnectionFailure) {
		return probs.Connection(err.Error())
	}
	if errors.Is(err, berrors.Unauthorized) {
		return probs.Unauthorized(err.Error())
	}
	if errors.Is(err, berrors.DNS) {
		return probs.DNS(err.Error())
	}
	if errors.Is(err, berrors.Malformed) {
		return probs.Malformed(err.Error())
	}
	if errors.Is(err, berrors.CAA) {
		return probs.CAA(err.Error())
	}

	if h2SettingsFrameErrRegex.MatchString(err.Error()) {
		return probs.Connection("Server is speaking HTTP/2 over HTTP")
	}
	return probs.Connection("Error getting validation data")
}

// validateChallenge simply passes through to the appropriate validation method
// depending on the challenge type.
func (va *ValidationAuthorityImpl) validateChallenge(
	ctx context.Context,
	ident identifier.ACMEIdentifier,
	kind core.AcmeChallenge,
	token string,
	keyAuthorization string,
) ([]core.ValidationRecord, error) {
	// Strip a (potential) leading wildcard token from the identifier.
	ident.Value = strings.TrimPrefix(ident.Value, "*.")

	switch kind {
	case core.ChallengeTypeHTTP01:
		return va.validateHTTP01(ctx, ident, token, keyAuthorization)
	case core.ChallengeTypeDNS01:
		return va.validateDNS01(ctx, ident, keyAuthorization)
	case core.ChallengeTypeTLSALPN01:
		return va.validateTLSALPN01(ctx, ident, keyAuthorization)
	}
	return nil, berrors.MalformedError("invalid challenge type %s", kind)
}

// performRemoteValidation coordinates the whole process of kicking off and
// collecting results from calls to remote VAs' PerformValidation function. It
// returns a problem if too many remote perspectives failed to corroborate
// domain control, or nil if enough succeeded to surpass our corroboration
// threshold.
func (va *ValidationAuthorityImpl) performRemoteValidation(
	ctx context.Context,
	req *vapb.PerformValidationRequest,
) *probs.ProblemDetails {
	if len(va.remoteVAs) == 0 {
		return nil
	}

	start := va.clk.Now()
	defer func() {
		va.metrics.remoteValidationTime.With(prometheus.Labels{
			"type": req.Challenge.Type,
		}).Observe(va.clk.Since(start).Seconds())
	}()

	type rvaResult struct {
		hostname string
		response *vapb.ValidationResult
		err      error
	}

	results := make(chan *rvaResult)

	for _, i := range rand.Perm(len(va.remoteVAs)) {
		remoteVA := va.remoteVAs[i]
		go func(rva RemoteVA, out chan<- *rvaResult) {
			res, err := rva.PerformValidation(ctx, req)
			out <- &rvaResult{
				hostname: rva.Address,
				response: res,
				err:      err,
			}
		}(remoteVA, results)
	}

	required := len(va.remoteVAs) - va.maxRemoteFailures
	good := 0
	bad := 0
	var firstProb *probs.ProblemDetails

	for res := range results {
		var currProb *probs.ProblemDetails

		if res.err != nil {
			bad++

			if canceled.Is(res.err) {
				currProb = probs.ServerInternal("Remote PerformValidation RPC canceled")
			} else {
				va.log.Errf("Remote VA %q.PerformValidation failed: %s", res.hostname, res.err)
				currProb = probs.ServerInternal("Remote PerformValidation RPC failed")
			}
		} else if res.response.Problems != nil {
			bad++

			var err error
			currProb, err = bgrpc.PBToProblemDetails(res.response.Problems)
			if err != nil {
				va.log.Errf("Remote VA %q.PerformValidation returned malformed problem: %s", res.hostname, err)
				currProb = probs.ServerInternal("Remote PerformValidation RPC returned malformed result")
			}
		} else {
			good++
		}

		if firstProb == nil && currProb != nil {
			firstProb = currProb
		}

		// Return as soon as we have enough successes or failures for a definitive result.
		if good >= required {
			return nil
		}
		if bad > va.maxRemoteFailures {
			va.metrics.remoteValidationFailures.Inc()
			firstProb.Detail = fmt.Sprintf("During secondary validation: %s", firstProb.Detail)
			return firstProb
		}

		// If we somehow haven't returned early, we need to break the loop once all
		// of the VAs have returned a result.
		if good+bad >= len(va.remoteVAs) {
			break
		}
	}

	// This condition should not occur - it indicates the good/bad counts neither
	// met the required threshold nor the maxRemoteFailures threshold.
	return probs.ServerInternal("Too few remote PerformValidation RPC results")
}

// logRemoteResults is called by `processRemoteCAAResults` when the
// `MultiCAAFullResults` feature flag is enabled. It produces a JSON log line
// that contains the results each remote VA returned.
func (va *ValidationAuthorityImpl) logRemoteResults(
	domain string,
	acctID int64,
	challengeType string,
	remoteResults []*remoteVAResult) {

	var successes, failures []*remoteVAResult

	for _, result := range remoteResults {
		if result.Problem != nil {
			failures = append(failures, result)
		} else {
			successes = append(successes, result)
		}
	}
	if len(failures) == 0 {
		// There's no point logging a differential line if everything succeeded.
		return
	}

	logOb := struct {
		Domain          string
		AccountID       int64
		ChallengeType   string
		RemoteSuccesses int
		RemoteFailures  []*remoteVAResult
	}{
		Domain:          domain,
		AccountID:       acctID,
		ChallengeType:   challengeType,
		RemoteSuccesses: len(successes),
		RemoteFailures:  failures,
	}

	logJSON, err := json.Marshal(logOb)
	if err != nil {
		// log a warning - a marshaling failure isn't expected given the data
		// isn't critical enough to break validation by returning an error the
		// caller.
		va.log.Warningf("Could not marshal log object in "+
			"logRemoteDifferential: %s", err)
		return
	}

	va.log.Infof("remoteVADifferentials JSON=%s", string(logJSON))
}

// remoteVAResult is a struct that combines a problem details instance (that may
// be nil) with the remote VA hostname that produced it.
type remoteVAResult struct {
	VAHostname string
	Problem    *probs.ProblemDetails
}

// performLocalValidation performs primary domain control validation and then
// checks CAA. If either step fails, it immediately returns a bare error so
// that our audit logging can include the underlying error.
func (va *ValidationAuthorityImpl) performLocalValidation(
	ctx context.Context,
	ident identifier.ACMEIdentifier,
	regid int64,
	kind core.AcmeChallenge,
	token string,
	keyAuthorization string,
) ([]core.ValidationRecord, error) {
	// Do primary domain control validation. Any kind of error returned by this
	// counts as a validation error, and will be converted into an appropriate
	// probs.ProblemDetails by the calling function.
	records, err := va.validateChallenge(ctx, ident, kind, token, keyAuthorization)
	if err != nil {
		return records, err
	}

	// Do primary CAA checks. Any kind of error returned by this counts as not
	// receiving permission to issue, and will be converted into an appropriate
	// probs.ProblemDetails by the calling function.
	err = va.checkCAA(ctx, ident, &caaParams{
		accountURIID:     regid,
		validationMethod: kind,
	})
	if err != nil {
		return records, err
	}

	return records, nil
}

// PerformValidation validates the challenge for the domain in the request.
// The returned result will always contain a list of validation records, even
// when it also contains a problem.
func (va *ValidationAuthorityImpl) PerformValidation(ctx context.Context, req *vapb.PerformValidationRequest) (*vapb.ValidationResult, error) {
	// TODO(#7514): Add req.ExpectedKeyAuthorization to this check
	if core.IsAnyNilOrZero(req, req.Domain, req.Challenge, req.Authz) {
		return nil, berrors.InternalServerError("Incomplete validation request")
	}

	challenge, err := bgrpc.PBToChallenge(req.Challenge)
	if err != nil {
		return nil, errors.New("challenge failed to deserialize")
	}

	err = challenge.CheckPending()
	if err != nil {
		return nil, berrors.MalformedError("challenge failed consistency check: %s", err)
	}

	// TODO(#7514): Remove this fallback and belt-and-suspenders check.
	keyAuthorization := req.ExpectedKeyAuthorization
	if len(keyAuthorization) == 0 {
		keyAuthorization = req.Challenge.KeyAuthorization
	}
	if len(keyAuthorization) == 0 {
		return nil, errors.New("no expected keyAuthorization provided")
	}

	// Set up variables and a deferred closure to report validation latency
	// metrics and log validation errors. Below here, do not use := to redeclare
	// `prob`, or this will fail.
	var prob *probs.ProblemDetails
	var localLatency time.Duration
	vStart := va.clk.Now()
	logEvent := verificationRequestEvent{
		ID:        req.Authz.Id,
		Requester: req.Authz.RegID,
		Hostname:  req.Domain,
		Challenge: challenge,
	}
	defer func() {
		problemType := ""
		if prob != nil {
			problemType = string(prob.Type)
			logEvent.Error = prob.Error()
			logEvent.Challenge.Error = prob
			logEvent.Challenge.Status = core.StatusInvalid
		} else {
			logEvent.Challenge.Status = core.StatusValid
		}

		va.metrics.localValidationTime.With(prometheus.Labels{
			"type":   string(logEvent.Challenge.Type),
			"result": string(logEvent.Challenge.Status),
		}).Observe(localLatency.Seconds())

		va.metrics.validationTime.With(prometheus.Labels{
			"type":         string(logEvent.Challenge.Type),
			"result":       string(logEvent.Challenge.Status),
			"problem_type": problemType,
		}).Observe(time.Since(vStart).Seconds())

		logEvent.ValidationLatency = time.Since(vStart).Round(time.Millisecond).Seconds()
		va.log.AuditObject("Validation result", logEvent)
	}()

	// Do local validation. Note that we process the result in a couple ways
	// *before* checking whether it returned an error. These few checks are
	// carefully written to ensure that they work whether the local validation
	// was successful or not, and cannot themselves fail.
	records, err := va.performLocalValidation(
		ctx,
		identifier.DNSIdentifier(req.Domain),
		req.Authz.RegID,
		challenge.Type,
		challenge.Token,
		keyAuthorization)
	localLatency = time.Since(vStart)

	// Check for malformed ValidationRecords
	logEvent.Challenge.ValidationRecord = records
	if err == nil && !logEvent.Challenge.RecordsSane() {
		err = errors.New("records from local validation failed sanity check")
	}

	// Copy the "UsedRSAKEX" value from the last validationRecord into the log
	// event. Only the last record should have this bool set, because we only
	// record it if/when validation is finally successful, but we use the loop
	// just in case that assumption changes.
	// TODO(#7321): Remove this when we have collected enough data.
	for _, record := range records {
		logEvent.UsedRSAKEX = record.UsedRSAKEX || logEvent.UsedRSAKEX
	}

	if err != nil {
		logEvent.InternalError = err.Error()
		prob = detailedError(err)
		return bgrpc.ValidationResultToPB(records, filterProblemDetails(prob))
	}

	// Do remote validation. We do this after local validation is complete to
	// avoid wasting work when validation will fail anyway. This only returns a
	// singular problem, because the remote VAs have already audit-logged their
	// own validation records, and it's not helpful to present multiple large
	// errors to the end user.
	prob = va.performRemoteValidation(ctx, req)
	return bgrpc.ValidationResultToPB(records, filterProblemDetails(prob))
}

// usedRSAKEX returns true if the given cipher suite involves the use of an
// RSA key exchange mechanism.
// TODO(#7321): Remove this when we have collected enough data.
func usedRSAKEX(cs uint16) bool {
	return strings.HasPrefix(tls.CipherSuiteName(cs), "TLS_RSA_")
}
