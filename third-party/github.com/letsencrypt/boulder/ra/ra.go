package ra

import (
	"context"
	"crypto"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/crypto/ocsp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/letsencrypt/boulder/akamai"
	akamaipb "github.com/letsencrypt/boulder/akamai/proto"
	capb "github.com/letsencrypt/boulder/ca/proto"
	"github.com/letsencrypt/boulder/core"
	corepb "github.com/letsencrypt/boulder/core/proto"
	csrlib "github.com/letsencrypt/boulder/csr"
	"github.com/letsencrypt/boulder/ctpolicy"
	berrors "github.com/letsencrypt/boulder/errors"
	"github.com/letsencrypt/boulder/features"
	"github.com/letsencrypt/boulder/goodkey"
	bgrpc "github.com/letsencrypt/boulder/grpc"
	"github.com/letsencrypt/boulder/identifier"
	"github.com/letsencrypt/boulder/issuance"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/policy"
	"github.com/letsencrypt/boulder/probs"
	pubpb "github.com/letsencrypt/boulder/publisher/proto"
	rapb "github.com/letsencrypt/boulder/ra/proto"
	"github.com/letsencrypt/boulder/ratelimit"
	"github.com/letsencrypt/boulder/ratelimits"
	"github.com/letsencrypt/boulder/revocation"
	sapb "github.com/letsencrypt/boulder/sa/proto"
	vapb "github.com/letsencrypt/boulder/va/proto"

	"github.com/letsencrypt/boulder/web"
)

var (
	errIncompleteGRPCRequest  = errors.New("incomplete gRPC request message")
	errIncompleteGRPCResponse = errors.New("incomplete gRPC response message")

	// caaRecheckDuration is the amount of time after a CAA check that we will
	// recheck the CAA records for a domain. Per Baseline Requirements, we must
	// recheck CAA records within 8 hours of issuance. We set this to 7 hours to
	// stay on the safe side.
	caaRecheckDuration = -7 * time.Hour
)

type caaChecker interface {
	IsCAAValid(
		ctx context.Context,
		in *vapb.IsCAAValidRequest,
		opts ...grpc.CallOption,
	) (*vapb.IsCAAValidResponse, error)
}

// RegistrationAuthorityImpl defines an RA.
//
// NOTE: All of the fields in RegistrationAuthorityImpl need to be
// populated, or there is a risk of panic.
type RegistrationAuthorityImpl struct {
	rapb.UnsafeRegistrationAuthorityServer
	CA        capb.CertificateAuthorityClient
	OCSP      capb.OCSPGeneratorClient
	VA        vapb.VAClient
	SA        sapb.StorageAuthorityClient
	PA        core.PolicyAuthority
	publisher pubpb.PublisherClient
	caa       caaChecker

	clk       clock.Clock
	log       blog.Logger
	keyPolicy goodkey.KeyPolicy
	// How long before a newly created authorization expires.
	authorizationLifetime        time.Duration
	pendingAuthorizationLifetime time.Duration
	rlPolicies                   ratelimit.Limits
	maxContactsPerReg            int
	limiter                      *ratelimits.Limiter
	txnBuilder                   *ratelimits.TransactionBuilder
	maxNames                     int
	orderLifetime                time.Duration
	finalizeTimeout              time.Duration
	finalizeWG                   sync.WaitGroup

	issuersByNameID map[issuance.NameID]*issuance.Certificate
	purger          akamaipb.AkamaiPurgerClient

	ctpolicy *ctpolicy.CTPolicy

	ctpolicyResults             *prometheus.HistogramVec
	revocationReasonCounter     *prometheus.CounterVec
	namesPerCert                *prometheus.HistogramVec
	rlCheckLatency              *prometheus.HistogramVec
	rlOverrideUsageGauge        *prometheus.GaugeVec
	newRegCounter               prometheus.Counter
	recheckCAACounter           prometheus.Counter
	newCertCounter              *prometheus.CounterVec
	recheckCAAUsedAuthzLifetime prometheus.Counter
	authzAges                   *prometheus.HistogramVec
	orderAges                   *prometheus.HistogramVec
	inflightFinalizes           prometheus.Gauge
	certCSRMismatch             prometheus.Counter
}

var _ rapb.RegistrationAuthorityServer = (*RegistrationAuthorityImpl)(nil)

// NewRegistrationAuthorityImpl constructs a new RA object.
func NewRegistrationAuthorityImpl(
	clk clock.Clock,
	logger blog.Logger,
	stats prometheus.Registerer,
	maxContactsPerReg int,
	keyPolicy goodkey.KeyPolicy,
	limiter *ratelimits.Limiter,
	txnBuilder *ratelimits.TransactionBuilder,
	maxNames int,
	authorizationLifetime time.Duration,
	pendingAuthorizationLifetime time.Duration,
	pubc pubpb.PublisherClient,
	caaClient caaChecker,
	orderLifetime time.Duration,
	finalizeTimeout time.Duration,
	ctp *ctpolicy.CTPolicy,
	purger akamaipb.AkamaiPurgerClient,
	issuers []*issuance.Certificate,
) *RegistrationAuthorityImpl {
	ctpolicyResults := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ctpolicy_results",
			Help:    "Histogram of latencies of ctpolicy.GetSCTs calls with success/failure/deadlineExceeded labels",
			Buckets: metrics.InternetFacingBuckets,
		},
		[]string{"result"},
	)
	stats.MustRegister(ctpolicyResults)

	namesPerCert := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "names_per_cert",
			Help: "Histogram of the number of SANs in requested and issued certificates",
			// The namesPerCert buckets are chosen based on the current Let's Encrypt
			// limit of 100 SANs per certificate.
			Buckets: []float64{1, 5, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100},
		},
		// Type label value is either "requested" or "issued".
		[]string{"type"},
	)
	stats.MustRegister(namesPerCert)

	rlCheckLatency := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "ratelimitsv1_check_latency_seconds",
		Help: fmt.Sprintf("Latency of ratelimit checks labeled by limit=[name] and decision=[%s|%s], in seconds", ratelimits.Allowed, ratelimits.Denied),
	}, []string{"limit", "decision"})
	stats.MustRegister(rlCheckLatency)

	overrideUsageGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "ratelimitsv1_override_usage",
		Help: "Proportion of override limit used, by limit name and client identifier.",
	}, []string{"limit", "override_key"})
	stats.MustRegister(overrideUsageGauge)

	newRegCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "new_registrations",
		Help: "A counter of new registrations",
	})
	stats.MustRegister(newRegCounter)

	recheckCAACounter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "recheck_caa",
		Help: "A counter of CAA rechecks",
	})
	stats.MustRegister(recheckCAACounter)

	recheckCAAUsedAuthzLifetime := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "recheck_caa_used_authz_lifetime",
		Help: "A counter times the old codepath was used for CAA recheck time",
	})
	stats.MustRegister(recheckCAAUsedAuthzLifetime)

	newCertCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "new_certificates",
		Help: "A counter of new certificates including the certificate profile name and hexadecimal certificate profile hash",
	}, []string{"profileName", "profileHash"})
	stats.MustRegister(newCertCounter)

	revocationReasonCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "revocation_reason",
		Help: "A counter of certificate revocation reasons",
	}, []string{"reason"})
	stats.MustRegister(revocationReasonCounter)

	authzAges := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "authz_ages",
		Help: "Histogram of ages, in seconds, of Authorization objects, labelled by method and type",
		// authzAges keeps track of how old, in seconds, authorizations are when
		// we attach them to a new order and again when we finalize that order.
		// We give it a non-standard bucket distribution so that the leftmost
		// (closest to zero) bucket can be used exclusively for brand-new (i.e.
		// not reused) authzs. Our buckets are: one nanosecond, one second, one
		// minute, one hour, 7 hours (our CAA reuse time), 1 day, 2 days, 7
		// days, 30 days, +inf (should be empty).
		Buckets: []float64{0.000000001, 1, 60, 3600, 25200, 86400, 172800, 604800, 2592000, 7776000},
	}, []string{"method", "type"})
	stats.MustRegister(authzAges)

	orderAges := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "order_ages",
		Help: "Histogram of ages, in seconds, of Order objects when they're reused and finalized, labelled by method",
		// Orders currently have a max age of 7 days (168hrs), so our buckets
		// are: one nanosecond (new), 1 second, 10 seconds, 1 minute, 10
		// minutes, 1 hour, 7 hours (our CAA reuse time), 1 day, 2 days, 7 days, +inf.
		Buckets: []float64{0.000000001, 1, 10, 60, 600, 3600, 25200, 86400, 172800, 604800},
	}, []string{"method"})
	stats.MustRegister(orderAges)

	inflightFinalizes := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "inflight_finalizes",
		Help: "Gauge of the number of current asynchronous finalize goroutines",
	})
	stats.MustRegister(inflightFinalizes)

	certCSRMismatch := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "cert_csr_mismatch",
		Help: "Number of issued certificates that have failed ra.matchesCSR for any reason. This is _real bad_ and should be alerted upon.",
	})
	stats.MustRegister(certCSRMismatch)

	issuersByNameID := make(map[issuance.NameID]*issuance.Certificate)
	for _, issuer := range issuers {
		issuersByNameID[issuer.NameID()] = issuer
	}

	ra := &RegistrationAuthorityImpl{
		clk:                          clk,
		log:                          logger,
		authorizationLifetime:        authorizationLifetime,
		pendingAuthorizationLifetime: pendingAuthorizationLifetime,
		rlPolicies:                   ratelimit.New(),
		maxContactsPerReg:            maxContactsPerReg,
		keyPolicy:                    keyPolicy,
		limiter:                      limiter,
		txnBuilder:                   txnBuilder,
		maxNames:                     maxNames,
		publisher:                    pubc,
		caa:                          caaClient,
		orderLifetime:                orderLifetime,
		finalizeTimeout:              finalizeTimeout,
		ctpolicy:                     ctp,
		ctpolicyResults:              ctpolicyResults,
		purger:                       purger,
		issuersByNameID:              issuersByNameID,
		namesPerCert:                 namesPerCert,
		rlCheckLatency:               rlCheckLatency,
		rlOverrideUsageGauge:         overrideUsageGauge,
		newRegCounter:                newRegCounter,
		recheckCAACounter:            recheckCAACounter,
		newCertCounter:               newCertCounter,
		revocationReasonCounter:      revocationReasonCounter,
		recheckCAAUsedAuthzLifetime:  recheckCAAUsedAuthzLifetime,
		authzAges:                    authzAges,
		orderAges:                    orderAges,
		inflightFinalizes:            inflightFinalizes,
		certCSRMismatch:              certCSRMismatch,
	}
	return ra
}

func (ra *RegistrationAuthorityImpl) LoadRateLimitPoliciesFile(filename string) error {
	configBytes, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	err = ra.rlPolicies.LoadPolicies(configBytes)
	if err != nil {
		return err
	}

	return nil
}

// certificateRequestAuthz is a struct for holding information about a valid
// authz referenced during a certificateRequestEvent. It holds both the
// authorization ID and the challenge type that made the authorization valid. We
// specifically include the challenge type that solved the authorization to make
// some common analysis easier.
type certificateRequestAuthz struct {
	ID            string
	ChallengeType core.AcmeChallenge
}

// certificateRequestEvent is a struct for holding information that is logged as
// JSON to the audit log as the result of an issuance event.
type certificateRequestEvent struct {
	ID string `json:",omitempty"`
	// Requester is the associated account ID
	Requester int64 `json:",omitempty"`
	// OrderID is the associated order ID (may be empty for an ACME v1 issuance)
	OrderID int64 `json:",omitempty"`
	// SerialNumber is the string representation of the issued certificate's
	// serial number
	SerialNumber string `json:",omitempty"`
	// VerifiedFields are required by the baseline requirements and are always
	// a static value for Boulder.
	VerifiedFields []string `json:",omitempty"`
	// CommonName is the subject common name from the issued cert
	CommonName string `json:",omitempty"`
	// Names are the DNS SAN entries from the issued cert
	Names []string `json:",omitempty"`
	// NotBefore is the starting timestamp of the issued cert's validity period
	NotBefore time.Time `json:",omitempty"`
	// NotAfter is the ending timestamp of the issued cert's validity period
	NotAfter time.Time `json:",omitempty"`
	// RequestTime and ResponseTime are for tracking elapsed time during issuance
	RequestTime  time.Time `json:",omitempty"`
	ResponseTime time.Time `json:",omitempty"`
	// Error contains any encountered errors
	Error string `json:",omitempty"`
	// Authorizations is a map of identifier names to certificateRequestAuthz
	// objects. It can be used to understand how the names in a certificate
	// request were authorized.
	Authorizations map[string]certificateRequestAuthz
	// CertProfileName is a human readable name used to refer to the certificate
	// profile.
	CertProfileName string `json:",omitempty"`
	// CertProfileHash is SHA256 sum over every exported field of an
	// issuance.ProfileConfig, represented here as a hexadecimal string.
	CertProfileHash string `json:",omitempty"`
}

// certificateRevocationEvent is a struct for holding information that is logged
// as JSON to the audit log as the result of a revocation event.
type certificateRevocationEvent struct {
	ID string `json:",omitempty"`
	// SerialNumber is the string representation of the revoked certificate's
	// serial number.
	SerialNumber string `json:",omitempty"`
	// Reason is the integer representing the revocation reason used.
	Reason int64 `json:",omitempty"`
	// Method is the way in which revocation was requested.
	// It will be one of the strings: "applicant", "subscriber", "control", "key", or "admin".
	Method string `json:",omitempty"`
	// RequesterID is the account ID of the requester.
	// Will be zero for admin revocations.
	RequesterID int64 `json:",omitempty"`
	// AdminName is the name of the admin requester.
	// Will be zero for subscriber revocations.
	AdminName string `json:",omitempty"`
	// Error contains any error encountered during revocation.
	Error string `json:",omitempty"`
}

