package ratelimits

import (
	"fmt"
	"net/netip"
	"sort"
	"testing"
	"time"

	"github.com/letsencrypt/boulder/config"
	"github.com/letsencrypt/boulder/core"
	"github.com/letsencrypt/boulder/identifier"
	"github.com/letsencrypt/boulder/test"
)

func TestNewTransactionBuilderFromFiles_WithBadLimitsPath(t *testing.T) {
	t.Parallel()
	_, err := NewTransactionBuilderFromFiles("testdata/does-not-exist.yml", "")
	test.AssertError(t, err, "should error")

	_, err = NewTransactionBuilderFromFiles("testdata/defaults.yml", "testdata/does-not-exist.yml")
	test.AssertError(t, err, "should error")
}

func sortTransactions(txns []Transaction) []Transaction {
	sort.Slice(txns, func(i, j int) bool {
		return txns[i].bucketKey < txns[j].bucketKey
	})
	return txns
}

func TestNewRegistrationsPerIPAddressTransactions(t *testing.T) {
	t.Parallel()

	tb, err := NewTransactionBuilderFromFiles("../test/config-next/wfe2-ratelimit-defaults.yml", "")
	test.AssertNotError(t, err, "creating TransactionBuilder")

	// A check-and-spend transaction for the global limit.
	txn, err := tb.registrationsPerIPAddressTransaction(netip.MustParseAddr("1.2.3.4"))
	test.AssertNotError(t, err, "creating transaction")
	test.AssertEquals(t, txn.bucketKey, "1:1.2.3.4")
	test.Assert(t, txn.check && txn.spend, "should be check-and-spend")
}

func TestNewRegistrationsPerIPv6AddressTransactions(t *testing.T) {
	t.Parallel()

	tb, err := NewTransactionBuilderFromFiles("../test/config-next/wfe2-ratelimit-defaults.yml", "")
	test.AssertNotError(t, err, "creating TransactionBuilder")

	// A check-and-spend transaction for the global limit.
	txn, err := tb.registrationsPerIPv6RangeTransaction(netip.MustParseAddr("2001:db8::1"))
	test.AssertNotError(t, err, "creating transaction")
	test.AssertEquals(t, txn.bucketKey, "2:2001:db8::/48")
	test.Assert(t, txn.check && txn.spend, "should be check-and-spend")
}

func TestNewOrdersPerAccountTransactions(t *testing.T) {
	t.Parallel()

	tb, err := NewTransactionBuilderFromFiles("../test/config-next/wfe2-ratelimit-defaults.yml", "")
	test.AssertNotError(t, err, "creating TransactionBuilder")

	// A check-and-spend transaction for the global limit.
	txn, err := tb.ordersPerAccountTransaction(123456789)
	test.AssertNotError(t, err, "creating transaction")
	test.AssertEquals(t, txn.bucketKey, "3:123456789")
	test.Assert(t, txn.check && txn.spend, "should be check-and-spend")
}

func TestFailedAuthorizationsPerDomainPerAccountTransactions(t *testing.T) {
	t.Parallel()

	tb, err := NewTransactionBuilderFromFiles("../test/config-next/wfe2-ratelimit-defaults.yml", "testdata/working_override_13371338.yml")
	test.AssertNotError(t, err, "creating TransactionBuilder")

	// A check-only transaction for the default per-account limit.
	txns, err := tb.FailedAuthorizationsPerDomainPerAccountCheckOnlyTransactions(123456789, identifier.NewDNSSlice([]string{"so.many.labels.here.example.com"}))
	test.AssertNotError(t, err, "creating transactions")
	test.AssertEquals(t, len(txns), 1)
	test.AssertEquals(t, txns[0].bucketKey, "4:123456789:so.many.labels.here.example.com")
	test.Assert(t, txns[0].checkOnly(), "should be check-only")
	test.Assert(t, !txns[0].limit.isOverride, "should not be an override")

	// A spend-only transaction for the default per-account limit.
	txn, err := tb.FailedAuthorizationsPerDomainPerAccountSpendOnlyTransaction(123456789, identifier.NewDNS("so.many.labels.here.example.com"))
	test.AssertNotError(t, err, "creating transaction")
	test.AssertEquals(t, txn.bucketKey, "4:123456789:so.many.labels.here.example.com")
	test.Assert(t, txn.spendOnly(), "should be spend-only")
	test.Assert(t, !txn.limit.isOverride, "should not be an override")

	// A check-only transaction for the per-account limit override.
	txns, err = tb.FailedAuthorizationsPerDomainPerAccountCheckOnlyTransactions(13371338, identifier.NewDNSSlice([]string{"so.many.labels.here.example.com"}))
	test.AssertNotError(t, err, "creating transactions")
	test.AssertEquals(t, len(txns), 1)
	test.AssertEquals(t, txns[0].bucketKey, "4:13371338:so.many.labels.here.example.com")
	test.Assert(t, txns[0].checkOnly(), "should be check-only")
	test.Assert(t, txns[0].limit.isOverride, "should be an override")

	// A spend-only transaction for the per-account limit override.
	txn, err = tb.FailedAuthorizationsPerDomainPerAccountSpendOnlyTransaction(13371338, identifier.NewDNS("so.many.labels.here.example.com"))
	test.AssertNotError(t, err, "creating transaction")
	test.AssertEquals(t, txn.bucketKey, "4:13371338:so.many.labels.here.example.com")
	test.Assert(t, txn.spendOnly(), "should be spend-only")
	test.Assert(t, txn.limit.isOverride, "should be an override")
}

