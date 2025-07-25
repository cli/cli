package wfe2

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"math/rand/v2"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/letsencrypt/boulder/core"
	corepb "github.com/letsencrypt/boulder/core/proto"
	emailpb "github.com/letsencrypt/boulder/email/proto"
	berrors "github.com/letsencrypt/boulder/errors"
	"github.com/letsencrypt/boulder/features"
	"github.com/letsencrypt/boulder/goodkey"
	bgrpc "github.com/letsencrypt/boulder/grpc"
	_ "github.com/letsencrypt/boulder/grpc/noncebalancer" // imported for its init function.
	"github.com/letsencrypt/boulder/identifier"
	"github.com/letsencrypt/boulder/issuance"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/metrics/measured_http"
	"github.com/letsencrypt/boulder/nonce"
	"github.com/letsencrypt/boulder/policy"
	"github.com/letsencrypt/boulder/probs"
	rapb "github.com/letsencrypt/boulder/ra/proto"
	"github.com/letsencrypt/boulder/ratelimits"
	"github.com/letsencrypt/boulder/revocation"
	sapb "github.com/letsencrypt/boulder/sa/proto"
	"github.com/letsencrypt/boulder/unpause"
	"github.com/letsencrypt/boulder/web"
)

// Paths are the ACME-spec identified URL path-segments for various methods.
// NOTE: In metrics/measured_http we make the assumption that these are all
// lowercase plus hyphens. If you violate that assumption you should update
// measured_http.
const (
	directoryPath     = "/directory"
	newNoncePath      = "/acme/new-nonce"
	newAcctPath       = "/acme/new-acct"
	newOrderPath      = "/acme/new-order"
	rolloverPath      = "/acme/key-change"
	revokeCertPath    = "/acme/revoke-cert"
	acctPath          = "/acme/acct/"
	orderPath         = "/acme/order/"
	authzPath         = "/acme/authz/"
	challengePath     = "/acme/chall/"
	finalizeOrderPath = "/acme/finalize/"
	certPath          = "/acme/cert/"
	renewalInfoPath   = "/acme/renewal-info/"

	// Non-ACME paths.
	getCertPath = "/get/cert/"
	buildIDPath = "/build"
)

const (
	headerRetryAfter = "Retry-After"
	// Our 99th percentile finalize latency is 2.3s. Asking clients to wait 3s
	// before polling the order to get an updated status means that >99% of
	// clients will fetch the updated order object exactly once,.
	orderRetryAfter = 3
)

var errIncompleteGRPCResponse = errors.New("incomplete gRPC response message")

// WebFrontEndImpl provides all the logic for Boulder's web-facing interface,
// i.e., ACME.  Its members configure the paths for various ACME functions,
// plus a few other data items used in ACME.  Its methods are primarily handlers
// for HTTPS requests for the various ACME functions.
type WebFrontEndImpl struct {
	ra rapb.RegistrationAuthorityClient
	sa sapb.StorageAuthorityReadOnlyClient
	ee emailpb.ExporterClient
	// gnc is a nonce-service client used exclusively for the issuance of
	// nonces. It's configured to route requests to backends colocated with the
	// WFE.
	gnc nonce.Getter
	// rnc is a nonce-service client used exclusively for the redemption of
	// nonces. It uses a custom RPC load balancer which is configured to route
	// requests to backends based on the prefix and HMAC key passed as in the
	// context of the request. The HMAC and prefix are passed using context keys
	// `nonce.HMACKeyCtxKey` and `nonce.PrefixCtxKey`.
	rnc nonce.Redeemer
	// rncKey is the HMAC key used to derive the prefix of nonce backends used
	// for nonce redemption.
	rncKey        []byte
	accountGetter AccountGetter
	log           blog.Logger
	clk           clock.Clock
	stats         wfe2Stats

	// certificateChains maps IssuerNameIDs to slice of []byte containing a leading
	// newline and one or more PEM encoded certificates separated by a newline,
	// sorted from leaf to root. The first []byte is the default certificate chain,
	// and any subsequent []byte is an alternate certificate chain.
	certificateChains map[issuance.NameID][][]byte

	// issuerCertificates is a map of IssuerNameIDs to issuer certificates built with the
	// first entry from each of the certificateChains. These certificates are used
	// to verify the signature of certificates provided in revocation requests.
	issuerCertificates map[issuance.NameID]*issuance.Certificate

	// URL to the current subscriber agreement (should contain some version identifier)
	SubscriberAgreementURL string

	// DirectoryCAAIdentity is used for the /directory response's "meta"
	// element's "caaIdentities" field. It should match the VA's issuerDomain
	// field value.
	DirectoryCAAIdentity string

	// DirectoryWebsite is used for the /directory response's "meta" element's
	// "website" field.
	DirectoryWebsite string

	// Allowed prefix for legacy accounts used by verify.go's `lookupJWK`.
	// See `cmd/boulder-wfe2/main.go`'s comment on the configuration field
	// `LegacyKeyIDPrefix` for more information.
	LegacyKeyIDPrefix string

	// Key policy.
	keyPolicy goodkey.KeyPolicy

	// CORS settings
	AllowOrigins []string

	// How many contacts to allow in a single NewAccount request.
	maxContactsPerReg int

	// requestTimeout is the per-request overall timeout.
	requestTimeout time.Duration

	// StaleTimeout determines the required staleness for certificates to be
	// accessed via the Boulder-specific GET API. Certificates newer than
	// staleTimeout must be accessed via POST-as-GET and the RFC 8555 ACME API. We
	// do this to incentivize client developers to use the standard API.
	staleTimeout time.Duration

	limiter    *ratelimits.Limiter
	txnBuilder *ratelimits.TransactionBuilder

	unpauseSigner      unpause.JWTSigner
	unpauseJWTLifetime time.Duration
	unpauseURL         string

	// certProfiles is a map of acceptable certificate profile names to
	// descriptions (perhaps including URLs) of those profiles. NewOrder
	// Requests with a profile name not present in this map will be rejected.
	certProfiles map[string]string
}

// NewWebFrontEndImpl constructs a web service for Boulder
func NewWebFrontEndImpl(
	stats prometheus.Registerer,
	clk clock.Clock,
	keyPolicy goodkey.KeyPolicy,
	certificateChains map[issuance.NameID][][]byte,
	issuerCertificates map[issuance.NameID]*issuance.Certificate,
	logger blog.Logger,
	requestTimeout time.Duration,
	staleTimeout time.Duration,
	maxContactsPerReg int,
	rac rapb.RegistrationAuthorityClient,
	sac sapb.StorageAuthorityReadOnlyClient,
	eec emailpb.ExporterClient,
	gnc nonce.Getter,
	rnc nonce.Redeemer,
	rncKey []byte,
	accountGetter AccountGetter,
	limiter *ratelimits.Limiter,
	txnBuilder *ratelimits.TransactionBuilder,
	certProfiles map[string]string,
	unpauseSigner unpause.JWTSigner,
	unpauseJWTLifetime time.Duration,
	unpauseURL string,
) (WebFrontEndImpl, error) {
	if len(issuerCertificates) == 0 {
		return WebFrontEndImpl{}, errors.New("must provide at least one issuer certificate")
	}

	if len(certificateChains) == 0 {
		return WebFrontEndImpl{}, errors.New("must provide at least one certificate chain")
	}

	if gnc == nil {
		return WebFrontEndImpl{}, errors.New("must provide a service for nonce issuance")
	}

	if rnc == nil {
		return WebFrontEndImpl{}, errors.New("must provide a service for nonce redemption")
	}

	wfe := WebFrontEndImpl{
		log:                logger,
		clk:                clk,
		keyPolicy:          keyPolicy,
		certificateChains:  certificateChains,
		issuerCertificates: issuerCertificates,
		stats:              initStats(stats),
		requestTimeout:     requestTimeout,
		staleTimeout:       staleTimeout,
		maxContactsPerReg:  maxContactsPerReg,
		ra:                 rac,
		sa:                 sac,
		ee:                 eec,
		gnc:                gnc,
		rnc:                rnc,
		rncKey:             rncKey,
		accountGetter:      accountGetter,
		limiter:            limiter,
		txnBuilder:         txnBuilder,
		certProfiles:       certProfiles,
		unpauseSigner:      unpauseSigner,
		unpauseJWTLifetime: unpauseJWTLifetime,
		unpauseURL:         unpauseURL,
	}

	return wfe, nil
}

// HandleFunc registers a handler at the given path. It's
// http.HandleFunc(), but with a wrapper around the handler that
// provides some generic per-request functionality:
//
// * Set a Replay-Nonce header.
//
// * Respond to OPTIONS requests, including CORS preflight requests.
//
// * Set a no cache header
//
// * Respond http.StatusMethodNotAllowed for HTTP methods other than
// those listed.
//
// * Set CORS headers when responding to CORS "actual" requests.
//
// * Never send a body in response to a HEAD request. Anything
// written by the handler will be discarded if the method is HEAD.
// Also, all handlers that accept GET automatically accept HEAD.
func (wfe *WebFrontEndImpl) HandleFunc(mux *http.ServeMux, pattern string, h web.WFEHandlerFunc, methods ...string) {
	methodsMap := make(map[string]bool)
	for _, m := range methods {
		methodsMap[m] = true
	}
	if methodsMap["GET"] && !methodsMap["HEAD"] {
		// Allow HEAD for any resource that allows GET
		methods = append(methods, "HEAD")
		methodsMap["HEAD"] = true
	}
	methodsStr := strings.Join(methods, ", ")
	handler := http.StripPrefix(pattern, web.NewTopHandler(wfe.log,
		web.WFEHandlerFunc(func(ctx context.Context, logEvent *web.RequestEvent, response http.ResponseWriter, request *http.Request) {
			span := trace.SpanFromContext(ctx)
			span.SetName(pattern)

			logEvent.Endpoint = pattern
			if request.URL != nil {
				logEvent.Slug = request.URL.Path
			}
			if request.Method != "GET" || pattern == newNoncePath {
				nonceMsg, err := wfe.gnc.Nonce(ctx, &emptypb.Empty{})
				if err != nil {
					wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "unable to get nonce"), err)
					return
				}
				response.Header().Set("Replay-Nonce", nonceMsg.Nonce)
			}
			// Per section 7.1 "Resources":
			//   The "index" link relation is present on all resources other than the
			//   directory and indicates the URL of the directory.
			if pattern != directoryPath {
				directoryURL := web.RelativeEndpoint(request, directoryPath)
				response.Header().Add("Link", link(directoryURL, "index"))
			}

			switch request.Method {
			case "HEAD":
				// Go's net/http (and httptest) servers will strip out the body
				// of responses for us. This keeps the Content-Length for HEAD
				// requests as the same as GET requests per the spec.
			case "OPTIONS":
				wfe.Options(response, request, methodsStr, methodsMap)
				return
			}

			// No cache header is set for all requests, succeed or fail.
			addNoCacheHeader(response)

			if !methodsMap[request.Method] {
				response.Header().Set("Allow", methodsStr)
				wfe.sendError(response, logEvent, probs.MethodNotAllowed(), nil)
				return
			}

			wfe.setCORSHeaders(response, request, "")

			timeout := wfe.requestTimeout
			if timeout == 0 {
				timeout = 5 * time.Minute
			}
			ctx, cancel := context.WithTimeout(ctx, timeout)

			// Call the wrapped handler.
			h(ctx, logEvent, response, request)
			cancel()
		}),
	))
	mux.Handle(pattern, handler)
}

func marshalIndent(v interface{}) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

func (wfe *WebFrontEndImpl) writeJsonResponse(response http.ResponseWriter, logEvent *web.RequestEvent, status int, v interface{}) error {
	jsonReply, err := marshalIndent(v)
	if err != nil {
		return err // All callers are responsible for handling this error
	}

	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_, err = response.Write(jsonReply)
	if err != nil {
		// Don't worry about returning this error because the caller will
		// never handle it.
		wfe.log.Warningf("Could not write response: %s", err)
		logEvent.AddError("failed to write response: %s", err)
	}
	return nil
}

// requestProto returns "http" for HTTP requests and "https" for HTTPS
// requests. It supports the use of "X-Forwarded-Proto" to override the protocol.
func requestProto(request *http.Request) string {
	proto := "http"

	// If the request was received via TLS, use `https://` for the protocol
	if request.TLS != nil {
		proto = "https"
	}

	// Allow upstream proxies  to specify the forwarded protocol. Allow this value
	// to override our own guess.
	if specifiedProto := request.Header.Get("X-Forwarded-Proto"); specifiedProto != "" {
		proto = specifiedProto
	}

	return proto
}

const randomDirKeyExplanationLink = "https://community.letsencrypt.org/t/adding-random-entries-to-the-directory/33417"

