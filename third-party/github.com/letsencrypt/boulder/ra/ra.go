package ra

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/crypto/ocsp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/letsencrypt/boulder/akamai"
	akamaipb "github.com/letsencrypt/boulder/akamai/proto"
	"github.com/letsencrypt/boulder/allowlist"
	capb "github.com/letsencrypt/boulder/ca/proto"
	"github.com/letsencrypt/boulder/config"
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
	"github.com/letsencrypt/boulder/ratelimits"
	"github.com/letsencrypt/boulder/revocation"
	sapb "github.com/letsencrypt/boulder/sa/proto"
	"github.com/letsencrypt/boulder/va"
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

// RegistrationAuthorityImpl defines an RA.
//
// NOTE: All of the fields in RegistrationAuthorityImpl need to be
// populated, or there is a risk of panic.
type RegistrationAuthorityImpl struct {
	rapb.UnsafeRegistrationAuthorityServer
	rapb.UnsafeSCTProviderServer
	CA        capb.CertificateAuthorityClient
	OCSP      capb.OCSPGeneratorClient
	VA        va.RemoteClients
	SA        sapb.StorageAuthorityClient
	PA        core.PolicyAuthority
	publisher pubpb.PublisherClient

	clk               clock.Clock
	log               blog.Logger
	keyPolicy         goodkey.KeyPolicy
	profiles          *validationProfiles
	maxContactsPerReg int
	limiter           *ratelimits.Limiter
	txnBuilder        *ratelimits.TransactionBuilder
	finalizeTimeout   time.Duration
	drainWG           sync.WaitGroup

	issuersByNameID map[issuance.NameID]*issuance.Certificate
	purger          akamaipb.AkamaiPurgerClient

	ctpolicy *ctpolicy.CTPolicy

	ctpolicyResults         *prometheus.HistogramVec
	revocationReasonCounter *prometheus.CounterVec
	namesPerCert            *prometheus.HistogramVec
	newRegCounter           prometheus.Counter
	recheckCAACounter       prometheus.Counter
	newCertCounter          prometheus.Counter
	authzAges               *prometheus.HistogramVec
	orderAges               *prometheus.HistogramVec
	inflightFinalizes       prometheus.Gauge
	certCSRMismatch         prometheus.Counter
	pauseCounter            *prometheus.CounterVec
	// TODO(#8177): Remove once the rate of requests failing to finalize due to
	// requesting Must-Staple has diminished.
	mustStapleRequestsCounter *prometheus.CounterVec
	// TODO(#7966): Remove once the rate of registrations with contacts has been
	// determined.
	newOrUpdatedContactCounter *prometheus.CounterVec
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
	profiles *validationProfiles,
	pubc pubpb.PublisherClient,
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

	newCertCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "new_certificates",
		Help: "A counter of issued certificates",
	})
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

	pauseCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "paused_pairs",
		Help: "Number of times a pause operation is performed, labeled by paused=[bool], repaused=[bool], grace=[bool]",
	}, []string{"paused", "repaused", "grace"})
	stats.MustRegister(pauseCounter)

	mustStapleRequestsCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "must_staple_requests",
		Help: "Number of times a must-staple request is made, labeled by allowlist=[allowed|denied]",
	}, []string{"allowlist"})
	stats.MustRegister(mustStapleRequestsCounter)

	// TODO(#7966): Remove once the rate of registrations with contacts has been
	// determined.
	newOrUpdatedContactCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "new_or_updated_contact",
		Help: "A counter of new or updated contacts, labeled by new=[bool]",
	}, []string{"new"})
	stats.MustRegister(newOrUpdatedContactCounter)

	issuersByNameID := make(map[issuance.NameID]*issuance.Certificate)
	for _, issuer := range issuers {
		issuersByNameID[issuer.NameID()] = issuer
	}

	ra := &RegistrationAuthorityImpl{
		clk:                        clk,
		log:                        logger,
		profiles:                   profiles,
		maxContactsPerReg:          maxContactsPerReg,
		keyPolicy:                  keyPolicy,
		limiter:                    limiter,
		txnBuilder:                 txnBuilder,
		publisher:                  pubc,
		finalizeTimeout:            finalizeTimeout,
		ctpolicy:                   ctp,
		ctpolicyResults:            ctpolicyResults,
		purger:                     purger,
		issuersByNameID:            issuersByNameID,
		namesPerCert:               namesPerCert,
		newRegCounter:              newRegCounter,
		recheckCAACounter:          recheckCAACounter,
		newCertCounter:             newCertCounter,
		revocationReasonCounter:    revocationReasonCounter,
		authzAges:                  authzAges,
		orderAges:                  orderAges,
		inflightFinalizes:          inflightFinalizes,
		certCSRMismatch:            certCSRMismatch,
		pauseCounter:               pauseCounter,
		mustStapleRequestsCounter:  mustStapleRequestsCounter,
		newOrUpdatedContactCounter: newOrUpdatedContactCounter,
	}
	return ra
}

// ValidationProfileConfig is a config struct which can be used to create a
// ValidationProfile.
type ValidationProfileConfig struct {
	// PendingAuthzLifetime defines how far in the future an authorization's
	// "expires" timestamp is set when it is first created, i.e. how much
	// time the applicant has to attempt the challenge.
	PendingAuthzLifetime config.Duration `validate:"required"`
	// ValidAuthzLifetime defines how far in the future an authorization's
	// "expires" timestamp is set when one of its challenges is fulfilled,
	// i.e. how long a validated authorization may be reused.
	ValidAuthzLifetime config.Duration `validate:"required"`
	// OrderLifetime defines how far in the future an order's "expires"
	// timestamp is set when it is first created, i.e. how much time the
	// applicant has to fulfill all challenges and finalize the order. This is
	// a maximum time: if the order reuses an authorization and that authz
	// expires earlier than this OrderLifetime would otherwise set, then the
	// order's expiration is brought in to match that authorization.
	OrderLifetime config.Duration `validate:"required"`
	// MaxNames is the maximum number of subjectAltNames in a single cert.
	// The value supplied MUST be greater than 0 and no more than 100. These
	// limits are per section 7.1 of our combined CP/CPS, under "DV-SSL
	// Subscriber Certificate". The value must be less than or equal to the
	// global (i.e. not per-profile) value configured in the CA.
	MaxNames int `validate:"omitempty,min=1,max=100"`
	// AllowList specifies the path to a YAML file containing a list of
	// account IDs permitted to use this profile. If no path is
	// specified, the profile is open to all accounts. If the file
	// exists but is empty, the profile is closed to all accounts.
	AllowList string `validate:"omitempty"`
	// IdentifierTypes is a list of identifier types that may be issued under
	// this profile.
	IdentifierTypes []identifier.IdentifierType `validate:"required,dive,oneof=dns ip"`
}

// validationProfile holds the attributes of a given validation profile.
type validationProfile struct {
	// pendingAuthzLifetime defines how far in the future an authorization's
	// "expires" timestamp is set when it is first created, i.e. how much
	// time the applicant has to attempt the challenge.
	pendingAuthzLifetime time.Duration
	// validAuthzLifetime defines how far in the future an authorization's
	// "expires" timestamp is set when one of its challenges is fulfilled,
	// i.e. how long a validated authorization may be reused.
	validAuthzLifetime time.Duration
	// orderLifetime defines how far in the future an order's "expires"
	// timestamp is set when it is first created, i.e. how much time the
	// applicant has to fulfill all challenges and finalize the order. This is
	// a maximum time: if the order reuses an authorization and that authz
	// expires earlier than this OrderLifetime would otherwise set, then the
	// order's expiration is brought in to match that authorization.
	orderLifetime time.Duration
	// maxNames is the maximum number of subjectAltNames in a single cert.
	maxNames int
	// allowList holds the set of account IDs allowed to use this profile. If
	// nil, the profile is open to all accounts (everyone is allowed).
	allowList *allowlist.List[int64]
	// identifierTypes is a list of identifier types that may be issued under
	// this profile.
	identifierTypes []identifier.IdentifierType
}

