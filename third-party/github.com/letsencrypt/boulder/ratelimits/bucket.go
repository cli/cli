package ratelimits

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/letsencrypt/boulder/core"
)

// ErrInvalidCost indicates that the cost specified was < 0.
var ErrInvalidCost = fmt.Errorf("invalid cost, must be >= 0")

// ErrInvalidCostOverLimit indicates that the cost specified was > limit.Burst.
var ErrInvalidCostOverLimit = fmt.Errorf("invalid cost, must be <= limit.Burst")

// newIPAddressBucketKey validates and returns a bucketKey for limits that use
// the 'enum:ipAddress' bucket key format.
func newIPAddressBucketKey(name Name, ip net.IP) (string, error) { //nolint: unparam
	id := ip.String()
	err := validateIdForName(name, id)
	if err != nil {
		return "", err
	}
	return joinWithColon(name.EnumString(), id), nil
}

// newIPv6RangeCIDRBucketKey validates and returns a bucketKey for limits that
// use the 'enum:ipv6RangeCIDR' bucket key format.
func newIPv6RangeCIDRBucketKey(name Name, ip net.IP) (string, error) {
	if ip.To4() != nil {
		return "", fmt.Errorf("invalid IPv6 address, %q must be an IPv6 address", ip.String())
	}
	ipMask := net.CIDRMask(48, 128)
	ipNet := &net.IPNet{IP: ip.Mask(ipMask), Mask: ipMask}
	id := ipNet.String()
	err := validateIdForName(name, id)
	if err != nil {
		return "", err
	}
	return joinWithColon(name.EnumString(), id), nil
}

// newRegIdBucketKey validates and returns a bucketKey for limits that use the
// 'enum:regId' bucket key format.
func newRegIdBucketKey(name Name, regId int64) (string, error) {
	id := strconv.FormatInt(regId, 10)
	err := validateIdForName(name, id)
	if err != nil {
		return "", err
	}
	return joinWithColon(name.EnumString(), id), nil
}

// newDomainBucketKey validates and returns a bucketKey for limits that use the
// 'enum:domain' bucket key format.
func newDomainBucketKey(name Name, orderName string) (string, error) {
	err := validateIdForName(name, orderName)
	if err != nil {
		return "", err
	}
	return joinWithColon(name.EnumString(), orderName), nil
}

// newRegIdDomainBucketKey validates and returns a bucketKey for limits that use
// the 'enum:regId:domain' bucket key format.
func newRegIdDomainBucketKey(name Name, regId int64, orderName string) (string, error) {
	regIdStr := strconv.FormatInt(regId, 10)
	err := validateIdForName(name, joinWithColon(regIdStr, orderName))
	if err != nil {
		return "", err
	}
	return joinWithColon(name.EnumString(), regIdStr, orderName), nil
}