func (wfe *WebFrontEndImpl) relativeDirectory(request *http.Request, directory map[string]interface{}) ([]byte, error) {
	// Create an empty map sized equal to the provided directory to store the
	// relative-ized result
	relativeDir := make(map[string]interface{}, len(directory))

	// Copy each entry of the provided directory into the new relative map,
	// prefixing it with the request protocol and host.
	for k, v := range directory {
		if v == randomDirKeyExplanationLink {
			relativeDir[k] = v
			continue
		}
		switch v := v.(type) {
		case string:
			// Only relative-ize top level string values, e.g. not the "meta" element
			relativeDir[k] = web.RelativeEndpoint(request, v)
		default:
			// If it isn't a string, put it into the results unmodified
			relativeDir[k] = v
		}
	}

	directoryJSON, err := marshalIndent(relativeDir)
	// This should never happen since we are just marshalling known strings
	if err != nil {
		return nil, err
	}

	return directoryJSON, nil
}

// Handler returns an http.Handler that uses various functions for
// various ACME-specified paths.
func (wfe *WebFrontEndImpl) Handler(stats prometheus.Registerer, oTelHTTPOptions ...otelhttp.Option) http.Handler {
	m := http.NewServeMux()

	// POSTable ACME endpoints
	wfe.HandleFunc(m, newAcctPath, wfe.NewAccount, "POST")
	wfe.HandleFunc(m, acctPath, wfe.Account, "POST")
	wfe.HandleFunc(m, revokeCertPath, wfe.RevokeCertificate, "POST")
	wfe.HandleFunc(m, rolloverPath, wfe.KeyRollover, "POST")
	wfe.HandleFunc(m, newOrderPath, wfe.NewOrder, "POST")
	wfe.HandleFunc(m, finalizeOrderPath, wfe.FinalizeOrder, "POST")

	// GETable and POST-as-GETable ACME endpoints
	wfe.HandleFunc(m, directoryPath, wfe.Directory, "GET", "POST")
	wfe.HandleFunc(m, newNoncePath, wfe.Nonce, "GET", "POST")
	wfe.HandleFunc(m, orderPath, wfe.GetOrder, "GET", "POST")
	wfe.HandleFunc(m, authzPath, wfe.AuthorizationHandler, "GET", "POST")
	wfe.HandleFunc(m, challengePath, wfe.ChallengeHandler, "GET", "POST")
	wfe.HandleFunc(m, certPath, wfe.Certificate, "GET", "POST")

	// Boulder specific endpoints
	wfe.HandleFunc(m, getCertPath, wfe.Certificate, "GET")
	wfe.HandleFunc(m, buildIDPath, wfe.BuildID, "GET")

	// Endpoint for draft-ietf-acme-ari
	if features.Get().ServeRenewalInfo {
		wfe.HandleFunc(m, renewalInfoPath, wfe.RenewalInfo, "GET", "POST")
	}

	// We don't use our special HandleFunc for "/" because it matches everything,
	// meaning we can wind up returning 405 when we mean to return 404. See
	// https://github.com/letsencrypt/boulder/issues/717
	m.Handle("/", web.NewTopHandler(wfe.log, web.WFEHandlerFunc(wfe.Index)))
	return measured_http.New(m, wfe.clk, stats, oTelHTTPOptions...)
}

// Method implementations

// Index serves a simple identification page. It is not part of the ACME spec.
func (wfe *WebFrontEndImpl) Index(ctx context.Context, logEvent *web.RequestEvent, response http.ResponseWriter, request *http.Request) {
	// All requests that are not handled by our ACME endpoints ends up
	// here. Set the our logEvent endpoint to "/" and the slug to the path
	// minus "/" to make sure that we properly set log information about
	// the request, even in the case of a 404
	logEvent.Endpoint = "/"
	logEvent.Slug = request.URL.Path[1:]

	// http://golang.org/pkg/net/http/#example_ServeMux_Handle
	// The "/" pattern matches everything, so we need to check
	// that we're at the root here.
	if request.URL.Path != "/" {
		logEvent.AddError("Resource not found")
		http.NotFound(response, request)
		response.Header().Set("Content-Type", "application/problem+json")
		return
	}

	if request.Method != "GET" {
		response.Header().Set("Allow", "GET")
		wfe.sendError(response, logEvent, probs.MethodNotAllowed(), errors.New("Bad method"))
		return
	}

	addNoCacheHeader(response)
	response.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(response, `<html>
		<body>
			This is an <a href="https://tools.ietf.org/html/rfc8555">ACME</a>
			Certificate Authority running <a href="https://github.com/letsencrypt/boulder">Boulder</a>.
			JSON directory is available at <a href="%s">%s</a>.
		</body>
	</html>
	`, directoryPath, directoryPath)
}

func addNoCacheHeader(w http.ResponseWriter) {
	w.Header().Add("Cache-Control", "public, max-age=0, no-cache")
}

func addRequesterHeader(w http.ResponseWriter, requester int64) {
	if requester > 0 {
		w.Header().Set("Boulder-Requester", strconv.FormatInt(requester, 10))
	}
}

// Directory is an HTTP request handler that provides the directory
// object stored in the WFE's DirectoryEndpoints member with paths prefixed
// using the `request.Host` of the HTTP request.
func (wfe *WebFrontEndImpl) Directory(
	ctx context.Context,
	logEvent *web.RequestEvent,
	response http.ResponseWriter,
	request *http.Request) {
	directoryEndpoints := map[string]interface{}{
		"newAccount": newAcctPath,
		"newNonce":   newNoncePath,
		"revokeCert": revokeCertPath,
		"newOrder":   newOrderPath,
		"keyChange":  rolloverPath,
	}

	if features.Get().ServeRenewalInfo {
		// ARI-capable clients are expected to add the trailing slash per the
		// draft. We explicitly strip the trailing slash here so that clients
		// don't need to add trailing slash handling in their own code, saving
		// them minimal amounts of complexity.
		directoryEndpoints["renewalInfo"] = strings.TrimRight(renewalInfoPath, "/")
	}

	if request.Method == http.MethodPost {
		acct, err := wfe.validPOSTAsGETForAccount(request, ctx, logEvent)
		if err != nil {
			wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Unable to validate JWS"), err)
			return
		}
		logEvent.Requester = acct.ID
	}

	// Add a random key to the directory in order to make sure that clients don't hardcode an
	// expected set of keys. This ensures that we can properly extend the directory when we
	// need to add a new endpoint or meta element.
	directoryEndpoints[core.RandomString(8)] = randomDirKeyExplanationLink

	// ACME since draft-02 describes an optional "meta" directory entry. The
	// meta entry may optionally contain a "termsOfService" URI for the
	// current ToS.
	metaMap := map[string]interface{}{
		"termsOfService": wfe.SubscriberAgreementURL,
	}
	// The "meta" directory entry may also include a []string of CAA identities
	if wfe.DirectoryCAAIdentity != "" {
		// The specification says caaIdentities is an array of strings. In
		// practice Boulder's VA only allows configuring ONE CAA identity. Given
		// that constraint it doesn't make sense to allow multiple directory CAA
		// identities so we use just the `wfe.DirectoryCAAIdentity` alone.
		metaMap["caaIdentities"] = []string{
			wfe.DirectoryCAAIdentity,
		}
	}
	if len(wfe.certProfiles) != 0 {
		metaMap["profiles"] = wfe.certProfiles
	}
	// The "meta" directory entry may also include a string with a website URL
	if wfe.DirectoryWebsite != "" {
		metaMap["website"] = wfe.DirectoryWebsite
	}
	directoryEndpoints["meta"] = metaMap

	response.Header().Set("Content-Type", "application/json")

	relDir, err := wfe.relativeDirectory(request, directoryEndpoints)
	if err != nil {
		marshalProb := probs.ServerInternal("unable to marshal JSON directory")
		wfe.sendError(response, logEvent, marshalProb, nil)
		return
	}

	logEvent.Suppress()
	response.Write(relDir)
}

// Nonce is an endpoint for getting a fresh nonce with an HTTP GET or HEAD
// request. This endpoint only returns a status code header - the `HandleFunc`
// wrapper ensures that a nonce is written in the correct response header.
func (wfe *WebFrontEndImpl) Nonce(
	ctx context.Context,
	logEvent *web.RequestEvent,
	response http.ResponseWriter,
	request *http.Request) {
	if request.Method == http.MethodPost {
		acct, err := wfe.validPOSTAsGETForAccount(request, ctx, logEvent)
		if err != nil {
			wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Unable to validate JWS"), err)
			return
		}
		logEvent.Requester = acct.ID
	}

	statusCode := http.StatusNoContent
	// The ACME specification says GET requests should receive http.StatusNoContent
	// and HEAD/POST-as-GET requests should receive http.StatusOK.
	if request.Method != "GET" {
		statusCode = http.StatusOK
	}
	response.WriteHeader(statusCode)

	// The ACME specification says the server MUST include a Cache-Control header
	// field with the "no-store" directive in responses for the newNonce resource,
	// in order to prevent caching of this resource.
	response.Header().Set("Cache-Control", "no-store")

	// No need to log successful nonce requests, they're boring.
	logEvent.Suppress()
}

// sendError wraps web.SendError
func (wfe *WebFrontEndImpl) sendError(response http.ResponseWriter, logEvent *web.RequestEvent, eerr any, ierr error) {
	// TODO(#4980): Simplify this function to only take a single error argument,
	// and use web.ProblemDetailsForError to extract the corresponding prob from
	// that. For now, though, the third argument has to be `any` so that it can
	// be either an error or a problem, and this function can handle either one.
	var prob *probs.ProblemDetails
	switch v := eerr.(type) {
	case *probs.ProblemDetails:
		prob = v
	case error:
		prob = web.ProblemDetailsForError(v, "")
	default:
		panic(fmt.Sprintf("wfe.sendError got %#v (type %T), but expected ProblemDetails or error", eerr, eerr))
	}

	if prob.Type == probs.BadSignatureAlgorithmProblem {
		prob.Algorithms = getSupportedAlgs()
	}

	var bErr *berrors.BoulderError
	if errors.As(ierr, &bErr) {
		retryAfterSeconds := int(bErr.RetryAfter.Round(time.Second).Seconds())
		if retryAfterSeconds > 0 {
			response.Header().Add(headerRetryAfter, strconv.Itoa(retryAfterSeconds))
			if bErr.Type == berrors.RateLimit {
				response.Header().Add("Link", link("https://letsencrypt.org/docs/rate-limits", "help"))
			}
		}
	}
	if prob.HTTPStatus == http.StatusInternalServerError {
		response.Header().Add(headerRetryAfter, "60")
	}
	wfe.stats.httpErrorCount.With(prometheus.Labels{"type": string(prob.Type)}).Inc()
	web.SendError(wfe.log, response, logEvent, prob, ierr)
}

func link(url, relation string) string {
	return fmt.Sprintf("<%s>;rel=\"%s\"", url, relation)
}

// contactsToEmails converts a slice of ACME contacts (e.g.
// "mailto:person@example.com") to a slice of valid email addresses. If any of
// the contacts contain non-mailto schemes, unparsable addresses, or forbidden
// mail domains, it returns an error so that we can provide feedback to
// misconfigured clients.
func (wfe *WebFrontEndImpl) contactsToEmails(contacts []string) ([]string, error) {
	if len(contacts) == 0 {
		return nil, nil
	}

	if wfe.maxContactsPerReg > 0 && len(contacts) > wfe.maxContactsPerReg {
		return nil, berrors.MalformedError("too many contacts provided: %d > %d", len(contacts), wfe.maxContactsPerReg)
	}

	var emails []string
	for _, contact := range contacts {
		if contact == "" {
			return nil, berrors.InvalidEmailError("empty contact")
		}

		parsed, err := url.Parse(contact)
		if err != nil {
			return nil, berrors.InvalidEmailError("unparsable contact")
		}

		if parsed.Scheme != "mailto" {
			return nil, berrors.UnsupportedContactError("only contact scheme 'mailto:' is supported")
		}

		if parsed.RawQuery != "" || contact[len(contact)-1] == '?' {
			return nil, berrors.InvalidEmailError("contact email contains a question mark")
		}

		if parsed.Fragment != "" || contact[len(contact)-1] == '#' {
			return nil, berrors.InvalidEmailError("contact email contains a '#'")
		}

		if !core.IsASCII(contact) {
			return nil, berrors.InvalidEmailError("contact email contains non-ASCII characters")
		}

		err = policy.ValidEmail(parsed.Opaque)
		if err != nil {
			return nil, err
		}

		emails = append(emails, parsed.Opaque)
	}

	return emails, nil
}

