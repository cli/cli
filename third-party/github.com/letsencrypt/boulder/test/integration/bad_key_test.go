//go:build integration

package integration

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"testing"

	"github.com/eggsampler/acme/v3"

	"github.com/letsencrypt/boulder/test"
)

// TestFermat ensures that a certificate public key which can be factored using
// less than 100 rounds of Fermat's Algorithm is rejected.
func TestFermat(t *testing.T) {
	t.Parallel()

	// Create a client and complete an HTTP-01 challenge for a fake domain.
	c, err := makeClient()
	test.AssertNotError(t, err, "creating acme client")

	domain := random_domain()

	order, err := c.Client.NewOrder(
		c.Account, []acme.Identifier{{Type: "dns", Value: domain}})
	test.AssertNotError(t, err, "creating new order")
	test.AssertEquals(t, len(order.Authorizations), 1)

	authUrl := order.Authorizations[0]

	auth, err := c.Client.FetchAuthorization(c.Account, authUrl)
	test.AssertNotError(t, err, "fetching authorization")

	chal, ok := auth.ChallengeMap[acme.ChallengeTypeHTTP01]
	test.Assert(t, ok, "getting HTTP-01 challenge")

	_, err = testSrvClient.AddHTTP01Response(chal.Token, chal.KeyAuthorization)
	test.AssertNotError(t, err, "")
	defer func() {
		_, err = testSrvClient.RemoveHTTP01Response(chal.Token)
		test.AssertNotError(t, err, "")
	}()

	chal, err = c.Client.UpdateChallenge(c.Account, chal)
	test.AssertNotError(t, err, "updating HTTP-01 challenge")

	// Load the Fermat-weak CSR that we'll submit for finalize. This CSR was
	// generated using test/integration/testdata/fermat_csr.go, has prime factors
	// that differ by only 2^516 + 254, and can be factored in 42 rounds.
	csrPem, err := os.ReadFile("test/integration/testdata/fermat_csr.pem")
	test.AssertNotError(t, err, "reading CSR PEM from disk")

	csrDer, _ := pem.Decode(csrPem)
	if csrDer == nil {
		t.Fatal("failed to decode CSR PEM")
	}

	csr, err := x509.ParseCertificateRequest(csrDer.Bytes)
	test.AssertNotError(t, err, "parsing CSR")

	// Finalizing the order should fail as we reject the public key.
	_, err = c.Client.FinalizeOrder(c.Account, order, csr)
	test.AssertError(t, err, "finalizing order")
	test.AssertContains(t, err.Error(), "urn:ietf:params:acme:error:badCSR")
	test.AssertContains(t, err.Error(), "key generated with factors too close together")
}