// newFQDNSetBucketKey validates and returns a bucketKey for limits that use the
// 'enum:fqdnSet' bucket key format.
func newFQDNSetBucketKey(name Name, orderNames []string) (string, error) { //nolint: unparam
	err := validateIdForName(name, strings.Join(orderNames, ","))
	if err != nil {
		return "", err
	}
	id := fmt.Sprintf("%x", core.HashNames(orderNames))
	return joinWithColon(name.EnumString(), id), nil
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
type Transaction struct {
	bucketKey string
	limit     limit
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
	if txn.cost > txn.limit.Burst {
		return Transaction{}, ErrInvalidCostOverLimit
	}
	return txn, nil
}

func newTransaction(limit limit, bucketKey string, cost int64) (Transaction, error) {
	return validateTransaction(Transaction{
		bucketKey: bucketKey,
		limit:     limit,
		cost:      cost,
		check:     true,
		spend:     true,
	})
}

func newCheckOnlyTransaction(limit limit, bucketKey string, cost int64) (Transaction, error) {
	return validateTransaction(Transaction{
		bucketKey: bucketKey,
		limit:     limit,
		cost:      cost,
		check:     true,
	})
}

func newSpendOnlyTransaction(limit limit, bucketKey string, cost int64) (Transaction, error) {
	return validateTransaction(Transaction{
		bucketKey: bucketKey,
		limit:     limit,
		cost:      cost,
		spend:     true,
	})
}

func newAllowOnlyTransaction() (Transaction, error) {
	// Zero values are sufficient.
	return validateTransaction(Transaction{})
}

// TransactionBuilder is used to build Transactions for various rate limits.
// Each rate limit has a corresponding method that returns a Transaction for
// that limit. Call NewTransactionBuilder to create a new *TransactionBuilder.
type TransactionBuilder struct {
	*limitRegistry
}

// NewTransactionBuilder returns a new *TransactionBuilder. The provided
// defaults and overrides paths are expected to be paths to YAML files that
// contain the default and override limits, respectively. Overrides is optional,
// defaults is required.
func NewTransactionBuilder(defaults, overrides string) (*TransactionBuilder, error) {
	registry, err := newLimitRegistry(defaults, overrides)
	if err != nil {
		return nil, err
	}
	return &TransactionBuilder{registry}, nil
}

// RegistrationsPerIPAddressTransaction returns a Transaction for the
// NewRegistrationsPerIPAddress limit for the provided IP address.
func (builder *TransactionBuilder) RegistrationsPerIPAddressTransaction(ip net.IP) (Transaction, error) {
	bucketKey, err := newIPAddressBucketKey(NewRegistrationsPerIPAddress, ip)
	if err != nil {
		return Transaction{}, err
	}
	limit, err := builder.getLimit(NewRegistrationsPerIPAddress, bucketKey)
	if err != nil {
		if errors.Is(err, errLimitDisabled) {
			return newAllowOnlyTransaction()
		}
		return Transaction{}, err
	}
	return newTransaction(limit, bucketKey, 1)
}

// RegistrationsPerIPv6RangeTransaction returns a Transaction for the
// NewRegistrationsPerIPv6Range limit for the /48 IPv6 range which contains the
// provided IPv6 address.
func (builder *TransactionBuilder) RegistrationsPerIPv6RangeTransaction(ip net.IP) (Transaction, error) {
	bucketKey, err := newIPv6RangeCIDRBucketKey(NewRegistrationsPerIPv6Range, ip)
	if err != nil {
		return Transaction{}, err
	}
	limit, err := builder.getLimit(NewRegistrationsPerIPv6Range, bucketKey)
	if err != nil {
		if errors.Is(err, errLimitDisabled) {
			return newAllowOnlyTransaction()
		}
		return Transaction{}, err
	}
	return newTransaction(limit, bucketKey, 1)
}

// OrdersPerAccountTransaction returns a Transaction for the NewOrdersPerAccount
// limit for the provided ACME registration Id.
func (builder *TransactionBuilder) OrdersPerAccountTransaction(regId int64) (Transaction, error) {
	bucketKey, err := newRegIdBucketKey(NewOrdersPerAccount, regId)
	if err != nil {
		return Transaction{}, err
	}
	limit, err := builder.getLimit(NewOrdersPerAccount, bucketKey)
	if err != nil {
		if errors.Is(err, errLimitDisabled) {
			return newAllowOnlyTransaction()
		}
		return Transaction{}, err
	}
	return newTransaction(limit, bucketKey, 1)
}

// FailedAuthorizationsPerDomainPerAccountCheckOnlyTransactions returns a slice
// of Transactions for the provided order domain names. An error is returned if
// any of the order domain names are invalid. This method should be used for
// checking capacity, before allowing more authorizations to be created.
//
// Precondition: orderDomains must all pass policy.WellFormedDomainNames.
// Precondition: len(orderDomains) < maxNames.
func (builder *TransactionBuilder) FailedAuthorizationsPerDomainPerAccountCheckOnlyTransactions(regId int64, orderDomains []string, maxNames int) ([]Transaction, error) {
	if len(orderDomains) > maxNames {
		return nil, fmt.Errorf("order contains more than %d DNS names", maxNames)
	}

	// FailedAuthorizationsPerDomainPerAccount limit uses the 'enum:regId'
	// bucket key format for overrides.
	perAccountBucketKey, err := newRegIdBucketKey(FailedAuthorizationsPerDomainPerAccount, regId)
	if err != nil {
		return nil, err
	}
	limit, err := builder.getLimit(FailedAuthorizationsPerDomainPerAccount, perAccountBucketKey)
	if err != nil && !errors.Is(err, errLimitDisabled) {
		return nil, err
	}

	var txns []Transaction
	for _, name := range DomainsForRateLimiting(orderDomains) {
		// FailedAuthorizationsPerDomainPerAccount limit uses the
		// 'enum:regId:domain' bucket key format for transactions.
		perDomainPerAccountBucketKey, err := newRegIdDomainBucketKey(FailedAuthorizationsPerDomainPerAccount, regId, name)
		if err != nil {
			return nil, err
		}

		// Add a check-only transaction for each per domain per account bucket.
		// The cost is 0, as we are only checking that the account and domain
		// pair aren't already over the limit.
		txn, err := newCheckOnlyTransaction(limit, perDomainPerAccountBucketKey, 0)
		if err != nil {
			return nil, err
		}
		txns = append(txns, txn)
	}
	return txns, nil
}

// FailedAuthorizationsPerDomainPerAccountSpendOnlyTransaction returns a spend-
// only Transaction for the provided order domain name. An error is returned if
// the order domain name is invalid. This method should be used for spending
// capacity, as a result of a failed authorization.
func (builder *TransactionBuilder) FailedAuthorizationsPerDomainPerAccountSpendOnlyTransaction(regId int64, orderDomain string) (Transaction, error) {
	// FailedAuthorizationsPerDomainPerAccount limit uses the 'enum:regId'
	// bucket key format for overrides.
	perAccountBucketKey, err := newRegIdBucketKey(FailedAuthorizationsPerDomainPerAccount, regId)
	if err != nil {
		return Transaction{}, err
	}
	limit, err := builder.getLimit(FailedAuthorizationsPerDomainPerAccount, perAccountBucketKey)
	if err != nil && !errors.Is(err, errLimitDisabled) {
		return Transaction{}, err
	}

	// FailedAuthorizationsPerDomainPerAccount limit uses the
	// 'enum:regId:domain' bucket key format for transactions.
	perDomainPerAccountBucketKey, err := newRegIdDomainBucketKey(FailedAuthorizationsPerDomainPerAccount, regId, orderDomain)
	if err != nil {
		return Transaction{}, err
	}
	txn, err := newSpendOnlyTransaction(limit, perDomainPerAccountBucketKey, 1)
	if err != nil {
		return Transaction{}, err
	}

	return txn, nil
}

// CertificatesPerDomainTransactions returns a slice of Transactions for the
// provided order domain names. An error is returned if any of the order domain
// names are invalid. When a CertificatesPerDomainPerAccount override is
// configured, two types of Transactions are returned:
//   - A spend-only Transaction for each per domain bucket. Spend-only transactions
//     will not be denied if the bucket lacks the capacity to satisfy the cost.
//   - A check-and-spend Transaction for each per account per domain bucket. Check-
//     and-spend transactions will be denied if the bucket lacks the capacity to
//     satisfy the cost.
//
// When a CertificatesPerDomainPerAccount override is not configured, a check-
// and-spend Transaction is returned for each per domain bucket.
//
// Precondition: orderDomains must all pass policy.WellFormedDomainNames.
// Precondition: len(orderDomains) < maxNames.
func (builder *TransactionBuilder) CertificatesPerDomainTransactions(regId int64, orderDomains []string, maxNames int) ([]Transaction, error) {
	if len(orderDomains) > maxNames {
		return nil, fmt.Errorf("order contains more than %d DNS names", maxNames)
	}

	perAccountLimitBucketKey, err := newRegIdBucketKey(CertificatesPerDomainPerAccount, regId)
	if err != nil {
		return nil, err
	}
	perAccountLimit, err := builder.getLimit(CertificatesPerDomainPerAccount, perAccountLimitBucketKey)
	if err != nil && !errors.Is(err, errLimitDisabled) {
		return nil, err
	}

	var txns []Transaction
	for _, name := range DomainsForRateLimiting(orderDomains) {
		perDomainBucketKey, err := newDomainBucketKey(CertificatesPerDomain, name)
		if err != nil {
			return nil, err
		}
		if perAccountLimit.isOverride() {
			// An override is configured for the CertificatesPerDomainPerAccount
			// limit.
			perAccountPerDomainKey, err := newRegIdDomainBucketKey(CertificatesPerDomainPerAccount, regId, name)
			if err != nil {
				return nil, err
			}
			// Add a check-and-spend transaction for each per account per domain
			// bucket.
			txn, err := newTransaction(perAccountLimit, perAccountPerDomainKey, 1)
			if err != nil {
				return nil, err
			}
			txns = append(txns, txn)

			perDomainLimit, err := builder.getLimit(CertificatesPerDomain, perDomainBucketKey)
			if errors.Is(err, errLimitDisabled) {
				// Skip disabled limit.
				continue
			}
			if err != nil {
				return nil, err
			}

			// Add a spend-only transaction for each per domain bucket.
			txn, err = newSpendOnlyTransaction(perDomainLimit, perDomainBucketKey, 1)
			if err != nil {
				return nil, err
			}
			txns = append(txns, txn)
		} else {
			// Use the per domain bucket key when no per account per domain override
			// is configured.
			perDomainLimit, err := builder.getLimit(CertificatesPerDomain, perDomainBucketKey)
			if errors.Is(err, errLimitDisabled) {
				// Skip disabled limit.
				continue
			}
			if err != nil {
				return nil, err
			}
			// Add a check-and-spend transaction for each per domain bucket.
			txn, err := newTransaction(perDomainLimit, perDomainBucketKey, 1)
			if err != nil {
				return nil, err
			}
			txns = append(txns, txn)
		}
	}
	return txns, nil
}

// CertificatesPerFQDNSetTransaction returns a Transaction for the provided
// order domain names.
func (builder *TransactionBuilder) CertificatesPerFQDNSetTransaction(orderNames []string) (Transaction, error) {
	bucketKey, err := newFQDNSetBucketKey(CertificatesPerFQDNSet, orderNames)
	if err != nil {
		return Transaction{}, err
	}
	limit, err := builder.getLimit(CertificatesPerFQDNSet, bucketKey)
	if err != nil {
		if errors.Is(err, errLimitDisabled) {
			return newAllowOnlyTransaction()
		}
		return Transaction{}, err
	}
	return newTransaction(limit, bucketKey, 1)
}
