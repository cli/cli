//go:build integration

package integration

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/eggsampler/acme/v3"

	"github.com/letsencrypt/boulder/test"
)

// TestCommonNameInCSR ensures that CSRs which have a CN set result in certs
// with the same CN set.
func TestCommonNameInCSR(t *testing.T) {
	t.Parallel()

	// Create an account.
	client, err := makeClient("mailto:example@letsencrypt.org")
	test.AssertNotError(t, err, "creating acme client")

	// Create a private key.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "creating random cert key")

	// Put together some names.
	cn := random_domain()
	san1 := random_domain()
	san2 := random_domain()
	idents := []acme.Identifier{
		{Type: "dns", Value: cn},
		{Type: "dns", Value: san1},
		{Type: "dns", Value: san2},
	}

	// Issue a cert. authAndIssue includes the 0th name as the CN by default.
	ir, err := authAndIssue(client, key, idents, true, "")
	test.AssertNotError(t, err, "failed to issue test cert")
	cert := ir.certs[0]

	// Ensure that the CN is incorporated into the SANs.
	test.AssertSliceContains(t, cert.DNSNames, cn)
	test.AssertSliceContains(t, cert.DNSNames, san1)
	test.AssertSliceContains(t, cert.DNSNames, san2)

	// Ensure that the CN is preserved as the CN.
	test.AssertEquals(t, cert.Subject.CommonName, cn)
}

// TestFirstCSRSANHoistedToCN ensures that CSRs which have no CN set result in
// certs with the first CSR SAN hoisted into the CN field.
func TestFirstCSRSANHoistedToCN(t *testing.T) {
	t.Parallel()

	// Create an account.
	client, err := makeClient("mailto:example@letsencrypt.org")
	test.AssertNotError(t, err, "creating acme client")

	// Create a private key.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "creating random cert key")

	// Create some names that we can sort.
	san1 := "a" + random_domain()
	san2 := "b" + random_domain()
	idents := []acme.Identifier{
		{Type: "dns", Value: san2},
		{Type: "dns", Value: san1},
	}

	// Issue a cert using a CSR with no CN set, and the SANs in *non*-alpha order.
	ir, err := authAndIssue(client, key, idents, false, "")
	test.AssertNotError(t, err, "failed to issue test cert")
	cert := ir.certs[0]

	// Ensure that the SANs are correct, and sorted alphabetically.
	test.AssertEquals(t, cert.DNSNames[0], san1)
	test.AssertEquals(t, cert.DNSNames[1], san2)

	// Ensure that the first SAN from the CSR is the CN.
	test.Assert(t, cert.Subject.CommonName == san2, "first SAN should have been hoisted")
}

// TestCommonNameSANsTooLong tests that, when the names in an order and CSR are
// too long to be hoisted into the CN, the correct behavior results.
func TestCommonNameSANsTooLong(t *testing.T) {
	t.Parallel()

	// Create an account.
	client, err := makeClient("mailto:example@letsencrypt.org")
	test.AssertNotError(t, err, "creating acme client")

	// Create a private key.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "creating random cert key")

	// Put together some names.
	san1 := fmt.Sprintf("thisdomainnameis.morethan64characterslong.forthesakeoftesting.%s", random_domain())
	san2 := fmt.Sprintf("thisdomainnameis.morethan64characterslong.forthesakeoftesting.%s", random_domain())
	idents := []acme.Identifier{
		{Type: "dns", Value: san1},
		{Type: "dns", Value: san2},
	}

	// Issue a cert using a CSR with no CN set.
	ir, err := authAndIssue(client, key, idents, false, "")
	test.AssertNotError(t, err, "failed to issue test cert")
	cert := ir.certs[0]

	// Ensure that the SANs are correct.
	test.AssertSliceContains(t, cert.DNSNames, san1)
	test.AssertSliceContains(t, cert.DNSNames, san2)

	// Ensure that the CN is empty.
	test.AssertEquals(t, cert.Subject.CommonName, "")
}