// checkNewAccountLimits checks whether sufficient limit quota exists for the
// creation of a new account. If so, that quota is spent. If an error is
// encountered during the check, it is logged but not returned. A refund
// function is returned that can be called to refund the quota if the account
// creation fails, the func will be nil if any error was encountered during the
// check.
func (wfe *WebFrontEndImpl) checkNewAccountLimits(ctx context.Context, ip netip.Addr) (func(), error) {
	txns, err := wfe.txnBuilder.NewAccountLimitTransactions(ip)
	if err != nil {
		return nil, fmt.Errorf("building new account limit transactions: %w", err)
	}

	d, err := wfe.limiter.BatchSpend(ctx, txns)
	if err != nil {
		return nil, fmt.Errorf("spending new account limits: %w", err)
	}

	err = d.Result(wfe.clk.Now())
	if err != nil {
		return nil, err
	}

	return func() {
		_, err := wfe.limiter.BatchRefund(ctx, txns)
		if err != nil {
			wfe.log.Warningf("refunding new account limits: %s", err)
		}
	}, nil
}

// NewAccount is used by clients to submit a new account
func (wfe *WebFrontEndImpl) NewAccount(
	ctx context.Context,
	logEvent *web.RequestEvent,
	response http.ResponseWriter,
	request *http.Request) {

	// NewAccount uses `validSelfAuthenticatedPOST` instead of
	// `validPOSTforAccount` because there is no account to authenticate against
	// until after it is created!
	body, key, err := wfe.validSelfAuthenticatedPOST(ctx, request)
	if err != nil {
		// validSelfAuthenticatedPOST handles its own setting of logEvent.Errors
		wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Unable to validate JWS"), err)
		return
	}

	var accountCreateRequest struct {
		Contact              []string `json:"contact"`
		TermsOfServiceAgreed bool     `json:"termsOfServiceAgreed"`
		OnlyReturnExisting   bool     `json:"onlyReturnExisting"`
	}

	err = json.Unmarshal(body, &accountCreateRequest)
	if err != nil {
		wfe.sendError(response, logEvent, probs.Malformed("Error unmarshaling JSON"), err)
		return
	}

	returnExistingAcct := func(acctPB *corepb.Registration) {
		if core.AcmeStatus(acctPB.Status) == core.StatusDeactivated {
			// If there is an existing, but deactivated account, then return an unauthorized
			// problem informing the user that this account was deactivated
			wfe.sendError(response, logEvent, probs.Unauthorized(
				"An account with the provided public key exists but is deactivated"), nil)
			return
		}

		response.Header().Set("Location",
			web.RelativeEndpoint(request, fmt.Sprintf("%s%d", acctPath, acctPB.Id)))
		logEvent.Requester = acctPB.Id
		addRequesterHeader(response, acctPB.Id)

		acct, err := bgrpc.PbToRegistration(acctPB)
		if err != nil {
			wfe.sendError(response, logEvent, probs.ServerInternal("Error marshaling account"), err)
			return
		}
		prepAccountForDisplay(&acct)

		err = wfe.writeJsonResponse(response, logEvent, http.StatusOK, acct)
		if err != nil {
			// ServerInternal because we just created this account, and it
			// should be OK.
			wfe.sendError(response, logEvent, probs.ServerInternal("Error marshaling account"), err)
			return
		}
	}

	keyBytes, err := key.MarshalJSON()
	if err != nil {
		wfe.sendError(response, logEvent,
			web.ProblemDetailsForError(err, "Error creating new account"), err)
		return
	}
	existingAcct, err := wfe.sa.GetRegistrationByKey(ctx, &sapb.JSONWebKey{Jwk: keyBytes})
	if err == nil {
		returnExistingAcct(existingAcct)
		return
	} else if !errors.Is(err, berrors.NotFound) {
		wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "failed check for existing account"), err)
		return
	}

	// If the request included a true "OnlyReturnExisting" field and we did not
	// find an existing registration with the key specified then we must return an
	// error and not create a new account.
	if accountCreateRequest.OnlyReturnExisting {
		wfe.sendError(response, logEvent, probs.AccountDoesNotExist(
			"No account exists with the provided key"), nil)
		return
	}

	if !accountCreateRequest.TermsOfServiceAgreed {
		wfe.sendError(response, logEvent, probs.Malformed("must agree to terms of service"), nil)
		return
	}

	// Do this extraction now, so that we can reject requests whose contact field
	// does not contain valid contacts before we actually create the account.
	emails, err := wfe.contactsToEmails(accountCreateRequest.Contact)
	if err != nil {
		wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Error validating contact(s)"), nil)
		return
	}

	ip, err := extractRequesterIP(request)
	if err != nil {
		wfe.sendError(
			response,
			logEvent,
			probs.ServerInternal("couldn't parse the remote (that is, the client's) address"),
			fmt.Errorf("couldn't parse RemoteAddr: %s", request.RemoteAddr),
		)
		return
	}

	refundLimits, err := wfe.checkNewAccountLimits(ctx, ip)
	if err != nil {
		if errors.Is(err, berrors.RateLimit) {
			wfe.sendError(response, logEvent, probs.RateLimited(err.Error()), err)
			return
		} else {
			// Proceed, since we don't want internal rate limit system failures to
			// block all account creation.
			logEvent.IgnoredRateLimitError = err.Error()
		}
	}

	var newRegistrationSuccessful bool
	defer func() {
		if !newRegistrationSuccessful && refundLimits != nil {
			go refundLimits()
		}
	}()

	// Create corepb.Registration from provided account information
	reg := corepb.Registration{
		Agreement: wfe.SubscriberAgreementURL,
		Key:       keyBytes,
	}

	acctPB, err := wfe.ra.NewRegistration(ctx, &reg)
	if err != nil {
		if errors.Is(err, berrors.Duplicate) {
			existingAcct, err := wfe.sa.GetRegistrationByKey(ctx, &sapb.JSONWebKey{Jwk: keyBytes})
			if err == nil {
				returnExistingAcct(existingAcct)
				return
			}
			// return error even if berrors.NotFound, as the duplicate key error we got from
			// ra.NewRegistration indicates it _does_ already exist.
			wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "checking for existing account"), err)
			return
		}
		wfe.sendError(response, logEvent,
			web.ProblemDetailsForError(err, "Error creating new account"), err)
		return
	}

	registrationValid := func(reg *corepb.Registration) bool {
		return !(len(reg.Key) == 0) && reg.Id != 0
	}

	if acctPB == nil || !registrationValid(acctPB) {
		wfe.sendError(response, logEvent,
			web.ProblemDetailsForError(err, "Error creating new account"), err)
		return
	}
	acct, err := bgrpc.PbToRegistration(acctPB)
	if err != nil {
		wfe.sendError(response, logEvent,
			web.ProblemDetailsForError(err, "Error creating new account"), err)
		return
	}
	logEvent.Requester = acct.ID
	addRequesterHeader(response, acct.ID)

	acctURL := web.RelativeEndpoint(request, fmt.Sprintf("%s%d", acctPath, acct.ID))

	response.Header().Add("Location", acctURL)
	if len(wfe.SubscriberAgreementURL) > 0 {
		response.Header().Add("Link", link(wfe.SubscriberAgreementURL, "terms-of-service"))
	}

	prepAccountForDisplay(&acct)

	err = wfe.writeJsonResponse(response, logEvent, http.StatusCreated, acct)
	if err != nil {
		// ServerInternal because we just created this account, and it
		// should be OK.
		wfe.sendError(response, logEvent, probs.ServerInternal("Error marshaling account"), err)
		return
	}
	newRegistrationSuccessful = true

	if wfe.ee != nil && len(emails) > 0 {
		_, err := wfe.ee.SendContacts(ctx, &emailpb.SendContactsRequest{
			// Note: We are explicitly using the contacts provided by the
			// subscriber here. The RA will eventually stop accepting contacts.
			Emails: emails,
		})
		if err != nil {
			wfe.sendError(response, logEvent, probs.ServerInternal("Error sending contacts"), err)
			return
		}
	}
}

// parseRevocation accepts the payload for a revocation request and parses it
// into both the certificate to be revoked and the requested revocation reason
// (if any). Returns an error if any of the parsing fails, or if the given cert
// or revocation reason don't pass simple static checks. Also populates some
// metadata fields on the given logEvent.
func (wfe *WebFrontEndImpl) parseRevocation(
	jwsBody []byte, logEvent *web.RequestEvent) (*x509.Certificate, revocation.Reason, error) {
	// Read the revoke request from the JWS payload
	var revokeRequest struct {
		CertificateDER core.JSONBuffer    `json:"certificate"`
		Reason         *revocation.Reason `json:"reason"`
	}
	err := json.Unmarshal(jwsBody, &revokeRequest)
	if err != nil {
		return nil, 0, berrors.MalformedError("Unable to JSON parse revoke request")
	}

	// Parse the provided certificate
	parsedCertificate, err := x509.ParseCertificate(revokeRequest.CertificateDER)
	if err != nil {
		return nil, 0, berrors.MalformedError("Unable to parse certificate DER")
	}

	// Compute and record the serial number of the provided certificate
	serial := core.SerialToString(parsedCertificate.SerialNumber)
	logEvent.Extra["CertificateSerial"] = serial
	if revokeRequest.Reason != nil {
		logEvent.Extra["RevocationReason"] = *revokeRequest.Reason
	}

	// Try to validate the signature on the provided cert using its corresponding
	// issuer certificate.
	issuerCert, ok := wfe.issuerCertificates[issuance.IssuerNameID(parsedCertificate)]
	if !ok || issuerCert == nil {
		return nil, 0, berrors.NotFoundError("Certificate from unrecognized issuer")
	}
	err = parsedCertificate.CheckSignatureFrom(issuerCert.Certificate)
	if err != nil {
		return nil, 0, berrors.NotFoundError("No such certificate")
	}
	logEvent.Identifiers = identifier.FromCert(parsedCertificate)

	if parsedCertificate.NotAfter.Before(wfe.clk.Now()) {
		return nil, 0, berrors.UnauthorizedError("Certificate is expired")
	}

	// Verify the revocation reason supplied is allowed
	reason := revocation.Reason(0)
	if revokeRequest.Reason != nil {
		if _, present := revocation.UserAllowedReasons[*revokeRequest.Reason]; !present {
			return nil, 0, berrors.BadRevocationReasonError(int64(*revokeRequest.Reason))
		}
		reason = *revokeRequest.Reason
	}

	return parsedCertificate, reason, nil
}

type revocationEvidence struct {
	Serial string
	Reason revocation.Reason
	RegID  int64
	Method string
}

// revokeCertBySubscriberKey processes an outer JWS as a revocation request that
// is authenticated by a KeyID and the associated account.
func (wfe *WebFrontEndImpl) revokeCertBySubscriberKey(
	ctx context.Context,
	outerJWS *bJSONWebSignature,
	request *http.Request,
	logEvent *web.RequestEvent) error {
	// For Key ID revocations we authenticate the outer JWS by using
	// `validJWSForAccount` similar to other WFE endpoints
	jwsBody, _, acct, err := wfe.validJWSForAccount(outerJWS, request, ctx, logEvent)
	if err != nil {
		return err
	}

	cert, reason, err := wfe.parseRevocation(jwsBody, logEvent)
	if err != nil {
		return err
	}

	wfe.log.AuditObject("Authenticated revocation", revocationEvidence{
		Serial: core.SerialToString(cert.SerialNumber),
		Reason: reason,
		RegID:  acct.ID,
		Method: "applicant",
	})

	// The RA will confirm that the authenticated account either originally
	// issued the certificate, or has demonstrated control over all identifiers
	// in the certificate.
	_, err = wfe.ra.RevokeCertByApplicant(ctx, &rapb.RevokeCertByApplicantRequest{
		Cert:  cert.Raw,
		Code:  int64(reason),
		RegID: acct.ID,
	})
	if err != nil {
		return err
	}

	return nil
}

// revokeCertByCertKey processes an outer JWS as a revocation request that is
// authenticated by an embedded JWK. E.g. in the case where someone is
// requesting a revocation by using the keypair associated with the certificate
// to be revoked
func (wfe *WebFrontEndImpl) revokeCertByCertKey(
	ctx context.Context,
	outerJWS *bJSONWebSignature,
	request *http.Request,
	logEvent *web.RequestEvent) error {
	// For embedded JWK revocations we authenticate the outer JWS by using
	// `validSelfAuthenticatedJWS` similar to new-reg and key rollover.
	// We do *not* use `validSelfAuthenticatedPOST` here because we've already
	// read the HTTP request body in `parseJWSRequest` and it is now empty.
	jwsBody, jwk, prob := wfe.validSelfAuthenticatedJWS(ctx, outerJWS, request)
	if prob != nil {
		return prob
	}

	cert, reason, err := wfe.parseRevocation(jwsBody, logEvent)
	if err != nil {
		return err
	}

	// For embedded JWK revocations we decide if a requester is able to revoke a specific
	// certificate by checking that to-be-revoked certificate has the same public
	// key as the JWK that was used to authenticate the request
	if !core.KeyDigestEquals(jwk, cert.PublicKey) {
		return berrors.UnauthorizedError(
			"JWK embedded in revocation request must be the same public key as the cert to be revoked")
	}

	wfe.log.AuditObject("Authenticated revocation", revocationEvidence{
		Serial: core.SerialToString(cert.SerialNumber),
		Reason: reason,
		RegID:  0,
		Method: "privkey",
	})

	// The RA assumes here that the WFE2 has validated the JWS as proving
	// control of the private key corresponding to this certificate.
	_, err = wfe.ra.RevokeCertByKey(ctx, &rapb.RevokeCertByKeyRequest{
		Cert: cert.Raw,
	})
	if err != nil {
		return err
	}

	return nil
}

