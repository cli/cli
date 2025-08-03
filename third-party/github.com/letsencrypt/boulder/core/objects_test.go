package core

import (
	"crypto/rsa"
	"encoding/json"
	"math/big"
	"net/netip"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"

	"github.com/letsencrypt/boulder/test"
)

func TestExpectedKeyAuthorization(t *testing.T) {
	ch := Challenge{Token: "hi"}
	jwk1 := &jose.JSONWebKey{Key: &rsa.PublicKey{N: big.NewInt(1234), E: 1234}}
	jwk2 := &jose.JSONWebKey{Key: &rsa.PublicKey{N: big.NewInt(5678), E: 5678}}

	ka1, err := ch.ExpectedKeyAuthorization(jwk1)
	test.AssertNotError(t, err, "Failed to calculate expected key authorization 1")
	ka2, err := ch.ExpectedKeyAuthorization(jwk2)
	test.AssertNotError(t, err, "Failed to calculate expected key authorization 2")

	expected1 := "hi.sIMEyhkWCCSYqDqZqPM1bKkvb5T9jpBOb7_w5ZNorF4"
	expected2 := "hi.FPoiyqWPod2T0fKqkPI1uXPYUsRK1DSyzsQsv0oMuGg"
	if ka1 != expected1 {
		t.Errorf("Incorrect ka1. Expected [%s], got [%s]", expected1, ka1)
	}
	if ka2 != expected2 {
		t.Errorf("Incorrect ka2. Expected [%s], got [%s]", expected2, ka2)
	}
}

func TestRecordSanityCheckOnUnsupportedChallengeType(t *testing.T) {
	rec := []ValidationRecord{
		{
			URL:               "http://localhost/test",
			Hostname:          "localhost",
			Port:              "80",
			AddressesResolved: []netip.Addr{netip.MustParseAddr("127.0.0.1")},
			AddressUsed:       netip.MustParseAddr("127.0.0.1"),
			ResolverAddrs:     []string{"eastUnboundAndDown"},
		},
	}

	chall := Challenge{Type: "obsoletedChallenge", ValidationRecord: rec}
	test.Assert(t, !chall.RecordsSane(), "Record with unsupported challenge type should not be sane")
}

func TestChallengeSanityCheck(t *testing.T) {
	// Make a temporary account key
	var accountKey *jose.JSONWebKey
	err := json.Unmarshal([]byte(`{
    "kty":"RSA",
    "n":"yNWVhtYEKJR21y9xsHV-PD_bYwbXSeNuFal46xYxVfRL5mqha7vttvjB_vc7Xg2RvgCxHPCqoxgMPTzHrZT75LjCwIW2K_klBYN8oYvTwwmeSkAz6ut7ZxPv-nZaT5TJhGk0NT2kh_zSpdriEJ_3vW-mqxYbbBmpvHqsa1_zx9fSuHYctAZJWzxzUZXykbWMWQZpEiE0J4ajj51fInEzVn7VxV-mzfMyboQjujPh7aNJxAWSq4oQEJJDgWwSh9leyoJoPpONHxh5nEE5AjE01FkGICSxjpZsF-w8hOTI3XXohUdu29Se26k2B0PolDSuj0GIQU6-W9TdLXSjBb2SpQ",
    "e":"AQAB"
  }`), &accountKey)
	test.AssertNotError(t, err, "Error unmarshaling JWK")

	types := []AcmeChallenge{ChallengeTypeHTTP01, ChallengeTypeDNS01, ChallengeTypeTLSALPN01}
	for _, challengeType := range types {
		chall := Challenge{
			Type:   challengeType,
			Status: StatusInvalid,
		}
		test.AssertError(t, chall.CheckPending(), "CheckConsistencyForClientOffer didn't return an error")

		chall.Status = StatusPending
		test.AssertError(t, chall.CheckPending(), "CheckConsistencyForClientOffer didn't return an error")

		chall.Token = "KQqLsiS5j0CONR_eUXTUSUDNVaHODtc-0pD6ACif7U4"
		test.AssertNotError(t, chall.CheckPending(), "CheckConsistencyForClientOffer returned an error")
	}
}