// validationProfiles provides access to the set of configured profiles,
// including the default profile for orders/authzs which do not specify one.
type validationProfiles struct {
	defaultName string
	byName      map[string]*validationProfile
}

// NewValidationProfiles builds a new validationProfiles struct from the given
// configs and default name. It enforces that the given authorization lifetimes
// are within the bounds mandated by the Baseline Requirements.
func NewValidationProfiles(defaultName string, configs map[string]*ValidationProfileConfig) (*validationProfiles, error) {
	if defaultName == "" {
		return nil, errors.New("default profile name must be configured")
	}

	profiles := make(map[string]*validationProfile, len(configs))

	for name, config := range configs {
		// The Baseline Requirements v1.8.1 state that validation tokens "MUST
		// NOT be used for more than 30 days from its creation". If unconfigured
		// or the configured value pendingAuthorizationLifetimeDays is greater
		// than 29 days, bail out.
		if config.PendingAuthzLifetime.Duration <= 0 || config.PendingAuthzLifetime.Duration > 29*(24*time.Hour) {
			return nil, fmt.Errorf("PendingAuthzLifetime value must be greater than 0 and less than 30d, but got %q", config.PendingAuthzLifetime.Duration)
		}

		// Baseline Requirements v1.8.1 section 4.2.1: "any reused data, document,
		// or completed validation MUST be obtained no more than 398 days prior
		// to issuing the Certificate". If unconfigured or the configured value is
		// greater than 397 days, bail out.
		if config.ValidAuthzLifetime.Duration <= 0 || config.ValidAuthzLifetime.Duration > 397*(24*time.Hour) {
			return nil, fmt.Errorf("ValidAuthzLifetime value must be greater than 0 and less than 398d, but got %q", config.ValidAuthzLifetime.Duration)
		}

		if config.MaxNames <= 0 || config.MaxNames > 100 {
			return nil, fmt.Errorf("MaxNames must be greater than 0 and at most 100")
		}

		var allowList *allowlist.List[int64]
		if config.AllowList != "" {
			data, err := os.ReadFile(config.AllowList)
			if err != nil {
				return nil, fmt.Errorf("reading allowlist: %w", err)
			}
			allowList, err = allowlist.NewFromYAML[int64](data)
			if err != nil {
				return nil, fmt.Errorf("parsing allowlist: %w", err)
			}
		}

		profiles[name] = &validationProfile{
			pendingAuthzLifetime: config.PendingAuthzLifetime.Duration,
			validAuthzLifetime:   config.ValidAuthzLifetime.Duration,
			orderLifetime:        config.OrderLifetime.Duration,
			maxNames:             config.MaxNames,
			allowList:            allowList,
			identifierTypes:      config.IdentifierTypes,
		}
	}

	_, ok := profiles[defaultName]
	if !ok {
		return nil, fmt.Errorf("no profile configured matching default profile name %q", defaultName)
	}

	return &validationProfiles{
		defaultName: defaultName,
		byName:      profiles,
	}, nil
}

func (vp *validationProfiles) get(name string) (*validationProfile, error) {
	if name == "" {
		name = vp.defaultName
	}
	profile, ok := vp.byName[name]
	if !ok {
		return nil, berrors.InvalidProfileError("unrecognized profile name %q", name)
	}
	return profile, nil
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
	// Identifiers are the identifiers from the issued cert
	Identifiers identifier.ACMEIdentifiers `json:",omitempty"`
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
	// PreviousCertificateIssued is present when this certificate uses the same set
	// of FQDNs as a previous certificate (from any account) and contains the
	// notBefore of the most recent such certificate.
	PreviousCertificateIssued time.Time `json:",omitempty"`
	// UserAgent is the User-Agent header from the ACME client (provided to the
	// RA via gRPC metadata).
	UserAgent string
}