// RevokeCertificate is used by clients to request the revocation of a cert. The
// revocation request is handled uniquely based on the method of authentication
// used.
func (wfe *WebFrontEndImpl) RevokeCertificate(
	ctx context.Context,
	logEvent *web.RequestEvent,
	response http.ResponseWriter,
	request *http.Request) {

	// The ACME specification handles the verification of revocation requests
	// differently from other endpoints. For this reason we do *not* immediately
	// call `wfe.validPOSTForAccount` like all of the other endpoints.
	// For this endpoint we need to accept a JWS with an embedded JWK, or a JWS
	// with an embedded key ID, handling each case differently in terms of which
	// certificates are authorized to be revoked by the requester

	// Parse the JWS from the HTTP Request
	jws, err := wfe.parseJWSRequest(request)
	if err != nil {
		wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Unable to validate JWS"), err)
		return
	}

	// Figure out which type of authentication this JWS uses
	authType, err := checkJWSAuthType(jws.Signatures[0].Header)
	if err != nil {
		wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Unable to validate JWS"), err)
		return
	}

	// Handle the revocation request according to how it is authenticated, or if
	// the authentication type is unknown, error immediately
	switch authType {
	case embeddedKeyID:
		err = wfe.revokeCertBySubscriberKey(ctx, jws, request, logEvent)
	case embeddedJWK:
		err = wfe.revokeCertByCertKey(ctx, jws, request, logEvent)
	default:
		err = berrors.MalformedError("Malformed JWS, no KeyID or embedded JWK")
	}
	if err != nil {
		wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Unable to revoke"), err)
		return
	}

	response.WriteHeader(http.StatusOK)
}

// ChallengeHandler handles POST requests to challenge URLs of the form /acme/chall/{regID}/{authzID}/{challID}.
func (wfe *WebFrontEndImpl) ChallengeHandler(
	ctx context.Context,
	logEvent *web.RequestEvent,
	response http.ResponseWriter,
	request *http.Request) {
	slug := strings.Split(request.URL.Path, "/")
	if len(slug) != 3 {
		wfe.sendError(response, logEvent, probs.NotFound("No such challenge"), nil)
		return
	}
	// TODO(#7683): the regID is currently ignored.
	wfe.Challenge(ctx, logEvent, response, request, slug[1], slug[2])
}

// Challenge handles POSTS to both formats of challenge URLs.
func (wfe *WebFrontEndImpl) Challenge(
	ctx context.Context,
	logEvent *web.RequestEvent,
	response http.ResponseWriter,
	request *http.Request,
	authorizationIDStr string,
	challengeID string) {
	authorizationID, err := strconv.ParseInt(authorizationIDStr, 10, 64)
	if err != nil {
		wfe.sendError(response, logEvent, probs.Malformed("Invalid authorization ID"), nil)
		return
	}
	authzPB, err := wfe.ra.GetAuthorization(ctx, &rapb.GetAuthorizationRequest{Id: authorizationID})
	if err != nil {
		if errors.Is(err, berrors.NotFound) {
			wfe.sendError(response, logEvent, probs.NotFound("No such challenge"), nil)
		} else {
			wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Problem getting authorization"), err)
		}
		return
	}

	// Ensure gRPC response is complete.
	if core.IsAnyNilOrZero(authzPB.Id, authzPB.Identifier, authzPB.Status, authzPB.Expires) {
		wfe.sendError(response, logEvent, probs.ServerInternal("Problem getting authorization"), errIncompleteGRPCResponse)
		return
	}

	authz, err := bgrpc.PBToAuthz(authzPB)
	if err != nil {
		wfe.sendError(response, logEvent, probs.ServerInternal("Problem getting authorization"), err)
		return
	}
	challengeIndex := authz.FindChallengeByStringID(challengeID)
	if challengeIndex == -1 {
		wfe.sendError(response, logEvent, probs.NotFound("No such challenge"), nil)
		return
	}

	if authz.Expires == nil || authz.Expires.Before(wfe.clk.Now()) {
		wfe.sendError(response, logEvent, probs.NotFound("Expired authorization"), nil)
		return
	}

	logEvent.Identifiers = identifier.ACMEIdentifiers{authz.Identifier}
	logEvent.Status = string(authz.Status)

	challenge := authz.Challenges[challengeIndex]
	switch request.Method {
	case "GET", "HEAD":
		wfe.getChallenge(response, request, authz, &challenge, logEvent)

	case "POST":
		logEvent.ChallengeType = string(challenge.Type)
		wfe.postChallenge(ctx, response, request, authz, challengeIndex, logEvent)
	}
}

// prepAccountForDisplay takes a core.Registration and mutates it to be ready
// for display in a JSON response. Primarily it papers over legacy ACME v1
// features or non-standard details internal to Boulder we don't want clients to
// rely on.
func prepAccountForDisplay(acct *core.Registration) {
	// Zero out the account ID so that it isn't marshalled. RFC 8555 specifies
	// using the Location header for learning the account ID.
	acct.ID = 0

	// We populate the account Agreement field when creating a new response to
	// track which terms-of-service URL was in effect when an account with
	// "termsOfServiceAgreed":"true" is created. That said, we don't want to send
	// this value back to a V2 client. The "Agreement" field of an
	// account/registration is a V1 notion so we strip it here in the WFE2 before
	// returning the account.
	acct.Agreement = ""
}

// prepChallengeForDisplay takes a core.Challenge and prepares it for display to
// the client by filling in its URL field and clearing several unnecessary
// fields.
func (wfe *WebFrontEndImpl) prepChallengeForDisplay(
	request *http.Request,
	authz core.Authorization,
	challenge *core.Challenge,
) {
	// Update the challenge URL to be relative to the HTTP request Host
	challenge.URL = web.RelativeEndpoint(request, fmt.Sprintf("%s%d/%s/%s", challengePath, authz.RegistrationID, authz.ID, challenge.StringID()))

	// Internally, we store challenge error problems with just the short form
	// (e.g. "CAA") of the problem type. But for external display, we need to
	// prefix the error type with the RFC8555 ACME Error namespace.
	if challenge.Error != nil {
		challenge.Error.Type = probs.ErrorNS + challenge.Error.Type
	}

	// If the authz has been marked invalid, consider all challenges on that authz
	// to be invalid as well.
	if authz.Status == core.StatusInvalid {
		challenge.Status = authz.Status
	}

	// This field is not useful for the client, only internal debugging,
	for idx := range challenge.ValidationRecord {
		challenge.ValidationRecord[idx].ResolverAddrs = nil
	}
}

// prepAuthorizationForDisplay takes a core.Authorization and prepares it for
// display to the client by preparing all its challenges.
func (wfe *WebFrontEndImpl) prepAuthorizationForDisplay(request *http.Request, authz *core.Authorization) {
	for i := range authz.Challenges {
		wfe.prepChallengeForDisplay(request, *authz, &authz.Challenges[i])
	}

	// Shuffle the challenges so no one relies on their order.
	rand.Shuffle(len(authz.Challenges), func(i, j int) {
		authz.Challenges[i], authz.Challenges[j] = authz.Challenges[j], authz.Challenges[i]
	})

	// The ACME spec forbids allowing "*" in authorization identifiers. Boulder
	// allows this internally as a means of tracking when an authorization
	// corresponds to a wildcard request (e.g. to handle CAA properly). We strip
	// the "*." prefix from the Authz's Identifier's Value here to respect the law
	// of the protocol.
	if strings.HasPrefix(authz.Identifier.Value, "*.") {
		authz.Identifier.Value = strings.TrimPrefix(authz.Identifier.Value, "*.")
		// Mark that the authorization corresponds to a wildcard request since we've
		// now removed the wildcard prefix from the identifier.
		authz.Wildcard = true
	}
}

func (wfe *WebFrontEndImpl) getChallenge(
	response http.ResponseWriter,
	request *http.Request,
	authz core.Authorization,
	challenge *core.Challenge,
	logEvent *web.RequestEvent) {
	wfe.prepChallengeForDisplay(request, authz, challenge)

	authzURL := urlForAuthz(authz, request)
	response.Header().Add("Location", challenge.URL)
	response.Header().Add("Link", link(authzURL, "up"))

	err := wfe.writeJsonResponse(response, logEvent, http.StatusOK, challenge)
	if err != nil {
		// InternalServerError because this is a failure to decode data passed in
		// by the caller, which got it from the DB.
		wfe.sendError(response, logEvent, probs.ServerInternal("Failed to marshal challenge"), err)
		return
	}
}

func (wfe *WebFrontEndImpl) postChallenge(
	ctx context.Context,
	response http.ResponseWriter,
	request *http.Request,
	authz core.Authorization,
	challengeIndex int,
	logEvent *web.RequestEvent) {
	body, _, currAcct, err := wfe.validPOSTForAccount(request, ctx, logEvent)
	addRequesterHeader(response, logEvent.Requester)
	if err != nil {
		// validPOSTForAccount handles its own setting of logEvent.Errors
		wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Unable to validate JWS"), err)
		return
	}

	// Check that the account ID matching the key used matches
	// the account ID on the authz object
	if currAcct.ID != authz.RegistrationID {
		wfe.sendError(response,
			logEvent,
			probs.Unauthorized("User account ID doesn't match account ID in authorization"),
			nil,
		)
		return
	}

	// If the JWS body is empty then this POST is a POST-as-GET to retrieve
	// challenge details, not a POST to initiate a challenge
	if string(body) == "" {
		challenge := authz.Challenges[challengeIndex]
		wfe.getChallenge(response, request, authz, &challenge, logEvent)
		return
	}

	// We can expect some clients to try and update a challenge for an authorization
	// that is already valid. In this case we don't need to process the challenge
	// update. It wouldn't be helpful, the overall authorization is already good!
	var returnAuthz core.Authorization
	if authz.Status == core.StatusValid {
		returnAuthz = authz
	} else {

		// NOTE(@cpu): Historically a challenge update needed to include
		// a KeyAuthorization field. This is no longer the case, since both sides can
		// calculate the key authorization as needed. We unmarshal here only to check
		// that the POST body is valid JSON. Any data/fields included are ignored to
		// be kind to ACMEv2 implementations that still send a key authorization.
		var challengeUpdate struct{}
		err := json.Unmarshal(body, &challengeUpdate)
		if err != nil {
			wfe.sendError(response, logEvent, probs.Malformed("Error unmarshaling challenge response"), err)
			return
		}

		authzPB, err := bgrpc.AuthzToPB(authz)
		if err != nil {
			wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Unable to serialize authz"), err)
			return
		}

		authzPB, err = wfe.ra.PerformValidation(ctx, &rapb.PerformValidationRequest{
			Authz:          authzPB,
			ChallengeIndex: int64(challengeIndex),
		})
		if err != nil || core.IsAnyNilOrZero(authzPB, authzPB.Id, authzPB.Identifier, authzPB.Status, authzPB.Expires) {
			wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Unable to update challenge"), err)
			return
		}

		updatedAuthz, err := bgrpc.PBToAuthz(authzPB)
		if err != nil {
			wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Unable to deserialize authz"), err)
			return
		}
		returnAuthz = updatedAuthz
	}

	// assumption: PerformValidation does not modify order of challenges
	challenge := returnAuthz.Challenges[challengeIndex]
	wfe.prepChallengeForDisplay(request, authz, &challenge)

	authzURL := urlForAuthz(authz, request)
	response.Header().Add("Location", challenge.URL)
	response.Header().Add("Link", link(authzURL, "up"))

	err = wfe.writeJsonResponse(response, logEvent, http.StatusOK, challenge)
	if err != nil {
		// ServerInternal because we made the challenges, they should be OK
		wfe.sendError(response, logEvent, probs.ServerInternal("Failed to marshal challenge"), err)
		return
	}
}