// finalizationCAACheckEvent is a struct for holding information logged as JSON
// to the info log as the result of an issuance event. It is logged when the RA
// performs the final CAA check of a certificate finalization request.
type finalizationCAACheckEvent struct {
	// Requester is the associated account ID.
	Requester int64 `json:",omitempty"`
	// Reused is a count of Authz where the original CAA check was performed in
	// the last 7 hours.
	Reused int `json:",omitempty"`
	// Rechecked is a count of Authz where a new CAA check was performed because
	// the original check was older than 7 hours.
	Rechecked int `json:",omitempty"`
}

// noRegistrationID is used for the regID parameter to GetThreshold when no
// registration-based overrides are necessary.
const noRegistrationID = -1

// registrationCounter is a type to abstract the use of `CountRegistrationsByIP`
// or `CountRegistrationsByIPRange` SA methods.
type registrationCounter func(context.Context, *sapb.CountRegistrationsByIPRequest, ...grpc.CallOption) (*sapb.Count, error)

// checkRegistrationIPLimit checks a specific registraton limit by using the
// provided registrationCounter function to determine if the limit has been
// exceeded for a given IP or IP range
func (ra *RegistrationAuthorityImpl) checkRegistrationIPLimit(ctx context.Context, limit ratelimit.RateLimitPolicy, ip net.IP, counter registrationCounter) error {
	now := ra.clk.Now()
	count, err := counter(ctx, &sapb.CountRegistrationsByIPRequest{
		Ip: ip,
		Range: &sapb.Range{
			Earliest: timestamppb.New(limit.WindowBegin(now)),
			Latest:   timestamppb.New(now),
		},
	})
	if err != nil {
		return err
	}

	threshold, overrideKey := limit.GetThreshold(ip.String(), noRegistrationID)
	if count.Count >= threshold {
		return berrors.RegistrationsPerIPError(0, "too many registrations for this IP")
	}
	if overrideKey != "" {
		// We do not support overrides for the NewRegistrationsPerIPRange limit.
		utilization := float64(count.Count+1) / float64(threshold)
		ra.rlOverrideUsageGauge.WithLabelValues(ratelimit.RegistrationsPerIP, overrideKey).Set(utilization)
	}

	return nil
}

// checkRegistrationLimits enforces the RegistrationsPerIP and
// RegistrationsPerIPRange limits
func (ra *RegistrationAuthorityImpl) checkRegistrationLimits(ctx context.Context, ip net.IP) error {
	// Check the registrations per IP limit using the CountRegistrationsByIP SA
	// function that matches IP addresses exactly
	exactRegLimit := ra.rlPolicies.RegistrationsPerIP()
	if exactRegLimit.Enabled() {
		started := ra.clk.Now()
		err := ra.checkRegistrationIPLimit(ctx, exactRegLimit, ip, ra.SA.CountRegistrationsByIP)
		elapsed := ra.clk.Since(started)
		if err != nil {
			if errors.Is(err, berrors.RateLimit) {
				ra.rlCheckLatency.WithLabelValues(ratelimit.RegistrationsPerIP, ratelimits.Denied).Observe(elapsed.Seconds())
				ra.log.Infof("Rate limit exceeded, RegistrationsPerIP, by IP: %q", ip)
			}
			return err
		}
		ra.rlCheckLatency.WithLabelValues(ratelimit.RegistrationsPerIP, ratelimits.Allowed).Observe(elapsed.Seconds())
	}

	// We only apply the fuzzy reg limit to IPv6 addresses.
	// Per https://golang.org/pkg/net/#IP.To4 "If ip is not an IPv4 address, To4
	// returns nil"
	if ip.To4() != nil {
		return nil
	}

	// Check the registrations per IP range limit using the
	// CountRegistrationsByIPRange SA function that fuzzy-matches IPv6 addresses
	// within a larger address range
	fuzzyRegLimit := ra.rlPolicies.RegistrationsPerIPRange()
	if fuzzyRegLimit.Enabled() {
		started := ra.clk.Now()
		err := ra.checkRegistrationIPLimit(ctx, fuzzyRegLimit, ip, ra.SA.CountRegistrationsByIPRange)
		elapsed := ra.clk.Since(started)
		if err != nil {
			if errors.Is(err, berrors.RateLimit) {
				ra.rlCheckLatency.WithLabelValues(ratelimit.RegistrationsPerIPRange, ratelimits.Denied).Observe(elapsed.Seconds())
				ra.log.Infof("Rate limit exceeded, RegistrationsByIPRange, IP: %q", ip)

				// For the fuzzyRegLimit we use a new error message that specifically
				// mentions that the limit being exceeded is applied to a *range* of IPs
				return berrors.RateLimitError(0, "too many registrations for this IP range")
			}
			return err
		}
		ra.rlCheckLatency.WithLabelValues(ratelimit.RegistrationsPerIPRange, ratelimits.Allowed).Observe(elapsed.Seconds())
	}

	return nil
}

// NewRegistration constructs a new Registration from a request.
func (ra *RegistrationAuthorityImpl) NewRegistration(ctx context.Context, request *corepb.Registration) (*corepb.Registration, error) {
	// Error if the request is nil, there is no account key or IP address
	if request == nil || len(request.Key) == 0 || len(request.InitialIP) == 0 {
		return nil, errIncompleteGRPCRequest
	}

	// Check if account key is acceptable for use.
	var key jose.JSONWebKey
	err := key.UnmarshalJSON(request.Key)
	if err != nil {
		return nil, berrors.InternalServerError("failed to unmarshal account key: %s", err.Error())
	}
	err = ra.keyPolicy.GoodKey(ctx, key.Key)
	if err != nil {
		return nil, berrors.MalformedError("invalid public key: %s", err.Error())
	}

	// Check IP address rate limits.
	var ipAddr net.IP
	err = ipAddr.UnmarshalText(request.InitialIP)
	if err != nil {
		return nil, berrors.InternalServerError("failed to unmarshal ip address: %s", err.Error())
	}
	err = ra.checkRegistrationLimits(ctx, ipAddr)
	if err != nil {
		return nil, err
	}

	// Check that contacts conform to our expectations.
	err = validateContactsPresent(request.Contact, request.ContactsPresent)
	if err != nil {
		return nil, err
	}
	err = ra.validateContacts(request.Contact)
	if err != nil {
		return nil, err
	}

	// Don't populate ID or CreatedAt because those will be set by the SA.
	req := &corepb.Registration{
		Key:             request.Key,
		Contact:         request.Contact,
		ContactsPresent: request.ContactsPresent,
		Agreement:       request.Agreement,
		InitialIP:       request.InitialIP,
		Status:          string(core.StatusValid),
	}

	// Store the registration object, then return the version that got stored.
	res, err := ra.SA.NewRegistration(ctx, req)
	if err != nil {
		return nil, err
	}

	ra.newRegCounter.Inc()
	return res, nil
}

// validateContacts checks the provided list of contacts, returning an error if
// any are not acceptable. Unacceptable contacts lists include:
// * An empty list
// * A list has more than maxContactsPerReg contacts
// * A list containing an empty contact
// * A list containing a contact that does not parse as a URL
// * A list containing a contact that has a URL scheme other than mailto
// * A list containing a mailto contact that contains hfields
// * A list containing a contact that has non-ascii characters
// * A list containing a contact that doesn't pass `policy.ValidEmail`
func (ra *RegistrationAuthorityImpl) validateContacts(contacts []string) error {
	if len(contacts) == 0 {
		return nil // Nothing to validate
	}
	if ra.maxContactsPerReg > 0 && len(contacts) > ra.maxContactsPerReg {
		return berrors.MalformedError(
			"too many contacts provided: %d > %d",
			len(contacts),
			ra.maxContactsPerReg,
		)
	}

	for _, contact := range contacts {
		if contact == "" {
			return berrors.InvalidEmailError("empty contact")
		}
		parsed, err := url.Parse(contact)
		if err != nil {
			return berrors.InvalidEmailError("invalid contact")
		}
		if parsed.Scheme != "mailto" {
			return berrors.UnsupportedContactError("contact method %q is not supported", parsed.Scheme)
		}
		if parsed.RawQuery != "" || contact[len(contact)-1] == '?' {
			return berrors.InvalidEmailError("contact email %q contains a question mark", contact)
		}
		if parsed.Fragment != "" || contact[len(contact)-1] == '#' {
			return berrors.InvalidEmailError("contact email %q contains a '#'", contact)
		}
		if !core.IsASCII(contact) {
			return berrors.InvalidEmailError(
				"contact email [%q] contains non-ASCII characters",
				contact,
			)
		}
		err = policy.ValidEmail(parsed.Opaque)
		if err != nil {
			return err
		}
	}

	// NOTE(@cpu): For historical reasons (</3) we store ACME account contact
	// information de-normalized in a fixed size `contact` field on the
	// `registrations` table. At the time of writing this field is VARCHAR(191)
	// That means the largest marshalled JSON value we can store is 191 bytes.
	const maxContactBytes = 191
	if jsonBytes, err := json.Marshal(contacts); err != nil {
		// This shouldn't happen with a simple []string but if it does we want the
		// error to be logged internally but served as a 500 to the user so we
		// return a bare error and not a berror here.
		return fmt.Errorf("failed to marshal reg.Contact to JSON: %#v", contacts)
	} else if len(jsonBytes) >= maxContactBytes {
		return berrors.InvalidEmailError(
			"too many/too long contact(s). Please use shorter or fewer email addresses")
	}

	return nil
}

func (ra *RegistrationAuthorityImpl) checkPendingAuthorizationLimit(ctx context.Context, regID int64, limit ratelimit.RateLimitPolicy) error {
	// This rate limit's threshold can only be overridden on a per-regID basis,
	// not based on any other key.
	threshold, overrideKey := limit.GetThreshold("", regID)
	if threshold == -1 {
		return nil
	}
	countPB, err := ra.SA.CountPendingAuthorizations2(ctx, &sapb.RegistrationID{
		Id: regID,
	})
	if err != nil {
		return err
	}
	if countPB.Count >= threshold {
		ra.log.Infof("Rate limit exceeded, PendingAuthorizationsByRegID, regID: %d", regID)
		return berrors.RateLimitError(0, "too many currently pending authorizations: %d", countPB.Count)
	}
	if overrideKey != "" {
		utilization := float64(countPB.Count) / float64(threshold)
		ra.rlOverrideUsageGauge.WithLabelValues(ratelimit.PendingAuthorizationsPerAccount, overrideKey).Set(utilization)
	}
	return nil
}

// checkInvalidAuthorizationLimits checks the failed validation limit for each
// of the provided hostnames. It returns the first error.
func (ra *RegistrationAuthorityImpl) checkInvalidAuthorizationLimits(ctx context.Context, regID int64, hostnames []string, limits ratelimit.RateLimitPolicy) error {
	results := make(chan error, len(hostnames))
	for _, hostname := range hostnames {
		go func(hostname string) {
			results <- ra.checkInvalidAuthorizationLimit(ctx, regID, hostname, limits)
		}(hostname)
	}
	// We don't have to wait for all of the goroutines to finish because there's
	// enough capacity in the chan for them all to write their result even if
	// nothing is reading off the chan anymore.
	for range len(hostnames) {
		err := <-results
		if err != nil {
			return err
		}
	}
	return nil
}

func (ra *RegistrationAuthorityImpl) checkInvalidAuthorizationLimit(ctx context.Context, regID int64, hostname string, limit ratelimit.RateLimitPolicy) error {
	latest := ra.clk.Now().Add(ra.pendingAuthorizationLifetime)
	earliest := latest.Add(-limit.Window.Duration)
	req := &sapb.CountInvalidAuthorizationsRequest{
		RegistrationID: regID,
		Hostname:       hostname,
		Range: &sapb.Range{
			Earliest: timestamppb.New(earliest),
			Latest:   timestamppb.New(latest),
		},
	}
	count, err := ra.SA.CountInvalidAuthorizations2(ctx, req)
	if err != nil {
		return err
	}
	// Most rate limits have a key for overrides, but there is no meaningful key
	// here.
	noKey := ""
	threshold, overrideKey := limit.GetThreshold(noKey, regID)
	if count.Count >= threshold {
		ra.log.Infof("Rate limit exceeded, InvalidAuthorizationsByRegID, regID: %d", regID)
		return berrors.FailedValidationError(0, "too many failed authorizations recently")
	}
	if overrideKey != "" {
		utilization := float64(count.Count) / float64(threshold)
		ra.rlOverrideUsageGauge.WithLabelValues(ratelimit.InvalidAuthorizationsPerAccount, overrideKey).Set(utilization)
	}
	return nil
}

// checkNewOrdersPerAccountLimit enforces the rlPolicies `NewOrdersPerAccount`
// rate limit. This rate limit ensures a client can not create more than the
// specified threshold of new orders within the specified time window.
func (ra *RegistrationAuthorityImpl) checkNewOrdersPerAccountLimit(ctx context.Context, acctID int64, names []string, limit ratelimit.RateLimitPolicy) error {
	// Check if there is already an existing certificate for the exact name set we
	// are issuing for. If so bypass the newOrders limit.
	exists, err := ra.SA.FQDNSetExists(ctx, &sapb.FQDNSetExistsRequest{Domains: names})
	if err != nil {
		return fmt.Errorf("checking renewal exemption for %q: %s", names, err)
	}
	if exists.Exists {
		return nil
	}

	now := ra.clk.Now()
	count, err := ra.SA.CountOrders(ctx, &sapb.CountOrdersRequest{
		AccountID: acctID,
		Range: &sapb.Range{
			Earliest: timestamppb.New(now.Add(-limit.Window.Duration)),
			Latest:   timestamppb.New(now),
		},
	})
	if err != nil {
		return err
	}
	// There is no meaningful override key to use for this rate limit
	noKey := ""
	threshold, overrideKey := limit.GetThreshold(noKey, acctID)
	if count.Count >= threshold {
		return berrors.RateLimitError(0, "too many new orders recently")
	}
	if overrideKey != "" {
		utilization := float64(count.Count+1) / float64(threshold)
		ra.rlOverrideUsageGauge.WithLabelValues(ratelimit.NewOrdersPerAccount, overrideKey).Set(utilization)
	}
	return nil
}

