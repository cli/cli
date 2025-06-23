//go:build integration

package integration

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509/pkix"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/eggsampler/acme/v3"

	"github.com/letsencrypt/boulder/test"
)

// certID matches the ASN.1 structure of the CertID sequence defined by RFC6960.
type certID struct {
	HashAlgorithm  pkix.AlgorithmIdentifier
	IssuerNameHash []byte
	IssuerKeyHash  []byte
	SerialNumber   *big.Int
}

func TestARI(t *testing.T) {
	t.Parallel()

	// Create an account.
	client, err := makeClient("mailto:example@letsencrypt.org")
	test.AssertNotError(t, err, "creating acme client")

	// Create a private key.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "creating random cert key")

	// Issue a cert, request ARI, and check that both the suggested window and
	// the retry-after header are approximately the right amount of time in the
	// future.
	name := random_domain()
	ir, err := authAndIssue(client, key, []string{name}, true)
	test.AssertNotError(t, err, "failed to issue test cert")

	cert := ir.certs[0]
	ari, err := client.GetRenewalInfo(cert)
	test.AssertNotError(t, err, "ARI request should have succeeded")
	test.AssertEquals(t, ari.SuggestedWindow.Start.Sub(time.Now()).Round(time.Hour), 1415*time.Hour)
	test.AssertEquals(t, ari.SuggestedWindow.End.Sub(time.Now()).Round(time.Hour), 1463*time.Hour)
	test.AssertEquals(t, ari.RetryAfter.Sub(time.Now()).Round(time.Hour), 6*time.Hour)

	// TODO(@pgporada): Clean this up when 'test/config/{sa,wfe2}.json' sets
	// TrackReplacementCertificatesARI=true.
	if os.Getenv("BOULDER_CONFIG_DIR") == "test/config-next" {
		// Make a new order which indicates that it replaces the cert issued above.
		_, order, err := makeClientAndOrder(client, key, []string{name}, true, cert)
		test.AssertNotError(t, err, "failed to issue test cert")
		replaceID, err := acme.GenerateARICertID(cert)
		test.AssertNotError(t, err, "failed to generate ARI certID")
		test.AssertEquals(t, order.Replaces, replaceID)
		test.AssertNotEquals(t, order.Replaces, "")

		// Try it again and verify it fails
		_, order, err = makeClientAndOrder(client, key, []string{name}, true, cert)
		test.AssertError(t, err, "subsequent ARI replacements for a replaced cert should fail, but didn't")
	} else {
		// ARI is disabled so we only use the client to POST the replacement
		// order, but we never finalize it.
		replacementOrder, err := client.ReplacementOrder(client.Account, cert, []acme.Identifier{{Type: "dns", Value: name}})
		test.AssertNotError(t, err, "ARI replacement request should have succeeded")
		test.AssertNotEquals(t, replacementOrder.Replaces, "")
	}

	// Revoke the cert and re-request ARI. The renewal window should now be in
	// the past indicating to the client that a renewal should happen
	// immediately.
	err = client.RevokeCertificate(client.Account, cert, client.PrivateKey, 0)
	test.AssertNotError(t, err, "failed to revoke cert")

	ari, err = client.GetRenewalInfo(cert)
	test.AssertNotError(t, err, "ARI request should have succeeded")
	test.Assert(t, ari.SuggestedWindow.End.Before(time.Now()), "suggested window should end in the past")
	test.Assert(t, ari.SuggestedWindow.Start.Before(ari.SuggestedWindow.End), "suggested window should start before it ends")

	// Try to make a new cert for a new domain, but sabotage the CT logs so
	// issuance fails. Recover the precert from CT, then request ARI and check
	// that it fails, because we don't serve ARI for non-issued certs.
	name = random_domain()
	err = ctAddRejectHost(name)
	test.AssertNotError(t, err, "failed to add ct-test-srv reject host")
	_, err = authAndIssue(client, key, []string{name}, true)
	test.AssertError(t, err, "expected error from authAndIssue, was nil")

	cert, err = ctFindRejection([]string{name})
	test.AssertNotError(t, err, "failed to find rejected precert")

	ari, err = client.GetRenewalInfo(cert)
	test.AssertError(t, err, "ARI request should have failed")
	test.AssertEquals(t, err.(acme.Problem).Status, 404)
}