// Account is used by a client to submit an update to their account.
func (wfe *WebFrontEndImpl) Account(
	ctx context.Context,
	logEvent *web.RequestEvent,
	response http.ResponseWriter,
	request *http.Request) {
	body, _, currAcct, err := wfe.validPOSTForAccount(request, ctx, logEvent)
	addRequesterHeader(response, logEvent.Requester)
	if err != nil {
		// validPOSTForAccount handles its own setting of logEvent.Errors
		wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Unable to validate JWS"), err)
		return
	}

	// Requests to this handler should have a path that leads to a known
	// account
	idStr := request.URL.Path
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		wfe.sendError(response, logEvent, probs.Malformed(fmt.Sprintf("Account ID must be an integer, was %q", idStr)), err)
		return
	} else if id <= 0 {
		wfe.sendError(response, logEvent, probs.Malformed(fmt.Sprintf("Account ID must be a positive non-zero integer, was %d", id)), nil)
		return
	} else if id != currAcct.ID {
		wfe.sendError(response, logEvent, probs.Unauthorized("Request signing key did not match account key"), nil)
		return
	}

	var acct *core.Registration
	if string(body) == "" || string(body) == "{}" {
		// An empty string means POST-as-GET (i.e. no update). A body of "{}" means
		// an update of zero fields, returning the unchanged object. This was the
		// recommended way to fetch the account object in ACMEv1.
		acct = currAcct
	} else {
		acct, err = wfe.updateAccount(ctx, body, currAcct)
		if err != nil {
			wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Unable to update account"), nil)
			return
		}
	}

	if len(wfe.SubscriberAgreementURL) > 0 {
		response.Header().Add("Link", link(wfe.SubscriberAgreementURL, "terms-of-service"))
	}

	prepAccountForDisplay(acct)

	err = wfe.writeJsonResponse(response, logEvent, http.StatusOK, acct)
	if err != nil {
		wfe.sendError(response, logEvent, probs.ServerInternal("Failed to marshal account"), err)
		return
	}
}

// updateAccount unmarshals an account update request from the provided
// requestBody to update the given registration. Important: It is assumed the
// request has already been authenticated by the caller. If the request is a
// valid update the resulting updated account is returned, otherwise a problem
// is returned.
func (wfe *WebFrontEndImpl) updateAccount(ctx context.Context, requestBody []byte, currAcct *core.Registration) (*core.Registration, error) {
	// Only the Status field of an account may be updated this way.
	// For key updates clients should be using the key change endpoint.
	var accountUpdateRequest struct {
		Status core.AcmeStatus `json:"status"`
	}

	err := json.Unmarshal(requestBody, &accountUpdateRequest)
	if err != nil {
		return nil, berrors.MalformedError("parsing account update request: %s", err)
	}

	switch accountUpdateRequest.Status {
	case core.StatusValid, "":
		// They probably intended to update their contact address, but we don't do
		// that anymore, so simply return their account as-is. We don't error out
		// here because it would break too many clients.
		return currAcct, nil

	case core.StatusDeactivated:
		updatedAcct, err := wfe.ra.DeactivateRegistration(
			ctx, &rapb.DeactivateRegistrationRequest{RegistrationID: currAcct.ID})
		if err != nil {
			return nil, fmt.Errorf("deactivating account: %w", err)
		}

		updatedReg, err := bgrpc.PbToRegistration(updatedAcct)
		if err != nil {
			return nil, fmt.Errorf("parsing deactivated account: %w", err)
		}
		return &updatedReg, nil

	default:
		return nil, berrors.MalformedError("invalid status %q for account update request, must be %q or %q", accountUpdateRequest.Status, core.StatusValid, core.StatusDeactivated)
	}
}

// deactivateAuthorization processes the given JWS POST body as a request to
// deactivate the provided authorization. If an error occurs it is written to
// the response writer. Important: `deactivateAuthorization` does not check that
// the requester is authorized to deactivate the given authorization. It is
// assumed that this check is performed prior to calling deactivateAuthorzation.
func (wfe *WebFrontEndImpl) deactivateAuthorization(
	ctx context.Context,
	authzPB *corepb.Authorization,
	logEvent *web.RequestEvent,
	response http.ResponseWriter,
	body []byte) bool {
	var req struct {
		Status core.AcmeStatus
	}
	err := json.Unmarshal(body, &req)
	if err != nil {
		wfe.sendError(response, logEvent, probs.Malformed("Error unmarshaling JSON"), err)
		return false
	}
	if req.Status != core.StatusDeactivated {
		wfe.sendError(response, logEvent, probs.Malformed("Invalid status value"), err)
		return false
	}
	_, err = wfe.ra.DeactivateAuthorization(ctx, authzPB)
	if err != nil {
		wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Error deactivating authorization"), err)
		return false
	}
	// Since the authorization passed to DeactivateAuthorization isn't
	// mutated locally by the function we must manually set the status
	// here before displaying the authorization to the user
	authzPB.Status = string(core.StatusDeactivated)
	return true
}

// AuthorizationHandler handles requests to authorization URLs of the form /acme/authz/{regID}/{authzID}.
func (wfe *WebFrontEndImpl) AuthorizationHandler(
	ctx context.Context,
	logEvent *web.RequestEvent,
	response http.ResponseWriter,
	request *http.Request) {
	slug := strings.Split(request.URL.Path, "/")
	if len(slug) != 2 {
		wfe.sendError(response, logEvent, probs.NotFound("No such authorization"), nil)
		return
	}
	// TODO(#7683): The regID is currently ignored.
	wfe.Authorization(ctx, logEvent, response, request, slug[1])
}

// Authorization handles both `/acme/authz/{authzID}` and `/acme/authz/{regID}/{authzID}` requests,
// after the calling function has parsed out the authzID.
func (wfe *WebFrontEndImpl) Authorization(
	ctx context.Context,
	logEvent *web.RequestEvent,
	response http.ResponseWriter,
	request *http.Request,
	authzIDStr string) {
	var requestAccount *core.Registration
	var requestBody []byte
	// If the request is a POST it is either:
	//   A) an update to an authorization to deactivate it
	//   B) a POST-as-GET to query the authorization details
	if request.Method == "POST" {
		// Both POST options need to be authenticated by an account
		body, _, acct, err := wfe.validPOSTForAccount(request, ctx, logEvent)
		addRequesterHeader(response, logEvent.Requester)
		if err != nil {
			wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Unable to validate JWS"), err)
			return
		}
		requestAccount = acct
		requestBody = body
	}

	authzID, err := strconv.ParseInt(authzIDStr, 10, 64)
	if err != nil {
		wfe.sendError(response, logEvent, probs.Malformed("Invalid authorization ID"), nil)
		return
	}

	authzPB, err := wfe.ra.GetAuthorization(ctx, &rapb.GetAuthorizationRequest{Id: authzID})
	if errors.Is(err, berrors.NotFound) {
		wfe.sendError(response, logEvent, probs.NotFound("No such authorization"), nil)
		return
	} else if errors.Is(err, berrors.Malformed) {
		wfe.sendError(response, logEvent, probs.Malformed(err.Error()), nil)
		return
	} else if err != nil {
		wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Problem getting authorization"), err)
		return
	}

	ident := identifier.FromProto(authzPB.Identifier)

	// Ensure gRPC response is complete.
	if core.IsAnyNilOrZero(authzPB.Id, ident, authzPB.Status, authzPB.Expires) {
		wfe.sendError(response, logEvent, probs.ServerInternal("Problem getting authorization"), errIncompleteGRPCResponse)
		return
	}

	logEvent.Identifiers = identifier.ACMEIdentifiers{ident}
	logEvent.Status = authzPB.Status

	// After expiring, authorizations are inaccessible
	if authzPB.Expires.AsTime().Before(wfe.clk.Now()) {
		wfe.sendError(response, logEvent, probs.NotFound("Expired authorization"), nil)
		return
	}

	// If this was a POST that has an associated requestAccount and that account
	// doesn't own the authorization, abort before trying to deactivate the authz
	// or return its details
	if requestAccount != nil && requestAccount.ID != authzPB.RegistrationID {
		wfe.sendError(response, logEvent,
			probs.Unauthorized("Account ID doesn't match ID for authorization"), nil)
		return
	}

	// If the body isn't empty we know it isn't a POST-as-GET and must be an
	// attempt to deactivate an authorization.
	if string(requestBody) != "" {
		// If the deactivation fails return early as errors and return codes
		// have already been set. Otherwise continue so that the user gets
		// sent the deactivated authorization.
		if !wfe.deactivateAuthorization(ctx, authzPB, logEvent, response, requestBody) {
			return
		}
	}

	authz, err := bgrpc.PBToAuthz(authzPB)
	if err != nil {
		wfe.sendError(response, logEvent, probs.ServerInternal("Problem getting authorization"), err)
		return
	}

	wfe.prepAuthorizationForDisplay(request, &authz)

	err = wfe.writeJsonResponse(response, logEvent, http.StatusOK, authz)
	if err != nil {
		// InternalServerError because this is a failure to decode from our DB.
		wfe.sendError(response, logEvent, probs.ServerInternal("Failed to JSON marshal authz"), err)
		return
	}
}

// Certificate is used by clients to request a copy of their current certificate, or to
// request a reissuance of the certificate.
func (wfe *WebFrontEndImpl) Certificate(ctx context.Context, logEvent *web.RequestEvent, response http.ResponseWriter, request *http.Request) {
	var requesterAccount *core.Registration
	// Any POSTs to the Certificate endpoint should be POST-as-GET requests. There are
	// no POSTs with a body allowed for this endpoint.
	if request.Method == "POST" {
		acct, err := wfe.validPOSTAsGETForAccount(request, ctx, logEvent)
		if err != nil {
			wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Unable to validate JWS"), err)
			return
		}
		requesterAccount = acct
	}

	requestedChain := 0
	serial := request.URL.Path

	// An alternate chain may be requested with the request path {serial}/{chain}, where chain
	// is a number - an index into the slice of chains for the issuer. If a specific chain is
	// not requested, then it defaults to zero - the default certificate chain for the issuer.
	serialAndChain := strings.SplitN(serial, "/", 2)
	if len(serialAndChain) == 2 {
		idx, err := strconv.Atoi(serialAndChain[1])
		if err != nil || idx < 0 {
			wfe.sendError(response, logEvent, probs.Malformed("Chain ID must be a non-negative integer"),
				fmt.Errorf("certificate chain id provided was not valid: %s", serialAndChain[1]))
			return
		}
		serial = serialAndChain[0]
		requestedChain = idx
	}

	// Certificate paths consist of the CertBase path, plus exactly sixteen hex
	// digits.
	if !core.ValidSerial(serial) {
		wfe.sendError(
			response,
			logEvent,
			probs.NotFound("Certificate not found"),
			fmt.Errorf("certificate serial provided was not valid: %s", serial),
		)
		return
	}
	logEvent.Extra["RequestedSerial"] = serial

	cert, err := wfe.sa.GetCertificate(ctx, &sapb.Serial{Serial: serial})
	if err != nil {
		if errors.Is(err, berrors.NotFound) {
			wfe.sendError(response, logEvent, probs.NotFound("Certificate not found"), nil)
		} else {
			wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Failed to retrieve certificate"), err)
		}
		return
	}

	// Don't serve certificates from the /get/ path until they're a little stale,
	// to prevent ACME clients from using that path.
	if strings.HasPrefix(logEvent.Endpoint, getCertPath) && wfe.clk.Since(cert.Issued.AsTime()) < wfe.staleTimeout {
		wfe.sendError(response, logEvent, probs.Unauthorized(fmt.Sprintf(
			"Certificate is too new for GET API. You should only use this non-standard API to access resources created more than %s ago",
			wfe.staleTimeout)), nil)
		return
	}

	// If there was a requesterAccount (e.g. because it was a POST-as-GET request)
	// then the requesting account must be the owner of the certificate, otherwise
	// return an unauthorized error.
	if requesterAccount != nil && requesterAccount.ID != cert.RegistrationID {
		wfe.sendError(response, logEvent, probs.Unauthorized("Account in use did not issue specified certificate"), nil)
		return
	}

	responsePEM, prob := func() ([]byte, *probs.ProblemDetails) {
		leafPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: cert.Der,
		})

		parsedCert, err := x509.ParseCertificate(cert.Der)
		if err != nil {
			// If we can't parse one of our own certs there's a serious problem
			return nil, probs.ServerInternal(
				fmt.Sprintf(
					"unable to parse Boulder issued certificate with serial %#v: %s",
					serial,
					err),
			)
		}

		issuerNameID := issuance.IssuerNameID(parsedCert)
		availableChains, ok := wfe.certificateChains[issuerNameID]
		if !ok || len(availableChains) == 0 {
			// If there is no wfe.certificateChains entry for the IssuerNameID then
			// we can't provide a chain for this cert. If the certificate is expired,
			// just return the bare cert. If the cert is still valid, then there is
			// a misconfiguration and we should treat it as an internal server error.
			if parsedCert.NotAfter.Before(wfe.clk.Now()) {
				return leafPEM, nil
			}
			return nil, probs.ServerInternal(
				fmt.Sprintf(
					"Certificate serial %#v has an unknown IssuerNameID %d - no PEM certificate chain associated.",
					serial,
					issuerNameID),
			)
		}

		// If the requested chain is outside the bounds of the available chains,
		// then it is an error by the client - not found.
		if requestedChain < 0 || requestedChain >= len(availableChains) {
			return nil, probs.NotFound("Unknown issuance chain")
		}

		// Double check that the signature validates.
		err = parsedCert.CheckSignatureFrom(wfe.issuerCertificates[issuerNameID].Certificate)
		if err != nil {
			return nil, probs.ServerInternal(
				fmt.Sprintf(
					"Certificate serial %#v has a signature which cannot be verified from issuer %d.",
					serial,
					issuerNameID),
			)
		}

		// Add rel="alternate" links for every chain available for this issuer,
		// excluding the currently requested chain.
		for chainID := range availableChains {
			if chainID == requestedChain {
				continue
			}
			chainURL := web.RelativeEndpoint(request,
				fmt.Sprintf("%s%s/%d", certPath, serial, chainID))
			response.Header().Add("Link", link(chainURL, "alternate"))
		}

		// Prepend the chain with the leaf certificate
		return append(leafPEM, availableChains[requestedChain]...), nil
	}()
	if prob != nil {
		wfe.sendError(response, logEvent, prob, nil)
		return
	}

	// NOTE(@cpu): We must explicitly set the Content-Length header here. The Go
	// HTTP library will only add this header if the body is below a certain size
	// and with the addition of a PEM encoded certificate chain the body size of
	// this endpoint will exceed this threshold. Since we know the length we can
	// reliably set it ourselves and not worry.
	response.Header().Set("Content-Length", strconv.Itoa(len(responsePEM)))
	response.Header().Set("Content-Type", "application/pem-certificate-chain")
	response.WriteHeader(http.StatusOK)
	if _, err = response.Write(responsePEM); err != nil {
		wfe.log.Warningf("Could not write response: %s", err)
	}
}