// certificateRevocationEvent is a struct for holding information that is logged
// as JSON to the audit log as the result of a revocation event.
type certificateRevocationEvent struct {
	ID string `json:",omitempty"`
	// SerialNumber is the string representation of the revoked certificate's
	// serial number.
	SerialNumber string `json:",omitempty"`
	// Reason is the integer representing the revocation reason used.
	Reason int64 `json:"reason"`
	// Method is the way in which revocation was requested.
	// It will be one of the strings: "applicant", "subscriber", "control", "key", or "admin".
	Method string `json:",omitempty"`
	// RequesterID is the account ID of the requester.
	// Will be zero for admin revocations.
	RequesterID int64 `json:",omitempty"`
	CRLShard    int64
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

// NewRegistration constructs a new Registration from a request.
func (ra *RegistrationAuthorityImpl) NewRegistration(ctx context.Context, request *corepb.Registration) (*corepb.Registration, error) {
	// Error if the request is nil, there is no account key or IP address
	if request == nil || len(request.Key) == 0 {
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

	// Check that contacts conform to our expectations.
	// TODO(#8199): Remove this when no contacts are included in any requests.
	err = ra.validateContacts(request.Contact)
	if err != nil {
		return nil, err
	}

	// Don't populate ID or CreatedAt because those will be set by the SA.
	req := &corepb.Registration{
		Key:       request.Key,
		Contact:   request.Contact,
		Agreement: request.Agreement,
		Status:    string(core.StatusValid),
	}

	// Store the registration object, then return the version that got stored.
	res, err := ra.SA.NewRegistration(ctx, req)
	if err != nil {
		return nil, err
	}

	// TODO(#7966): Remove once the rate of registrations with contacts has been
	// determined.
	for range request.Contact {
		ra.newOrUpdatedContactCounter.With(prometheus.Labels{"new": "true"}).Inc()
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
			return berrors.InvalidEmailError("unparsable contact")
		}
		if parsed.Scheme != "mailto" {
			return berrors.UnsupportedContactError("only contact scheme 'mailto:' is supported")
		}
		if parsed.RawQuery != "" || contact[len(contact)-1] == '?' {
			return berrors.InvalidEmailError("contact email contains a question mark")
		}
		if parsed.Fragment != "" || contact[len(contact)-1] == '#' {
			return berrors.InvalidEmailError("contact email contains a '#'")
		}
		if !core.IsASCII(contact) {
			return berrors.InvalidEmailError("contact email contains non-ASCII characters")
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
		return fmt.Errorf("failed to marshal reg.Contact to JSON: %w", err)
	} else if len(jsonBytes) >= maxContactBytes {
		return berrors.InvalidEmailError(
			"too many/too long contact(s). Please use shorter or fewer email addresses")
	}

	return nil
}

// matchesCSR tests the contents of a generated certificate to make sure
// that the PublicKey, CommonName, and identifiers match those provided in
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

	csrIdents := identifier.FromCSR(csr)
	if parsedCertificate.Subject.CommonName != "" {
		// Only check that the issued common name matches one of the SANs if there
		// is an issued CN at all: this allows flexibility on whether we include
		// the CN.
		if !slices.Contains(csrIdents, identifier.NewDNS(parsedCertificate.Subject.CommonName)) {
			return berrors.InternalServerError("generated certificate CommonName doesn't match any CSR name")
		}
	}

	parsedIdents := identifier.FromCert(parsedCertificate)
	if !slices.Equal(csrIdents, parsedIdents) {
		return berrors.InternalServerError("generated certificate identifiers don't match CSR identifiers")
	}

	if !slices.Equal(parsedCertificate.EmailAddresses, csr.EmailAddresses) {
		return berrors.InternalServerError("generated certificate EmailAddresses don't match CSR EmailAddresses")
	}

	if !slices.Equal(parsedCertificate.URIs, csr.URIs) {
		return berrors.InternalServerError("generated certificate URIs don't match CSR URIs")
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
	for _, eku := range parsedCertificate.ExtKeyUsage {
		if eku != x509.ExtKeyUsageServerAuth && eku != x509.ExtKeyUsageClientAuth {
			return berrors.InternalServerError("generated certificate has unacceptable EKU")
		}
	}
	if !slices.Contains(parsedCertificate.ExtKeyUsage, x509.ExtKeyUsageServerAuth) {
		return berrors.InternalServerError("generated certificate doesn't have serverAuth EKU")
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
	orderID orderID,
	acctID accountID,
	idents identifier.ACMEIdentifiers,
	now time.Time) (map[identifier.ACMEIdentifier]*core.Authorization, error) {
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

	// Ensure that every identifier has a matching authz, and vice-versa.
	var missing []string
	var invalid []string
	var expired []string
	for _, ident := range idents {
		authz, ok := authzs[ident]
		if !ok || authz == nil {
			missing = append(missing, ident.Value)
			continue
		}
		if authz.Status != core.StatusValid {
			invalid = append(invalid, ident.Value)
			continue
		}
		if authz.Expires.Before(now) {
			expired = append(expired, ident.Value)
			continue
		}
		err = ra.PA.CheckAuthzChallenges(authz)
		if err != nil {
			invalid = append(invalid, ident.Value)
			continue
		}
	}

	if len(missing) > 0 {
		return nil, berrors.UnauthorizedError(
			"authorizations for these identifiers not found: %s",
			strings.Join(missing, ", "),
		)
	}

	if len(invalid) > 0 {
		return nil, berrors.UnauthorizedError(
			"authorizations for these identifiers not valid: %s",
			strings.Join(invalid, ", "),
		)
	}
	if len(expired) > 0 {
		return nil, berrors.UnauthorizedError(
			"authorizations for these identifiers expired: %s",
			strings.Join(expired, ", "),
		)
	}

	// Even though this check is cheap, we do it after the more specific checks
	// so that we can return more specific error messages.
	if len(idents) != len(authzs) {
		return nil, berrors.UnauthorizedError("incorrect number of identifiers requested for finalization")
	}

	// Check that the authzs either don't need CAA rechecking, or do the
	// necessary CAA rechecks right now.
	err = ra.checkAuthorizationsCAA(ctx, int64(acctID), authzs, now)
	if err != nil {
		return nil, err
	}

	return authzs, nil
}

// validatedBefore checks if a given authorization's challenge was
// validated before a given time. Returns a bool.
func validatedBefore(authz *core.Authorization, caaRecheckTime time.Time) (bool, error) {
	numChallenges := len(authz.Challenges)
	if numChallenges != 1 {
		return false, berrors.InternalServerError("authorization has incorrect number of challenges. 1 expected, %d found for: id %s", numChallenges, authz.ID)
	}
	if authz.Challenges[0].Validated == nil {
		return false, berrors.InternalServerError("authorization's challenge has no validated timestamp for: id %s", authz.ID)
	}
	return authz.Challenges[0].Validated.Before(caaRecheckTime), nil
}

// checkAuthorizationsCAA ensures that we have sufficiently-recent CAA checks
// for every input identifier/authz. If any authz was validated too long ago, it
// kicks off a CAA recheck for that identifier If it returns an error, it will
// be of type BoulderError.
func (ra *RegistrationAuthorityImpl) checkAuthorizationsCAA(
	ctx context.Context,
	acctID int64,
	authzs map[identifier.ACMEIdentifier]*core.Authorization,
	now time.Time) error {
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

	for _, authz := range authzs {
		if staleCAA, err := validatedBefore(authz, caaRecheckAfter); err != nil {
			return err
		} else if staleCAA {
			switch authz.Identifier.Type {
			case identifier.TypeDNS:
				// Ensure that CAA is rechecked for this name
				recheckAuthzs = append(recheckAuthzs, authz)
			case identifier.TypeIP:
			default:
				return berrors.MalformedError("invalid identifier type: %s", authz.Identifier.Type)
			}
		}
	}

	if len(recheckAuthzs) > 0 {
		err := ra.recheckCAA(ctx, recheckAuthzs)
		if err != nil {
			return err
		}
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
						authz.ID, authz.Identifier.Value),
				}
				return
			}
			var resp *vapb.IsCAAValidResponse
			var err error
			resp, err = ra.VA.DoCAA(ctx, &vapb.IsCAAValidRequest{
				Identifier:       authz.Identifier.ToProto(),
				ValidationMethod: method,
				AccountURIID:     authz.RegistrationID,
			})
			if err != nil {
				ra.log.AuditErrf("Rechecking CAA: %s", err)
				err = berrors.InternalServerError(
					"Internal error rechecking CAA for authorization ID %v (%v)",
					authz.ID, authz.Identifier.Value,
				)
			} else if resp.Problem != nil {
				err = berrors.CAAError("rechecking caa: %s", resp.Problem.Detail)
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
		UserAgent:   web.UserAgent(ctx),
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
		ra.drainWG.Add(1)
		go func() {
			// The original context will be canceled in the RPC layer when FinalizeOrder returns,
			// so split off a context that won't be canceled (and has its own timeout).
			ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ra.finalizeTimeout)
			defer cancel()
			_, err := ra.issueCertificateOuter(ctx, proto.Clone(order).(*corepb.Order), csr, logEvent)
			if err != nil {
				// We only log here, because this is in a background goroutine with
				// no parent goroutine waiting for it to receive the error.
				ra.log.AuditErrf("Asynchronous finalization failed: %s", err.Error())
			}
			ra.drainWG.Done()
		}()
		return order, nil
	} else {
		return ra.issueCertificateOuter(ctx, order, csr, logEvent)
	}
}

// containsMustStaple returns true if the provided set of extensions includes
// an entry whose OID and value both match the expected values for the OCSP
// Must-Staple (a.k.a. id-pe-tlsFeature) extension.
func containsMustStaple(extensions []pkix.Extension) bool {
	// RFC 7633: id-pe-tlsfeature OBJECT IDENTIFIER ::=  { id-pe 24 }
	var mustStapleExtId = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 24}
	// ASN.1 encoding of:
	// SEQUENCE
	//   INTEGER 5
	// where "5" is the status_request feature (RFC 6066)
	var mustStapleExtValue = []byte{0x30, 0x03, 0x02, 0x01, 0x05}

	for _, ext := range extensions {
		if ext.Id.Equal(mustStapleExtId) && bytes.Equal(ext.Value, mustStapleExtValue) {
			return true
		}
	}
	return false
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

	profile, err := ra.profiles.get(req.Order.CertificateProfileName)
	if err != nil {
		return nil, err
	}

	orderIdents := identifier.Normalize(identifier.FromProtoSlice(req.Order.Identifiers))

	// There should never be an order with 0 identifiers at the stage, but we check to
	// be on the safe side, throwing an internal server error if this assumption
	// is ever violated.
	if len(orderIdents) == 0 {
		return nil, berrors.InternalServerError("Order has no associated identifiers")
	}

	// Parse the CSR from the request
	csr, err := x509.ParseCertificateRequest(req.Csr)
	if err != nil {
		return nil, berrors.BadCSRError("unable to parse CSR: %s", err.Error())
	}

	if containsMustStaple(csr.Extensions) {
		ra.mustStapleRequestsCounter.WithLabelValues("denied").Inc()
		return nil, berrors.UnauthorizedError(
			"OCSP must-staple extension is no longer available: see https://letsencrypt.org/2024/12/05/ending-ocsp",
		)
	}

	err = csrlib.VerifyCSR(ctx, csr, profile.maxNames, &ra.keyPolicy, ra.PA)
	if err != nil {
		// VerifyCSR returns berror instances that can be passed through as-is
		// without wrapping.
		return nil, err
	}

	// Dedupe, lowercase and sort both the names from the CSR and the names in the
	// order.
	csrIdents := identifier.FromCSR(csr)
	// Check that the order names and the CSR names are an exact match
	if !slices.Equal(csrIdents, orderIdents) {
		return nil, berrors.UnauthorizedError("CSR does not specify same identifiers as Order")
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

	// Double-check that all authorizations on this order are valid, are also
	// associated with the same account as the order itself, and have recent CAA.
	authzs, err := ra.checkOrderAuthorizations(
		ctx, orderID(req.Order.Id), accountID(req.Order.RegistrationID), csrIdents, ra.clk.Now())
	if err != nil {
		// Pass through the error without wrapping it because the called functions
		// return BoulderError and we don't want to lose the type.
		return nil, err
	}

	// Collect up a certificateRequestAuthz that stores the ID and challenge type
	// of each of the valid authorizations we used for this issuance.
	logEventAuthzs := make(map[string]certificateRequestAuthz, len(csrIdents))
	for _, authz := range authzs {
		// No need to check for error here because we know this same call just
		// succeeded inside ra.checkOrderAuthorizations
		solvedByChallengeType, _ := authz.SolvedBy()
		logEventAuthzs[authz.Identifier.Value] = certificateRequestAuthz{
			ID:            authz.ID,
			ChallengeType: solvedByChallengeType,
		}
		authzAge := (profile.validAuthzLifetime - authz.Expires.Sub(ra.clk.Now())).Seconds()
		ra.authzAges.WithLabelValues("FinalizeOrder", string(authz.Status)).Observe(authzAge)
	}
	logEvent.Authorizations = logEventAuthzs

	// Mark that we verified the CN and SANs
	logEvent.VerifiedFields = []string{"subject.commonName", "subjectAltName"}

	return csr, nil
}

