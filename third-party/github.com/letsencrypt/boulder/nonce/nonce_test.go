package nonce

import (
	"fmt"
	"testing"

	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/test"
)

func TestValidNonce(t *testing.T) {
	ns, err := NewNonceService(metrics.NoopRegisterer, 0, "")
	test.AssertNotError(t, err, "Could not create nonce service")
	n, err := ns.Nonce()
	test.AssertNotError(t, err, "Could not create nonce")
	test.Assert(t, ns.Valid(n), fmt.Sprintf("Did not recognize fresh nonce %s", n))
}

func TestAlreadyUsed(t *testing.T) {
	ns, err := NewNonceService(metrics.NoopRegisterer, 0, "")
	test.AssertNotError(t, err, "Could not create nonce service")
	n, err := ns.Nonce()
	test.AssertNotError(t, err, "Could not create nonce")
	test.Assert(t, ns.Valid(n), "Did not recognize fresh nonce")
	test.Assert(t, !ns.Valid(n), "Recognized the same nonce twice")
}

func TestRejectMalformed(t *testing.T) {
	ns, err := NewNonceService(metrics.NoopRegisterer, 0, "")
	test.AssertNotError(t, err, "Could not create nonce service")
	n, err := ns.Nonce()
	test.AssertNotError(t, err, "Could not create nonce")
	test.Assert(t, !ns.Valid("asdf"+n), "Accepted an invalid nonce")
}

func TestRejectShort(t *testing.T) {
	ns, err := NewNonceService(metrics.NoopRegisterer, 0, "")
	test.AssertNotError(t, err, "Could not create nonce service")
	test.Assert(t, !ns.Valid("aGkK"), "Accepted an invalid nonce")
}

func TestRejectUnknown(t *testing.T) {
	ns1, err := NewNonceService(metrics.NoopRegisterer, 0, "")
	test.AssertNotError(t, err, "Could not create nonce service")
	ns2, err := NewNonceService(metrics.NoopRegisterer, 0, "")
	test.AssertNotError(t, err, "Could not create nonce service")

	n, err := ns1.Nonce()
	test.AssertNotError(t, err, "Could not create nonce")
	test.Assert(t, !ns2.Valid(n), "Accepted a foreign nonce")
}

func TestRejectTooLate(t *testing.T) {
	ns, err := NewNonceService(metrics.NoopRegisterer, 0, "")
	test.AssertNotError(t, err, "Could not create nonce service")

	ns.latest = 2
	n, err := ns.Nonce()
	test.AssertNotError(t, err, "Could not create nonce")
	ns.latest = 1
	test.Assert(t, !ns.Valid(n), "Accepted a nonce with a too-high counter")
}

func TestRejectTooEarly(t *testing.T) {
	ns, err := NewNonceService(metrics.NoopRegisterer, 0, "")
	test.AssertNotError(t, err, "Could not create nonce service")

	n0, err := ns.Nonce()
	test.AssertNotError(t, err, "Could not create nonce")

	for range ns.maxUsed {
		n, err := ns.Nonce()
		test.AssertNotError(t, err, "Could not create nonce")
		if !ns.Valid(n) {
			t.Errorf("generated invalid nonce")
		}
	}

	n1, err := ns.Nonce()
	test.AssertNotError(t, err, "Could not create nonce")
	n2, err := ns.Nonce()
	test.AssertNotError(t, err, "Could not create nonce")
	n3, err := ns.Nonce()
	test.AssertNotError(t, err, "Could not create nonce")

	test.Assert(t, ns.Valid(n3), "Rejected a valid nonce")
	test.Assert(t, ns.Valid(n2), "Rejected a valid nonce")
	test.Assert(t, ns.Valid(n1), "Rejected a valid nonce")
	test.Assert(t, !ns.Valid(n0), "Accepted a nonce that we should have forgotten")
}

func BenchmarkNonces(b *testing.B) {
	ns, err := NewNonceService(metrics.NoopRegisterer, 0, "")
	if err != nil {
		b.Fatal("creating nonce service", err)
	}

	for range ns.maxUsed {
		n, err := ns.Nonce()
		if err != nil {
			b.Fatal("noncing", err)
		}
		if !ns.Valid(n) {
			b.Fatal("generated invalid nonce")
		}
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			n, err := ns.Nonce()
			if err != nil {
				b.Fatal("noncing", err)
			}
			if !ns.Valid(n) {
				b.Fatal("generated invalid nonce")
			}
		}
	})
}

func TestNoncePrefixing(t *testing.T) {
	ns, err := NewNonceService(metrics.NoopRegisterer, 0, "aluminum")
	test.AssertNotError(t, err, "Could not create nonce service")

	n, err := ns.Nonce()
	test.AssertNotError(t, err, "Could not create nonce")
	test.Assert(t, ns.Valid(n), "Valid nonce rejected")

	n, err = ns.Nonce()
	test.AssertNotError(t, err, "Could not create nonce")
	n = n[1:]
	test.Assert(t, !ns.Valid(n), "Valid nonce with incorrect prefix accepted")

	n, err = ns.Nonce()
	test.AssertNotError(t, err, "Could not create nonce")
	test.Assert(t, !ns.Valid(n[6:]), "Valid nonce without prefix accepted")
}

func TestNoncePrefixValidation(t *testing.T) {
	_, err := NewNonceService(metrics.NoopRegisterer, 0, "whatsup")
	test.AssertError(t, err, "NewNonceService didn't fail with short prefix")
	_, err = NewNonceService(metrics.NoopRegisterer, 0, "whatsup!")
	test.AssertError(t, err, "NewNonceService didn't fail with invalid base64")
	_, err = NewNonceService(metrics.NoopRegisterer, 0, "whatsupp")
	test.AssertNotError(t, err, "NewNonceService failed with valid nonce prefix")
}

func TestDerivePrefix(t *testing.T) {
	prefix := DerivePrefix("192.168.1.1:8080", []byte("3b8c758dd85e113ea340ce0b3a99f389d40a308548af94d1730a7692c1874f1f"))
	test.AssertEquals(t, prefix, "P9qQaK4o")
}
