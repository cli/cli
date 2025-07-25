package ratelimits

import (
	"errors"
	"fmt"
	"net/netip"
	"strconv"

	"github.com/letsencrypt/boulder/core"
	"github.com/letsencrypt/boulder/identifier"
)

// ErrInvalidCost indicates that the cost specified was < 0.
var ErrInvalidCost = fmt.Errorf("invalid cost, must be >= 0")

// ErrInvalidCostOverLimit indicates that the cost specified was > limit.Burst.
var ErrInvalidCostOverLimit = fmt.Errorf("invalid cost, must be <= limit.Burst")

// newIPAddressBucketKey validates and returns a bucketKey for limits that use
// the 'enum:ipAddress' bucket key format.
func newIPAddressBucketKey(name Name, ip netip.Addr) string { //nolint:unparam // Only one named rate limit uses this helper
	return joinWithColon(name.EnumString(), ip.String())
}

// newIPv6RangeCIDRBucketKey validates and returns a bucketKey for limits that
// use the 'enum:ipv6RangeCIDR' bucket key format.
func newIPv6RangeCIDRBucketKey(name Name, ip netip.Addr) (string, error) {
	if ip.Is4() {
		return "", fmt.Errorf("invalid IPv6 address, %q must be an IPv6 address", ip.String())
	}
	prefix, err := ip.Prefix(48)
	if err != nil {
		return "", fmt.Errorf("invalid IPv6 address, can't calculate prefix of %q: %s", ip.String(), err)
	}
	return joinWithColon(name.EnumString(), prefix.String()), nil
}

// newRegIdBucketKey validates and returns a bucketKey for limits that use the
// 'enum:regId' bucket key format.
func newRegIdBucketKey(name Name, regId int64) string {
	return joinWithColon(name.EnumString(), strconv.FormatInt(regId, 10))
}

// newDomainOrCIDRBucketKey validates and returns a bucketKey for limits that use
// the 'enum:domainOrCIDR' bucket key formats.
func newDomainOrCIDRBucketKey(name Name, domainOrCIDR string) string {
	return joinWithColon(name.EnumString(), domainOrCIDR)
}

// NewRegIdIdentValueBucketKey returns a bucketKey for limits that use the
// 'enum:regId:identValue' bucket key format. This function is exported for use
// by the RA when resetting the account pausing limit.
func NewRegIdIdentValueBucketKey(name Name, regId int64, orderIdent string) string {
	return joinWithColon(name.EnumString(), strconv.FormatInt(regId, 10), orderIdent)
}

// newFQDNSetBucketKey validates and returns a bucketKey for limits that use the
// 'enum:fqdnSet' bucket key format.
func newFQDNSetBucketKey(name Name, orderIdents identifier.ACMEIdentifiers) string { //nolint: unparam // Only one named rate limit uses this helper
	return joinWithColon(name.EnumString(), fmt.Sprintf("%x", core.HashIdentifiers(orderIdents)))
}

// Transaction represents a single rate limit operation. It includes a
// bucketKey, which combines the specific rate limit enum with a unique
// identifier to form the key where the state of the "bucket" can be referenced
// or stored by the Limiter, the rate limit being enforced, a cost which MUST be
// >= 0, and check/spend fields, which indicate how the Transaction should be
// processed. The following are acceptable combinations of check/spend:
//   - check-and-spend: when check and spend are both true, the cost will be
//     checked against the bucket's capacity and spent/refunded, when possible.
//   - check-only: when only check is true, the cost will be checked against the
//     bucket's capacity, but will never be spent/refunded.
//   - spend-only: when only spend is true, spending is best-effort. Regardless
//     of the bucket's capacity, the transaction will be considered "allowed".
//   - allow-only: when neither check nor spend are true, the transaction will
//     be considered "allowed" regardless of the bucket's capacity. This is
//     useful for limits that are disabled.
//
// The zero value of Transaction is an allow-only transaction and is valid even if
// it would fail validateTransaction (for instance because cost and burst are zero).
type Transaction struct {
	bucketKey string
	limit     *limit
	cost      int64
	check     bool
	spend     bool
}

func (txn Transaction) checkOnly() bool {
	return txn.check && !txn.spend
}