func TestFailedAuthorizationsForPausingPerDomainPerAccountTransactions(t *testing.T) {
	t.Parallel()

	tb, err := NewTransactionBuilderFromFiles("../test/config-next/wfe2-ratelimit-defaults.yml", "testdata/working_override_13371338.yml")
	test.AssertNotError(t, err, "creating TransactionBuilder")

	// A transaction for the per-account limit override.
	txn, err := tb.FailedAuthorizationsForPausingPerDomainPerAccountTransaction(13371338, identifier.NewDNS("so.many.labels.here.example.com"))
	test.AssertNotError(t, err, "creating transaction")
	test.AssertEquals(t, txn.bucketKey, "8:13371338:so.many.labels.here.example.com")
	test.Assert(t, txn.check && txn.spend, "should be check and spend")
	test.Assert(t, txn.limit.isOverride, "should be an override")
}

func TestCertificatesPerDomainTransactions(t *testing.T) {
	t.Parallel()

	tb, err := NewTransactionBuilderFromFiles("../test/config-next/wfe2-ratelimit-defaults.yml", "")
	test.AssertNotError(t, err, "creating TransactionBuilder")

	// One check-only transaction for the global limit.
	txns, err := tb.certificatesPerDomainCheckOnlyTransactions(123456789, identifier.NewDNSSlice([]string{"so.many.labels.here.example.com"}))
	test.AssertNotError(t, err, "creating transactions")
	test.AssertEquals(t, len(txns), 1)
	test.AssertEquals(t, txns[0].bucketKey, "5:example.com")
	test.Assert(t, txns[0].checkOnly(), "should be check-only")

	// One spend-only transaction for the global limit.
	txns, err = tb.CertificatesPerDomainSpendOnlyTransactions(123456789, identifier.NewDNSSlice([]string{"so.many.labels.here.example.com"}))
	test.AssertNotError(t, err, "creating transactions")
	test.AssertEquals(t, len(txns), 1)
	test.AssertEquals(t, txns[0].bucketKey, "5:example.com")
	test.Assert(t, txns[0].spendOnly(), "should be spend-only")
}

