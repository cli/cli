package ratelimit

import (
	"strconv"
	"time"

	"github.com/letsencrypt/boulder/config"
	"github.com/letsencrypt/boulder/strictyaml"
)

const (
	// CertificatesPerName is the name of the CertificatesPerName rate limit
	// when referenced in metric labels.
	CertificatesPerName = "certificates_per_domain"

	// RegistrationsPerIP is the name of the RegistrationsPerIP rate limit when
	// referenced in metric labels.
	RegistrationsPerIP = "registrations_per_ip"

	// RegistrationsPerIPRange is the name of the RegistrationsPerIPRange rate
	// limit when referenced in metric labels.
	RegistrationsPerIPRange = "registrations_per_ipv6_range"

	// PendingAuthorizationsPerAccount is the name of the
	// PendingAuthorizationsPerAccount rate limit when referenced in metric
	// labels.
	PendingAuthorizationsPerAccount = "pending_authorizations_per_account"

	// InvalidAuthorizationsPerAccount is the name of the
	// InvalidAuthorizationsPerAccount rate limit when referenced in metric
	// labels.
	InvalidAuthorizationsPerAccount = "failed_authorizations_per_account"

	// CertificatesPerFQDNSet is the name of the CertificatesPerFQDNSet rate
	// limit when referenced in metric labels.
	CertificatesPerFQDNSet = "certificates_per_fqdn_set"

	// CertificatesPerFQDNSetFast is the name of the CertificatesPerFQDNSetFast
	// rate limit when referenced in metric labels.
	CertificatesPerFQDNSetFast = "certificates_per_fqdn_set_fast"

	// NewOrdersPerAccount is the name of the NewOrdersPerAccount rate limit
	// when referenced in metric labels.
	NewOrdersPerAccount = "new_orders_per_account"
)

// Limits is defined to allow mock implementations be provided during unit
// testing
type Limits interface {
	CertificatesPerName() RateLimitPolicy
	RegistrationsPerIP() RateLimitPolicy
	RegistrationsPerIPRange() RateLimitPolicy
	PendingAuthorizationsPerAccount() RateLimitPolicy
	InvalidAuthorizationsPerAccount() RateLimitPolicy
	CertificatesPerFQDNSet() RateLimitPolicy
	CertificatesPerFQDNSetFast() RateLimitPolicy
	NewOrdersPerAccount() RateLimitPolicy
	LoadPolicies(contents []byte) error
}

// limitsImpl is an unexported implementation of the Limits interface. It acts
// as a container for a rateLimitConfig.
type limitsImpl struct {
	rlPolicy *rateLimitConfig
}

func (r *limitsImpl) CertificatesPerName() RateLimitPolicy {
	if r.rlPolicy == nil {
		return RateLimitPolicy{}
	}
	return r.rlPolicy.CertificatesPerName
}

func (r *limitsImpl) RegistrationsPerIP() RateLimitPolicy {
	if r.rlPolicy == nil {
		return RateLimitPolicy{}
	}
	return r.rlPolicy.RegistrationsPerIP
}

func (r *limitsImpl) RegistrationsPerIPRange() RateLimitPolicy {
	if r.rlPolicy == nil {
		return RateLimitPolicy{}
	}
	return r.rlPolicy.RegistrationsPerIPRange
}

func (r *limitsImpl) PendingAuthorizationsPerAccount() RateLimitPolicy {
	if r.rlPolicy == nil {
		return RateLimitPolicy{}
	}
	return r.rlPolicy.PendingAuthorizationsPerAccount
}

func (r *limitsImpl) InvalidAuthorizationsPerAccount() RateLimitPolicy {
	if r.rlPolicy == nil {
		return RateLimitPolicy{}
	}
	return r.rlPolicy.InvalidAuthorizationsPerAccount
}

func (r *limitsImpl) CertificatesPerFQDNSet() RateLimitPolicy {
	if r.rlPolicy == nil {
		return RateLimitPolicy{}
	}
	return r.rlPolicy.CertificatesPerFQDNSet
}

func (r *limitsImpl) CertificatesPerFQDNSetFast() RateLimitPolicy {
	if r.rlPolicy == nil {
		return RateLimitPolicy{}
	}
	return r.rlPolicy.CertificatesPerFQDNSetFast
}

func (r *limitsImpl) NewOrdersPerAccount() RateLimitPolicy {
	if r.rlPolicy == nil {
		return RateLimitPolicy{}
	}
	return r.rlPolicy.NewOrdersPerAccount
}

// LoadPolicies loads various rate limiting policies from a byte array of
// YAML configuration.
func (r *limitsImpl) LoadPolicies(contents []byte) error {
	var newPolicy rateLimitConfig
	err := strictyaml.Unmarshal(contents, &newPolicy)
	if err != nil {
		return err
	}
	r.rlPolicy = &newPolicy
	return nil
}

func New() Limits {
	return &limitsImpl{}
}