// TestIssuanceProfiles verifies that profile selection works, and results in
// measurable differences between certificates issued under different profiles.
// It does not test the omission of the keyEncipherment KU, because all of our
// integration test framework assumes ECDSA pubkeys for the sake of speed,
// and ECDSA certs don't get the keyEncipherment KU in either profile.
func TestIssuanceProfiles(t *testing.T) {
	t.Parallel()

	// Create an account.
	client, err := makeClient("mailto:example@letsencrypt.org")
	test.AssertNotError(t, err, "creating acme client")

	profiles := client.Directory().Meta.Profiles
	if len(profiles) < 2 {
		t.Fatal("ACME server not advertising multiple profiles")
	}

	// Create a private key.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "creating random cert key")

	// Create a set of identifiers to request.
	idents := []acme.Identifier{
		{Type: "dns", Value: random_domain()},
	}

	// Get one cert for each profile that we know the test server advertises.
	res, err := authAndIssue(client, key, idents, true, "legacy")
	test.AssertNotError(t, err, "failed to issue under legacy profile")
	test.AssertEquals(t, res.Order.Profile, "legacy")
	legacy := res.certs[0]

	res, err = authAndIssue(client, key, idents, true, "modern")
	test.AssertNotError(t, err, "failed to issue under modern profile")
	test.AssertEquals(t, res.Order.Profile, "modern")
	modern := res.certs[0]

	// Check that each profile worked as expected.
	test.AssertEquals(t, legacy.Subject.CommonName, idents[0].Value)
	test.AssertEquals(t, modern.Subject.CommonName, "")

	test.AssertDeepEquals(t, legacy.ExtKeyUsage, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth})
	test.AssertDeepEquals(t, modern.ExtKeyUsage, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth})

	test.AssertEquals(t, len(legacy.SubjectKeyId), 20)
	test.AssertEquals(t, len(modern.SubjectKeyId), 0)
}

// TestIPShortLived verifies that we will allow IP address identifiers only in
// orders that use the shortlived profile.
func TestIPShortLived(t *testing.T) {
	t.Parallel()

	// Create an account.
	client, err := makeClient("mailto:example@letsencrypt.org")
	if err != nil {
		t.Fatalf("creating acme client: %s", err)
	}

	// Create a private key.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("creating random cert key: %s", err)
	}

	// Create an IP address identifier to request.
	ip := "64.112.117.122"
	idents := []acme.Identifier{
		{Type: "ip", Value: ip},
	}

	// Ensure we fail under each other profile that we know the test server advertises.
	_, err = authAndIssue(client, key, idents, false, "legacy")
	if err == nil {
		t.Error("issued for IP address identifier under legacy profile")
	}
	if !strings.Contains(err.Error(), "Profile \"legacy\" does not permit ip type identifiers") {
		t.Fatalf("issuing under legacy profile failed for the wrong reason: %s", err)
	}

	_, err = authAndIssue(client, key, idents, false, "modern")
	if err == nil {
		t.Error("issued for IP address identifier under modern profile")
	}
	if !strings.Contains(err.Error(), "Profile \"modern\" does not permit ip type identifiers") {
		t.Fatalf("issuing under legacy profile failed for the wrong reason: %s", err)
	}

	// Get one cert for the shortlived profile.
	res, err := authAndIssue(client, key, idents, false, "shortlived")
	if os.Getenv("BOULDER_CONFIG_DIR") == "test/config-next" {
		if err != nil {
			t.Errorf("issuing under shortlived profile: %s", err)
		}
		if res.Order.Profile != "shortlived" {
			t.Errorf("got '%s' profile, wanted 'shortlived'", res.Order.Profile)
		}
		cert := res.certs[0]

		// Check that the shortlived profile worked as expected.
		if cert.IPAddresses[0].String() != ip {
			t.Errorf("got cert with first IP SAN '%s', wanted '%s'", cert.IPAddresses[0], ip)
		}
	} else {
		if !strings.Contains(err.Error(), "Profile \"shortlived\" does not permit ip type identifiers") {
			t.Errorf("issuing under shortlived profile failed for the wrong reason: %s", err)
		}
	}
}