func TestCertificatesPerDomainPerAccountTransactions(t *testing.T) {
	t.Parallel()

	tb, err := NewTransactionBuilderFromFiles("../test/config-next/wfe2-ratelimit-defaults.yml", "testdata/working_override_13371338.yml")
	test.AssertNotError(t, err, "creating TransactionBuilder")

	// We only expect a single check-only transaction for the per-account limit
	// override. We can safely ignore the global limit when an override is
	// present.
	txns, err := tb.certificatesPerDomainCheckOnlyTransactions(13371338, identifier.NewDNSSlice([]string{"so.many.labels.here.example.com"}))
	test.AssertNotError(t, err, "creating transactions")
	test.AssertEquals(t, len(txns), 1)
	test.AssertEquals(t, txns[0].bucketKey, "6:13371338:example.com")
	test.Assert(t, txns[0].checkOnly(), "should be check-only")
	test.Assert(t, txns[0].limit.isOverride, "should be an override")

	// Same as above, but with multiple example.com domains.
	txns, err = tb.certificatesPerDomainCheckOnlyTransactions(13371338, identifier.NewDNSSlice([]string{"so.many.labels.here.example.com", "z.example.com"}))
	test.AssertNotError(t, err, "creating transactions")
	test.AssertEquals(t, len(txns), 1)
	test.AssertEquals(t, txns[0].bucketKey, "6:13371338:example.com")
	test.Assert(t, txns[0].checkOnly(), "should be check-only")
	test.Assert(t, txns[0].limit.isOverride, "should be an override")

	// Same as above, but with different domains.
	txns, err = tb.certificatesPerDomainCheckOnlyTransactions(13371338, identifier.NewDNSSlice([]string{"so.many.labels.here.example.com", "z.example.net"}))
	test.AssertNotError(t, err, "creating transactions")
	txns = sortTransactions(txns)
	test.AssertEquals(t, len(txns), 2)
	test.AssertEquals(t, txns[0].bucketKey, "6:13371338:example.com")
	test.Assert(t, txns[0].checkOnly(), "should be check-only")
	test.Assert(t, txns[0].limit.isOverride, "should be an override")
	test.AssertEquals(t, txns[1].bucketKey, "6:13371338:example.net")
	test.Assert(t, txns[1].checkOnly(), "should be check-only")
	test.Assert(t, txns[1].limit.isOverride, "should be an override")

	// Two spend-only transactions, one for the global limit and one for the
	// per-account limit override.
	txns, err = tb.CertificatesPerDomainSpendOnlyTransactions(13371338, identifier.NewDNSSlice([]string{"so.many.labels.here.example.com"}))
	test.AssertNotError(t, err, "creating TransactionBuilder")
	test.AssertEquals(t, len(txns), 2)
	txns = sortTransactions(txns)
	test.AssertEquals(t, txns[0].bucketKey, "5:example.com")
	test.Assert(t, txns[0].spendOnly(), "should be spend-only")
	test.Assert(t, !txns[0].limit.isOverride, "should not be an override")

	test.AssertEquals(t, txns[1].bucketKey, "6:13371338:example.com")
	test.Assert(t, txns[1].spendOnly(), "should be spend-only")
	test.Assert(t, txns[1].limit.isOverride, "should be an override")
}

func TestCertificatesPerFQDNSetTransactions(t *testing.T) {
	t.Parallel()

	tb, err := NewTransactionBuilderFromFiles("../test/config-next/wfe2-ratelimit-defaults.yml", "")
	test.AssertNotError(t, err, "creating TransactionBuilder")

	// A single check-only transaction for the global limit.
	txn, err := tb.certificatesPerFQDNSetCheckOnlyTransaction(identifier.NewDNSSlice([]string{"example.com", "example.net", "example.org"}))
	test.AssertNotError(t, err, "creating transaction")
	namesHash := fmt.Sprintf("%x", core.HashIdentifiers(identifier.NewDNSSlice([]string{"example.com", "example.net", "example.org"})))
	test.AssertEquals(t, txn.bucketKey, "7:"+namesHash)
	test.Assert(t, txn.checkOnly(), "should be check-only")
	test.Assert(t, !txn.limit.isOverride, "should not be an override")
}

func TestNewTransactionBuilder(t *testing.T) {
	t.Parallel()

	expectedBurst := int64(10000)
	expectedCount := int64(10000)
	expectedPeriod := config.Duration{Duration: time.Hour * 168}

	tb, err := NewTransactionBuilder(LimitConfigs{
		NewRegistrationsPerIPAddress.String(): &LimitConfig{
			Burst:  expectedBurst,
			Count:  expectedCount,
			Period: expectedPeriod},
	})
	test.AssertNotError(t, err, "creating TransactionBuilder")

	newRegDefault, ok := tb.limitRegistry.defaults[NewRegistrationsPerIPAddress.EnumString()]
	test.Assert(t, ok, "NewRegistrationsPerIPAddress was not populated in registry")
	test.AssertEquals(t, newRegDefault.burst, expectedBurst)
	test.AssertEquals(t, newRegDefault.count, expectedCount)
	test.AssertEquals(t, newRegDefault.period, expectedPeriod)
}