func (txn Transaction) spendOnly() bool {
	return txn.spend && !txn.check
}

func (txn Transaction) allowOnly() bool {
	return !txn.check && !txn.spend
}

func validateTransaction(txn Transaction) (Transaction, error) {
	if txn.cost < 0 {
		return Transaction{}, ErrInvalidCost
	}
	if txn.limit.burst == 0 {
		// This should never happen. If the limit was loaded from a file,
		// Burst was validated then. If this is a zero-valued Transaction
		// (that is, an allow-only transaction), then validateTransaction
		// shouldn't be called because zero-valued transactions are automatically
		// valid.
		return Transaction{}, fmt.Errorf("invalid limit, burst must be > 0")
	}
	if txn.cost > txn.limit.burst {
		return Transaction{}, ErrInvalidCostOverLimit
	}
	return txn, nil
}

func newTransaction(limit *limit, bucketKey string, cost int64) (Transaction, error) {
	return validateTransaction(Transaction{
		bucketKey: bucketKey,
		limit:     limit,
		cost:      cost,
		check:     true,
		spend:     true,
	})
}

func newCheckOnlyTransaction(limit *limit, bucketKey string, cost int64) (Transaction, error) {
	return validateTransaction(Transaction{
		bucketKey: bucketKey,
		limit:     limit,
		cost:      cost,
		check:     true,
	})
}

func newSpendOnlyTransaction(limit *limit, bucketKey string, cost int64) (Transaction, error) {
	return validateTransaction(Transaction{
		bucketKey: bucketKey,
		limit:     limit,
		cost:      cost,
		spend:     true,
	})
}

func newAllowOnlyTransaction() Transaction {
	// Zero values are sufficient.
	return Transaction{}
}

// TransactionBuilder is used to build Transactions for various rate limits.
// Each rate limit has a corresponding method that returns a Transaction for
// that limit. Call NewTransactionBuilder to create a new *TransactionBuilder.
type TransactionBuilder struct {
	*limitRegistry
}

// NewTransactionBuilderFromFiles returns a new *TransactionBuilder. The
// provided defaults and overrides paths are expected to be paths to YAML files
// that contain the default and override limits, respectively. Overrides is
// optional, defaults is required.
func NewTransactionBuilderFromFiles(defaults, overrides string) (*TransactionBuilder, error) {
	registry, err := newLimitRegistryFromFiles(defaults, overrides)
	if err != nil {
		return nil, err
	}
	return &TransactionBuilder{registry}, nil
}

// NewTransactionBuilder returns a new *TransactionBuilder. The provided
// defaults map is expected to contain default limit data. Overrides are not
// supported. Defaults is required.
func NewTransactionBuilder(defaults LimitConfigs) (*TransactionBuilder, error) {
	registry, err := newLimitRegistry(defaults, nil)
	if err != nil {
		return nil, err
	}
	return &TransactionBuilder{registry}, nil
}

// registrationsPerIPAddressTransaction returns a Transaction for the
// NewRegistrationsPerIPAddress limit for the provided IP address.
func (builder *TransactionBuilder) registrationsPerIPAddressTransaction(ip netip.Addr) (Transaction, error) {
	bucketKey := newIPAddressBucketKey(NewRegistrationsPerIPAddress, ip)
	limit, err := builder.getLimit(NewRegistrationsPerIPAddress, bucketKey)
	if err != nil {
		if errors.Is(err, errLimitDisabled) {
			return newAllowOnlyTransaction(), nil
		}
		return Transaction{}, err
	}
	return newTransaction(limit, bucketKey, 1)
}

// registrationsPerIPv6RangeTransaction returns a Transaction for the
// NewRegistrationsPerIPv6Range limit for the /48 IPv6 range which contains the
// provided IPv6 address.
func (builder *TransactionBuilder) registrationsPerIPv6RangeTransaction(ip netip.Addr) (Transaction, error) {
	bucketKey, err := newIPv6RangeCIDRBucketKey(NewRegistrationsPerIPv6Range, ip)
	if err != nil {
		return Transaction{}, err
	}
	limit, err := builder.getLimit(NewRegistrationsPerIPv6Range, bucketKey)
	if err != nil {
		if errors.Is(err, errLimitDisabled) {
			return newAllowOnlyTransaction(), nil
		}
		return Transaction{}, err
	}
	return newTransaction(limit, bucketKey, 1)
}