// matchesCSR tests the contents of a generated certificate to make sure
// that the PublicKey, CommonName, and DNSNames match those provided in
// the CSR that was used to generate the certificate. It also checks the
// following fields for:
//   - notBefore is not more than 24 hours ago
//   - BasicConstraintsValid is true
//   - IsCA is false
//   - ExtKeyUsage only contains ExtKeyUsageServerAuth & ExtKeyUsageClientAuth
//   - Subject only contains CommonName & Names
func (ra *RegistrationAuthorityImpl) matchesCSR(parsedCertificate *x509.Certificate, csr *x509.CertificateRequest) error {
	if !core.KeyDigestEquals(parsedCertificate.PublicKey, csr.PublicKey) {
		return berrors.InternalServerError("generated certificate public key doesn't match CSR public key")
	}

	csrNames := csrlib.NamesFromCSR(csr)
	if parsedCertificate.Subject.CommonName != "" {
		// Only check that the issued common name matches one of the SANs if there
		// is an issued CN at all: this allows flexibility on whether we include
		// the CN.
		if !slices.Contains(csrNames.SANs, parsedCertificate.Subject.CommonName) {
			return berrors.InternalServerError("generated certificate CommonName doesn't match any CSR name")
		}
	}

	parsedNames := parsedCertificate.DNSNames
	sort.Strings(parsedNames)
	if !slices.Equal(parsedNames, csrNames.SANs) {
		return berrors.InternalServerError("generated certificate DNSNames don't match CSR DNSNames")
	}

	if !slices.EqualFunc(parsedCertificate.IPAddresses, csr.IPAddresses, func(l, r net.IP) bool { return l.Equal(r) }) {
		return berrors.InternalServerError("generated certificate IPAddresses don't match CSR IPAddresses")
	}
	if !slices.Equal(parsedCertificate.EmailAddresses, csr.EmailAddresses) {
		return berrors.InternalServerError("generated certificate EmailAddresses don't match CSR EmailAddresses")
	}

	if len(parsedCertificate.Subject.Country) > 0 || len(parsedCertificate.Subject.Organization) > 0 ||
		len(parsedCertificate.Subject.OrganizationalUnit) > 0 || len(parsedCertificate.Subject.Locality) > 0 ||
		len(parsedCertificate.Subject.Province) > 0 || len(parsedCertificate.Subject.StreetAddress) > 0 ||
		len(parsedCertificate.Subject.PostalCode) > 0 {
		return berrors.InternalServerError("generated certificate Subject contains fields other than CommonName, or SerialNumber")
	}
	now := ra.clk.Now()
	if now.Sub(parsedCertificate.NotBefore) > time.Hour*24 {
		return berrors.InternalServerError("generated certificate is back dated %s", now.Sub(parsedCertificate.NotBefore))
	}
	if !parsedCertificate.BasicConstraintsValid {
		return berrors.InternalServerError("generated certificate doesn't have basic constraints set")
	}
	if parsedCertificate.IsCA {
		return berrors.InternalServerError("generated certificate can sign other certificates")
	}
	if !slices.Equal(parsedCertificate.ExtKeyUsage, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}) {
		return berrors.InternalServerError("generated certificate doesn't have correct key usage extensions")
	}

	return nil
}

// checkOrderAuthorizations verifies that a provided set of names associated
// with a specific order and account has all of the required valid, unexpired
// authorizations to proceed with issuance. It returns the authorizations that
// satisfied the set of names or it returns an error. If it returns an error, it
// will be of type BoulderError.
func (ra *RegistrationAuthorityImpl) checkOrderAuthorizations(
	ctx context.Context,
	names []string,
	acctID accountID,
	orderID orderID) (map[string]*core.Authorization, error) {
	// Get all of the valid authorizations for this account/order
	req := &sapb.GetValidOrderAuthorizationsRequest{
		Id:     int64(orderID),
		AcctID: int64(acctID),
	}
	authzMapPB, err := ra.SA.GetValidOrderAuthorizations2(ctx, req)
	if err != nil {
		return nil, berrors.InternalServerError("error in GetValidOrderAuthorizations: %s", err)
	}
	authzs, err := bgrpc.PBToAuthzMap(authzMapPB)
	if err != nil {
		return nil, err
	}

	// Ensure the names from the CSR are free of duplicates & lowercased.
	names = core.UniqueLowerNames(names)

	// Check the authorizations to ensure validity for the names required.
	err = ra.checkAuthorizationsCAA(ctx, int64(acctID), names, authzs, ra.clk.Now())
	if err != nil {
		return nil, err
	}

	// Check the challenges themselves too.
	for _, authz := range authzs {
		err = ra.PA.CheckAuthz(authz)
		if err != nil {
			return nil, err
		}
	}

	return authzs, nil
}

// validatedBefore checks if a given authorization's challenge was
// validated before a given time. Returns a bool.
func validatedBefore(authz *core.Authorization, caaRecheckTime time.Time) (bool, error) {
	numChallenges := len(authz.Challenges)
	if numChallenges != 1 {
		return false, fmt.Errorf("authorization has incorrect number of challenges. 1 expected, %d found for: id %s", numChallenges, authz.ID)
	}
	if authz.Challenges[0].Validated == nil {
		return false, fmt.Errorf("authorization's challenge has no validated timestamp for: id %s", authz.ID)
	}
	return authz.Challenges[0].Validated.Before(caaRecheckTime), nil
}

// checkAuthorizationsCAA implements the common logic of validating a set of
// authorizations against a set of names that is used by both
// `checkAuthorizations` and `checkOrderAuthorizations`. If required CAA will be
// rechecked for authorizations that are too old.
// If it returns an error, it will be of type BoulderError.
func (ra *RegistrationAuthorityImpl) checkAuthorizationsCAA(
	ctx context.Context,
	acctID int64,
	names []string,
	authzs map[string]*core.Authorization,
	now time.Time) error {
	// badNames contains the names that were unauthorized
	var badNames []string
	// recheckAuthzs is a list of authorizations that must have their CAA records rechecked
	var recheckAuthzs []*core.Authorization

	// Per Baseline Requirements, CAA must be checked within 8 hours of
	// issuance. CAA is checked when an authorization is validated, so as
	// long as that was less than 8 hours ago, we're fine. We recheck if
	// that was more than 7 hours ago, to be on the safe side. We can
	// check to see if the authorized challenge `AttemptedAt`
	// (`Validated`) value from the database is before our caaRecheckTime.
	// Set the recheck time to 7 hours ago.
	caaRecheckAfter := now.Add(caaRecheckDuration)

	// Set a CAA recheck time based on the assumption of a 30 day authz
	// lifetime. This has been deprecated in favor of a new check based
	// off the Validated time stored in the database, but we want to check
	// both for a time and increment a stat if this code path is hit for
	// compliance safety.
	caaRecheckTime := now.Add(ra.authorizationLifetime).Add(caaRecheckDuration)

	for _, name := range names {
		authz := authzs[name]
		if authz == nil {
			badNames = append(badNames, name)
		} else if authz.Expires == nil {
			return berrors.InternalServerError("found an authorization with a nil Expires field: id %s", authz.ID)
		} else if authz.Expires.Before(now) {
			badNames = append(badNames, name)
		} else if staleCAA, err := validatedBefore(authz, caaRecheckAfter); err != nil {
			return berrors.InternalServerError(err.Error())
		} else if staleCAA {
			// Ensure that CAA is rechecked for this name
			recheckAuthzs = append(recheckAuthzs, authz)
		} else if authz.Expires.Before(caaRecheckTime) {
			// Ensure that CAA is rechecked for this name
			recheckAuthzs = append(recheckAuthzs, authz)
			// This codepath should not be used, but is here as a safety
			// net until the new codepath is proven. Increment metric if
			// it is used.
			ra.recheckCAAUsedAuthzLifetime.Add(1)
		}
	}

	if len(recheckAuthzs) > 0 {
		err := ra.recheckCAA(ctx, recheckAuthzs)
		if err != nil {
			return err
		}
	}

	if len(badNames) > 0 {
		return berrors.UnauthorizedError(
			"authorizations for these names not found or expired: %s",
			strings.Join(badNames, ", "),
		)
	}

	caaEvent := &finalizationCAACheckEvent{
		Requester: acctID,
		Reused:    len(authzs) - len(recheckAuthzs),
		Rechecked: len(recheckAuthzs),
	}
	ra.log.InfoObject("FinalizationCaaCheck", caaEvent)

	return nil
}

// recheckCAA accepts a list of names that need to have their CAA records
// rechecked because their associated authorizations are sufficiently old and
// performs the CAA checks required for each. If any of the rechecks fail an
// error is returned.
func (ra *RegistrationAuthorityImpl) recheckCAA(ctx context.Context, authzs []*core.Authorization) error {
	ra.recheckCAACounter.Add(float64(len(authzs)))

	type authzCAAResult struct {
		authz *core.Authorization
		err   error
	}
	ch := make(chan authzCAAResult, len(authzs))
	for _, authz := range authzs {
		go func(authz *core.Authorization) {
			name := authz.Identifier.Value

			// If an authorization has multiple valid challenges,
			// the type of the first valid challenge is used for
			// the purposes of CAA rechecking.
			var method string
			for _, challenge := range authz.Challenges {
				if challenge.Status == core.StatusValid {
					method = string(challenge.Type)
					break
				}
			}
			if method == "" {
				ch <- authzCAAResult{
					authz: authz,
					err: berrors.InternalServerError(
						"Internal error determining validation method for authorization ID %v (%v)",
						authz.ID, name),
				}
				return
			}

			resp, err := ra.caa.IsCAAValid(ctx, &vapb.IsCAAValidRequest{
				Domain:           name,
				ValidationMethod: method,
				AccountURIID:     authz.RegistrationID,
			})
			if err != nil {
				ra.log.AuditErrf("Rechecking CAA: %s", err)
				err = berrors.InternalServerError(
					"Internal error rechecking CAA for authorization ID %v (%v)",
					authz.ID, name,
				)
			} else if resp.Problem != nil {
				err = berrors.CAAError(resp.Problem.Detail)
			}
			ch <- authzCAAResult{
				authz: authz,
				err:   err,
			}
		}(authz)
	}
	var subErrors []berrors.SubBoulderError
	// Read a recheckResult for each authz from the results channel
	for range len(authzs) {
		recheckResult := <-ch
		// If the result had a CAA boulder error, construct a suberror with the
		// identifier from the authorization that was checked.
		err := recheckResult.err
		if err != nil {
			var bErr *berrors.BoulderError
			if errors.As(err, &bErr) && bErr.Type == berrors.CAA {
				subErrors = append(subErrors, berrors.SubBoulderError{
					Identifier:   recheckResult.authz.Identifier,
					BoulderError: bErr})
			} else {
				return err
			}
		}
	}
	if len(subErrors) > 0 {
		var detail string
		// If there was only one error, then use it as the top level error that is
		// returned.
		if len(subErrors) == 1 {
			return subErrors[0].BoulderError
		}
		detail = fmt.Sprintf(
			"Rechecking CAA for %q and %d more identifiers failed. "+
				"Refer to sub-problems for more information",
			subErrors[0].Identifier.Value,
			len(subErrors)-1)
		return (&berrors.BoulderError{
			Type:   berrors.CAA,
			Detail: detail,
		}).WithSubErrors(subErrors)
	}
	return nil
}

// failOrder marks an order as failed by setting the problem details field of
// the order & persisting it through the SA. If an error occurs doing this we
// log it and don't modify the input order. There aren't any alternatives if we
// can't add the error to the order. This function MUST only be called when we
// are already returning an error for another reason.
func (ra *RegistrationAuthorityImpl) failOrder(
	ctx context.Context,
	order *corepb.Order,
	prob *probs.ProblemDetails) {
	// Use a separate context with its own timeout, since the error we encountered
	// may have been a context cancellation or timeout, and these operations still
	// need to succeed.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 1*time.Second)
	defer cancel()

	// Convert the problem to a protobuf problem for the *corepb.Order field
	pbProb, err := bgrpc.ProblemDetailsToPB(prob)
	if err != nil {
		ra.log.AuditErrf("Could not convert order error problem to PB: %q", err)
		return
	}

	// Assign the protobuf problem to the field and save it via the SA
	order.Error = pbProb
	_, err = ra.SA.SetOrderError(ctx, &sapb.SetOrderErrorRequest{
		Id:    order.Id,
		Error: order.Error,
	})
	if err != nil {
		ra.log.AuditErrf("Could not persist order error: %q", err)
	}
}

// To help minimize the chance that an accountID would be used as an order ID
// (or vice versa) when calling functions that use both we define internal
// `accountID` and `orderID` types so that callers must explicitly cast.
type accountID int64
type orderID int64