// BuildID tells the requester what build we're running.
func (wfe *WebFrontEndImpl) BuildID(ctx context.Context, logEvent *web.RequestEvent, response http.ResponseWriter, request *http.Request) {
	response.Header().Set("Content-Type", "text/plain")
	response.WriteHeader(http.StatusOK)
	detailsString := fmt.Sprintf("Boulder=(%s %s)", core.GetBuildID(), core.GetBuildTime())
	if _, err := fmt.Fprintln(response, detailsString); err != nil {
		wfe.log.Warningf("Could not write response: %s", err)
	}
}

// Options responds to an HTTP OPTIONS request.
func (wfe *WebFrontEndImpl) Options(response http.ResponseWriter, request *http.Request, methodsStr string, methodsMap map[string]bool) {
	// Every OPTIONS request gets an Allow header with a list of supported methods.
	response.Header().Set("Allow", methodsStr)

	// CORS preflight requests get additional headers. See
	// http://www.w3.org/TR/cors/#resource-preflight-requests
	reqMethod := request.Header.Get("Access-Control-Request-Method")
	if reqMethod == "" {
		reqMethod = "GET"
	}
	if methodsMap[reqMethod] {
		wfe.setCORSHeaders(response, request, methodsStr)
	}
}

// setCORSHeaders() tells the client that CORS is acceptable for this
// request. If allowMethods == "" the request is assumed to be a CORS
// actual request and no Access-Control-Allow-Methods header will be
// sent.
func (wfe *WebFrontEndImpl) setCORSHeaders(response http.ResponseWriter, request *http.Request, allowMethods string) {
	reqOrigin := request.Header.Get("Origin")
	if reqOrigin == "" {
		// This is not a CORS request.
		return
	}

	// Allow CORS if the current origin (or "*") is listed as an
	// allowed origin in config. Otherwise, disallow by returning
	// without setting any CORS headers.
	allow := false
	for _, ao := range wfe.AllowOrigins {
		if ao == "*" {
			response.Header().Set("Access-Control-Allow-Origin", "*")
			allow = true
			break
		} else if ao == reqOrigin {
			response.Header().Set("Vary", "Origin")
			response.Header().Set("Access-Control-Allow-Origin", ao)
			allow = true
			break
		}
	}
	if !allow {
		return
	}

	if allowMethods != "" {
		// For an OPTIONS request: allow all methods handled at this URL.
		response.Header().Set("Access-Control-Allow-Methods", allowMethods)
	}
	// NOTE(@cpu): "Content-Type" is considered a 'simple header' that doesn't
	// need to be explicitly allowed in 'access-control-allow-headers', but only
	// when the value is one of: `application/x-www-form-urlencoded`,
	// `multipart/form-data`, or `text/plain`. Since `application/jose+json` is
	// not one of these values we must be explicit in saying that `Content-Type`
	// is an allowed header. See MDN for more details:
	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Access-Control-Allow-Headers
	response.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	response.Header().Set("Access-Control-Expose-Headers", "Link, Replay-Nonce, Location")
	response.Header().Set("Access-Control-Max-Age", "86400")
}

// KeyRollover allows a user to change their signing key
func (wfe *WebFrontEndImpl) KeyRollover(
	ctx context.Context,
	logEvent *web.RequestEvent,
	response http.ResponseWriter,
	request *http.Request) {
	// Validate the outer JWS on the key rollover in standard fashion using
	// validPOSTForAccount
	outerBody, outerJWS, acct, err := wfe.validPOSTForAccount(request, ctx, logEvent)
	addRequesterHeader(response, logEvent.Requester)
	if err != nil {
		wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Unable to validate JWS"), err)
		return
	}
	oldKey := acct.Key

	// Parse the inner JWS from the validated outer JWS body
	innerJWS, err := wfe.parseJWS(outerBody)
	if err != nil {
		wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Unable to validate JWS"), err)
		return
	}

	// Validate the inner JWS as a key rollover request for the outer JWS
	rolloverOperation, err := wfe.validKeyRollover(ctx, outerJWS, innerJWS, oldKey)
	if err != nil {
		wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Unable to validate JWS"), err)
		return
	}
	newKey := rolloverOperation.NewKey

	// Check that the rollover request's account URL matches the account URL used
	// to validate the outer JWS
	header := outerJWS.Signatures[0].Header
	if rolloverOperation.Account != header.KeyID {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "KeyRolloverMismatchedAccount"}).Inc()
		wfe.sendError(response, logEvent, probs.Malformed(
			fmt.Sprintf("Inner key rollover request specified Account %q, but outer JWS has Key ID %q",
				rolloverOperation.Account, header.KeyID)), nil)
		return
	}

	// Check that the new key isn't the same as the old key. This would fail as
	// part of the subsequent `wfe.SA.GetRegistrationByKey` check since the new key
	// will find the old account if its equal to the old account key. We
	// check new key against old key explicitly to save an RPC round trip and a DB
	// query for this easy rejection case
	keysEqual, err := core.PublicKeysEqual(newKey.Key, oldKey.Key)
	if err != nil {
		// This should not happen - both the old and new key have been validated by now
		wfe.sendError(response, logEvent, probs.ServerInternal("Unable to compare new and old keys"), err)
		return
	}
	if keysEqual {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "KeyRolloverUnchangedKey"}).Inc()
		wfe.sendError(response, logEvent, probs.Malformed(
			"New key specified by rollover request is the same as the old key"), nil)
		return
	}

	// Marshal key to bytes
	newKeyBytes, err := newKey.MarshalJSON()
	if err != nil {
		wfe.sendError(response, logEvent, probs.ServerInternal("Error marshaling new key"), err)
	}
	// Check that the new key isn't already being used for an existing account
	existingAcct, err := wfe.sa.GetRegistrationByKey(ctx, &sapb.JSONWebKey{Jwk: newKeyBytes})
	if err == nil {
		response.Header().Set("Location",
			web.RelativeEndpoint(request, fmt.Sprintf("%s%d", acctPath, existingAcct.Id)))
		wfe.sendError(response, logEvent,
			probs.Conflict("New key is already in use for a different account"), err)
		return
	} else if !errors.Is(err, berrors.NotFound) {
		wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Failed to lookup existing keys"), err)
		return
	}

	// Update the account key to the new key
	updatedAcctPb, err := wfe.ra.UpdateRegistrationKey(ctx, &rapb.UpdateRegistrationKeyRequest{RegistrationID: acct.ID, Jwk: newKeyBytes})
	if err != nil {
		if errors.Is(err, berrors.Duplicate) {
			// It is possible that between checking for the existing key, and performing the update
			// a parallel update or new account request happened and claimed the key. In this case
			// just retrieve the account again, and return an error as we would above with a Location
			// header
			existingAcct, err := wfe.sa.GetRegistrationByKey(ctx, &sapb.JSONWebKey{Jwk: newKeyBytes})
			if err != nil {
				wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "looking up account by key"), err)
				return
			}
			response.Header().Set("Location",
				web.RelativeEndpoint(request, fmt.Sprintf("%s%d", acctPath, existingAcct.Id)))
			wfe.sendError(response, logEvent,
				probs.Conflict("New key is already in use for a different account"), err)
			return
		}
		wfe.sendError(response, logEvent,
			web.ProblemDetailsForError(err, "Unable to update account with new key"), err)
		return
	}
	// Convert proto to registration for display
	updatedAcct, err := bgrpc.PbToRegistration(updatedAcctPb)
	if err != nil {
		wfe.sendError(response, logEvent, probs.ServerInternal("Error marshaling proto to registration"), err)
		return
	}
	prepAccountForDisplay(&updatedAcct)

	err = wfe.writeJsonResponse(response, logEvent, http.StatusOK, updatedAcct)
	if err != nil {
		wfe.sendError(response, logEvent, probs.ServerInternal("Failed to marshal updated account"), err)
	}
}

type orderJSON struct {
	Status         core.AcmeStatus            `json:"status"`
	Expires        time.Time                  `json:"expires"`
	Identifiers    identifier.ACMEIdentifiers `json:"identifiers"`
	Authorizations []string                   `json:"authorizations"`
	Finalize       string                     `json:"finalize"`
	Profile        string                     `json:"profile,omitempty"`
	Certificate    string                     `json:"certificate,omitempty"`
	Error          *probs.ProblemDetails      `json:"error,omitempty"`
	Replaces       string                     `json:"replaces,omitempty"`
}

// orderToOrderJSON converts a *corepb.Order instance into an orderJSON struct
// that is returned in HTTP API responses. It will convert the order names to
// DNS type identifiers and additionally create absolute URLs for the finalize
// URL and the certificate URL as appropriate.
func (wfe *WebFrontEndImpl) orderToOrderJSON(request *http.Request, order *corepb.Order) orderJSON {
	finalizeURL := web.RelativeEndpoint(request,
		fmt.Sprintf("%s%d/%d", finalizeOrderPath, order.RegistrationID, order.Id))
	respObj := orderJSON{
		Status:      core.AcmeStatus(order.Status),
		Expires:     order.Expires.AsTime(),
		Identifiers: identifier.FromProtoSlice(order.Identifiers),
		Finalize:    finalizeURL,
		Profile:     order.CertificateProfileName,
		Replaces:    order.Replaces,
	}
	// If there is an order error, prefix its type with the V2 namespace
	if order.Error != nil {
		prob, err := bgrpc.PBToProblemDetails(order.Error)
		if err != nil {
			wfe.log.AuditErrf("Internal error converting order ID %d "+
				"proto buf prob to problem details: %q", order.Id, err)
		}
		respObj.Error = prob
		respObj.Error.Type = probs.ErrorNS + respObj.Error.Type
	}
	for _, v2ID := range order.V2Authorizations {
		respObj.Authorizations = append(respObj.Authorizations, web.RelativeEndpoint(request, fmt.Sprintf("%s%d/%d", authzPath, order.RegistrationID, v2ID)))
	}
	if respObj.Status == core.StatusValid {
		certURL := web.RelativeEndpoint(request,
			fmt.Sprintf("%s%s", certPath, order.CertificateSerial))
		respObj.Certificate = certURL
	}
	return respObj
}

// checkNewOrderLimits checks whether sufficient limit quota exists for the
// creation of a new order. If so, that quota is spent. If an error is
// encountered during the check, it is logged but not returned. A refund
// function is returned that can be used to refund the quota if the order is not
// created, the func will be nil if any error was encountered during the check.
//
// Precondition: idents must be a list of identifiers that all pass
// policy.WellFormedIdentifiers.
func (wfe *WebFrontEndImpl) checkNewOrderLimits(ctx context.Context, regId int64, idents identifier.ACMEIdentifiers, isRenewal bool) (func(), error) {
	txns, err := wfe.txnBuilder.NewOrderLimitTransactions(regId, idents, isRenewal)
	if err != nil {
		return nil, fmt.Errorf("building new order limit transactions: %w", err)
	}

	d, err := wfe.limiter.BatchSpend(ctx, txns)
	if err != nil {
		return nil, fmt.Errorf("spending new order limits: %w", err)
	}

	err = d.Result(wfe.clk.Now())
	if err != nil {
		return nil, err
	}

	return func() {
		_, err := wfe.limiter.BatchRefund(ctx, txns)
		if err != nil {
			wfe.log.Warningf("refunding new order limits: %s", err)
		}
	}, nil
}

