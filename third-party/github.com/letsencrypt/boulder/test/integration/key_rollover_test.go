//go:build integration

package integration

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/eggsampler/acme/v3"
	"github.com/letsencrypt/boulder/test"
)

// TestAccountKeyChange tests that the whole account key rollover process works,
// including between different kinds of keys.
func TestAccountKeyChange(t *testing.T) {
	t.Parallel()

	c, err := acme.NewClient("http://boulder.service.consul:4001/directory")
	test.AssertNotError(t, err, "creating client")

	// We could test all five key types (RSA 2048, 3072, and 4096, and ECDSA P-256
	// and P-384) supported by go-jose and goodkey, but doing so results in a very
	// slow integration test. Instead, just test rollover once in each direction,
	// ECDSA->RSA and vice versa.
	key1, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "creating P-256 account key")

	acct1, err := c.NewAccount(key1, false, true)
	test.AssertNotError(t, err, "creating account")

	key2, err := rsa.GenerateKey(rand.Reader, 2048)
	test.AssertNotError(t, err, "creating RSA 2048 account key")

	acct2, err := c.AccountKeyChange(acct1, key2)
	test.AssertNotError(t, err, "rolling over account key")
	test.AssertEquals(t, acct2.URL, acct1.URL)

	key3, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	test.AssertNotError(t, err, "creating P-384 account key")

	acct3, err := c.AccountKeyChange(acct1, key3)
	test.AssertNotError(t, err, "rolling over account key")
	test.AssertEquals(t, acct3.URL, acct1.URL)
}
