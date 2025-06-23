//go:build integration

package integration

import (
	"testing"
	"time"

	"github.com/letsencrypt/boulder/test"
)

const (
	// validAuthorizationLifetime is the expected valid authorization lifetime. It
	// should match the value in the RA config's "authorizationLifetimeDays"
	// configuration field.
	validAuthorizationLifetime = 30
)

// TestValidAuthzExpires checks that a valid authorization has the expected
// expires time.
func TestValidAuthzExpires(t *testing.T) {
	t.Parallel()
	c, err := makeClient()
	test.AssertNotError(t, err, "makeClient failed")

	// Issue for a random domain
	domains := []string{random_domain()}
	result, err := authAndIssue(c, nil, domains, true)
	// There should be no error
	test.AssertNotError(t, err, "authAndIssue failed")
	// The order should be valid
	test.AssertEquals(t, result.Order.Status, "valid")
	// There should be one authorization URL
	test.AssertEquals(t, len(result.Order.Authorizations), 1)

	// Fetching the authz by URL shouldn't fail
	authzURL := result.Order.Authorizations[0]
	authzOb, err := c.FetchAuthorization(c.Account, authzURL)
	test.AssertNotError(t, err, "FetchAuthorization failed")

	// The authz should be valid and for the correct identifier
	test.AssertEquals(t, authzOb.Status, "valid")
	test.AssertEquals(t, authzOb.Identifier.Value, domains[0])

	// The authz should have the expected expiry date, plus or minus a minute
	expectedExpiresMin := time.Now().AddDate(0, 0, validAuthorizationLifetime).Add(-time.Minute)
	expectedExpiresMax := expectedExpiresMin.Add(2 * time.Minute)
	actualExpires := authzOb.Expires
	if actualExpires.Before(expectedExpiresMin) || actualExpires.After(expectedExpiresMax) {
		t.Errorf("Wrong expiry. Got %s, expected it to be between %s and %s",
			actualExpires, expectedExpiresMin, expectedExpiresMax)
	}
}