// orderMatchesReplacement checks if the order matches the provided certificate
// as identified by the provided ARI CertID. This function ensures that:
//   - the certificate being replaced exists,
//   - the requesting account owns that certificate, and
//   - an identifier in this new order matches an identifier in the certificate being
//     replaced.
func (wfe *WebFrontEndImpl) orderMatchesReplacement(ctx context.Context, acct *core.Registration, idents identifier.ACMEIdentifiers, serial string) error {
	// It's okay to use GetCertificate (vs trying to get a precertificate),
	// because we don't intend to serve ARI for certs that never made it past
	// the precert stage.
	oldCert, err := wfe.sa.GetCertificate(ctx, &sapb.Serial{Serial: serial})
	if err != nil {
		if errors.Is(err, berrors.NotFound) {
			return berrors.NotFoundError("request included `replaces` field, but no current certificate with serial %q exists", serial)
		}
		return errors.New("failed to retrieve existing certificate")
	}

	if oldCert.RegistrationID != acct.ID {
		return berrors.UnauthorizedError("requester account did not request the certificate being replaced by this order")
	}
	parsedCert, err := x509.ParseCertificate(oldCert.Der)
	if err != nil {
		return fmt.Errorf("error parsing certificate replaced by this order: %w", err)
	}

	var identMatch bool
	for _, ident := range idents {
		if parsedCert.VerifyHostname(ident.Value) == nil {
			// At least one identifier in the new order matches an identifier in
			// the predecessor certificate.
			identMatch = true
			break
		}
	}
	if !identMatch {
		return berrors.MalformedError("identifiers in this order do not match any identifiers in the certificate being replaced")
	}
	return nil
}

func (wfe *WebFrontEndImpl) determineARIWindow(ctx context.Context, serial string) (core.RenewalInfo, error) {
	// Check if the serial is impacted by an incident.
	result, err := wfe.sa.IncidentsForSerial(ctx, &sapb.Serial{Serial: serial})
	if err != nil {
		return core.RenewalInfo{}, fmt.Errorf("checking if existing certificate is impacted by an incident: %w", err)
	}

	if len(result.Incidents) > 0 {
		// Find the earliest incident.
		var earliest *sapb.Incident
		for _, incident := range result.Incidents {
			if earliest == nil || incident.RenewBy.AsTime().Before(earliest.RenewBy.AsTime()) {
				earliest = incident
			}
		}
		// The existing cert is impacted by an incident, renew immediately.
		return core.RenewalInfoImmediate(wfe.clk.Now(), earliest.Url), nil
	}

	// Check if the serial is revoked.
	status, err := wfe.sa.GetCertificateStatus(ctx, &sapb.Serial{Serial: serial})
	if err != nil {
		return core.RenewalInfo{}, fmt.Errorf("checking if existing certificate has been revoked: %w", err)
	}

	if status.Status == string(core.OCSPStatusRevoked) {
		// The existing certificate is revoked, renew immediately.
		return core.RenewalInfoImmediate(wfe.clk.Now(), ""), nil
	}

	// It's okay to use GetCertificate (vs trying to get a precertificate),
	// because we don't intend to serve ARI for certs that never made it past
	// the precert stage.
	cert, err := wfe.sa.GetCertificate(ctx, &sapb.Serial{Serial: serial})
	if err != nil {
		if errors.Is(err, berrors.NotFound) {
			return core.RenewalInfo{}, err
		}
		return core.RenewalInfo{}, fmt.Errorf("failed to retrieve existing certificate: %w", err)
	}

	return core.RenewalInfoSimple(cert.Issued.AsTime(), cert.Expires.AsTime()), nil
}

// validateReplacementOrder implements draft-ietf-acme-ari-03. For a new order
// to be considered a replacement for an existing certificate, the existing
// certificate:
//  1. MUST NOT have been replaced by another finalized order,
//  2. MUST be associated with the same ACME account as this request, and
//  3. MUST have at least one identifier in common with this request.
//
// There are three values returned by this function:
//   - The first return value is the serial number of the certificate being
//     replaced. If the order is not a replacement, this value is an empty
//     string.
//   - The second return value is a boolean indicating whether the order is
//     exempt from rate limits. If the order is a replacement and the request
//     is made within the suggested renewal window, this value is true.
//     Otherwise, this value is false.
//   - The last value is an error, this is non-nil unless the order is not a
//     replacement or there was an error while validating the replacement.
func (wfe *WebFrontEndImpl) validateReplacementOrder(ctx context.Context, acct *core.Registration, idents identifier.ACMEIdentifiers, replaces string) (string, bool, error) {
	if replaces == "" {
		// No replacement indicated.
		return "", false, nil
	}

	decodedSerial, err := parseARICertID(replaces, wfe.issuerCertificates)
	if err != nil {
		return "", false, fmt.Errorf("while parsing ARI CertID an error occurred: %w", err)
	}

	exists, err := wfe.sa.ReplacementOrderExists(ctx, &sapb.Serial{Serial: decodedSerial})
	if err != nil {
		return "", false, fmt.Errorf("checking replacement status of existing certificate: %w", err)
	}
	if exists.Exists {
		return "", false, berrors.AlreadyReplacedError(
			"cannot indicate an order replaces certificate with serial %q, which already has a replacement order",
			decodedSerial,
		)
	}

	err = wfe.orderMatchesReplacement(ctx, acct, idents, decodedSerial)
	if err != nil {
		// The provided replacement field value failed to meet the required
		// criteria. We're going to return the error to the caller instead
		// of trying to create a regular (non-replacement) order.
		return "", false, fmt.Errorf("while checking that this order is a replacement: %w", err)
	}
	// This order is a replacement for an existing certificate.
	replaces = decodedSerial

	// For an order to be exempt from rate limits, it must be a replacement
	// and the request must be made within the suggested renewal window.
	renewalInfo, err := wfe.determineARIWindow(ctx, replaces)
	if err != nil {
		return "", false, fmt.Errorf("while determining the current ARI renewal window: %w", err)
	}

	return replaces, renewalInfo.SuggestedWindow.IsWithin(wfe.clk.Now()), nil
}

func (wfe *WebFrontEndImpl) validateCertificateProfileName(profile string) error {
	if profile == "" {
		// No profile name is specified.
		return nil
	}
	if _, ok := wfe.certProfiles[profile]; !ok {
		// The profile name is not in the list of configured profiles.
		return fmt.Errorf("profile name %q not recognized", profile)
	}

	return nil
}

func (wfe *WebFrontEndImpl) checkIdentifiersPaused(ctx context.Context, orderIdents identifier.ACMEIdentifiers, regID int64) ([]string, error) {
	uniqueOrderIdents := identifier.Normalize(orderIdents)
	var idents []*corepb.Identifier
	for _, ident := range uniqueOrderIdents {
		idents = append(idents, &corepb.Identifier{
			Type:  string(ident.Type),
			Value: ident.Value,
		})
	}

	paused, err := wfe.sa.CheckIdentifiersPaused(ctx, &sapb.PauseRequest{
		RegistrationID: regID,
		Identifiers:    idents,
	})
	if err != nil {
		return nil, err
	}
	if len(paused.Identifiers) <= 0 {
		// No identifiers are paused.
		return nil, nil
	}

	// At least one of the requested identifiers is paused.
	pausedValues := make([]string, 0, len(paused.Identifiers))
	for _, ident := range paused.Identifiers {
		pausedValues = append(pausedValues, ident.Value)
	}

	return pausedValues, nil
}

// NewOrder is used by clients to create a new order object and a set of
// authorizations to fulfill for issuance.
func (wfe *WebFrontEndImpl) NewOrder(
	ctx context.Context,
	logEvent *web.RequestEvent,
	response http.ResponseWriter,
	request *http.Request) {
	body, _, acct, err := wfe.validPOSTForAccount(request, ctx, logEvent)
	addRequesterHeader(response, logEvent.Requester)
	if err != nil {
		// validPOSTForAccount handles its own setting of logEvent.Errors
		wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Unable to validate JWS"), err)
		return
	}

	// newOrderRequest is the JSON structure of the request body. We only
	// support the identifiers and replaces fields. If notBefore or notAfter are
	// sent we return a probs.Malformed as we do not support them.
	var newOrderRequest struct {
		Identifiers identifier.ACMEIdentifiers `json:"identifiers"`
		NotBefore   string
		NotAfter    string
		Replaces    string
		Profile     string
	}
	err = json.Unmarshal(body, &newOrderRequest)
	if err != nil {
		wfe.sendError(response, logEvent,
			probs.Malformed("Unable to unmarshal NewOrder request body"), err)
		return
	}

	if len(newOrderRequest.Identifiers) == 0 {
		wfe.sendError(response, logEvent,
			probs.Malformed("NewOrder request did not specify any identifiers"), nil)
		return
	}
	if newOrderRequest.NotBefore != "" || newOrderRequest.NotAfter != "" {
		wfe.sendError(response, logEvent, probs.Malformed("NotBefore and NotAfter are not supported"), nil)
		return
	}

	idents := newOrderRequest.Identifiers
	for _, ident := range idents {
		if !ident.Type.IsValid() {
			wfe.sendError(response, logEvent,
				probs.UnsupportedIdentifier("NewOrder request included unsupported identifier: type %q, value %q",
					ident.Type, ident.Value),
				nil)
			return
		}
		if ident.Value == "" {
			wfe.sendError(response, logEvent, probs.Malformed("NewOrder request included empty identifier"), nil)
			return
		}
	}
	idents = identifier.Normalize(idents)
	logEvent.Identifiers = idents

	err = policy.WellFormedIdentifiers(idents)
	if err != nil {
		wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Invalid identifiers requested"), nil)
		return
	}

	if features.Get().CheckIdentifiersPaused {
		pausedValues, err := wfe.checkIdentifiersPaused(ctx, idents, acct.ID)
		if err != nil {
			wfe.sendError(response, logEvent, probs.ServerInternal("Failure while checking pause status of identifiers"), err)
			return
		}
		if len(pausedValues) > 0 {
			jwt, err := unpause.GenerateJWT(wfe.unpauseSigner, acct.ID, pausedValues, wfe.unpauseJWTLifetime, wfe.clk)
			if err != nil {
				wfe.sendError(response, logEvent, probs.ServerInternal("Error generating JWT for unpause portal"), err)
			}
			msg := fmt.Sprintf(
				"Your account is temporarily prevented from requesting certificates for %s and possibly others. Please visit: %s",
				strings.Join(pausedValues, ", "),
				fmt.Sprintf("%s%s?jwt=%s", wfe.unpauseURL, unpause.GetForm, jwt),
			)
			wfe.sendError(response, logEvent, probs.Paused(msg), nil)
			return
		}
	}

	var replacesSerial string
	var isARIRenewal bool
	replacesSerial, isARIRenewal, err = wfe.validateReplacementOrder(ctx, acct, idents, newOrderRequest.Replaces)
	if err != nil {
		wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Could not validate ARI 'replaces' field"), err)
		return
	}

	var isRenewal bool
	if !isARIRenewal {
		// The Subscriber does not have an ARI exemption. However, we can check
		// if the order is a renewal, and thus exempt from the NewOrdersPerAccount
		// and CertificatesPerDomain limits.
		timestamps, err := wfe.sa.FQDNSetTimestampsForWindow(ctx, &sapb.CountFQDNSetsRequest{
			Identifiers: idents.ToProtoSlice(),
			Window:      durationpb.New(120 * 24 * time.Hour),
			Limit:       1,
		})
		if err != nil {
			wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "While checking renewal exemption status"), err)
			return
		}
		isRenewal = len(timestamps.Timestamps) > 0
	}

	err = wfe.validateCertificateProfileName(newOrderRequest.Profile)
	if err != nil {
		// TODO(#7392) Provide link to profile documentation.
		wfe.sendError(response, logEvent, probs.InvalidProfile(err.Error()), err)
		return
	}

	var refundLimits func()
	if !isARIRenewal {
		refundLimits, err = wfe.checkNewOrderLimits(ctx, acct.ID, idents, isRenewal)
		if err != nil {
			if errors.Is(err, berrors.RateLimit) {
				wfe.sendError(response, logEvent, probs.RateLimited(err.Error()), err)
				return
			} else {
				// Proceed, since we don't want internal rate limit system failures to
				// block all issuance.
				logEvent.IgnoredRateLimitError = err.Error()
			}
		}
	}

	var newOrderSuccessful bool
	defer func() {
		wfe.stats.ariReplacementOrders.With(prometheus.Labels{
			"isReplacement": fmt.Sprintf("%t", replacesSerial != ""),
			"limitsExempt":  fmt.Sprintf("%t", isARIRenewal),
		}).Inc()

		if !newOrderSuccessful && refundLimits != nil {
			go refundLimits()
		}
	}()

	order, err := wfe.ra.NewOrder(ctx, &rapb.NewOrderRequest{
		RegistrationID:         acct.ID,
		Identifiers:            idents.ToProtoSlice(),
		CertificateProfileName: newOrderRequest.Profile,
		Replaces:               newOrderRequest.Replaces,
		ReplacesSerial:         replacesSerial,
	})

	if err != nil || core.IsAnyNilOrZero(order, order.Id, order.RegistrationID, order.Identifiers, order.Created, order.Expires) {
		wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Error creating new order"), err)
		return
	}
	logEvent.Created = fmt.Sprintf("%d", order.Id)

	orderURL := web.RelativeEndpoint(request,
		fmt.Sprintf("%s%d/%d", orderPath, acct.ID, order.Id))
	response.Header().Set("Location", orderURL)

	respObj := wfe.orderToOrderJSON(request, order)
	err = wfe.writeJsonResponse(response, logEvent, http.StatusCreated, respObj)
	if err != nil {
		wfe.sendError(response, logEvent, probs.ServerInternal("Error marshaling order"), err)
		return
	}
	newOrderSuccessful = true
}