// ordersPerAccountTransaction returns a Transaction for the NewOrdersPerAccount
// limit for the provided ACME registration Id.
func (builder *TransactionBuilder) ordersPerAccountTransaction(regId int64) (Transaction, error) {
	bucketKey := newRegIdBucketKey(NewOrdersPerAccount, regId)
	limit, err := builder.getLimit(NewOrdersPerAccount, bucketKey)
	if err != nil {
		if errors.Is(err, errLimitDisabled) {
			return newAllowOnlyTransaction(), nil
		}
		return Transaction{}, err
	}
	return newTransaction(limit, bucketKey, 1)
}

// FailedAuthorizationsPerDomainPerAccountCheckOnlyTransactions returns a slice
// of Transactions for the provided order identifiers. An error is returned if
// any of the order identifiers' values are invalid. This method should be used
// for checking capacity, before allowing more authorizations to be created.
//
// Precondition: len(orderIdents) < maxNames.
func (builder *TransactionBuilder) FailedAuthorizationsPerDomainPerAccountCheckOnlyTransactions(regId int64, orderIdents identifier.ACMEIdentifiers) ([]Transaction, error) {
	// FailedAuthorizationsPerDomainPerAccount limit uses the 'enum:regId'
	// bucket key format for overrides.
	perAccountBucketKey := newRegIdBucketKey(FailedAuthorizationsPerDomainPerAccount, regId)
	limit, err := builder.getLimit(FailedAuthorizationsPerDomainPerAccount, perAccountBucketKey)
	if err != nil {
		if errors.Is(err, errLimitDisabled) {
			return []Transaction{newAllowOnlyTransaction()}, nil
		}
		return nil, err
	}

	var txns []Transaction
	for _, ident := range orderIdents {
		// FailedAuthorizationsPerDomainPerAccount limit uses the
		// 'enum:regId:identValue' bucket key format for transactions.
		perIdentValuePerAccountBucketKey := NewRegIdIdentValueBucketKey(FailedAuthorizationsPerDomainPerAccount, regId, ident.Value)

		// Add a check-only transaction for each per identValue per account
		// bucket.
		txn, err := newCheckOnlyTransaction(limit, perIdentValuePerAccountBucketKey, 1)
		if err != nil {
			return nil, err
		}
		txns = append(txns, txn)
	}
	return txns, nil
}

// FailedAuthorizationsPerDomainPerAccountSpendOnlyTransaction returns a spend-
// only Transaction for the provided order identifier. An error is returned if
// the order identifier's value is invalid. This method should be used for
// spending capacity, as a result of a failed authorization.
func (builder *TransactionBuilder) FailedAuthorizationsPerDomainPerAccountSpendOnlyTransaction(regId int64, orderIdent identifier.ACMEIdentifier) (Transaction, error) {
	// FailedAuthorizationsPerDomainPerAccount limit uses the 'enum:regId'
	// bucket key format for overrides.
	perAccountBucketKey := newRegIdBucketKey(FailedAuthorizationsPerDomainPerAccount, regId)
	limit, err := builder.getLimit(FailedAuthorizationsPerDomainPerAccount, perAccountBucketKey)
	if err != nil {
		if errors.Is(err, errLimitDisabled) {
			return newAllowOnlyTransaction(), nil
		}
		return Transaction{}, err
	}

	// FailedAuthorizationsPerDomainPerAccount limit uses the
	// 'enum:regId:identValue' bucket key format for transactions.
	perIdentValuePerAccountBucketKey := NewRegIdIdentValueBucketKey(FailedAuthorizationsPerDomainPerAccount, regId, orderIdent.Value)
	txn, err := newSpendOnlyTransaction(limit, perIdentValuePerAccountBucketKey, 1)
	if err != nil {
		return Transaction{}, err
	}

	return txn, nil
}