// FinalizeOrder accepts a request to finalize an order object and, if possible,
// issues a certificate to satisfy the order. If an order does not have valid,
// unexpired authorizations for all of its associated names an error is
// returned. Similarly we vet that all of the names in the order are acceptable
// based on current policy and return an error if the order can't be fulfilled.
// If successful the order will be returned in processing status for the client
// to poll while awaiting finalization to occur.
func (ra *RegistrationAuthorityImpl) FinalizeOrder(ctx context.Context, req *rapb.FinalizeOrderRequest) (*corepb.Order, error) {
	// Step 1: Set up logging/tracing and validate the Order
	if req == nil || req.Order == nil || len(req.Csr) == 0 {
		return nil, errIncompleteGRPCRequest
	}

	logEvent := certificateRequestEvent{
		ID:          core.NewToken(),
		OrderID:     req.Order.Id,
		Requester:   req.Order.RegistrationID,
		RequestTime: ra.clk.Now(),
	}
	csr, err := ra.validateFinalizeRequest(ctx, req, &logEvent)
	if err != nil {
		return nil, err
	}

	// Observe the age of this order, so we know how quickly most clients complete
	// issuance flows.
	ra.orderAges.WithLabelValues("FinalizeOrder").Observe(ra.clk.Since(req.Order.Created.AsTime()).Seconds())

	// Step 2: Set the Order to Processing status
	//
	// We do this separately from the issuance process itself so that, when we
	// switch to doing issuance asynchronously, we aren't lying to the client
	// when we say that their order is already Processing.
	//
	// NOTE(@cpu): After this point any errors that are encountered must update
	// the state of the order to invalid by setting the order's error field.
	// Otherwise the order will be "stuck" in processing state. It can not be
	// finalized because it isn't pending, but we aren't going to process it
	// further because we already did and encountered an error.
	_, err = ra.SA.SetOrderProcessing(ctx, &sapb.OrderRequest{Id: req.Order.Id})
	if err != nil {
		// Fail the order with a server internal error - we weren't able to set the
		// status to processing and that's unexpected & weird.
		ra.failOrder(ctx, req.Order, probs.ServerInternal("Error setting order processing"))
		return nil, err
	}

	// Update the order status locally since the SA doesn't return the updated
	// order itself after setting the status
	order := req.Order
	order.Status = string(core.StatusProcessing)

	// Steps 3 (issuance) and 4 (cleanup) are done inside a helper function so
	// that we can control whether or not that work happens asynchronously.
	if features.Get().AsyncFinalize {
		// We do this work in a goroutine so that we can better handle latency from
		// getting SCTs and writing the (pre)certificate to the database. This lets
		// us return the order in the Processing state to the client immediately,
		// prompting them to poll the Order object and wait for it to be put into
		// its final state.
		//
		// We track this goroutine's lifetime in a waitgroup global to this RA, so
		// that it can wait for all goroutines to drain during shutdown.
		ra.finalizeWG.Add(1)
		go func() {
			_, err := ra.issueCertificateOuter(ctx, proto.Clone(order).(*corepb.Order), csr, logEvent)
			if err != nil {
				// We only log here, because this is in a background goroutine with
				// no parent goroutine waiting for it to receive the error.
				ra.log.AuditErrf("Asynchronous finalization failed: %s", err.Error())
			}
			ra.finalizeWG.Done()
		}()
		return order, nil
	} else {
		return ra.issueCertificateOuter(ctx, order, csr, logEvent)
	}
}

// validateFinalizeRequest checks that a FinalizeOrder request is fully correct
// and ready for issuance.
func (ra *RegistrationAuthorityImpl) validateFinalizeRequest(
	ctx context.Context,
	req *rapb.FinalizeOrderRequest,
	logEvent *certificateRequestEvent) (*x509.CertificateRequest, error) {
	if req.Order.Id <= 0 {
		return nil, berrors.MalformedError("invalid order ID: %d", req.Order.Id)
	}

	if req.Order.RegistrationID <= 0 {
		return nil, berrors.MalformedError("invalid account ID: %d", req.Order.RegistrationID)
	}

	if core.AcmeStatus(req.Order.Status) != core.StatusReady {
		return nil, berrors.OrderNotReadyError(
			"Order's status (%q) is not acceptable for finalization",
			req.Order.Status)
	}

	// There should never be an order with 0 names at the stage, but we check to
	// be on the safe side, throwing an internal server error if this assumption
	// is ever violated.
	if len(req.Order.Names) == 0 {
		return nil, berrors.InternalServerError("Order has no associated names")
	}

	// Parse the CSR from the request
	csr, err := x509.ParseCertificateRequest(req.Csr)
	if err != nil {
		return nil, berrors.BadCSRError("unable to parse CSR: %s", err.Error())
	}

	err = csrlib.VerifyCSR(ctx, csr, ra.maxNames, &ra.keyPolicy, ra.PA)
	if err != nil {
		// VerifyCSR returns berror instances that can be passed through as-is
		// without wrapping.
		return nil, err
	}

	// Dedupe, lowercase and sort both the names from the CSR and the names in the
	// order.
	csrNames := csrlib.NamesFromCSR(csr).SANs
	orderNames := core.UniqueLowerNames(req.Order.Names)

	// Immediately reject the request if the number of names differ
	if len(orderNames) != len(csrNames) {
		return nil, berrors.UnauthorizedError("Order includes different number of names than CSR specifies")
	}

	// Check that the order names and the CSR names are an exact match
	for i, name := range orderNames {
		if name != csrNames[i] {
			return nil, berrors.UnauthorizedError("CSR is missing Order domain %q", name)
		}
	}

	// Get the originating account for use in the next check.
	regPB, err := ra.SA.GetRegistration(ctx, &sapb.RegistrationID{Id: req.Order.RegistrationID})
	if err != nil {
		return nil, err
	}

	account, err := bgrpc.PbToRegistration(regPB)
	if err != nil {
		return nil, err
	}

	// Make sure they're not using their account key as the certificate key too.
	if core.KeyDigestEquals(csr.PublicKey, account.Key) {
		return nil, berrors.MalformedError("certificate public key must be different than account key")
	}

	// Double-check that all authorizations on this order are also associated with
	// the same account as the order itself.
	authzs, err := ra.checkOrderAuthorizations(ctx, csrNames, accountID(req.Order.RegistrationID), orderID(req.Order.Id))
	if err != nil {
		// Pass through the error without wrapping it because the called functions
		// return BoulderError and we don't want to lose the type.
		return nil, err
	}

	// Collect up a certificateRequestAuthz that stores the ID and challenge type
	// of each of the valid authorizations we used for this issuance.
	logEventAuthzs := make(map[string]certificateRequestAuthz, len(csrNames))
	for name, authz := range authzs {
		// No need to check for error here because we know this same call just
		// succeeded inside ra.checkOrderAuthorizations
		solvedByChallengeType, _ := authz.SolvedBy()
		logEventAuthzs[name] = certificateRequestAuthz{
			ID:            authz.ID,
			ChallengeType: solvedByChallengeType,
		}
		authzAge := (ra.authorizationLifetime - authz.Expires.Sub(ra.clk.Now())).Seconds()
		ra.authzAges.WithLabelValues("FinalizeOrder", string(authz.Status)).Observe(authzAge)
	}
	logEvent.Authorizations = logEventAuthzs

	// Mark that we verified the CN and SANs
	logEvent.VerifiedFields = []string{"subject.commonName", "subjectAltName"}

	return csr, nil
}

// issueCertificateOuter exists solely to ensure that all calls to
// issueCertificateInner have their result handled uniformly, no matter what
// return path that inner function takes. It takes ownership of the logEvent,
// mutates it, and is responsible for outputting its final state.
func (ra *RegistrationAuthorityImpl) issueCertificateOuter(
	ctx context.Context,
	order *corepb.Order,
	csr *x509.CertificateRequest,
	logEvent certificateRequestEvent,
) (*corepb.Order, error) {
	ra.inflightFinalizes.Inc()
	defer ra.inflightFinalizes.Dec()

	// Step 3: Issue the Certificate
	cert, cpId, err := ra.issueCertificateInner(
		ctx, csr, order.CertificateProfileName, accountID(order.RegistrationID), orderID(order.Id))

	// Step 4: Fail the order if necessary, and update metrics and log fields
	var result string
	if err != nil {
		// The problem is computed using `web.ProblemDetailsForError`, the same
		// function the WFE uses to convert between `berrors` and problems. This
		// will turn normal expected berrors like berrors.UnauthorizedError into the
		// correct `urn:ietf:params:acme:error:unauthorized` problem while not
		// letting anything like a server internal error through with sensitive
		// info.
		ra.failOrder(ctx, order, web.ProblemDetailsForError(err, "Error finalizing order"))
		order.Status = string(core.StatusInvalid)

		logEvent.Error = err.Error()
		result = "error"
	} else {
		order.CertificateSerial = core.SerialToString(cert.SerialNumber)
		order.Status = string(core.StatusValid)

		ra.namesPerCert.With(
			prometheus.Labels{"type": "issued"},
		).Observe(float64(len(order.Names)))

		ra.newCertCounter.With(
			prometheus.Labels{
				"profileName": cpId.name,
				"profileHash": hex.EncodeToString(cpId.hash),
			}).Inc()

		logEvent.SerialNumber = core.SerialToString(cert.SerialNumber)
		logEvent.CommonName = cert.Subject.CommonName
		logEvent.Names = cert.DNSNames
		logEvent.NotBefore = cert.NotBefore
		logEvent.NotAfter = cert.NotAfter
		logEvent.CertProfileName = cpId.name
		logEvent.CertProfileHash = hex.EncodeToString(cpId.hash)

		result = "successful"
	}

	logEvent.ResponseTime = ra.clk.Now()
	ra.log.AuditObject(fmt.Sprintf("Certificate request - %s", result), logEvent)

	return order, err
}

// certProfileID contains the name and hash of a certificate profile returned by
// a CA.
type certProfileID struct {
	name string
	hash []byte
}

// issueCertificateInner is part of the [issuance cycle].
//
// It gets a precertificate from the CA, submits it to CT logs to get SCTs,
// then sends the precertificate and the SCTs to the CA to get a final certificate.
//
// This function is responsible for ensuring that we never try to issue a final
// certificate twice for the same precertificate, because that has the potential
// to create certificates with duplicate serials. For instance, this could
// happen if final certificates were created with different sets of SCTs. This
// function accomplishes that by bailing on issuance if there is any error in
// IssueCertificateForPrecertificate; there are no retries, and serials are
// generated in IssuePrecertificate, so serials with errors are dropped and
// never have final certificates issued for them (because there is a possibility
// that the certificate was actually issued but there was an error returning
// it).
//
// [issuance cycle]: https://github.com/letsencrypt/boulder/blob/main/docs/ISSUANCE-CYCLE.md
func (ra *RegistrationAuthorityImpl) issueCertificateInner(
	ctx context.Context,
	csr *x509.CertificateRequest,
	profileName string,
	acctID accountID,
	oID orderID) (*x509.Certificate, *certProfileID, error) {
	if features.Get().AsyncFinalize {
		// If we're in async mode, use a context with a much longer timeout.
		var cancel func()
		ctx, cancel = context.WithTimeout(context.WithoutCancel(ctx), ra.finalizeTimeout)
		defer cancel()
	}

	// wrapError adds a prefix to an error. If the error is a boulder error then
	// the problem detail is updated with the prefix. Otherwise a new error is
	// returned with the message prefixed using `fmt.Errorf`
	wrapError := func(e error, prefix string) error {
		if berr, ok := e.(*berrors.BoulderError); ok {
			berr.Detail = fmt.Sprintf("%s: %s", prefix, berr.Detail)
			return berr
		}
		return fmt.Errorf("%s: %s", prefix, e)
	}

	issueReq := &capb.IssueCertificateRequest{
		Csr:             csr.Raw,
		RegistrationID:  int64(acctID),
		OrderID:         int64(oID),
		CertProfileName: profileName,
	}
	// Once we get a precert from IssuePrecertificate, we must attempt issuing
	// a final certificate at most once. We achieve that by bailing on any error
	// between here and IssueCertificateForPrecertificate.
	precert, err := ra.CA.IssuePrecertificate(ctx, issueReq)
	if err != nil {
		return nil, nil, wrapError(err, "issuing precertificate")
	}

	parsedPrecert, err := x509.ParseCertificate(precert.DER)
	if err != nil {
		return nil, nil, wrapError(err, "parsing precertificate")
	}

	scts, err := ra.getSCTs(ctx, precert.DER, parsedPrecert.NotAfter)
	if err != nil {
		return nil, nil, wrapError(err, "getting SCTs")
	}

	cert, err := ra.CA.IssueCertificateForPrecertificate(ctx, &capb.IssueCertificateForPrecertificateRequest{
		DER:             precert.DER,
		SCTs:            scts,
		RegistrationID:  int64(acctID),
		OrderID:         int64(oID),
		CertProfileHash: precert.CertProfileHash,
	})
	if err != nil {
		return nil, nil, wrapError(err, "issuing certificate for precertificate")
	}

	parsedCertificate, err := x509.ParseCertificate(cert.Der)
	if err != nil {
		return nil, nil, wrapError(err, "parsing final certificate")
	}

	// Asynchronously submit the final certificate to any configured logs
	go ra.ctpolicy.SubmitFinalCert(cert.Der, parsedCertificate.NotAfter)

	err = ra.matchesCSR(parsedCertificate, csr)
	if err != nil {
		ra.certCSRMismatch.Inc()
		return nil, nil, err
	}

	_, err = ra.SA.FinalizeOrder(ctx, &sapb.FinalizeOrderRequest{
		Id:                int64(oID),
		CertificateSerial: core.SerialToString(parsedCertificate.SerialNumber),
	})
	if err != nil {
		return nil, nil, wrapError(err, "persisting finalized order")
	}

	return parsedCertificate, &certProfileID{name: precert.CertProfileName, hash: precert.CertProfileHash}, nil
}