func (ra *RegistrationAuthorityImpl) GetSCTs(ctx context.Context, sctRequest *rapb.SCTRequest) (*rapb.SCTResponse, error) {
	scts, err := ra.getSCTs(ctx, sctRequest.PrecertDER)
	if err != nil {
		return nil, err
	}
	return &rapb.SCTResponse{
		SctDER: scts,
	}, nil
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

	idents := identifier.FromProtoSlice(order.Identifiers)

	isRenewal := false
	timestamps, err := ra.SA.FQDNSetTimestampsForWindow(ctx, &sapb.CountFQDNSetsRequest{
		Identifiers: idents.ToProtoSlice(),
		Window:      durationpb.New(120 * 24 * time.Hour),
		Limit:       1,
	})
	if err != nil {
		return nil, fmt.Errorf("checking if certificate is a renewal: %w", err)
	}
	if len(timestamps.Timestamps) > 0 {
		isRenewal = true
		logEvent.PreviousCertificateIssued = timestamps.Timestamps[0].AsTime()
	}

	profileName := order.CertificateProfileName
	if profileName == "" {
		profileName = ra.profiles.defaultName
	}

	// Step 3: Issue the Certificate
	cert, err := ra.issueCertificateInner(
		ctx, csr, isRenewal, profileName, accountID(order.RegistrationID), orderID(order.Id))

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
		).Observe(float64(len(idents)))

		ra.newCertCounter.Inc()

		logEvent.SerialNumber = core.SerialToString(cert.SerialNumber)
		logEvent.CommonName = cert.Subject.CommonName
		logEvent.Identifiers = identifier.FromCert(cert)
		logEvent.NotBefore = cert.NotBefore
		logEvent.NotAfter = cert.NotAfter
		logEvent.CertProfileName = profileName

		result = "successful"
	}

	logEvent.ResponseTime = ra.clk.Now()
	ra.log.AuditObject(fmt.Sprintf("Certificate request - %s", result), logEvent)

	return order, err
}

// countCertificateIssued increments the certificates (per domain and per
// account) and duplicate certificate rate limits. There is no reason to surface
// errors from this function to the Subscriber, spends against these limit are
// best effort.
func (ra *RegistrationAuthorityImpl) countCertificateIssued(ctx context.Context, regId int64, orderIdents identifier.ACMEIdentifiers, isRenewal bool) {
	var transactions []ratelimits.Transaction
	if !isRenewal {
		txns, err := ra.txnBuilder.CertificatesPerDomainSpendOnlyTransactions(regId, orderIdents)
		if err != nil {
			ra.log.Warningf("building rate limit transactions at finalize: %s", err)
		}
		transactions = append(transactions, txns...)
	}

	txn, err := ra.txnBuilder.CertificatesPerFQDNSetSpendOnlyTransaction(orderIdents)
	if err != nil {
		ra.log.Warningf("building rate limit transaction at finalize: %s", err)
	}
	transactions = append(transactions, txn)

	_, err = ra.limiter.BatchSpend(ctx, transactions)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return
		}
		ra.log.Warningf("spending against rate limits at finalize: %s", err)
	}
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
	isRenewal bool,
	profileName string,
	acctID accountID,
	oID orderID) (*x509.Certificate, error) {
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

	resp, err := ra.CA.IssueCertificate(ctx, issueReq)
	if err != nil {
		return nil, err
	}

	parsedCertificate, err := x509.ParseCertificate(resp.DER)
	if err != nil {
		return nil, wrapError(err, "parsing final certificate")
	}

	ra.countCertificateIssued(ctx, int64(acctID), identifier.FromCert(parsedCertificate), isRenewal)

	// Asynchronously submit the final certificate to any configured logs
	go ra.ctpolicy.SubmitFinalCert(resp.DER, parsedCertificate.NotAfter)

	err = ra.matchesCSR(parsedCertificate, csr)
	if err != nil {
		ra.certCSRMismatch.Inc()
		return nil, err
	}

	_, err = ra.SA.FinalizeOrder(ctx, &sapb.FinalizeOrderRequest{
		Id:                int64(oID),
		CertificateSerial: core.SerialToString(parsedCertificate.SerialNumber),
	})
	if err != nil {
		return nil, wrapError(err, "persisting finalized order")
	}

	return parsedCertificate, nil
}

func (ra *RegistrationAuthorityImpl) getSCTs(ctx context.Context, precertDER []byte) (core.SCTDERs, error) {
	started := ra.clk.Now()
	precert, err := x509.ParseCertificate(precertDER)
	if err != nil {
		return nil, fmt.Errorf("parsing precertificate: %w", err)
	}

	scts, err := ra.ctpolicy.GetSCTs(ctx, precertDER, precert.NotAfter)
	took := ra.clk.Since(started)
	if err != nil {
		state := "failure"
		if err == context.DeadlineExceeded {
			state = "deadlineExceeded"
			// Convert the error to a missingSCTsError to communicate the timeout,
			// otherwise it will be a generic serverInternalError
			err = berrors.MissingSCTsError("failed to get SCTs: %s", err.Error())
		}
		ra.log.Warningf("ctpolicy.GetSCTs failed: %s", err)
		ra.ctpolicyResults.With(prometheus.Labels{"result": state}).Observe(took.Seconds())
		return nil, err
	}
	ra.ctpolicyResults.With(prometheus.Labels{"result": "success"}).Observe(took.Seconds())
	return scts, nil
}

// UpdateRegistrationContact updates an existing Registration's contact. The
// updated contacts field may be empty.
//
// Deprecated: This method has no callers. See
// https://github.com/letsencrypt/boulder/issues/8199 for removal.
func (ra *RegistrationAuthorityImpl) UpdateRegistrationContact(ctx context.Context, req *rapb.UpdateRegistrationContactRequest) (*corepb.Registration, error) {
	if core.IsAnyNilOrZero(req.RegistrationID) {
		return nil, errIncompleteGRPCRequest
	}

	err := ra.validateContacts(req.Contacts)
	if err != nil {
		return nil, fmt.Errorf("invalid contact: %w", err)
	}

	update, err := ra.SA.UpdateRegistrationContact(ctx, &sapb.UpdateRegistrationContactRequest{
		RegistrationID: req.RegistrationID,
		Contacts:       req.Contacts,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update registration contact: %w", err)
	}

	// TODO(#7966): Remove once the rate of registrations with contacts has
	// been determined.
	for range req.Contacts {
		ra.newOrUpdatedContactCounter.With(prometheus.Labels{"new": "false"}).Inc()
	}

	return update, nil
}

// UpdateRegistrationKey updates an existing Registration's key.
func (ra *RegistrationAuthorityImpl) UpdateRegistrationKey(ctx context.Context, req *rapb.UpdateRegistrationKeyRequest) (*corepb.Registration, error) {
	if core.IsAnyNilOrZero(req.RegistrationID, req.Jwk) {
		return nil, errIncompleteGRPCRequest
	}

	update, err := ra.SA.UpdateRegistrationKey(ctx, &sapb.UpdateRegistrationKeyRequest{
		RegistrationID: req.RegistrationID,
		Jwk:            req.Jwk,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update registration key: %w", err)
	}

	return update, nil
}

// recordValidation records an authorization validation event,
// it should only be used on v2 style authorizations.
func (ra *RegistrationAuthorityImpl) recordValidation(ctx context.Context, authID string, authExpires time.Time, challenge *core.Challenge) error {
	authzID, err := strconv.ParseInt(authID, 10, 64)
	if err != nil {
		return err
	}
	vr, err := bgrpc.ValidationResultToPB(challenge.ValidationRecord, challenge.Error, "", "")
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
		Expires:           timestamppb.New(authExpires),
		Attempted:         string(challenge.Type),
		AttemptedAt:       validated,
		ValidationRecords: vr.Records,
		ValidationError:   vr.Problem,
	})
	return err
}