// GetOrder is used to retrieve a existing order object
func (wfe *WebFrontEndImpl) GetOrder(ctx context.Context, logEvent *web.RequestEvent, response http.ResponseWriter, request *http.Request) {
	var requesterAccount *core.Registration
	// Any POSTs to the Order endpoint should be POST-as-GET requests. There are
	// no POSTs with a body allowed for this endpoint.
	if request.Method == http.MethodPost {
		acct, err := wfe.validPOSTAsGETForAccount(request, ctx, logEvent)
		if err != nil {
			wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Unable to validate JWS"), err)
			return
		}
		requesterAccount = acct
	}

	// Path prefix is stripped, so this should be like "<account ID>/<order ID>"
	fields := strings.SplitN(request.URL.Path, "/", 2)
	if len(fields) != 2 {
		wfe.sendError(response, logEvent, probs.NotFound("Invalid request path"), nil)
		return
	}
	acctID, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		wfe.sendError(response, logEvent, probs.Malformed("Invalid account ID"), err)
		return
	}
	orderID, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		wfe.sendError(response, logEvent, probs.Malformed("Invalid order ID"), err)
		return
	}

	order, err := wfe.sa.GetOrder(ctx, &sapb.OrderRequest{Id: orderID})
	if err != nil {
		if errors.Is(err, berrors.NotFound) {
			wfe.sendError(response, logEvent, probs.NotFound(fmt.Sprintf("No order for ID %d", orderID)), nil)
			return
		}
		wfe.sendError(response, logEvent, web.ProblemDetailsForError(err,
			fmt.Sprintf("Failed to retrieve order for ID %d", orderID)), err)
		return
	}

	if core.IsAnyNilOrZero(order.Id, order.Status, order.RegistrationID, order.Identifiers, order.Created, order.Expires) {
		wfe.sendError(response, logEvent, probs.ServerInternal(fmt.Sprintf("Failed to retrieve order for ID %d", orderID)), errIncompleteGRPCResponse)
		return
	}

	if order.RegistrationID != acctID {
		wfe.sendError(response, logEvent, probs.NotFound(fmt.Sprintf("No order found for account ID %d", acctID)), nil)
		return
	}

	// If the requesterAccount is not nil then this was an authenticated
	// POST-as-GET request and we need to verify the requesterAccount is the
	// order's owner.
	if requesterAccount != nil && order.RegistrationID != requesterAccount.ID {
		wfe.sendError(response, logEvent, probs.NotFound(fmt.Sprintf("No order found for account ID %d", acctID)), nil)
		return
	}

	respObj := wfe.orderToOrderJSON(request, order)

	if respObj.Status == core.StatusProcessing {
		response.Header().Set(headerRetryAfter, strconv.Itoa(orderRetryAfter))
	}

	orderURL := web.RelativeEndpoint(request,
		fmt.Sprintf("%s%d/%d", orderPath, acctID, order.Id))
	response.Header().Set("Location", orderURL)

	err = wfe.writeJsonResponse(response, logEvent, http.StatusOK, respObj)
	if err != nil {
		wfe.sendError(response, logEvent, probs.ServerInternal("Error marshaling order"), err)
		return
	}
}

// FinalizeOrder is used to request issuance for a existing order object.
// Most processing of the order details is handled by the RA but
// we do attempt to throw away requests with invalid CSRs here.
func (wfe *WebFrontEndImpl) FinalizeOrder(ctx context.Context, logEvent *web.RequestEvent, response http.ResponseWriter, request *http.Request) {
	// Validate the POST body signature and get the authenticated account for this
	// finalize order request
	body, _, acct, err := wfe.validPOSTForAccount(request, ctx, logEvent)
	addRequesterHeader(response, logEvent.Requester)
	if err != nil {
		wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Unable to validate JWS"), err)
		return
	}

	// Order URLs are like: /acme/finalize/<account>/<order>/. The prefix is
	// stripped by the time we get here.
	fields := strings.SplitN(request.URL.Path, "/", 2)
	if len(fields) != 2 {
		wfe.sendError(response, logEvent, probs.NotFound("Invalid request path"), nil)
		return
	}
	acctID, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		wfe.sendError(response, logEvent, probs.Malformed("Invalid account ID"), nil)
		return
	}
	orderID, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		wfe.sendError(response, logEvent, probs.Malformed("Invalid order ID"), nil)
		return
	}

	if acct.ID != acctID {
		wfe.sendError(response, logEvent, probs.Malformed("Mismatched account ID"), nil)
		return
	}

	order, err := wfe.sa.GetOrder(ctx, &sapb.OrderRequest{Id: orderID})
	if err != nil {
		if errors.Is(err, berrors.NotFound) {
			wfe.sendError(response, logEvent, probs.NotFound(fmt.Sprintf("No order for ID %d", orderID)), nil)
			return
		}
		wfe.sendError(response, logEvent, web.ProblemDetailsForError(err,
			fmt.Sprintf("Failed to retrieve order for ID %d", orderID)), err)
		return
	}

	orderIdents := identifier.FromProtoSlice(order.Identifiers)
	if core.IsAnyNilOrZero(order.Id, order.Status, order.RegistrationID, orderIdents, order.Created, order.Expires) {
		wfe.sendError(response, logEvent, probs.ServerInternal(fmt.Sprintf("Failed to retrieve order for ID %d", orderID)), errIncompleteGRPCResponse)
		return
	}

	// If the authenticated account ID doesn't match the order's registration ID
	// pretend it doesn't exist and abort.
	if acct.ID != order.RegistrationID {
		wfe.sendError(response, logEvent, probs.NotFound(fmt.Sprintf("No order found for account ID %d", acct.ID)), nil)
		return
	}

	// Only ready orders can be finalized.
	if order.Status != string(core.StatusReady) {
		wfe.sendError(response, logEvent, probs.OrderNotReady(fmt.Sprintf("Order's status (%q) is not acceptable for finalization", order.Status)), nil)
		return
	}

	// If the order is expired we can not finalize it and must return an error
	orderExpiry := order.Expires.AsTime()
	if orderExpiry.Before(wfe.clk.Now()) {
		wfe.sendError(response, logEvent, probs.NotFound(fmt.Sprintf("Order %d is expired", order.Id)), nil)
		return
	}

	// Don't finalize orders with profiles we no longer recognize.
	if order.CertificateProfileName != "" {
		err = wfe.validateCertificateProfileName(order.CertificateProfileName)
		if err != nil {
			// TODO(#7392) Provide link to profile documentation.
			wfe.sendError(response, logEvent, probs.InvalidProfile(err.Error()), err)
			return
		}
	}

	// The authenticated finalize message body should be an encoded CSR
	var rawCSR core.RawCertificateRequest
	err = json.Unmarshal(body, &rawCSR)
	if err != nil {
		wfe.sendError(response, logEvent,
			probs.Malformed("Error unmarshaling finalize order request"), err)
		return
	}

	// Check for a malformed CSR early to avoid unnecessary RPCs
	csr, err := x509.ParseCertificateRequest(rawCSR.CSR)
	if err != nil {
		wfe.sendError(response, logEvent, probs.Malformed("Error parsing certificate request: %s", err), err)
		return
	}

	logEvent.Identifiers = orderIdents
	logEvent.Extra["KeyType"] = web.KeyTypeToString(csr.PublicKey)

	updatedOrder, err := wfe.ra.FinalizeOrder(ctx, &rapb.FinalizeOrderRequest{
		Csr:   rawCSR.CSR,
		Order: order,
	})
	if err != nil {
		wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Error finalizing order"), err)
		return
	}
	if core.IsAnyNilOrZero(updatedOrder.Id, updatedOrder.RegistrationID, updatedOrder.Identifiers, updatedOrder.Created, updatedOrder.Expires) {
		wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "Error validating order"), errIncompleteGRPCResponse)
		return
	}

	// Inc CSR signature algorithm counter
	wfe.stats.csrSignatureAlgs.With(prometheus.Labels{"type": csr.SignatureAlgorithm.String()}).Inc()

	orderURL := web.RelativeEndpoint(request,
		fmt.Sprintf("%s%d/%d", orderPath, acct.ID, updatedOrder.Id))
	response.Header().Set("Location", orderURL)

	respObj := wfe.orderToOrderJSON(request, updatedOrder)

	if respObj.Status == core.StatusProcessing {
		response.Header().Set(headerRetryAfter, strconv.Itoa(orderRetryAfter))
	}

	err = wfe.writeJsonResponse(response, logEvent, http.StatusOK, respObj)
	if err != nil {
		wfe.sendError(response, logEvent, probs.ServerInternal("Unable to write finalize order response"), err)
		return
	}
}

// parseARICertID parses the "certID", a unique identifier specified in
// draft-ietf-acme-ari-03. It takes the composite string as input returns a
// extracted and decoded certificate serial. If the decoded AKID does not match
// any known issuer or the serial number is not valid, an error is returned. For
// more details see:
// https://datatracker.ietf.org/doc/html/draft-ietf-acme-ari-03#section-4.1.
func parseARICertID(path string, issuerCertificates map[issuance.NameID]*issuance.Certificate) (string, error) {
	parts := strings.Split(path, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", berrors.MalformedError("Invalid path")
	}

	akid, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", berrors.MalformedError("Authority Key Identifier was not base64url-encoded or contained padding: %s", err)
	}

	var found bool
	for _, issuer := range issuerCertificates {
		if bytes.Equal(issuer.SubjectKeyId, akid) {
			found = true
			break
		}
	}
	if !found {
		return "", berrors.NotFoundError("path contained an Authority Key Identifier that did not match a known issuer")
	}

	serialNumber, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", berrors.NotFoundError("serial number was not base64url-encoded or contained padding: %s", err)
	}

	return core.SerialToString(new(big.Int).SetBytes(serialNumber)), nil
}

// RenewalInfo is used to get information about the suggested renewal window
// for the given certificate. It only accepts unauthenticated GET requests.
func (wfe *WebFrontEndImpl) RenewalInfo(ctx context.Context, logEvent *web.RequestEvent, response http.ResponseWriter, request *http.Request) {
	if !features.Get().ServeRenewalInfo {
		wfe.sendError(response, logEvent, probs.NotFound("Feature not enabled"), nil)
		return
	}

	if len(request.URL.Path) == 0 {
		wfe.sendError(response, logEvent, probs.NotFound("Must specify a request path"), nil)
		return
	}

	decodedSerial, err := parseARICertID(request.URL.Path, wfe.issuerCertificates)
	if err != nil {
		wfe.sendError(response, logEvent, web.ProblemDetailsForError(err, "While parsing ARI CertID an error occurred"), err)
		return
	}

	// We can do all of our processing based just on the serial, because Boulder
	// does not re-use the same serial across multiple issuers.
	logEvent.Extra["RequestedSerial"] = decodedSerial

	renewalInfo, err := wfe.determineARIWindow(ctx, decodedSerial)
	if err != nil {
		if errors.Is(err, berrors.NotFound) {
			wfe.sendError(response, logEvent, probs.NotFound("Requested certificate was not found"), nil)
			return
		}
		wfe.sendError(response, logEvent, probs.ServerInternal("Error determining renewal window"), err)
		return
	}

	response.Header().Set(headerRetryAfter, fmt.Sprintf("%d", int(6*time.Hour/time.Second)))
	err = wfe.writeJsonResponse(response, logEvent, http.StatusOK, renewalInfo)
	if err != nil {
		wfe.sendError(response, logEvent, probs.ServerInternal("Error marshalling renewalInfo"), err)
		return
	}
}

func extractRequesterIP(req *http.Request) (netip.Addr, error) {
	ip, err := netip.ParseAddr(req.Header.Get("X-Real-IP"))
	if err == nil {
		return ip, nil
	}
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return netip.Addr{}, err
	}
	return netip.ParseAddr(host)
}

func urlForAuthz(authz core.Authorization, request *http.Request) string {
	return web.RelativeEndpoint(request, fmt.Sprintf("%s%d/%s", authzPath, authz.RegistrationID, authz.ID))
}