func (ra *RegistrationAuthorityImpl) getSCTs(ctx context.Context, cert []byte, expiration time.Time) (core.SCTDERs, error) {
	started := ra.clk.Now()
	scts, err := ra.ctpolicy.GetSCTs(ctx, cert, expiration)
	took := ra.clk.Since(started)
	// The final cert has already been issued so actually return it to the
	// user even if this fails since we aren't actually doing anything with
	// the SCTs yet.
	if err != nil {
		state := "failure"
		if err == context.DeadlineExceeded {
			state = "deadlineExceeded"
			// Convert the error to a missingSCTsError to communicate the timeout,
			// otherwise it will be a generic serverInternalError
			err = berrors.MissingSCTsError(err.Error())
		}
		ra.log.Warningf("ctpolicy.GetSCTs failed: %s", err)
		ra.ctpolicyResults.With(prometheus.Labels{"result": state}).Observe(took.Seconds())
		return nil, err
	}
	ra.ctpolicyResults.With(prometheus.Labels{"result": "success"}).Observe(took.Seconds())
	return scts, nil
}

// enforceNameCounts uses the provided count RPC to find a count of certificates
// for each of the names. If the count for any of the names exceeds the limit
// for the given registration then the names out of policy are returned to be
// used for a rate limit error.
func (ra *RegistrationAuthorityImpl) enforceNameCounts(ctx context.Context, names []string, limit ratelimit.RateLimitPolicy, regID int64) ([]string, time.Time, error) {
	now := ra.clk.Now()
	req := &sapb.CountCertificatesByNamesRequest{
		Names: names,
		Range: &sapb.Range{
			Earliest: timestamppb.New(limit.WindowBegin(now)),
			Latest:   timestamppb.New(now),
		},
	}

	response, err := ra.SA.CountCertificatesByNames(ctx, req)
	if err != nil {
		return nil, time.Time{}, err
	}

	if len(response.Counts) == 0 {
		return nil, time.Time{}, errIncompleteGRPCResponse
	}

	var badNames []string
	var metricsData []struct {
		overrideKey string
		utilization float64
	}

	// Find the names that have counts at or over the threshold. Range
	// over the names slice input to ensure the order of badNames will
	// return the badNames in the same order they were input.
	for _, name := range names {
		threshold, overrideKey := limit.GetThreshold(name, regID)
		if response.Counts[name] >= threshold {
			badNames = append(badNames, name)
		}
		if overrideKey != "" {
			// Name is under threshold due to an override.
			utilization := float64(response.Counts[name]+1) / float64(threshold)
			metricsData = append(metricsData, struct {
				overrideKey string
				utilization float64
			}{overrideKey, utilization})
		}
	}

	if len(badNames) == 0 {
		// All names were under the threshold, emit override utilization metrics.
		for _, data := range metricsData {
			ra.rlOverrideUsageGauge.WithLabelValues(ratelimit.CertificatesPerName, data.overrideKey).Set(data.utilization)
		}
	}
	return badNames, response.Earliest.AsTime(), nil
}

func (ra *RegistrationAuthorityImpl) checkCertificatesPerNameLimit(ctx context.Context, names []string, limit ratelimit.RateLimitPolicy, regID int64) error {
	// check if there is already an existing certificate for
	// the exact name set we are issuing for. If so bypass the
	// the certificatesPerName limit.
	exists, err := ra.SA.FQDNSetExists(ctx, &sapb.FQDNSetExistsRequest{Domains: names})
	if err != nil {
		return fmt.Errorf("checking renewal exemption for %q: %s", names, err)
	}
	if exists.Exists {
		return nil
	}

	tldNames := ratelimits.DomainsForRateLimiting(names)
	namesOutOfLimit, earliest, err := ra.enforceNameCounts(ctx, tldNames, limit, regID)
	if err != nil {
		return fmt.Errorf("checking certificates per name limit for %q: %s",
			names, err)
	}

	if len(namesOutOfLimit) > 0 {
		// Determine the amount of time until the earliest event would fall out
		// of the window.
		retryAfter := earliest.Add(limit.Window.Duration).Sub(ra.clk.Now())
		retryString := earliest.Add(limit.Window.Duration).Format(time.RFC3339)

		ra.log.Infof("Rate limit exceeded, CertificatesForDomain, regID: %d, domains: %s", regID, strings.Join(namesOutOfLimit, ", "))
		if len(namesOutOfLimit) > 1 {
			var subErrors []berrors.SubBoulderError
			for _, name := range namesOutOfLimit {
				subErrors = append(subErrors, berrors.SubBoulderError{
					Identifier:   identifier.DNSIdentifier(name),
					BoulderError: berrors.RateLimitError(retryAfter, "too many certificates already issued. Retry after %s", retryString).(*berrors.BoulderError),
				})
			}
			return berrors.RateLimitError(retryAfter, "too many certificates already issued for multiple names (%q and %d others). Retry after %s", namesOutOfLimit[0], len(namesOutOfLimit), retryString).(*berrors.BoulderError).WithSubErrors(subErrors)
		}
		return berrors.RateLimitError(retryAfter, "too many certificates already issued for %q. Retry after %s", namesOutOfLimit[0], retryString)
	}

	return nil
}

func (ra *RegistrationAuthorityImpl) checkCertificatesPerFQDNSetLimit(ctx context.Context, names []string, limit ratelimit.RateLimitPolicy, regID int64) error {
	names = core.UniqueLowerNames(names)
	threshold, overrideKey := limit.GetThreshold(strings.Join(names, ","), regID)
	if threshold <= 0 {
		// No limit configured.
		return nil
	}

	prevIssuances, err := ra.SA.FQDNSetTimestampsForWindow(ctx, &sapb.CountFQDNSetsRequest{
		Domains: names,
		Window:  durationpb.New(limit.Window.Duration),
	})
	if err != nil {
		return fmt.Errorf("checking duplicate certificate limit for %q: %s", names, err)
	}

	if overrideKey != "" {
		utilization := float64(len(prevIssuances.Timestamps)) / float64(threshold)
		ra.rlOverrideUsageGauge.WithLabelValues(ratelimit.CertificatesPerFQDNSet, overrideKey).Set(utilization)
	}

	issuanceCount := int64(len(prevIssuances.Timestamps))
	if issuanceCount < threshold {
		// Issuance in window is below the threshold, no need to limit.
		if overrideKey != "" {
			utilization := float64(issuanceCount+1) / float64(threshold)
			ra.rlOverrideUsageGauge.WithLabelValues(ratelimit.CertificatesPerFQDNSet, overrideKey).Set(utilization)
		}
		return nil
	} else {
		// Evaluate the rate limit using a leaky bucket algorithm. The bucket
		// has a capacity of threshold and is refilled at a rate of 1 token per
		// limit.Window/threshold from the time of each issuance timestamp. The
		// timestamps start from the most recent issuance and go back in time.
		now := ra.clk.Now()
		nsPerToken := limit.Window.Nanoseconds() / threshold
		for i, timestamp := range prevIssuances.Timestamps {
			tokensGeneratedSince := now.Add(-time.Duration(int64(i+1) * nsPerToken))
			if timestamp.AsTime().Before(tokensGeneratedSince) {
				// We know `i+1` tokens were generated since `tokenGeneratedSince`,
				// and only `i` certificates were issued, so there's room to allow
				// for an additional issuance.
				if overrideKey != "" {
					utilization := float64(issuanceCount) / float64(threshold)
					ra.rlOverrideUsageGauge.WithLabelValues(ratelimit.CertificatesPerFQDNSet, overrideKey).Set(utilization)
				}
				return nil
			}
		}
		retryTime := prevIssuances.Timestamps[0].AsTime().Add(time.Duration(nsPerToken))
		retryAfter := retryTime.Sub(now)
		return berrors.DuplicateCertificateError(
			retryAfter,
			"too many certificates (%d) already issued for this exact set of domains in the last %.0f hours: %s, retry after %s",
			threshold, limit.Window.Duration.Hours(), strings.Join(names, ","), retryTime.Format(time.RFC3339),
		)
	}
}

func (ra *RegistrationAuthorityImpl) checkNewOrderLimits(ctx context.Context, names []string, regID int64) error {
	newOrdersPerAccountLimits := ra.rlPolicies.NewOrdersPerAccount()
	if newOrdersPerAccountLimits.Enabled() {
		started := ra.clk.Now()
		err := ra.checkNewOrdersPerAccountLimit(ctx, regID, names, newOrdersPerAccountLimits)
		elapsed := ra.clk.Since(started)
		if err != nil {
			if errors.Is(err, berrors.RateLimit) {
				ra.rlCheckLatency.WithLabelValues(ratelimit.NewOrdersPerAccount, ratelimits.Denied).Observe(elapsed.Seconds())
			}
			return err
		}
		ra.rlCheckLatency.WithLabelValues(ratelimit.NewOrdersPerAccount, ratelimits.Allowed).Observe(elapsed.Seconds())
	}

	certNameLimits := ra.rlPolicies.CertificatesPerName()
	if certNameLimits.Enabled() {
		started := ra.clk.Now()
		err := ra.checkCertificatesPerNameLimit(ctx, names, certNameLimits, regID)
		elapsed := ra.clk.Since(started)
		if err != nil {
			if errors.Is(err, berrors.RateLimit) {
				ra.rlCheckLatency.WithLabelValues(ratelimit.CertificatesPerName, ratelimits.Denied).Observe(elapsed.Seconds())
			}
			return err
		}
		ra.rlCheckLatency.WithLabelValues(ratelimit.CertificatesPerName, ratelimits.Allowed).Observe(elapsed.Seconds())
	}

	fqdnLimitsFast := ra.rlPolicies.CertificatesPerFQDNSetFast()
	if fqdnLimitsFast.Enabled() {
		started := ra.clk.Now()
		err := ra.checkCertificatesPerFQDNSetLimit(ctx, names, fqdnLimitsFast, regID)
		elapsed := ra.clk.Since(started)
		if err != nil {
			if errors.Is(err, berrors.RateLimit) {
				ra.rlCheckLatency.WithLabelValues(ratelimit.CertificatesPerFQDNSetFast, ratelimits.Denied).Observe(elapsed.Seconds())
			}
			return err
		}
		ra.rlCheckLatency.WithLabelValues(ratelimit.CertificatesPerFQDNSetFast, ratelimits.Allowed).Observe(elapsed.Seconds())
	}

	fqdnLimits := ra.rlPolicies.CertificatesPerFQDNSet()
	if fqdnLimits.Enabled() {
		started := ra.clk.Now()
		err := ra.checkCertificatesPerFQDNSetLimit(ctx, names, fqdnLimits, regID)
		elapsed := ra.clk.Since(started)
		if err != nil {
			if errors.Is(err, berrors.RateLimit) {
				ra.rlCheckLatency.WithLabelValues(ratelimit.CertificatesPerFQDNSet, ratelimits.Denied).Observe(elapsed.Seconds())
			}
			return err
		}
		ra.rlCheckLatency.WithLabelValues(ratelimit.CertificatesPerFQDNSet, ratelimits.Allowed).Observe(elapsed.Seconds())
	}

	invalidAuthzPerAccountLimits := ra.rlPolicies.InvalidAuthorizationsPerAccount()
	if invalidAuthzPerAccountLimits.Enabled() {
		started := ra.clk.Now()
		err := ra.checkInvalidAuthorizationLimits(ctx, regID, names, invalidAuthzPerAccountLimits)
		elapsed := ra.clk.Since(started)
		if err != nil {
			if errors.Is(err, berrors.RateLimit) {
				ra.rlCheckLatency.WithLabelValues(ratelimit.InvalidAuthorizationsPerAccount, ratelimits.Denied).Observe(elapsed.Seconds())
			}
			return err
		}
		ra.rlCheckLatency.WithLabelValues(ratelimit.InvalidAuthorizationsPerAccount, ratelimits.Allowed).Observe(elapsed.Seconds())
	}

	return nil
}

// UpdateRegistration updates an existing Registration with new values. Caller
// is responsible for making sure that update.Key is only different from base.Key
// if it is being called from the WFE key change endpoint.
// TODO(#5554): Split this into separate methods for updating Contacts vs Key.
func (ra *RegistrationAuthorityImpl) UpdateRegistration(ctx context.Context, req *rapb.UpdateRegistrationRequest) (*corepb.Registration, error) {
	// Error if the request is nil, there is no account key or IP address
	if req.Base == nil || len(req.Base.Key) == 0 || len(req.Base.InitialIP) == 0 || req.Base.Id == 0 {
		return nil, errIncompleteGRPCRequest
	}

	err := validateContactsPresent(req.Base.Contact, req.Base.ContactsPresent)
	if err != nil {
		return nil, err
	}
	err = validateContactsPresent(req.Update.Contact, req.Update.ContactsPresent)
	if err != nil {
		return nil, err
	}
	err = ra.validateContacts(req.Update.Contact)
	if err != nil {
		return nil, err
	}

	update, changed := mergeUpdate(req.Base, req.Update)
	if !changed {
		// If merging the update didn't actually change the base then our work is
		// done, we can return before calling ra.SA.UpdateRegistration since there's
		// nothing for the SA to do
		return req.Base, nil
	}

	_, err = ra.SA.UpdateRegistration(ctx, update)
	if err != nil {
		// berrors.InternalServerError since the user-data was validated before being
		// passed to the SA.
		err = berrors.InternalServerError("Could not update registration: %s", err)
		return nil, err
	}

	return update, nil
}

func contactsEqual(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	// If there is an existing contact slice and it has the same length as the
	// new contact slice we need to look at each contact to determine if there
	// is a change being made. Use `sort.Strings` here to ensure a consistent
	// comparison
	sort.Strings(a)
	sort.Strings(b)
	for i := range len(b) {
		// If the contact's string representation differs at any index they aren't
		// equal
		if a[i] != b[i] {
			return false
		}
	}

	// They are equal!
	return true
}