// countFailedValidations increments the FailedAuthorizationsPerDomainPerAccount limit.
// and the FailedAuthorizationsForPausingPerDomainPerAccountTransaction limit.
func (ra *RegistrationAuthorityImpl) countFailedValidations(ctx context.Context, regId int64, ident identifier.ACMEIdentifier) error {
	txn, err := ra.txnBuilder.FailedAuthorizationsPerDomainPerAccountSpendOnlyTransaction(regId, ident)
	if err != nil {
		return fmt.Errorf("building rate limit transaction for the %s rate limit: %w", ratelimits.FailedAuthorizationsPerDomainPerAccount, err)
	}

	_, err = ra.limiter.Spend(ctx, txn)
	if err != nil {
		return fmt.Errorf("spending against the %s rate limit: %w", ratelimits.FailedAuthorizationsPerDomainPerAccount, err)
	}

	if features.Get().AutomaticallyPauseZombieClients {
		txn, err = ra.txnBuilder.FailedAuthorizationsForPausingPerDomainPerAccountTransaction(regId, ident)
		if err != nil {
			return fmt.Errorf("building rate limit transaction for the %s rate limit: %w", ratelimits.FailedAuthorizationsForPausingPerDomainPerAccount, err)
		}

		decision, err := ra.limiter.Spend(ctx, txn)
		if err != nil {
			return fmt.Errorf("spending against the %s rate limit: %s", ratelimits.FailedAuthorizationsForPausingPerDomainPerAccount, err)
		}

		if decision.Result(ra.clk.Now()) != nil {
			resp, err := ra.SA.PauseIdentifiers(ctx, &sapb.PauseRequest{
				RegistrationID: regId,
				Identifiers:    []*corepb.Identifier{ident.ToProto()},
			})
			if err != nil {
				return fmt.Errorf("failed to pause %d/%q: %w", regId, ident.Value, err)
			}
			ra.pauseCounter.With(prometheus.Labels{
				"paused":   strconv.FormatBool(resp.Paused > 0),
				"repaused": strconv.FormatBool(resp.Repaused > 0),
				"grace":    strconv.FormatBool(resp.Paused <= 0 && resp.Repaused <= 0),
			}).Inc()
		}
	}
	return nil
}

// resetAccountPausingLimit resets bucket to maximum capacity for given account.
// There is no reason to surface errors from this function to the Subscriber.
func (ra *RegistrationAuthorityImpl) resetAccountPausingLimit(ctx context.Context, regId int64, ident identifier.ACMEIdentifier) {
	bucketKey := ratelimits.NewRegIdIdentValueBucketKey(ratelimits.FailedAuthorizationsForPausingPerDomainPerAccount, regId, ident.Value)
	err := ra.limiter.Reset(ctx, bucketKey)
	if err != nil {
		ra.log.Warningf("resetting bucket for regID=[%d] identifier=[%s]: %s", regId, ident.Value, err)
	}
}