// FailedAuthorizationsForPausingPerDomainPerAccountTransaction returns a
// Transaction for the provided order identifier. An error is returned if the
// order identifier's value is invalid. This method should be used for spending
// capacity, as a result of a failed authorization.
func (builder *TransactionBuilder) FailedAuthorizationsForPausingPerDomainPerAccountTransaction(regId int64, orderIdent identifier.ACMEIdentifier) (Transaction, error) {
	// FailedAuthorizationsForPausingPerDomainPerAccount limit uses the 'enum:regId'
	// bucket key format for overrides.
	perAccountBucketKey := newRegIdBucketKey(FailedAuthorizationsForPausingPerDomainPerAccount, regId)
	limit, err := builder.getLimit(FailedAuthorizationsForPausingPerDomainPerAccount, perAccountBucketKey)
	if err != nil {
		if errors.Is(err, errLimitDisabled) {
			return newAllowOnlyTransaction(), nil
		}
		return Transaction{}, err
	}

	// FailedAuthorizationsForPausingPerDomainPerAccount limit uses the
	// 'enum:regId:identValue' bucket key format for transactions.
	perIdentValuePerAccountBucketKey := NewRegIdIdentValueBucketKey(FailedAuthorizationsForPausingPerDomainPerAccount, regId, orderIdent.Value)
	txn, err := newTransaction(limit, perIdentValuePerAccountBucketKey, 1)
	if err != nil {
		return Transaction{}, err
	}

	return txn, nil
}

// certificatesPerDomainCheckOnlyTransactions returns a slice of Transactions
// for the provided order identifiers. It returns an error if any of the order
// identifiers' values are invalid. This method should be used for checking
// capacity, before allowing more orders to be created. If a
// CertificatesPerDomainPerAccount override is active, a check-only Transaction
// is created for each per account per domainOrCIDR bucket. Otherwise, a
// check-only Transaction is generated for each global per domainOrCIDR bucket.
// This method should be used for checking capacity, before allowing more orders
// to be created.
//
// Precondition: All orderIdents must comply with policy.WellFormedIdentifiers.
func (builder *TransactionBuilder) certificatesPerDomainCheckOnlyTransactions(regId int64, orderIdents identifier.ACMEIdentifiers) ([]Transaction, error) {
	if len(orderIdents) > 100 {
		return nil, fmt.Errorf("unwilling to process more than 100 rate limit transactions, got %d", len(orderIdents))
	}

	perAccountLimitBucketKey := newRegIdBucketKey(CertificatesPerDomainPerAccount, regId)
	accountOverride := true
	perAccountLimit, err := builder.getLimit(CertificatesPerDomainPerAccount, perAccountLimitBucketKey)
	if err != nil {
		// The CertificatesPerDomainPerAccount limit never has a default. If there is an override for it,
		// the above call will return the override. But if there is none, it will return errLimitDisabled.
		// In that case we want to continue, but make sure we don't reference `perAccountLimit` because it
		// is not a valid limit.
		if errors.Is(err, errLimitDisabled) {
			accountOverride = false
		} else {
			return nil, err
		}
	}

	coveringIdents, err := coveringIdentifiers(orderIdents)
	if err != nil {
		return nil, err
	}

	var txns []Transaction
	for _, ident := range coveringIdents {
		perDomainOrCIDRBucketKey := newDomainOrCIDRBucketKey(CertificatesPerDomain, ident)
		if accountOverride {
			if !perAccountLimit.isOverride {
				return nil, fmt.Errorf("shouldn't happen: CertificatesPerDomainPerAccount limit is not an override")
			}
			perAccountPerDomainOrCIDRBucketKey := NewRegIdIdentValueBucketKey(CertificatesPerDomainPerAccount, regId, ident)
			// Add a check-only transaction for each per account per identValue
			// bucket.
			txn, err := newCheckOnlyTransaction(perAccountLimit, perAccountPerDomainOrCIDRBucketKey, 1)
			if err != nil {
				if errors.Is(err, errLimitDisabled) {
					continue
				}
				return nil, err
			}
			txns = append(txns, txn)
		} else {
			// Use the per domainOrCIDR bucket key when no per account per
			// domainOrCIDR override is configured.
			perDomainOrCIDRLimit, err := builder.getLimit(CertificatesPerDomain, perDomainOrCIDRBucketKey)
			if err != nil {
				if errors.Is(err, errLimitDisabled) {
					continue
				}
				return nil, err
			}
			// Add a check-only transaction for each per domainOrCIDR bucket.
			txn, err := newCheckOnlyTransaction(perDomainOrCIDRLimit, perDomainOrCIDRBucketKey, 1)
			if err != nil {
				return nil, err
			}
			txns = append(txns, txn)
		}
	}
	return txns, nil
}

