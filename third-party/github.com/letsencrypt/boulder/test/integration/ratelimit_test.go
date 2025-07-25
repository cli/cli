//go:build integration

package integration

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"testing"

	"github.com/eggsampler/acme/v3"

	"github.com/letsencrypt/boulder/test"
)

func TestDuplicateFQDNRateLimit(t *testing.T) {
	t.Parallel()
	idents := []acme.Identifier{{Type: "dns", Value: random_domain()}}

	// TODO(#8235): Remove this conditional once IP address identifiers are
	// enabled in test/config.
	if os.Getenv("BOULDER_CONFIG_DIR") == "test/config-next" {
		idents = append(idents, acme.Identifier{Type: "ip", Value: "64.112.117.122"})
	}

	// The global rate limit for a duplicate certificates is 2 per 3 hours.
	_, err := authAndIssue(nil, nil, idents, true, "shortlived")
	test.AssertNotError(t, err, "Failed to issue first certificate")

	_, err = authAndIssue(nil, nil, idents, true, "shortlived")
	test.AssertNotError(t, err, "Failed to issue second certificate")

	_, err = authAndIssue(nil, nil, idents, true, "shortlived")
	test.AssertError(t, err, "Somehow managed to issue third certificate")

	test.AssertContains(t, err.Error(), "too many certificates (2) already issued for this exact set of identifiers in the last 3h0m0s")
}

func TestCertificatesPerDomain(t *testing.T) {
	t.Parallel()

	randomDomain := random_domain()
	randomSubDomain := func() string {
		var bytes [3]byte
		rand.Read(bytes[:])
		return fmt.Sprintf("%s.%s", hex.EncodeToString(bytes[:]), randomDomain)
	}

	firstSubDomain := randomSubDomain()
	_, err := authAndIssue(nil, nil, []acme.Identifier{{Type: "dns", Value: firstSubDomain}}, true, "")
	test.AssertNotError(t, err, "Failed to issue first certificate")

	_, err = authAndIssue(nil, nil, []acme.Identifier{{Type: "dns", Value: randomSubDomain()}}, true, "")
	test.AssertNotError(t, err, "Failed to issue second certificate")

	_, err = authAndIssue(nil, nil, []acme.Identifier{{Type: "dns", Value: randomSubDomain()}}, true, "")
	test.AssertError(t, err, "Somehow managed to issue third certificate")

	test.AssertContains(t, err.Error(), fmt.Sprintf("too many certificates (2) already issued for %q in the last 2160h0m0s", randomDomain))

	// Issue a certificate for the first subdomain, which should succeed because
	// it's a renewal.
	_, err = authAndIssue(nil, nil, []acme.Identifier{{Type: "dns", Value: firstSubDomain}}, true, "")
	test.AssertNotError(t, err, "Failed to issue renewal certificate")
}