// doDCVAndCAA performs DCV and CAA checks sequentially: DCV is performed first
// and CAA is only checked if DCV is successful. Validation records from the DCV
// check are returned even if the CAA check fails.
func (ra *RegistrationAuthorityImpl) checkDCVAndCAA(ctx context.Context, dcvReq *vapb.PerformValidationRequest, caaReq *vapb.IsCAAValidRequest) (*corepb.ProblemDetails, []*corepb.ValidationRecord, error) {
	doDCVRes, err := ra.VA.DoDCV(ctx, dcvReq)
	if err != nil {
		return nil, nil, err
	}
	if doDCVRes.Problem != nil {
		return doDCVRes.Problem, doDCVRes.Records, nil
	}

	switch identifier.FromProto(dcvReq.Identifier).Type {
	case identifier.TypeDNS:
		doCAAResp, err := ra.VA.DoCAA(ctx, caaReq)
		if err != nil {
			return nil, nil, err
		}
		return doCAAResp.Problem, doDCVRes.Records, nil
	case identifier.TypeIP:
		return nil, doDCVRes.Records, nil
	default:
		return nil, nil, berrors.MalformedError("invalid identifier type: %s", dcvReq.Identifier.Type)
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

	if core.IsAnyNilOrZero(req.Authz, req.Authz.Id, req.Authz.Identifier, req.Authz.Status, req.Authz.Expires) {
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

	profile, err := ra.profiles.get(authz.CertificateProfileName)
	if err != nil {
		return nil, err
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
		return nil, berrors.InternalServerError("getting acct for authorization: %s", err.Error())
	}
	reg, err := bgrpc.PbToRegistration(regPB)
	if err != nil {
		return nil, berrors.InternalServerError("getting acct for authorization: %s", err.Error())
	}

	// Compute the key authorization field based on the registration key
	expectedKeyAuthorization, err := ch.ExpectedKeyAuthorization(reg.Key)
	if err != nil {
		return nil, berrors.InternalServerError("could not compute expected key authorization value")
	}

	// Double check before sending to VA
	if cErr := ch.CheckPending(); cErr != nil {
		return nil, berrors.MalformedError("cannot validate challenge: %s", cErr.Error())
	}

	// Dispatch to the VA for service
	ra.drainWG.Add(1)
	vaCtx := context.Background()
	go func(authz core.Authorization) {
		defer ra.drainWG.Done()

		// We will mutate challenges later in this goroutine to change status and
		// add error, but we also return a copy of authz immediately. To avoid a
		// data race, make a copy of the challenges slice here for mutation.
		challenges := make([]core.Challenge, len(authz.Challenges))
		copy(challenges, authz.Challenges)
		authz.Challenges = challenges
		chall, _ := bgrpc.ChallengeToPB(authz.Challenges[challIndex])
		checkProb, checkRecords, err := ra.checkDCVAndCAA(
			vaCtx,
			&vapb.PerformValidationRequest{
				Identifier:               authz.Identifier.ToProto(),
				Challenge:                chall,
				Authz:                    &vapb.AuthzMeta{Id: authz.ID, RegID: authz.RegistrationID},
				ExpectedKeyAuthorization: expectedKeyAuthorization,
			},
			&vapb.IsCAAValidRequest{
				Identifier:       authz.Identifier.ToProto(),
				ValidationMethod: chall.Type,
				AccountURIID:     authz.RegistrationID,
				AuthzID:          authz.ID,
			},
		)
		challenge := &authz.Challenges[challIndex]
		var prob *probs.ProblemDetails
		if err != nil {
			prob = probs.ServerInternal("Could not communicate with VA")
			ra.log.AuditErrf("Could not communicate with VA: %s", err)
		} else {
			if checkProb != nil {
				prob, err = bgrpc.PBToProblemDetails(checkProb)
				if err != nil {
					prob = probs.ServerInternal("Could not communicate with VA")
					ra.log.AuditErrf("Could not communicate with VA: %s", err)
				}
			}
			// Save the updated records
			records := make([]core.ValidationRecord, len(checkRecords))
			for i, r := range checkRecords {
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

		expires := *authz.Expires
		if prob != nil {
			challenge.Status = core.StatusInvalid
			challenge.Error = prob
			err := ra.countFailedValidations(vaCtx, authz.RegistrationID, authz.Identifier)
			if err != nil {
				ra.log.Warningf("incrementing failed validations: %s", err)
			}
		} else {
			challenge.Status = core.StatusValid
			expires = ra.clk.Now().Add(profile.validAuthzLifetime)
			if features.Get().AutomaticallyPauseZombieClients {
				ra.resetAccountPausingLimit(vaCtx, authz.RegistrationID, authz.Identifier)
			}
		}
		challenge.Validated = &vStart
		authz.Challenges[challIndex] = *challenge

		err = ra.recordValidation(vaCtx, authz.ID, expires, challenge)
		if err != nil {
			if errors.Is(err, berrors.NotFound) {
				// We log NotFound at a lower level because this is largely due to a
				// parallel-validation race: a different validation attempt has already
				// updated this authz, so we failed to find a *pending* authz with the
				// given ID to update.
				ra.log.Infof("Failed to record validation (likely parallel validation race): regID=[%d] authzID=[%s] err=[%s]",
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
func (ra *RegistrationAuthorityImpl) revokeCertificate(ctx context.Context, cert *x509.Certificate, reason revocation.Reason) error {
	serialString := core.SerialToString(cert.SerialNumber)
	issuerID := issuance.IssuerNameID(cert)
	shardIdx, err := crlShard(cert)
	if err != nil {
		return err
	}

	_, err = ra.SA.RevokeCertificate(ctx, &sapb.RevokeCertificateRequest{
		Serial:   serialString,
		Reason:   int64(reason),
		Date:     timestamppb.New(ra.clk.Now()),
		IssuerID: int64(issuerID),
		ShardIdx: shardIdx,
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
func (ra *RegistrationAuthorityImpl) updateRevocationForKeyCompromise(ctx context.Context, serialString string, issuerID issuance.NameID) error {
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

	cert, err := ra.SA.GetCertificate(ctx, &sapb.Serial{Serial: serialString})
	if err != nil {
		return berrors.NotFoundError("unable to confirm that serial %q was ever issued: %s", serialString, err)
	}
	x509Cert, err := x509.ParseCertificate(cert.Der)
	if err != nil {
		return err
	}

	shardIdx, err := crlShard(x509Cert)
	if err != nil {
		return err
	}

	_, err = ra.SA.UpdateRevokedCertificate(ctx, &sapb.RevokeCertificateRequest{
		Serial:   serialString,
		Reason:   int64(ocsp.KeyCompromise),
		Date:     timestamppb.New(ra.clk.Now()),
		Backdate: status.RevokedDate,
		IssuerID: int64(issuerID),
		ShardIdx: shardIdx,
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
	if len(cert.OCSPServer) == 0 {
		// We can't purge the cache (and there should be no responses in the cache)
		// for certs that have no AIA OCSP URI.
		return nil
	}

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

		idents := identifier.FromCert(cert)
		var authzPB *sapb.Authorizations
		authzPB, err = ra.SA.GetValidAuthorizations2(ctx, &sapb.GetValidAuthorizationsRequest{
			RegistrationID: req.RegID,
			Identifiers:    idents.ToProtoSlice(),
			ValidUntil:     timestamppb.New(ra.clk.Now()),
		})
		if err != nil {
			return nil, err
		}

		var authzMap map[identifier.ACMEIdentifier]*core.Authorization
		authzMap, err = bgrpc.PBToAuthzMap(authzPB)
		if err != nil {
			return nil, err
		}

		for _, ident := range idents {
			if _, present := authzMap[ident]; !present {
				return nil, berrors.UnauthorizedError("requester does not control all identifiers in cert with serial %q", serialString)
			}
		}

		// Applicants who are not the original Subscriber are not allowed to
		// revoke for any reason other than cessationOfOperation, which covers
		// circumstances where "the certificate subscriber no longer owns the
		// domain names in the certificate". Override the reason code to match.
		req.Code = ocsp.CessationOfOperation
		logEvent.Reason = req.Code
	}

	err = ra.revokeCertificate(
		ctx,
		cert,
		revocation.Reason(req.Code),
	)
	if err != nil {
		return nil, err
	}

	// Don't propagate purger errors to the client.
	issuerID := issuance.IssuerNameID(cert)
	_ = ra.purgeOCSPCache(ctx, cert, issuerID)

	return &emptypb.Empty{}, nil
}

// crlShard extracts the CRL shard from a certificate's CRLDistributionPoint.
//
// If there is no CRLDistributionPoint, returns 0.
//
// If there is more than one CRLDistributionPoint, returns an error.
//
// Assumes the shard number is represented in the URL as an integer that
// occurs in the last path component, optionally followed by ".crl".
//
// Note: This assumes (a) the CA is generating well-formed, correct
// CRLDistributionPoints and (b) an earlier component has verified the signature
// on this certificate comes from one of our issuers.
func crlShard(cert *x509.Certificate) (int64, error) {
	if len(cert.CRLDistributionPoints) == 0 {
		return 0, nil
	}
	if len(cert.CRLDistributionPoints) > 1 {
		return 0, errors.New("too many crlDistributionPoints in certificate")
	}

	url := strings.TrimSuffix(cert.CRLDistributionPoints[0], ".crl")
	lastIndex := strings.LastIndex(url, "/")
	if lastIndex == -1 {
		return 0, fmt.Errorf("malformed CRLDistributionPoint %q", url)
	}
	shardStr := url[lastIndex+1:]
	shardIdx, err := strconv.Atoi(shardStr)
	if err != nil {
		return 0, fmt.Errorf("parsing CRLDistributionPoint: %s", err)
	}

	if shardIdx <= 0 {
		return 0, fmt.Errorf("invalid shard in CRLDistributionPoint: %d", shardIdx)
	}

	return int64(shardIdx), nil
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
		cert,
		revocation.Reason(ocsp.KeyCompromise),
	)

	// Failing to add the key to the blocked keys list is a worse failure than
	// failing to revoke in the first place, because it means that
	// bad-key-revoker won't revoke the cert anyway.
	err = ra.addToBlockedKeys(ctx, cert.PublicKey, "API", "")
	if err != nil {
		return nil, err
	}

	issuerID := issuance.IssuerNameID(cert)

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
		err = ra.updateRevocationForKeyCompromise(ctx, core.SerialToString(cert.SerialNumber), issuerID)

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
// method is only called from the `admin` tool. It trusts that the admin
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
	if req.CrlShard != 0 && !req.Malformed {
		return nil, errors.New("non-zero CRLShard is only allowed for malformed certificates (shard is automatic for well formed certificates)")
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
		CRLShard:     req.CrlShard,
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
	var shard int64
	if req.Cert != nil {
		// If the incoming request includes a certificate body, just use that and
		// avoid doing any database queries. This code path is deprecated and will
		// be removed when req.Cert is removed.
		cert, err = x509.ParseCertificate(req.Cert)
		if err != nil {
			return nil, err
		}
		issuerID = issuance.IssuerNameID(cert)
		shard, err = crlShard(cert)
		if err != nil {
			return nil, err
		}
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
		shard, err = crlShard(cert)
		if err != nil {
			return nil, err
		}
	} else {
		// But if the cert is malformed, we at least still need its IssuerID.
		var status *corepb.CertificateStatus
		status, err = ra.SA.GetCertificateStatus(ctx, &sapb.Serial{Serial: req.Serial})
		if err != nil {
			return nil, fmt.Errorf("unable to confirm that serial %q was ever issued: %w", req.Serial, err)
		}
		issuerID = issuance.NameID(status.IssuerID)
		shard = req.CrlShard
	}

	_, err = ra.SA.RevokeCertificate(ctx, &sapb.RevokeCertificateRequest{
		Serial:   req.Serial,
		Reason:   req.Code,
		Date:     timestamppb.New(ra.clk.Now()),
		IssuerID: int64(issuerID),
		ShardIdx: shard,
	})
	// Perform an Akamai cache purge to handle occurrences of a client
	// successfully revoking a certificate, but the initial cache purge failing.
	if errors.Is(err, berrors.AlreadyRevoked) {
		if cert != nil {
			err = ra.purgeOCSPCache(ctx, cert, issuerID)
			if err != nil {
				err = fmt.Errorf("OCSP cache purge for already revoked serial %v failed: %w", req.Serial, err)
				return nil, err
			}
		}
	}
	if err != nil {
		if req.Code == ocsp.KeyCompromise && errors.Is(err, berrors.AlreadyRevoked) {
			err = ra.updateRevocationForKeyCompromise(ctx, req.Serial, issuerID)
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
			err = fmt.Errorf("OCSP cache purge for serial %v failed: %w", req.Serial, err)
			return nil, err
		}
	}

	return &emptypb.Empty{}, nil
}

// DeactivateRegistration deactivates a valid registration
func (ra *RegistrationAuthorityImpl) DeactivateRegistration(ctx context.Context, req *rapb.DeactivateRegistrationRequest) (*corepb.Registration, error) {
	if req == nil || req.RegistrationID == 0 {
		return nil, errIncompleteGRPCRequest
	}

	updatedAcct, err := ra.SA.DeactivateRegistration(ctx, &sapb.RegistrationID{Id: req.RegistrationID})
	if err != nil {
		return nil, err
	}

	return updatedAcct, nil
}

// DeactivateAuthorization deactivates a currently valid authorization
func (ra *RegistrationAuthorityImpl) DeactivateAuthorization(ctx context.Context, req *corepb.Authorization) (*emptypb.Empty, error) {
	ident := identifier.FromProto(req.Identifier)

	if core.IsAnyNilOrZero(req, req.Id, ident, req.Status, req.RegistrationID) {
		return nil, errIncompleteGRPCRequest
	}
	authzID, err := strconv.ParseInt(req.Id, 10, 64)
	if err != nil {
		return nil, err
	}
	if _, err := ra.SA.DeactivateAuthorization2(ctx, &sapb.AuthorizationID2{Id: authzID}); err != nil {
		return nil, err
	}
	if req.Status == string(core.StatusPending) {
		// Some clients deactivate pending authorizations without attempting them.
		// We're not sure exactly when this happens but it's most likely due to
		// internal errors in the client. From our perspective this uses storage
		// resources similar to how failed authorizations do, so we increment the
		// failed authorizations limit.
		err = ra.countFailedValidations(ctx, req.RegistrationID, ident)
		if err != nil {
			return nil, fmt.Errorf("failed to update rate limits: %w", err)
		}
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
		Reason:    int32(status.RevokedReason), //nolint: gosec // Revocation reasons are guaranteed to be small, no risk of overflow.
		RevokedAt: status.RevokedDate,
		IssuerID:  status.IssuerID,
	})
}

// NewOrder creates a new order object
func (ra *RegistrationAuthorityImpl) NewOrder(ctx context.Context, req *rapb.NewOrderRequest) (*corepb.Order, error) {
	if req == nil || req.RegistrationID == 0 {
		return nil, errIncompleteGRPCRequest
	}

	idents := identifier.Normalize(identifier.FromProtoSlice(req.Identifiers))

	profile, err := ra.profiles.get(req.CertificateProfileName)
	if err != nil {
		return nil, err
	}

	if profile.allowList != nil && !profile.allowList.Contains(req.RegistrationID) {
		return nil, berrors.UnauthorizedError("account ID %d is not permitted to use certificate profile %q",
			req.RegistrationID,
			req.CertificateProfileName,
		)
	}

	if len(idents) > profile.maxNames {
		return nil, berrors.MalformedError(
			"Order cannot contain more than %d identifiers", profile.maxNames)
	}

	for _, ident := range idents {
		if !slices.Contains(profile.identifierTypes, ident.Type) {
			return nil, berrors.RejectedIdentifierError("Profile %q does not permit %s type identifiers", req.CertificateProfileName, ident.Type)
		}
	}

	// Validate that our policy allows issuing for each of the identifiers in
	// the order
	err = ra.PA.WillingToIssue(idents)
	if err != nil {
		return nil, err
	}

	err = wildcardOverlap(idents)
	if err != nil {
		return nil, err
	}

	// See if there is an existing unexpired pending (or ready) order that can be reused
	// for this account
	existingOrder, err := ra.SA.GetOrderForNames(ctx, &sapb.GetOrderForNamesRequest{
		AcctID:      req.RegistrationID,
		Identifiers: idents.ToProtoSlice(),
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
		if core.IsAnyNilOrZero(existingOrder.Id, existingOrder.Status, existingOrder.RegistrationID, existingOrder.Identifiers, existingOrder.Created, existingOrder.Expires) {
			return nil, errIncompleteGRPCResponse
		}

		// Only re-use the order if the profile (even if it is just the empty
		// string, leaving us to choose a default profile) matches.
		if existingOrder.CertificateProfileName == req.CertificateProfileName {
			// Track how often we reuse an existing order and how old that order is.
			ra.orderAges.WithLabelValues("NewOrder").Observe(ra.clk.Since(existingOrder.Created.AsTime()).Seconds())
			return existingOrder, nil
		}
	}

	// An order's lifetime is effectively bound by the shortest remaining lifetime
	// of its associated authorizations. For that reason it would be Uncool if
	// `sa.GetAuthorizations` returned an authorization that was very close to
	// expiry. The resulting pending order that references it would itself end up
	// expiring very soon.
	// What is considered "very soon" scales with the associated order's lifetime,
	// up to a point.
	minTimeToExpiry := profile.orderLifetime / 8
	if minTimeToExpiry < time.Hour {
		minTimeToExpiry = time.Hour
	} else if minTimeToExpiry > 24*time.Hour {
		minTimeToExpiry = 24 * time.Hour
	}
	authzExpiryCutoff := ra.clk.Now().Add(minTimeToExpiry)

	var existingAuthz *sapb.Authorizations
	if features.Get().NoPendingAuthzReuse {
		getAuthReq := &sapb.GetValidAuthorizationsRequest{
			RegistrationID: req.RegistrationID,
			ValidUntil:     timestamppb.New(authzExpiryCutoff),
			Identifiers:    idents.ToProtoSlice(),
			Profile:        req.CertificateProfileName,
		}
		existingAuthz, err = ra.SA.GetValidAuthorizations2(ctx, getAuthReq)
	} else {
		getAuthReq := &sapb.GetAuthorizationsRequest{
			RegistrationID: req.RegistrationID,
			ValidUntil:     timestamppb.New(authzExpiryCutoff),
			Identifiers:    idents.ToProtoSlice(),
			Profile:        req.CertificateProfileName,
		}
		existingAuthz, err = ra.SA.GetAuthorizations2(ctx, getAuthReq)
	}
	if err != nil {
		return nil, err
	}

	identToExistingAuthz, err := bgrpc.PBToAuthzMap(existingAuthz)
	if err != nil {
		return nil, err
	}

	// For each of the identifiers in the order, if there is an acceptable
	// existing authz, append it to the order to reuse it. Otherwise track that
	// there is a missing authz for that identifier.
	var newOrderAuthzs []int64
	var missingAuthzIdents identifier.ACMEIdentifiers
	for _, ident := range idents {
		// If there isn't an existing authz, note that its missing and continue
		authz, exists := identToExistingAuthz[ident]
		if !exists {
			// The existing authz was not acceptable for reuse, and we need to
			// mark the name as requiring a new pending authz.
			missingAuthzIdents = append(missingAuthzIdents, ident)
			continue
		}

		// If the authz is associated with the wrong profile, don't reuse it.
		if authz.CertificateProfileName != req.CertificateProfileName {
			missingAuthzIdents = append(missingAuthzIdents, ident)
			// Delete the authz from the identToExistingAuthz map since we are not reusing it.
			delete(identToExistingAuthz, ident)
			continue
		}

		// This is only used for our metrics.
		authzAge := (profile.validAuthzLifetime - authz.Expires.Sub(ra.clk.Now())).Seconds()
		if authz.Status == core.StatusPending {
			authzAge = (profile.pendingAuthzLifetime - authz.Expires.Sub(ra.clk.Now())).Seconds()
		}

		// If the identifier is a wildcard DNS name, it must have exactly one
		// DNS-01 type challenge. The PA guarantees this at order creation time,
		// but we verify again to be safe.
		if ident.Type == identifier.TypeDNS && strings.HasPrefix(ident.Value, "*.") &&
			(len(authz.Challenges) != 1 || authz.Challenges[0].Type != core.ChallengeTypeDNS01) {
			return nil, berrors.InternalServerError(
				"SA.GetAuthorizations returned a DNS wildcard authz (%s) with invalid challenge(s)",
				authz.ID)
		}

		// If we reached this point then the existing authz was acceptable for
		// reuse.
		authzID, err := strconv.ParseInt(authz.ID, 10, 64)
		if err != nil {
			return nil, err
		}
		newOrderAuthzs = append(newOrderAuthzs, authzID)
		ra.authzAges.WithLabelValues("NewOrder", string(authz.Status)).Observe(authzAge)
	}

	// Loop through each of the identifiers missing authzs and create a new
	// pending authorization for each.
	var newAuthzs []*sapb.NewAuthzRequest
	for _, ident := range missingAuthzIdents {
		challTypes, err := ra.PA.ChallengeTypesFor(ident)
		if err != nil {
			return nil, err
		}

		var challStrs []string
		for _, t := range challTypes {
			challStrs = append(challStrs, string(t))
		}

		newAuthzs = append(newAuthzs, &sapb.NewAuthzRequest{
			Identifier:     ident.ToProto(),
			RegistrationID: req.RegistrationID,
			Expires:        timestamppb.New(ra.clk.Now().Add(profile.pendingAuthzLifetime).Truncate(time.Second)),
			ChallengeTypes: challStrs,
			Token:          core.NewToken(),
		})

		ra.authzAges.WithLabelValues("NewOrder", string(core.StatusPending)).Observe(0)
	}

	// Start with the order's own expiry as the minExpiry. We only care
	// about authz expiries that are sooner than the order's expiry
	minExpiry := ra.clk.Now().Add(profile.orderLifetime)

	// Check the reused authorizations to see if any have an expiry before the
	// minExpiry (the order's lifetime)
	for _, authz := range identToExistingAuthz {
		// An authz without an expiry is an unexpected internal server event
		if core.IsAnyNilOrZero(authz.Expires) {
			return nil, berrors.InternalServerError(
				"SA.GetAuthorizations returned an authz (%s) with zero expiry",
				authz.ID)
		}
		// If the reused authorization expires before the minExpiry, it's expiry
		// is the new minExpiry.
		if authz.Expires.Before(minExpiry) {
			minExpiry = *authz.Expires
		}
	}
	// If the newly created pending authz's have an expiry closer than the
	// minExpiry the minExpiry is the pending authz expiry.
	if len(newAuthzs) > 0 {
		newPendingAuthzExpires := ra.clk.Now().Add(profile.pendingAuthzLifetime)
		if newPendingAuthzExpires.Before(minExpiry) {
			minExpiry = newPendingAuthzExpires
		}
	}

	newOrder := &sapb.NewOrderRequest{
		RegistrationID:         req.RegistrationID,
		Identifiers:            idents.ToProtoSlice(),
		CertificateProfileName: req.CertificateProfileName,
		Replaces:               req.Replaces,
		ReplacesSerial:         req.ReplacesSerial,
		// Set the order's expiry to the minimum expiry. The db doesn't store
		// sub-second values, so truncate here.
		Expires:          timestamppb.New(minExpiry.Truncate(time.Second)),
		V2Authorizations: newOrderAuthzs,
	}
	newOrderAndAuthzsReq := &sapb.NewOrderAndAuthzsRequest{
		NewOrder:  newOrder,
		NewAuthzs: newAuthzs,
	}
	storedOrder, err := ra.SA.NewOrderAndAuthzs(ctx, newOrderAndAuthzsReq)
	if err != nil {
		return nil, err
	}

	if core.IsAnyNilOrZero(storedOrder.Id, storedOrder.Status, storedOrder.RegistrationID, storedOrder.Identifiers, storedOrder.Created, storedOrder.Expires) {
		return nil, errIncompleteGRPCResponse
	}
	ra.orderAges.WithLabelValues("NewOrder").Observe(0)

	// Note how many identifiers are being requested in this certificate order.
	ra.namesPerCert.With(prometheus.Labels{"type": "requested"}).Observe(float64(len(storedOrder.Identifiers)))

	return storedOrder, nil
}

// wildcardOverlap takes a slice of identifiers and returns an error if any of
// them is a non-wildcard FQDN that overlaps with a wildcard domain in the map.
func wildcardOverlap(idents identifier.ACMEIdentifiers) error {
	nameMap := make(map[string]bool, len(idents))
	for _, v := range idents {
		if v.Type == identifier.TypeDNS {
			nameMap[v.Value] = true
		}
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

// UnpauseAccount receives a validated account unpause request from the SFE and
// instructs the SA to unpause that account. If the account cannot be unpaused,
// an error is returned.
func (ra *RegistrationAuthorityImpl) UnpauseAccount(ctx context.Context, request *rapb.UnpauseAccountRequest) (*rapb.UnpauseAccountResponse, error) {
	if core.IsAnyNilOrZero(request.RegistrationID) {
		return nil, errIncompleteGRPCRequest
	}

	count, err := ra.SA.UnpauseAccount(ctx, &sapb.RegistrationID{
		Id: request.RegistrationID,
	})
	if err != nil {
		return nil, berrors.InternalServerError("failed to unpause account ID %d", request.RegistrationID)
	}

	return &rapb.UnpauseAccountResponse{Count: count.Count}, nil
}

func (ra *RegistrationAuthorityImpl) GetAuthorization(ctx context.Context, req *rapb.GetAuthorizationRequest) (*corepb.Authorization, error) {
	if core.IsAnyNilOrZero(req, req.Id) {
		return nil, errIncompleteGRPCRequest
	}

	authz, err := ra.SA.GetAuthorization2(ctx, &sapb.AuthorizationID2{Id: req.Id})
	if err != nil {
		return nil, fmt.Errorf("getting authz from SA: %w", err)
	}

	// Filter out any challenges which are currently disabled, so that the client
	// doesn't attempt them.
	challs := []*corepb.Challenge{}
	for _, chall := range authz.Challenges {
		if ra.PA.ChallengeTypeEnabled(core.AcmeChallenge(chall.Type)) {
			challs = append(challs, chall)
		}
	}

	authz.Challenges = challs
	return authz, nil
}

// AddRateLimitOverride dispatches an SA RPC to add a rate limit override to the
// database. If the override already exists, it will be updated. If the override
// does not exist, it will be inserted and enabled. If the override exists but
// has been disabled, it will be updated but not be re-enabled. The status of
// the override is returned in Enabled field of the response. To re-enable an
// override, use sa.EnableRateLimitOverride.
func (ra *RegistrationAuthorityImpl) AddRateLimitOverride(ctx context.Context, req *rapb.AddRateLimitOverrideRequest) (*rapb.AddRateLimitOverrideResponse, error) {
	if core.IsAnyNilOrZero(req, req.LimitEnum, req.BucketKey, req.Count, req.Burst, req.Period, req.Comment) {
		return nil, errIncompleteGRPCRequest
	}

	resp, err := ra.SA.AddRateLimitOverride(ctx, &sapb.AddRateLimitOverrideRequest{
		Override: &sapb.RateLimitOverride{
			LimitEnum: req.LimitEnum,
			BucketKey: req.BucketKey,
			Comment:   req.Comment,
			Period:    req.Period,
			Count:     req.Count,
			Burst:     req.Burst,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("adding rate limit override: %w", err)
	}

	return &rapb.AddRateLimitOverrideResponse{
		Inserted: resp.Inserted,
		Enabled:  resp.Enabled,
	}, nil
}

// Drain blocks until all detached goroutines are done.
//
// The RA runs detached goroutines for challenge validation and finalization,
// so that ACME responses can be returned to the user promptly while work continues.
//
// The main goroutine should call this before exiting to avoid canceling the work
// being done in detached goroutines.
func (ra *RegistrationAuthorityImpl) Drain() {
	ra.drainWG.Wait()
}