// rateLimitConfig contains all application layer rate limiting policies. It is
// unexported and clients are expected to use the exported container struct
type rateLimitConfig struct {
	// Number of certificates that can be extant containing any given name.
	// These are counted by "base domain" aka eTLD+1, so any entries in the
	// overrides section must be an eTLD+1 according to the publicsuffix package.
	CertificatesPerName RateLimitPolicy `yaml:"certificatesPerName"`
	// Number of registrations that can be created per IP.
	// Note: Since this is checked before a registration is created, setting a
	// RegistrationOverride on it has no effect.
	RegistrationsPerIP RateLimitPolicy `yaml:"registrationsPerIP"`
	// Number of registrations that can be created per fuzzy IP range. Unlike
	// RegistrationsPerIP this will apply to a /48 for IPv6 addresses to help curb
	// abuse from easily obtained IPv6 ranges.
	// Note: Like RegistrationsPerIP, setting a RegistrationOverride has no
	// effect here.
	RegistrationsPerIPRange RateLimitPolicy `yaml:"registrationsPerIPRange"`
	// Number of pending authorizations that can exist per account. Overrides by
	// key are not applied, but overrides by registration are.
	PendingAuthorizationsPerAccount RateLimitPolicy `yaml:"pendingAuthorizationsPerAccount"`
	// Number of invalid authorizations that can be failed per account within the
	// given window. Overrides by key are not applied, but overrides by registration are.
	// Note that this limit is actually "per account, per hostname," but that
	// is too long for the variable name.
	InvalidAuthorizationsPerAccount RateLimitPolicy `yaml:"invalidAuthorizationsPerAccount"`
	// Number of new orders that can be created per account within the given
	// window. Overrides by key are not applied, but overrides by registration are.
	NewOrdersPerAccount RateLimitPolicy `yaml:"newOrdersPerAccount"`
	// Number of certificates that can be extant containing a specific set
	// of DNS names.
	CertificatesPerFQDNSet RateLimitPolicy `yaml:"certificatesPerFQDNSet"`
	// Same as above, but intended to both trigger and reset faster (i.e. a
	// lower threshold and smaller window), so that clients don't have to wait
	// a long time after a small burst of accidental duplicate issuance.
	CertificatesPerFQDNSetFast RateLimitPolicy `yaml:"certificatesPerFQDNSetFast"`
}

// RateLimitPolicy describes a general limiting policy
type RateLimitPolicy struct {
	// How long to count items for
	Window config.Duration `yaml:"window"`
	// The max number of items that can be present before triggering the rate
	// limit. Zero means "no limit."
	Threshold int64 `yaml:"threshold"`
	// A per-key override setting different limits than the default (higher or lower).
	// The key is defined on a per-limit basis and should match the key it counts on.
	// For instance, a rate limit on the number of certificates per name uses name as
	// a key, while a rate limit on the number of registrations per IP subnet would
	// use subnet as a key. Note that a zero entry in the overrides map does not
	// mean "no limit," it means a limit of zero. An entry of -1 means
	// "no limit", only for the pending authorizations rate limit.
	Overrides map[string]int64 `yaml:"overrides"`
	// A per-registration override setting. This can be used, e.g. if there are
	// hosting providers that we would like to grant a higher rate of issuance
	// than the default. If both key-based and registration-based overrides are
	// available, whichever is larger takes priority. Note that a zero entry in
	// the overrides map does not mean "no limit", it means a limit of zero.
	RegistrationOverrides map[int64]int64 `yaml:"registrationOverrides"`
}

// Enabled returns true iff the RateLimitPolicy is enabled.
func (rlp *RateLimitPolicy) Enabled() bool {
	return rlp.Threshold != 0
}

// GetThreshold returns the threshold for this rate limit and the override
// Id/Key if that threshold is the result of an override for the default limit,
// empty-string otherwise. The threshold returned takes into account any
// overrides for `key` or `regID`. If both `key` and `regID` have an override
// the largest of the two will be used.
func (rlp *RateLimitPolicy) GetThreshold(key string, regID int64) (int64, string) {
	regOverride, regOverrideExists := rlp.RegistrationOverrides[regID]
	keyOverride, keyOverrideExists := rlp.Overrides[key]

	if regOverrideExists && !keyOverrideExists {
		// If there is a regOverride and no keyOverride use the regOverride
		return regOverride, strconv.FormatInt(regID, 10)
	} else if !regOverrideExists && keyOverrideExists {
		// If there is a keyOverride and no regOverride use the keyOverride
		return keyOverride, key
	} else if regOverrideExists && keyOverrideExists {
		// If there is both a regOverride and a keyOverride use whichever is larger.
		if regOverride > keyOverride {
			return regOverride, strconv.FormatInt(regID, 10)
		} else {
			return keyOverride, key
		}
	}

	// Otherwise there was no regOverride and no keyOverride, use the base
	// Threshold
	return rlp.Threshold, ""
}

// WindowBegin returns the time that a RateLimitPolicy's window begins, given a
// particular end time (typically the current time).
func (rlp *RateLimitPolicy) WindowBegin(windowEnd time.Time) time.Time {
	return windowEnd.Add(-1 * rlp.Window.Duration)
}