// CertificatesPerDomainSpendOnlyTransactions returns a slice of Transactions
// for the provided order identifiers. It returns an error if any of the order
// identifiers' values are invalid. If a CertificatesPerDomainPerAccount
// override is configured, it generates two types of Transactions:
//   - A spend-only Transaction for each per-account, per-domainOrCIDR bucket,
//     which enforces the limit on certificates issued per domainOrCIDR for
//     each account.
//   - A spend-only Transaction for each per-domainOrCIDR bucket, which
//     enforces the global limit on certificates issued per domainOrCIDR.
//
// If no CertificatesPerDomainPerAccount override is present, it returns a
// spend-only Transaction for each global per-domainOrCIDR bucket. This method
// should be used for spending capacity, when a certificate is issued.
//
// Precondition: orderIdents must all pass policy.WellFormedIdentifiers.
func (builder *TransactionBuilder) CertificatesPerDomainSpendOnlyTransactions(regId int64, orderIdents identifier.ACMEIdentifiers) ([]Transaction, error) {
	if len(orderIdents) > 100 {
		return nil, fmt.Errorf("unwilling to process more than 100 rate limit transactions, got %d", len(orderIdents))
	}

	perAccountLimitBucketKey := newRegIdBucketKey(CertificatesPerDomainPerAccount, regId)
	accountOverride := true
	perAccountLimit, err := builder.getLimit(CertificatesPerDomainPerAccount, perAccountLimitBucketKey)
	if err != nil {
		// The CertificatesPerDomainPerAccount limit never has a default. If there is an override for it,
		// the above call will return the override. But if there is none, it will return errLimitDisabled.
		// In that case we want to continue, but make sure we don't reference `perAccountLimit` because it
		// is not a valid limit.
		if errors.Is(err, errLimitDisabled) {
			accountOverride = false
		} else {
			return nil, err
		}
	}

	coveringIdents, err := coveringIdentifiers(orderIdents)
	if err != nil {
		return nil, err
	}

	var txns []Transaction
	for _, ident := range coveringIdents {
		perDomainOrCIDRBucketKey := newDomainOrCIDRBucketKey(CertificatesPerDomain, ident)
		if accountOverride {
			if !perAccountLimit.isOverride {
				return nil, fmt.Errorf("shouldn't happen: CertificatesPerDomainPerAccount limit is not an override")
			}
			perAccountPerDomainOrCIDRBucketKey := NewRegIdIdentValueBucketKey(CertificatesPerDomainPerAccount, regId, ident)
			// Add a spend-only transaction for each per account per
			// domainOrCIDR bucket.
			txn, err := newSpendOnlyTransaction(perAccountLimit, perAccountPerDomainOrCIDRBucketKey, 1)
			if err != nil {
				return nil, err
			}
			txns = append(txns, txn)

			perDomainOrCIDRLimit, err := builder.getLimit(CertificatesPerDomain, perDomainOrCIDRBucketKey)
			if err != nil {
				if errors.Is(err, errLimitDisabled) {
					continue
				}
				return nil, err
			}

			// Add a spend-only transaction for each per domainOrCIDR bucket.
			txn, err = newSpendOnlyTransaction(perDomainOrCIDRLimit, perDomainOrCIDRBucketKey, 1)
			if err != nil {
				return nil, err
			}
			txns = append(txns, txn)
		} else {
			// Use the per domainOrCIDR bucket key when no per account per
			// domainOrCIDR override is configured.
			perDomainOrCIDRLimit, err := builder.getLimit(CertificatesPerDomain, perDomainOrCIDRBucketKey)
			if err != nil {
				if errors.Is(err, errLimitDisabled) {
					continue
				}
				return nil, err
			}
			// Add a spend-only transaction for each per domainOrCIDR bucket.
			txn, err := newSpendOnlyTransaction(perDomainOrCIDRLimit, perDomainOrCIDRBucketKey, 1)
			if err != nil {
				return nil, err
			}
			txns = append(txns, txn)
		}
	}
	return txns, nil
}

