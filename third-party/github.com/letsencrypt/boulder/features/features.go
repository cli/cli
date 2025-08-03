// features provides the Config struct, which is used to define feature flags
// that can affect behavior across Boulder components. It also maintains a
// global singleton Config which can be referenced by arbitrary Boulder code
// without having to pass a collection of feature flags through the function
// call graph.
package features

import (
	"sync"
)

// Config contains one boolean field for every Boulder feature flag. It can be
// included directly in an executable's Config struct to have feature flags be
// automatically parsed by the json config loader; executables that do so must
// then call features.Set(parsedConfig) to load the parsed struct into this
// package's global Config.
type Config struct {
	// Deprecated flags.
	IncrementRateLimits         bool
	UseKvLimitsForNewOrder      bool
	DisableLegacyLimitWrites    bool
	MultipleCertificateProfiles bool
	InsertAuthzsIndividually    bool
	EnforceMultiCAA             bool
	EnforceMPIC                 bool
	MPICFullResults             bool
	UnsplitIssuance             bool
	ExpirationMailerUsesJoin    bool
	DOH                         bool
	IgnoreAccountContacts       bool

	// ServeRenewalInfo exposes the renewalInfo endpoint in the directory and for
	// GET requests. WARNING: This feature is a draft and highly unstable.
	ServeRenewalInfo bool

	// CertCheckerChecksValidations enables an extra query for each certificate
	// checked, to find the relevant authzs. Since this query might be
	// expensive, we gate it behind a feature flag.
	CertCheckerChecksValidations bool

	// CertCheckerRequiresValidations causes cert-checker to fail if the
	// query enabled by CertCheckerChecksValidations didn't find corresponding
	// authorizations.
	CertCheckerRequiresValidations bool

	// AsyncFinalize enables the RA to return approximately immediately from
	// requests to finalize orders. This allows us to take longer getting SCTs,
	// issuing certs, and updating the database; it indirectly reduces the number
	// of issuances that fail due to timeouts during storage. However, it also
	// requires clients to properly implement polling the Order object to wait
	// for the cert URL to appear.
	AsyncFinalize bool

	// CheckIdentifiersPaused checks if any of the identifiers in the order are
	// currently paused at NewOrder time. If any are paused, an error is
	// returned to the Subscriber indicating that the order cannot be processed
	// until the paused identifiers are unpaused and the order is resubmitted.
	CheckIdentifiersPaused bool

	// PropagateCancels controls whether the WFE and ocsp-responder allows
	// cancellation of an inbound request to cancel downstream gRPC and other
	// queries. In practice, cancellation of an inbound request is achieved by
	// Nginx closing the connection on which the request was happening. This may
	// help shed load in overcapacity situations. However, note that in-progress
	// database queries (for instance, in the SA) are not cancelled. Database
	// queries waiting for an available connection may be cancelled.
	PropagateCancels bool

	// AutomaticallyPauseZombieClients configures the RA to automatically track
	// and pause issuance for each (account, hostname) pair that repeatedly
	// fails validation.
	AutomaticallyPauseZombieClients bool

	// NoPendingAuthzReuse causes the RA to only select already-validated authzs
	// to attach to a newly created order. This preserves important client-facing
	// functionality (valid authz reuse) while letting us simplify our code by
	// removing pending authz reuse.
	NoPendingAuthzReuse bool

	// StoreARIReplacesInOrders causes the SA to store and retrieve the optional
	// ARI replaces field in the orders table.
	StoreARIReplacesInOrders bool
}

var fMu = new(sync.RWMutex)
var global = Config{}

// Set changes the global FeatureSet to match the input FeatureSet. This
// overrides any previous changes made to the global FeatureSet.
//
// When used in tests, the caller must defer features.Reset() to avoid leaving
// dirty global state.
func Set(fs Config) {
	fMu.Lock()
	defer fMu.Unlock()
	// If the FeatureSet type ever changes, this must be updated to still copy
	// the input argument, never hold a reference to it.
	global = fs
}

// Reset resets all features to their initial state (false).
func Reset() {
	fMu.Lock()
	defer fMu.Unlock()
	global = Config{}
}

// Get returns a copy of the current global FeatureSet, indicating which
// features are currently enabled (set to true). Expected caller behavior looks
// like:
//
//	if features.Get().FeatureName { ...
func Get() Config {
	fMu.RLock()
	defer fMu.RUnlock()
	// If the FeatureSet type ever changes, this must be updated to still return
	// only a copy of the current state, never a reference directly to it.
	return global
}