// MergeUpdate returns a new corepb.Registration with the majority of its fields
// copies from the base Registration, and a subset (Contact, Agreement, and Key)
// copied from the update Registration. It also returns a boolean indicating
// whether or not this operation resulted in a Registration which differs from
// the base.
func mergeUpdate(base *corepb.Registration, update *corepb.Registration) (*corepb.Registration, bool) {
	var changed bool

	// Start by copying all of the fields.
	res := &corepb.Registration{
		Id:              base.Id,
		Key:             base.Key,
		Contact:         base.Contact,
		ContactsPresent: base.ContactsPresent,
		Agreement:       base.Agreement,
		InitialIP:       base.InitialIP,
		CreatedAt:       base.CreatedAt,
		Status:          base.Status,
	}

	// Note: we allow update.Contact to overwrite base.Contact even if the former
	// is empty in order to allow users to remove the contact associated with
	// a registration. If the update has ContactsPresent set to false, then we
	// know it is not attempting to update the contacts field.
	if update.ContactsPresent && !contactsEqual(base.Contact, update.Contact) {
		res.Contact = update.Contact
		res.ContactsPresent = update.ContactsPresent
		changed = true
	}

	if len(update.Agreement) > 0 && update.Agreement != base.Agreement {
		res.Agreement = update.Agreement
		changed = true
	}

	if len(update.Key) > 0 {
		if len(update.Key) != len(base.Key) {
			res.Key = update.Key
			changed = true
		} else {
			for i := range len(base.Key) {
				if update.Key[i] != base.Key[i] {
					res.Key = update.Key
					changed = true
					break
				}
			}
		}
	}

	return res, changed
}

// recordValidation records an authorization validation event,
// it should only be used on v2 style authorizations.
func (ra *RegistrationAuthorityImpl) recordValidation(ctx context.Context, authID string, authExpires *time.Time, challenge *core.Challenge) error {
	authzID, err := strconv.ParseInt(authID, 10, 64)
	if err != nil {
		return err
	}
	var expires time.Time
	if challenge.Status == core.StatusInvalid {
		expires = *authExpires
	} else {
		expires = ra.clk.Now().Add(ra.authorizationLifetime)
	}
	vr, err := bgrpc.ValidationResultToPB(challenge.ValidationRecord, challenge.Error)
	if err != nil {
		return err
	}
	var validated *timestamppb.Timestamp
	if challenge.Validated != nil {
		validated = timestamppb.New(*challenge.Validated)
	}
	_, err = ra.SA.FinalizeAuthorization2(ctx, &sapb.FinalizeAuthorizationRequest{
		Id:                authzID,
		Status:            string(challenge.Status),
		Expires:           timestamppb.New(expires),
		Attempted:         string(challenge.Type),
		AttemptedAt:       validated,
		ValidationRecords: vr.Records,
		ValidationError:   vr.Problems,
	})
	return err
}

func (ra *RegistrationAuthorityImpl) countFailedValidation(ctx context.Context, regId int64, name string) {
	if ra.limiter == nil || ra.txnBuilder == nil {
		// Limiter is disabled.
		return
	}

	txn, err := ra.txnBuilder.FailedAuthorizationsPerDomainPerAccountSpendOnlyTransaction(regId, name)
	if err != nil {
		ra.log.Errf("constructing rate limit transaction for the %s rate limit: %s", ratelimits.FailedAuthorizationsPerDomainPerAccount, err)
	}

	_, err = ra.limiter.Spend(ctx, txn)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		ra.log.Errf("checking the %s rate limit: %s", ratelimits.FailedAuthorizationsPerDomainPerAccount, err)
	}
}

// PerformValidation initiates validation for a specific challenge associated
// with the given base authorization. The authorization and challenge are
// updated based on the results.
func (ra *RegistrationAuthorityImpl) PerformValidation(
	ctx context.Context,
	req *rapb.PerformValidationRequest) (*corepb.Authorization, error) {

	// Clock for start of PerformValidation.
	vStart := ra.clk.Now()

	// TODO(#7153): Check each value via core.IsAnyNilOrZero
	if req.Authz == nil || req.Authz.Id == "" || req.Authz.Identifier == "" || req.Authz.Status == "" || core.IsAnyNilOrZero(req.Authz.Expires) {
		return nil, errIncompleteGRPCRequest
	}

	authz, err := bgrpc.PBToAuthz(req.Authz)
	if err != nil {
		return nil, err
	}

	// Refuse to update expired authorizations
	if authz.Expires == nil || authz.Expires.Before(ra.clk.Now()) {
		return nil, berrors.MalformedError("expired authorization")
	}

	challIndex := int(req.ChallengeIndex)
	if challIndex >= len(authz.Challenges) {
		return nil,
			berrors.MalformedError("invalid challenge index '%d'", challIndex)
	}

	ch := &authz.Challenges[challIndex]

	// This challenge type may have been disabled since the challenge was created.
	if !ra.PA.ChallengeTypeEnabled(ch.Type) {
		return nil, berrors.MalformedError("challenge type %q no longer allowed", ch.Type)
	}

	// We expect some clients to try and update a challenge for an authorization
	// that is already valid. In this case we don't need to process the
	// challenge update. It wouldn't be helpful, the overall authorization is
	// already good! We return early for the valid authz reuse case.
	if authz.Status == core.StatusValid {
		return req.Authz, nil
	}

	if authz.Status != core.StatusPending {
		return nil, berrors.MalformedError("authorization must be pending")
	}

	// Look up the account key for this authorization
	regPB, err := ra.SA.GetRegistration(ctx, &sapb.RegistrationID{Id: authz.RegistrationID})
	if err != nil {
		return nil, berrors.InternalServerError(err.Error())
	}
	reg, err := bgrpc.PbToRegistration(regPB)
	if err != nil {
		return nil, berrors.InternalServerError(err.Error())
	}

	// Compute the key authorization field based on the registration key
	expectedKeyAuthorization, err := ch.ExpectedKeyAuthorization(reg.Key)
	if err != nil {
		return nil, berrors.InternalServerError("could not compute expected key authorization value")
	}

	ch.ProvidedKeyAuthorization = expectedKeyAuthorization

	// Double check before sending to VA
	if cErr := ch.CheckPending(); cErr != nil {
		return nil, berrors.MalformedError(cErr.Error())
	}

	// Dispatch to the VA for service
	vaCtx := context.Background()
	go func(authz core.Authorization) {
		// We will mutate challenges later in this goroutine to change status and
		// add error, but we also return a copy of authz immediately. To avoid a
		// data race, make a copy of the challenges slice here for mutation.
		challenges := make([]core.Challenge, len(authz.Challenges))
		copy(challenges, authz.Challenges)
		authz.Challenges = challenges
		chall, _ := bgrpc.ChallengeToPB(authz.Challenges[challIndex])
		req := vapb.PerformValidationRequest{
			Domain:    authz.Identifier.Value,
			Challenge: chall,
			Authz: &vapb.AuthzMeta{
				Id:    authz.ID,
				RegID: authz.RegistrationID,
			},
			ExpectedKeyAuthorization: expectedKeyAuthorization,
		}
		res, err := ra.VA.PerformValidation(vaCtx, &req)
		challenge := &authz.Challenges[challIndex]
		var prob *probs.ProblemDetails
		if err != nil {
			prob = probs.ServerInternal("Could not communicate with VA")
			ra.log.AuditErrf("Could not communicate with VA: %s", err)
		} else {
			if res.Problems != nil {
				prob, err = bgrpc.PBToProblemDetails(res.Problems)
				if err != nil {
					prob = probs.ServerInternal("Could not communicate with VA")
					ra.log.AuditErrf("Could not communicate with VA: %s", err)
				}
			}
			// Save the updated records
			records := make([]core.ValidationRecord, len(res.Records))
			for i, r := range res.Records {
				records[i], err = bgrpc.PBToValidationRecord(r)
				if err != nil {
					prob = probs.ServerInternal("Records for validation corrupt")
				}
			}
			challenge.ValidationRecord = records
		}
		if !challenge.RecordsSane() && prob == nil {
			prob = probs.ServerInternal("Records for validation failed sanity check")
		}

		if prob != nil {
			challenge.Status = core.StatusInvalid
			challenge.Error = prob

			// TODO(#5545): Spending can be async until key-value rate limits
			// are authoritative. This saves us from adding latency to each
			// request. Goroutines spun out below will respect a context
			// deadline set by the ratelimits package and cannot be prematurely
			// canceled by the requester.
			go ra.countFailedValidation(vaCtx, authz.RegistrationID, authz.Identifier.Value)
		} else {
			challenge.Status = core.StatusValid
		}
		challenge.Validated = &vStart
		authz.Challenges[challIndex] = *challenge

		err = ra.recordValidation(vaCtx, authz.ID, authz.Expires, challenge)
		if err != nil {
			if errors.Is(err, berrors.AlreadyRevoked) {
				ra.log.Infof("Didn't record already-finalized validation: regID=[%d] authzID=[%s] err=[%s]",
					authz.RegistrationID, authz.ID, err)
			} else {
				ra.log.AuditErrf("Failed to record validation: regID=[%d] authzID=[%s] err=[%s]",
					authz.RegistrationID, authz.ID, err)
			}
		}
	}(authz)
	return bgrpc.AuthzToPB(authz)
}

// revokeCertificate updates the database to mark the certificate as revoked,
// with the given reason and current timestamp.
func (ra *RegistrationAuthorityImpl) revokeCertificate(ctx context.Context, serial *big.Int, issuerID issuance.NameID, reason revocation.Reason) error {
	serialString := core.SerialToString(serial)

	_, err := ra.SA.RevokeCertificate(ctx, &sapb.RevokeCertificateRequest{
		Serial:   serialString,
		Reason:   int64(reason),
		Date:     timestamppb.New(ra.clk.Now()),
		IssuerID: int64(issuerID),
	})
	if err != nil {
		return err
	}

	ra.revocationReasonCounter.WithLabelValues(revocation.ReasonToString[reason]).Inc()
	return nil
}

// updateRevocationForKeyCompromise updates the database to mark the certificate
// as revoked, with the given reason and current timestamp. This only works for
// certificates that were previously revoked for a reason other than
// keyCompromise, and which are now being updated to keyCompromise instead.
func (ra *RegistrationAuthorityImpl) updateRevocationForKeyCompromise(ctx context.Context, serial *big.Int, issuerID issuance.NameID) error {
	serialString := core.SerialToString(serial)

	status, err := ra.SA.GetCertificateStatus(ctx, &sapb.Serial{Serial: serialString})
	if err != nil {
		return berrors.NotFoundError("unable to confirm that serial %q was ever issued: %s", serialString, err)
	}

	if status.Status != string(core.OCSPStatusRevoked) {
		// Internal server error, because we shouldn't be in the function at all
		// unless the cert was already revoked.
		return fmt.Errorf("unable to re-revoke serial %q which is not currently revoked", serialString)
	}
	if status.RevokedReason == ocsp.KeyCompromise {
		return berrors.AlreadyRevokedError("unable to re-revoke serial %q which is already revoked for keyCompromise", serialString)
	}

	_, err = ra.SA.UpdateRevokedCertificate(ctx, &sapb.RevokeCertificateRequest{
		Serial:   serialString,
		Reason:   int64(ocsp.KeyCompromise),
		Date:     timestamppb.New(ra.clk.Now()),
		Backdate: status.RevokedDate,
		IssuerID: int64(issuerID),
	})
	if err != nil {
		return err
	}

	ra.revocationReasonCounter.WithLabelValues(revocation.ReasonToString[ocsp.KeyCompromise]).Inc()
	return nil
}

// purgeOCSPCache makes a request to akamai-purger to purge the cache entries
// for the given certificate.
func (ra *RegistrationAuthorityImpl) purgeOCSPCache(ctx context.Context, cert *x509.Certificate, issuerID issuance.NameID) error {
	issuer, ok := ra.issuersByNameID[issuerID]
	if !ok {
		return fmt.Errorf("unable to identify issuer of cert with serial %q", core.SerialToString(cert.SerialNumber))
	}

	purgeURLs, err := akamai.GeneratePurgeURLs(cert, issuer.Certificate)
	if err != nil {
		return err
	}

	_, err = ra.purger.Purge(ctx, &akamaipb.PurgeRequest{Urls: purgeURLs})
	if err != nil {
		return err
	}

	return nil
}