// certificatesPerFQDNSetCheckOnlyTransaction returns a check-only Transaction
// for the provided order identifiers. This method should only be used for
// checking capacity, before allowing more orders to be created.
func (builder *TransactionBuilder) certificatesPerFQDNSetCheckOnlyTransaction(orderIdents identifier.ACMEIdentifiers) (Transaction, error) {
	bucketKey := newFQDNSetBucketKey(CertificatesPerFQDNSet, orderIdents)
	limit, err := builder.getLimit(CertificatesPerFQDNSet, bucketKey)
	if err != nil {
		if errors.Is(err, errLimitDisabled) {
			return newAllowOnlyTransaction(), nil
		}
		return Transaction{}, err
	}
	return newCheckOnlyTransaction(limit, bucketKey, 1)
}

// CertificatesPerFQDNSetSpendOnlyTransaction returns a spend-only Transaction
// for the provided order identifiers. This method should only be used for
// spending capacity, when a certificate is issued.
func (builder *TransactionBuilder) CertificatesPerFQDNSetSpendOnlyTransaction(orderIdents identifier.ACMEIdentifiers) (Transaction, error) {
	bucketKey := newFQDNSetBucketKey(CertificatesPerFQDNSet, orderIdents)
	limit, err := builder.getLimit(CertificatesPerFQDNSet, bucketKey)
	if err != nil {
		if errors.Is(err, errLimitDisabled) {
			return newAllowOnlyTransaction(), nil
		}
		return Transaction{}, err
	}
	return newSpendOnlyTransaction(limit, bucketKey, 1)
}

// NewOrderLimitTransactions takes in values from a new-order request and
// returns the set of rate limit transactions that should be evaluated before
// allowing the request to proceed.
//
// Precondition: idents must be a list of identifiers that all pass
// policy.WellFormedIdentifiers.
func (builder *TransactionBuilder) NewOrderLimitTransactions(regId int64, idents identifier.ACMEIdentifiers, isRenewal bool) ([]Transaction, error) {
	makeTxnError := func(err error, limit Name) error {
		return fmt.Errorf("error constructing rate limit transaction for %s rate limit: %w", limit, err)
	}

	var transactions []Transaction
	if !isRenewal {
		txn, err := builder.ordersPerAccountTransaction(regId)
		if err != nil {
			return nil, makeTxnError(err, NewOrdersPerAccount)
		}
		transactions = append(transactions, txn)
	}

	txns, err := builder.FailedAuthorizationsPerDomainPerAccountCheckOnlyTransactions(regId, idents)
	if err != nil {
		return nil, makeTxnError(err, FailedAuthorizationsPerDomainPerAccount)
	}
	transactions = append(transactions, txns...)

	if !isRenewal {
		txns, err := builder.certificatesPerDomainCheckOnlyTransactions(regId, idents)
		if err != nil {
			return nil, makeTxnError(err, CertificatesPerDomain)
		}
		transactions = append(transactions, txns...)
	}

	txn, err := builder.certificatesPerFQDNSetCheckOnlyTransaction(idents)
	if err != nil {
		return nil, makeTxnError(err, CertificatesPerFQDNSet)
	}
	return append(transactions, txn), nil
}

// NewAccountLimitTransactions takes in an IP address from a new-account request
// and returns the set of rate limit transactions that should be evaluated
// before allowing the request to proceed.
func (builder *TransactionBuilder) NewAccountLimitTransactions(ip netip.Addr) ([]Transaction, error) {
	makeTxnError := func(err error, limit Name) error {
		return fmt.Errorf("error constructing rate limit transaction for %s rate limit: %w", limit, err)
	}

	var transactions []Transaction
	txn, err := builder.registrationsPerIPAddressTransaction(ip)
	if err != nil {
		return nil, makeTxnError(err, NewRegistrationsPerIPAddress)
	}
	transactions = append(transactions, txn)

	if ip.Is4() {
		// This request was made from an IPv4 address.
		return transactions, nil
	}

	txn, err = builder.registrationsPerIPv6RangeTransaction(ip)
	if err != nil {
		return nil, makeTxnError(err, NewRegistrationsPerIPv6Range)
	}
	return append(transactions, txn), nil
}
