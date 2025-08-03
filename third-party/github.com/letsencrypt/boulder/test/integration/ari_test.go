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

func TestARIAndReplacement(t *testing.T) {
	t.Parallel()

	// Setup
	client, err := makeClient("mailto:example@letsencrypt.org")
	test.AssertNotError(t, err, "creating acme client")
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "creating random cert key")

	// Issue a cert, request ARI, and check that both the suggested window and
	// the retry-after header are approximately the right amount of time in the
	// future.
	name := random_domain()
	ir, err := authAndIssue(client, key, []acme.Identifier{{Type: "dns", Value: name}}, true, "")
	test.AssertNotError(t, err, "failed to issue test cert")

	cert := ir.certs[0]
	ari, err := client.GetRenewalInfo(cert)
	test.AssertNotError(t, err, "ARI request should have succeeded")
	test.AssertEquals(t, ari.SuggestedWindow.Start.Sub(time.Now()).Round(time.Hour), 1418*time.Hour)
	test.AssertEquals(t, ari.SuggestedWindow.End.Sub(time.Now()).Round(time.Hour), 1461*time.Hour)
	test.AssertEquals(t, ari.RetryAfter.Sub(time.Now()).Round(time.Hour), 6*time.Hour)

	// Make a new order which indicates that it replaces the cert issued above,
	// and verify that the replacement order succeeds.
	_, order, err := makeClientAndOrder(client, key, []acme.Identifier{{Type: "dns", Value: name}}, true, "", cert)
	test.AssertNotError(t, err, "failed to issue test cert")
	replaceID, err := acme.GenerateARICertID(cert)
	test.AssertNotError(t, err, "failed to generate ARI certID")
	test.AssertEquals(t, order.Replaces, replaceID)
	test.AssertNotEquals(t, order.Replaces, "")

	// Retrieve the order and verify that it has the correct replaces field.
	resp, err := client.FetchOrder(client.Account, order.URL)
	test.AssertNotError(t, err, "failed to fetch order")
	if os.Getenv("BOULDER_CONFIG_DIR") == "test/config-next" {
		test.AssertEquals(t, resp.Replaces, order.Replaces)
	} else {
		test.AssertEquals(t, resp.Replaces, "")
	}

	// Try another replacement order and verify that it fails.
	_, order, err = makeClientAndOrder(client, key, []acme.Identifier{{Type: "dns", Value: name}}, true, "", cert)
	test.AssertError(t, err, "subsequent ARI replacements for a replaced cert should fail, but didn't")
	test.AssertContains(t, err.Error(), "urn:ietf:params:acme:error:alreadyReplaced")
	test.AssertContains(t, err.Error(), "already has a replacement order")
	test.AssertContains(t, err.Error(), "error code 409")
}

func TestARIShortLived(t *testing.T) {
	t.Parallel()

	// Setup
	client, err := makeClient("mailto:example@letsencrypt.org")
	test.AssertNotError(t, err, "creating acme client")
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "creating random cert key")

	// Issue a short-lived cert, request ARI, and check that both the suggested
	// window and the retry-after header are approximately the right amount of
	// time in the future.
	ir, err := authAndIssue(client, key, []acme.Identifier{{Type: "dns", Value: random_domain()}}, true, "shortlived")
	test.AssertNotError(t, err, "failed to issue test cert")

	cert := ir.certs[0]
	ari, err := client.GetRenewalInfo(cert)
	test.AssertNotError(t, err, "ARI request should have succeeded")
	test.AssertEquals(t, ari.SuggestedWindow.Start.Sub(time.Now()).Round(time.Hour), 78*time.Hour)
	test.AssertEquals(t, ari.SuggestedWindow.End.Sub(time.Now()).Round(time.Hour), 81*time.Hour)
	test.AssertEquals(t, ari.RetryAfter.Sub(time.Now()).Round(time.Hour), 6*time.Hour)
}

func TestARIRevoked(t *testing.T) {
	t.Parallel()

	// Setup
	client, err := makeClient("mailto:example@letsencrypt.org")
	test.AssertNotError(t, err, "creating acme client")
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "creating random cert key")

	// Issue a cert, revoke it, request ARI, and check that the suggested window
	// is in the past, indicating that a renewal should happen immediately.
	ir, err := authAndIssue(client, key, []acme.Identifier{{Type: "dns", Value: random_domain()}}, true, "")
	test.AssertNotError(t, err, "failed to issue test cert")

	cert := ir.certs[0]
	err = client.RevokeCertificate(client.Account, cert, client.PrivateKey, 0)
	test.AssertNotError(t, err, "failed to revoke cert")

	ari, err := client.GetRenewalInfo(cert)
	test.AssertNotError(t, err, "ARI request should have succeeded")
	test.Assert(t, ari.SuggestedWindow.End.Before(time.Now()), "suggested window should end in the past")
	test.Assert(t, ari.SuggestedWindow.Start.Before(ari.SuggestedWindow.End), "suggested window should start before it ends")
}

func TestARIForPrecert(t *testing.T) {
	t.Parallel()

	// Setup
	client, err := makeClient("mailto:example@letsencrypt.org")
	test.AssertNotError(t, err, "creating acme client")
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "creating random cert key")

	// Try to make a new cert for a new domain, but sabotage the CT logs so
	// issuance fails.
	name := random_domain()
	err = ctAddRejectHost(name)
	test.AssertNotError(t, err, "failed to add ct-test-srv reject host")
	_, err = authAndIssue(client, key, []acme.Identifier{{Type: "dns", Value: name}}, true, "")
	test.AssertError(t, err, "expected error from authAndIssue, was nil")

	// Recover the precert from CT, then request ARI and check
	// that it fails, because we don't serve ARI for non-issued certs.
	cert, err := ctFindRejection([]string{name})
	test.AssertNotError(t, err, "failed to find rejected precert")

	_, err = client.GetRenewalInfo(cert)
	test.AssertError(t, err, "ARI request should have failed")
	test.AssertEquals(t, err.(acme.Problem).Status, 404)
}