// RevokeCertByApplicant revokes the certificate in question. It allows any
// revocation reason from (0, 1, 3, 4, 5, 9), because Subscribers are allowed to
// request any revocation reason for their own certificates. However, if the
// requesting RegID is an account which has authorizations for all names in the
// cert but is *not* the original subscriber, it overrides the revocation reason
// to be 5 (cessationOfOperation), because that code is used to cover instances
// where "the certificate subscriber no longer owns the domain names in the
// certificate". It does not add the key to the blocked keys list, even if
// reason 1 (keyCompromise) is requested, as it does not demonstrate said
// compromise. It attempts to purge the certificate from the Akamai cache, but
// it does not hard-fail if doing so is not successful, because the cache will
// drop the old OCSP response in less than 24 hours anyway.
func (ra *RegistrationAuthorityImpl) RevokeCertByApplicant(ctx context.Context, req *rapb.RevokeCertByApplicantRequest) (*emptypb.Empty, error) {
	if req == nil || req.Cert == nil || req.RegID == 0 {
		return nil, errIncompleteGRPCRequest
	}

	if _, present := revocation.UserAllowedReasons[revocation.Reason(req.Code)]; !present {
		return nil, berrors.BadRevocationReasonError(req.Code)
	}

	cert, err := x509.ParseCertificate(req.Cert)
	if err != nil {
		return nil, err
	}

	serialString := core.SerialToString(cert.SerialNumber)

	logEvent := certificateRevocationEvent{
		ID:           core.NewToken(),
		SerialNumber: serialString,
		Reason:       req.Code,
		Method:       "applicant",
		RequesterID:  req.RegID,
	}

	// Below this point, do not re-declare `err` (i.e. type `err :=`) in a
	// nested scope. Doing so will create a new `err` variable that is not
	// captured by this closure.
	defer func() {
		if err != nil {
			logEvent.Error = err.Error()
		}
		ra.log.AuditObject("Revocation request:", logEvent)
	}()

	metadata, err := ra.SA.GetSerialMetadata(ctx, &sapb.Serial{Serial: serialString})
	if err != nil {
		return nil, err
	}

	if req.RegID == metadata.RegistrationID {
		// The requester is the original subscriber. They can revoke for any reason.
		logEvent.Method = "subscriber"
	} else {
		// The requester is a different account. We need to confirm that they have
		// authorizations for all names in the cert.
		logEvent.Method = "control"

		var authzMapPB *sapb.Authorizations
		authzMapPB, err = ra.SA.GetValidAuthorizations2(ctx, &sapb.GetValidAuthorizationsRequest{
			RegistrationID: req.RegID,
			Domains:        cert.DNSNames,
			Now:            timestamppb.New(ra.clk.Now()),
		})
		if err != nil {
			return nil, err
		}

		m := make(map[string]struct{})
		for _, authz := range authzMapPB.Authz {
			m[authz.Domain] = struct{}{}
		}
		for _, name := range cert.DNSNames {
			if _, present := m[name]; !present {
				return nil, berrors.UnauthorizedError("requester does not control all names in cert with serial %q", serialString)
			}
		}

		// Applicants who are not the original Subscriber are not allowed to
		// revoke for any reason other than cessationOfOperation, which covers
		// circumstances where "the certificate subscriber no longer owns the
		// domain names in the certificate". Override the reason code to match.
		req.Code = ocsp.CessationOfOperation
		logEvent.Reason = req.Code
	}

	issuerID := issuance.IssuerNameID(cert)
	err = ra.revokeCertificate(
		ctx,
		cert.SerialNumber,
		issuerID,
		revocation.Reason(req.Code),
	)
	if err != nil {
		return nil, err
	}

	// Don't propagate purger errors to the client.
	_ = ra.purgeOCSPCache(ctx, cert, issuerID)

	return &emptypb.Empty{}, nil
}

// addToBlockedKeys initiates a GRPC call to have the Base64-encoded SHA256
// digest of a provided public key added to the blockedKeys table.
func (ra *RegistrationAuthorityImpl) addToBlockedKeys(ctx context.Context, key crypto.PublicKey, src string, comment string) error {
	var digest core.Sha256Digest
	digest, err := core.KeyDigest(key)
	if err != nil {
		return err
	}

	// Add the public key to the blocked keys list.
	_, err = ra.SA.AddBlockedKey(ctx, &sapb.AddBlockedKeyRequest{
		KeyHash: digest[:],
		Added:   timestamppb.New(ra.clk.Now()),
		Source:  src,
		Comment: comment,
	})
	if err != nil {
		return err
	}

	return nil
}

// RevokeCertByKey revokes the certificate in question. It always uses
// reason code 1 (keyCompromise). It ensures that they public key is added to
// the blocked keys list, even if revocation otherwise fails. It attempts to
// purge the certificate from the Akamai cache, but it does not hard-fail if
// doing so is not successful, because the cache will drop the old OCSP response
// in less than 24 hours anyway.
func (ra *RegistrationAuthorityImpl) RevokeCertByKey(ctx context.Context, req *rapb.RevokeCertByKeyRequest) (*emptypb.Empty, error) {
	if req == nil || req.Cert == nil {
		return nil, errIncompleteGRPCRequest
	}

	cert, err := x509.ParseCertificate(req.Cert)
	if err != nil {
		return nil, err
	}

	issuerID := issuance.IssuerNameID(cert)

	logEvent := certificateRevocationEvent{
		ID:           core.NewToken(),
		SerialNumber: core.SerialToString(cert.SerialNumber),
		Reason:       ocsp.KeyCompromise,
		Method:       "key",
		RequesterID:  0,
	}

	// Below this point, do not re-declare `err` (i.e. type `err :=`) in a
	// nested scope. Doing so will create a new `err` variable that is not
	// captured by this closure.
	defer func() {
		if err != nil {
			logEvent.Error = err.Error()
		}
		ra.log.AuditObject("Revocation request:", logEvent)
	}()

	// We revoke the cert before adding it to the blocked keys list, to avoid a
	// race between this and the bad-key-revoker. But we don't check the error
	// from this operation until after we add the key to the blocked keys list,
	// since that addition needs to happen no matter what.
	revokeErr := ra.revokeCertificate(
		ctx,
		cert.SerialNumber,
		issuerID,
		revocation.Reason(ocsp.KeyCompromise),
	)

	// Failing to add the key to the blocked keys list is a worse failure than
	// failing to revoke in the first place, because it means that
	// bad-key-revoker won't revoke the cert anyway.
	err = ra.addToBlockedKeys(ctx, cert.PublicKey, "API", "")
	if err != nil {
		return nil, err
	}

	// Check the error returned from revokeCertificate itself.
	err = revokeErr
	if err == nil {
		// If the revocation and blocked keys list addition were successful, then
		// just purge and return.
		// Don't propagate purger errors to the client.
		_ = ra.purgeOCSPCache(ctx, cert, issuerID)
		return &emptypb.Empty{}, nil
	} else if errors.Is(err, berrors.AlreadyRevoked) {
		// If it was an AlreadyRevoked error, try to re-revoke the cert in case
		// it was revoked for a reason other than keyCompromise.
		err = ra.updateRevocationForKeyCompromise(ctx, cert.SerialNumber, issuerID)

		// Perform an Akamai cache purge to handle occurrences of a client
		// previously successfully revoking a certificate, but the cache purge had
		// unexpectedly failed. Allows clients to re-attempt revocation and purge the
		// Akamai cache.
		_ = ra.purgeOCSPCache(ctx, cert, issuerID)
		if err != nil {
			return nil, err
		}
		return &emptypb.Empty{}, nil
	} else {
		// Error out if the error was anything other than AlreadyRevoked.
		return nil, err
	}
}

// AdministrativelyRevokeCertificate terminates trust in the certificate
// provided and does not require the registration ID of the requester since this
// method is only called from the admin-revoker tool. It trusts that the admin
// is doing the right thing, so if the requested reason is keyCompromise, it
// blocks the key from future issuance even though compromise has not been
// demonstrated here. It purges the certificate from the Akamai cache, and
// returns an error if that purge fails, since this method may be called late
// in the BRs-mandated revocation timeframe.
func (ra *RegistrationAuthorityImpl) AdministrativelyRevokeCertificate(ctx context.Context, req *rapb.AdministrativelyRevokeCertificateRequest) (*emptypb.Empty, error) {
	if req == nil || req.AdminName == "" {
		return nil, errIncompleteGRPCRequest
	}
	if req.Serial == "" {
		return nil, errIncompleteGRPCRequest
	}

	reasonCode := revocation.Reason(req.Code)
	if _, present := revocation.AdminAllowedReasons[reasonCode]; !present {
		return nil, fmt.Errorf("cannot revoke for reason %d", reasonCode)
	}
	if req.SkipBlockKey && reasonCode != ocsp.KeyCompromise {
		return nil, fmt.Errorf("cannot skip key blocking for reasons other than KeyCompromise")
	}
	if reasonCode == ocsp.KeyCompromise && req.Malformed {
		return nil, fmt.Errorf("cannot revoke malformed certificate for KeyCompromise")
	}

	logEvent := certificateRevocationEvent{
		ID:           core.NewToken(),
		SerialNumber: req.Serial,
		Reason:       req.Code,
		Method:       "admin",
		AdminName:    req.AdminName,
	}

	// Below this point, do not re-declare `err` (i.e. type `err :=`) in a
	// nested scope. Doing so will create a new `err` variable that is not
	// captured by this closure.
	var err error
	defer func() {
		if err != nil {
			logEvent.Error = err.Error()
		}
		ra.log.AuditObject("Revocation request:", logEvent)
	}()

	var cert *x509.Certificate
	var issuerID issuance.NameID
	if req.Cert != nil {
		// If the incoming request includes a certificate body, just use that and
		// avoid doing any database queries. This code path is deprecated and will
		// be removed when req.Cert is removed.
		cert, err = x509.ParseCertificate(req.Cert)
		if err != nil {
			return nil, err
		}
		issuerID = issuance.IssuerNameID(cert)
	} else if !req.Malformed {
		// As long as we don't believe the cert will be malformed, we should
		// get the precertificate so we can block its pubkey if necessary and purge
		// the akamai OCSP cache.
		var certPB *corepb.Certificate
		certPB, err = ra.SA.GetLintPrecertificate(ctx, &sapb.Serial{Serial: req.Serial})
		if err != nil {
			return nil, err
		}
		// Note that, although the thing we're parsing here is actually a linting
		// precertificate, it has identical issuer info (and therefore an identical
		// issuer NameID) to the real thing.
		cert, err = x509.ParseCertificate(certPB.Der)
		if err != nil {
			return nil, err
		}
		issuerID = issuance.IssuerNameID(cert)
	} else {
		// But if the cert is malformed, we at least still need its IssuerID.
		var status *corepb.CertificateStatus
		status, err = ra.SA.GetCertificateStatus(ctx, &sapb.Serial{Serial: req.Serial})
		if err != nil {
			return nil, fmt.Errorf("unable to confirm that serial %q was ever issued: %w", req.Serial, err)
		}
		issuerID = issuance.NameID(status.IssuerID)
	}

	var serialInt *big.Int
	serialInt, err = core.StringToSerial(req.Serial)
	if err != nil {
		return nil, err
	}

	err = ra.revokeCertificate(ctx, serialInt, issuerID, revocation.Reason(req.Code))
	// Perform an Akamai cache purge to handle occurrences of a client
	// successfully revoking a certificate, but the initial cache purge failing.
	if errors.Is(err, berrors.AlreadyRevoked) {
		if cert != nil {
			err = ra.purgeOCSPCache(ctx, cert, issuerID)
			if err != nil {
				err = fmt.Errorf("OCSP cache purge for already revoked serial %v failed: %w", serialInt, err)
				return nil, err
			}
		}
	}
	if err != nil {
		if req.Code == ocsp.KeyCompromise && errors.Is(err, berrors.AlreadyRevoked) {
			err = ra.updateRevocationForKeyCompromise(ctx, serialInt, issuerID)
			if err != nil {
				return nil, err
			}
		}
		return nil, err
	}

	if req.Code == ocsp.KeyCompromise && !req.SkipBlockKey {
		if cert == nil {
			return nil, errors.New("revoking for key compromise requires providing the certificate's DER")
		}
		err = ra.addToBlockedKeys(ctx, cert.PublicKey, "admin-revoker", fmt.Sprintf("revoked by %s", req.AdminName))
		if err != nil {
			return nil, err
		}
	}

	if cert != nil {
		err = ra.purgeOCSPCache(ctx, cert, issuerID)
		if err != nil {
			err = fmt.Errorf("OCSP cache purge for serial %v failed: %w", serialInt, err)
			return nil, err
		}
	}

	return &emptypb.Empty{}, nil
}

// DeactivateRegistration deactivates a valid registration
func (ra *RegistrationAuthorityImpl) DeactivateRegistration(ctx context.Context, reg *corepb.Registration) (*emptypb.Empty, error) {
	if reg == nil || reg.Id == 0 {
		return nil, errIncompleteGRPCRequest
	}
	if reg.Status != string(core.StatusValid) {
		return nil, berrors.MalformedError("only valid registrations can be deactivated")
	}
	_, err := ra.SA.DeactivateRegistration(ctx, &sapb.RegistrationID{Id: reg.Id})
	if err != nil {
		return nil, berrors.InternalServerError(err.Error())
	}
	return &emptypb.Empty{}, nil
}