func TestJSONBufferUnmarshal(t *testing.T) {
	testStruct := struct {
		Buffer JSONBuffer
	}{}

	notValidBase64 := []byte(`{"Buffer":"!!!!"}`)
	err := json.Unmarshal(notValidBase64, &testStruct)
	test.Assert(t, err != nil, "Should have choked on invalid base64")
}

func TestAuthorizationSolvedBy(t *testing.T) {
	validHTTP01 := HTTPChallenge01("")
	validHTTP01.Status = StatusValid
	validDNS01 := DNSChallenge01("")
	validDNS01.Status = StatusValid
	testCases := []struct {
		Name           string
		Authz          Authorization
		ExpectedResult AcmeChallenge
		ExpectedError  string
	}{
		// An authz with no challenges should return nil
		{
			Name:          "No challenges",
			Authz:         Authorization{},
			ExpectedError: "authorization has no challenges",
		},
		// An authz with all non-valid challenges should return nil
		{
			Name: "All non-valid challenges",
			Authz: Authorization{
				Challenges: []Challenge{HTTPChallenge01(""), DNSChallenge01("")},
			},
			ExpectedError: "authorization not solved by any challenge",
		},
		// An authz with one valid HTTP01 challenge amongst other challenges should
		// return the HTTP01 challenge
		{
			Name: "Valid HTTP01 challenge",
			Authz: Authorization{
				Challenges: []Challenge{HTTPChallenge01(""), validHTTP01, DNSChallenge01("")},
			},
			ExpectedResult: ChallengeTypeHTTP01,
		},
		// An authz with both a valid HTTP01 challenge and a valid DNS01 challenge
		// among other challenges should return whichever valid challenge is first
		// (in this case DNS01)
		{
			Name: "Valid HTTP01 and DNS01 challenge",
			Authz: Authorization{
				Challenges: []Challenge{validDNS01, HTTPChallenge01(""), validHTTP01, DNSChallenge01("")},
			},
			ExpectedResult: ChallengeTypeDNS01,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			result, err := tc.Authz.SolvedBy()
			if tc.ExpectedError != "" {
				test.AssertEquals(t, err.Error(), tc.ExpectedError)
			}
			if tc.ExpectedResult != "" {
				test.AssertEquals(t, result, tc.ExpectedResult)
			}
		})
	}
}

func TestChallengeStringID(t *testing.T) {
	ch := Challenge{
		Token: "asd",
		Type:  ChallengeTypeDNS01,
	}
	test.AssertEquals(t, ch.StringID(), "iFVMwA")
	ch.Type = ChallengeTypeHTTP01
	test.AssertEquals(t, ch.StringID(), "0Gexug")
}

func TestFindChallengeByType(t *testing.T) {
	authz := Authorization{
		Challenges: []Challenge{
			{Token: "woo", Type: ChallengeTypeDNS01},
			{Token: "woo", Type: ChallengeTypeHTTP01},
		},
	}
	test.AssertEquals(t, 0, authz.FindChallengeByStringID(authz.Challenges[0].StringID()))
	test.AssertEquals(t, 1, authz.FindChallengeByStringID(authz.Challenges[1].StringID()))
	test.AssertEquals(t, -1, authz.FindChallengeByStringID("hello"))
}

func TestRenewalInfoSuggestedWindowIsWithin(t *testing.T) {
	now := time.Now().UTC()
	window := SuggestedWindow{
		Start: now,
		End:   now.Add(time.Hour),
	}

	// Exactly the beginning, inclusive of the first nanosecond.
	test.Assert(t, window.IsWithin(now), "Start of window should be within the window")

	// Exactly the middle.
	test.Assert(t, window.IsWithin(now.Add(time.Minute*30)), "Middle of window should be within the window")

	// Exactly the end time.
	test.Assert(t, !window.IsWithin(now.Add(time.Hour)), "End of window should be outside the window")

	// Exactly the end of the window.
	test.Assert(t, window.IsWithin(now.Add(time.Hour-time.Nanosecond)), "Should be just inside the window")

	// Just before the first nanosecond.
	test.Assert(t, !window.IsWithin(now.Add(-time.Nanosecond)), "Before the window should not be within the window")
}