// DeactivateAuthorization deactivates a currently valid authorization
func (ra *RegistrationAuthorityImpl) DeactivateAuthorization(ctx context.Context, req *corepb.Authorization) (*emptypb.Empty, error) {
	if req == nil || req.Id == "" || req.Status == "" {
		return nil, errIncompleteGRPCRequest
	}
	authzID, err := strconv.ParseInt(req.Id, 10, 64)
	if err != nil {
		return nil, err
	}
	if _, err := ra.SA.DeactivateAuthorization2(ctx, &sapb.AuthorizationID2{Id: authzID}); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// GenerateOCSP looks up a certificate's status, then requests a signed OCSP
// response for it from the CA. If the certificate status is not available
// or the certificate is expired, it returns berrors.NotFoundError.
func (ra *RegistrationAuthorityImpl) GenerateOCSP(ctx context.Context, req *rapb.GenerateOCSPRequest) (*capb.OCSPResponse, error) {
	status, err := ra.SA.GetCertificateStatus(ctx, &sapb.Serial{Serial: req.Serial})
	if errors.Is(err, berrors.NotFound) {
		_, err := ra.SA.GetSerialMetadata(ctx, &sapb.Serial{Serial: req.Serial})
		if errors.Is(err, berrors.NotFound) {
			return nil, berrors.UnknownSerialError()
		} else {
			return nil, berrors.NotFoundError("certificate not found")
		}
	} else if err != nil {
		return nil, err
	}

	// If we get an OCSP query for a certificate where the status is still
	// OCSPStatusNotReady, that means an error occurred, not here but at issuance
	// time. Specifically, we succeeded in storing the linting certificate (and
	// corresponding certificateStatus row), but failed before calling
	// SetCertificateStatusReady. We expect this to be rare, and we expect such
	// certificates not to get OCSP queries, so InternalServerError is appropriate.
	if status.Status == string(core.OCSPStatusNotReady) {
		return nil, errors.New("serial belongs to a certificate that errored during issuance")
	}

	if ra.clk.Now().After(status.NotAfter.AsTime()) {
		return nil, berrors.NotFoundError("certificate is expired")
	}

	return ra.OCSP.GenerateOCSP(ctx, &capb.GenerateOCSPRequest{
		Serial:    req.Serial,
		Status:    status.Status,
		Reason:    int32(status.RevokedReason),
		RevokedAt: status.RevokedDate,
		IssuerID:  status.IssuerID,
	})
}

// NewOrder creates a new order object
func (ra *RegistrationAuthorityImpl) NewOrder(ctx context.Context, req *rapb.NewOrderRequest) (*corepb.Order, error) {
	if req == nil || req.RegistrationID == 0 {
		return nil, errIncompleteGRPCRequest
	}

	newOrder := &sapb.NewOrderRequest{
		RegistrationID: req.RegistrationID,
		Names:          core.UniqueLowerNames(req.Names),
		ReplacesSerial: req.ReplacesSerial,
	}

	if len(newOrder.Names) > ra.maxNames {
		return nil, berrors.MalformedError(
			"Order cannot contain more than %d DNS names", ra.maxNames)
	}

	// Validate that our policy allows issuing for each of the names in the order
	err := ra.PA.WillingToIssue(newOrder.Names)
	if err != nil {
		return nil, err
	}

	err = wildcardOverlap(newOrder.Names)
	if err != nil {
		return nil, err
	}

	// See if there is an existing unexpired pending (or ready) order that can be reused
	// for this account
	existingOrder, err := ra.SA.GetOrderForNames(ctx, &sapb.GetOrderForNamesRequest{
		AcctID: newOrder.RegistrationID,
		Names:  newOrder.Names,
	})
	// If there was an error and it wasn't an acceptable "NotFound" error, return
	// immediately
	if err != nil && !errors.Is(err, berrors.NotFound) {
		return nil, err
	}

	// If there was an order, make sure it has expected fields and return it
	// Error if an incomplete order is returned.
	if existingOrder != nil {
		// Check to see if the expected fields of the existing order are set.
		// TODO(#7153): Check each value via core.IsAnyNilOrZero
		if existingOrder.Id == 0 || existingOrder.Status == "" || existingOrder.RegistrationID == 0 || len(existingOrder.Names) == 0 || core.IsAnyNilOrZero(existingOrder.Created, existingOrder.Expires) {
			return nil, errIncompleteGRPCResponse
		}
		// Track how often we reuse an existing order and how old that order is.
		ra.orderAges.WithLabelValues("NewOrder").Observe(ra.clk.Since(existingOrder.Created.AsTime()).Seconds())
		return existingOrder, nil
	}

	// Renewal orders, indicated by ARI, are exempt from NewOrder rate limits.
	if !req.LimitsExempt {

		// Check if there is rate limit space for issuing a certificate.
		err = ra.checkNewOrderLimits(ctx, newOrder.Names, newOrder.RegistrationID)
		if err != nil {
			return nil, err
		}
	}

	// An order's lifetime is effectively bound by the shortest remaining lifetime
	// of its associated authorizations. For that reason it would be Uncool if
	// `sa.GetAuthorizations` returned an authorization that was very close to
	// expiry. The resulting pending order that references it would itself end up
	// expiring very soon.
	// To prevent this we only return authorizations that are at least 1 day away
	// from expiring.
	authzExpiryCutoff := ra.clk.Now().AddDate(0, 0, 1)

	getAuthReq := &sapb.GetAuthorizationsRequest{
		RegistrationID: newOrder.RegistrationID,
		Now:            timestamppb.New(authzExpiryCutoff),
		Domains:        newOrder.Names,
	}
	existingAuthz, err := ra.SA.GetAuthorizations2(ctx, getAuthReq)
	if err != nil {
		return nil, err
	}

	// Collect up the authorizations we found into a map keyed by the domains the
	// authorizations correspond to
	nameToExistingAuthz := make(map[string]*corepb.Authorization, len(newOrder.Names))
	for _, v := range existingAuthz.Authz {
		nameToExistingAuthz[v.Domain] = v.Authz
	}

	// For each of the names in the order, if there is an acceptable
	// existing authz, append it to the order to reuse it. Otherwise track
	// that there is a missing authz for that name.
	var missingAuthzNames []string
	for _, name := range newOrder.Names {
		// If there isn't an existing authz, note that its missing and continue
		if _, exists := nameToExistingAuthz[name]; !exists {
			missingAuthzNames = append(missingAuthzNames, name)
			continue
		}
		authz := nameToExistingAuthz[name]
		authzAge := (ra.authorizationLifetime - authz.Expires.AsTime().Sub(ra.clk.Now())).Seconds()
		// If the identifier is a wildcard and the existing authz only has one
		// DNS-01 type challenge we can reuse it. In theory we will
		// never get back an authorization for a domain with a wildcard prefix
		// that doesn't meet this criteria from SA.GetAuthorizations but we verify
		// again to be safe.
		if strings.HasPrefix(name, "*.") &&
			len(authz.Challenges) == 1 && core.AcmeChallenge(authz.Challenges[0].Type) == core.ChallengeTypeDNS01 {
			authzID, err := strconv.ParseInt(authz.Id, 10, 64)
			if err != nil {
				return nil, err
			}
			newOrder.V2Authorizations = append(newOrder.V2Authorizations, authzID)
			ra.authzAges.WithLabelValues("NewOrder", authz.Status).Observe(authzAge)
			continue
		} else if !strings.HasPrefix(name, "*.") {
			// If the identifier isn't a wildcard, we can reuse any authz
			authzID, err := strconv.ParseInt(authz.Id, 10, 64)
			if err != nil {
				return nil, err
			}
			newOrder.V2Authorizations = append(newOrder.V2Authorizations, authzID)
			ra.authzAges.WithLabelValues("NewOrder", authz.Status).Observe(authzAge)
			continue
		}

		// Delete the authz from the nameToExistingAuthz map since we are not reusing it.
		delete(nameToExistingAuthz, name)
		// If we reached this point then the existing authz was not acceptable for
		// reuse and we need to mark the name as requiring a new pending authz
		missingAuthzNames = append(missingAuthzNames, name)
	}

	// Renewal orders, indicated by ARI, are exempt from NewOrder rate limits.
	if len(missingAuthzNames) > 0 && !req.LimitsExempt {
		pendingAuthzLimits := ra.rlPolicies.PendingAuthorizationsPerAccount()
		if pendingAuthzLimits.Enabled() {
			// The order isn't fully authorized we need to check that the client
			// has rate limit room for more pending authorizations.
			started := ra.clk.Now()
			err := ra.checkPendingAuthorizationLimit(ctx, newOrder.RegistrationID, pendingAuthzLimits)
			elapsed := ra.clk.Since(started)
			if err != nil {
				if errors.Is(err, berrors.RateLimit) {
					ra.rlCheckLatency.WithLabelValues(ratelimit.PendingAuthorizationsPerAccount, ratelimits.Denied).Observe(elapsed.Seconds())
				}
				return nil, err
			}
			ra.rlCheckLatency.WithLabelValues(ratelimit.PendingAuthorizationsPerAccount, ratelimits.Allowed).Observe(elapsed.Seconds())
		}
	}

	// Loop through each of the names missing authzs and create a new pending
	// authorization for each.
	var newAuthzs []*corepb.Authorization
	for _, name := range missingAuthzNames {
		pb, err := ra.createPendingAuthz(newOrder.RegistrationID, identifier.ACMEIdentifier{
			Type:  identifier.DNS,
			Value: name,
		})
		if err != nil {
			return nil, err
		}
		newAuthzs = append(newAuthzs, pb)
		ra.authzAges.WithLabelValues("NewOrder", pb.Status).Observe(0)
	}

	// Start with the order's own expiry as the minExpiry. We only care
	// about authz expiries that are sooner than the order's expiry
	minExpiry := ra.clk.Now().Add(ra.orderLifetime)

	// Check the reused authorizations to see if any have an expiry before the
	// minExpiry (the order's lifetime)
	for _, authz := range nameToExistingAuthz {
		// An authz without an expiry is an unexpected internal server event
		if core.IsAnyNilOrZero(authz.Expires) {
			return nil, berrors.InternalServerError(
				"SA.GetAuthorizations returned an authz (%s) with zero expiry",
				authz.Id)
		}
		// If the reused authorization expires before the minExpiry, it's expiry
		// is the new minExpiry.
		authzExpiry := authz.Expires.AsTime()
		if authzExpiry.Before(minExpiry) {
			minExpiry = authzExpiry
		}
	}
	// If the newly created pending authz's have an expiry closer than the
	// minExpiry the minExpiry is the pending authz expiry.
	if len(newAuthzs) > 0 {
		newPendingAuthzExpires := ra.clk.Now().Add(ra.pendingAuthorizationLifetime)
		if newPendingAuthzExpires.Before(minExpiry) {
			minExpiry = newPendingAuthzExpires
		}
	}
	// Set the order's expiry to the minimum expiry. The db doesn't store
	// sub-second values, so truncate here.
	newOrder.Expires = timestamppb.New(minExpiry.Truncate(time.Second))

	newOrderAndAuthzsReq := &sapb.NewOrderAndAuthzsRequest{
		NewOrder:  newOrder,
		NewAuthzs: newAuthzs,
	}
	storedOrder, err := ra.SA.NewOrderAndAuthzs(ctx, newOrderAndAuthzsReq)
	if err != nil {
		return nil, err
	}

	if core.IsAnyNilOrZero(storedOrder.Id, storedOrder.Status, storedOrder.RegistrationID, storedOrder.Names, storedOrder.Created, storedOrder.Expires) {
		return nil, errIncompleteGRPCResponse
	}
	ra.orderAges.WithLabelValues("NewOrder").Observe(0)

	// Note how many names are being requested in this certificate order.
	ra.namesPerCert.With(prometheus.Labels{"type": "requested"}).Observe(float64(len(storedOrder.Names)))

	return storedOrder, nil
}

// createPendingAuthz checks that a name is allowed for issuance and creates the
// necessary challenges for it and puts this and all of the relevant information
// into a corepb.Authorization for transmission to the SA to be stored
func (ra *RegistrationAuthorityImpl) createPendingAuthz(reg int64, identifier identifier.ACMEIdentifier) (*corepb.Authorization, error) {
	authz := &corepb.Authorization{
		Identifier:     identifier.Value,
		RegistrationID: reg,
		Status:         string(core.StatusPending),
		Expires:        timestamppb.New(ra.clk.Now().Add(ra.pendingAuthorizationLifetime).Truncate(time.Second)),
	}

	// Create challenges. The WFE will update them with URIs before sending them out.
	challenges, err := ra.PA.ChallengesFor(identifier)
	if err != nil {
		// The only time ChallengesFor errors it is a fatal configuration error
		// where challenges required by policy for an identifier are not enabled. We
		// want to treat this as an internal server error.
		return nil, berrors.InternalServerError(err.Error())
	}
	// Check each challenge for sanity.
	for _, challenge := range challenges {
		err := challenge.CheckPending()
		if err != nil {
			// berrors.InternalServerError because we generated these challenges, they should
			// be OK.
			err = berrors.InternalServerError("challenge didn't pass sanity check: %+v", challenge)
			return nil, err
		}
		challPB, err := bgrpc.ChallengeToPB(challenge)
		if err != nil {
			return nil, err
		}
		authz.Challenges = append(authz.Challenges, challPB)
	}
	return authz, nil
}

// wildcardOverlap takes a slice of domain names and returns an error if any of
// them is a non-wildcard FQDN that overlaps with a wildcard domain in the map.
func wildcardOverlap(dnsNames []string) error {
	nameMap := make(map[string]bool, len(dnsNames))
	for _, v := range dnsNames {
		nameMap[v] = true
	}
	for name := range nameMap {
		if name[0] == '*' {
			continue
		}
		labels := strings.Split(name, ".")
		labels[0] = "*"
		if nameMap[strings.Join(labels, ".")] {
			return berrors.MalformedError(
				"Domain name %q is redundant with a wildcard domain in the same request. Remove one or the other from the certificate request.", name)
		}
	}
	return nil
}

// validateContactsPresent will return an error if the contacts []string
// len is greater than zero and the contactsPresent bool is false. We
// don't care about any other cases. If the length of the contacts is zero
// and contactsPresent is true, it seems like a mismatch but we have to
// assume that the client is requesting to update the contacts field with
// by removing the existing contacts value so we don't want to return an
// error here.
func validateContactsPresent(contacts []string, contactsPresent bool) error {
	if len(contacts) > 0 && !contactsPresent {
		return berrors.InternalServerError("account contacts present but contactsPresent false")
	}
	return nil
}

func (ra *RegistrationAuthorityImpl) DrainFinalize() {
	ra.finalizeWG.Wait()
}

// UnpauseAccount receives a validated account unpause request from the SFE and
// instructs the SA to unpause that account. If the account cannot be unpaused,
// an error is returned.
func (ra *RegistrationAuthorityImpl) UnpauseAccount(ctx context.Context, request *rapb.UnpauseAccountRequest) (*emptypb.Empty, error) {
	if core.IsAnyNilOrZero(request.RegistrationID) {
		return nil, errIncompleteGRPCRequest
	}

	return nil, status.Errorf(codes.Unimplemented, "method UnpauseAccount not implemented")
}
