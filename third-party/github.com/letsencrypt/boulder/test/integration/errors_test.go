//go:build integration

package integration

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/eggsampler/acme/v3"
	"github.com/go-jose/go-jose/v4"

	"github.com/letsencrypt/boulder/test"
)

// TestTooBigOrderError tests that submitting an order with more than 100
// identifiers produces the expected problem result.
func TestTooBigOrderError(t *testing.T) {
	t.Parallel()

	var idents []acme.Identifier
	for i := range 101 {
		idents = append(idents, acme.Identifier{Type: "dns", Value: fmt.Sprintf("%d.example.com", i)})
	}

	_, err := authAndIssue(nil, nil, idents, true, "")
	test.AssertError(t, err, "authAndIssue failed")

	var prob acme.Problem
	test.AssertErrorWraps(t, err, &prob)
	test.AssertEquals(t, prob.Type, "urn:ietf:params:acme:error:malformed")
	test.AssertContains(t, prob.Detail, "Order cannot contain more than 100 identifiers")
}

// TestAccountEmailError tests that registering a new account, or updating an
// account, with invalid contact information produces the expected problem
// result to ACME clients.
func TestAccountEmailError(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name               string
		contacts           []string
		expectedProbType   string
		expectedProbDetail string
	}{
		{
			name:               "empty contact",
			contacts:           []string{"mailto:valid@valid.com", ""},
			expectedProbType:   "urn:ietf:params:acme:error:invalidContact",
			expectedProbDetail: `empty contact`,
		},
		{
			name:               "empty proto",
			contacts:           []string{"mailto:valid@valid.com", " "},
			expectedProbType:   "urn:ietf:params:acme:error:unsupportedContact",
			expectedProbDetail: `only contact scheme 'mailto:' is supported`,
		},
		{
			name:               "empty mailto",
			contacts:           []string{"mailto:valid@valid.com", "mailto:"},
			expectedProbType:   "urn:ietf:params:acme:error:invalidContact",
			expectedProbDetail: `unable to parse email address`,
		},
		{
			name:               "non-ascii mailto",
			contacts:           []string{"mailto:valid@valid.com", "mailto:cpu@l̴etsencrypt.org"},
			expectedProbType:   "urn:ietf:params:acme:error:invalidContact",
			expectedProbDetail: `contact email contains non-ASCII characters`,
		},
		{
			name:               "too many contacts",
			contacts:           slices.Repeat([]string{"mailto:lots@valid.com"}, 11),
			expectedProbType:   "urn:ietf:params:acme:error:malformed",
			expectedProbDetail: `too many contacts provided`,
		},
		{
			name:               "invalid contact",
			contacts:           []string{"mailto:valid@valid.com", "mailto:a@"},
			expectedProbType:   "urn:ietf:params:acme:error:invalidContact",
			expectedProbDetail: `unable to parse email address`,
		},
		{
			name:               "forbidden contact domain",
			contacts:           []string{"mailto:valid@valid.com", "mailto:a@example.com"},
			expectedProbType:   "urn:ietf:params:acme:error:invalidContact",
			expectedProbDetail: "contact email has forbidden domain \"example.com\"",
		},
		{
			name:               "contact domain invalid TLD",
			contacts:           []string{"mailto:valid@valid.com", "mailto:a@example.cpu"},
			expectedProbType:   "urn:ietf:params:acme:error:invalidContact",
			expectedProbDetail: `contact email has invalid domain: Domain name does not end with a valid public suffix (TLD)`,
		},
		{
			name:               "contact domain invalid",
			contacts:           []string{"mailto:valid@valid.com", "mailto:a@example./.com"},
			expectedProbType:   "urn:ietf:params:acme:error:invalidContact",
			expectedProbDetail: "contact email has invalid domain: Domain name contains an invalid character",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var prob acme.Problem
			_, err := makeClient(tc.contacts...)
			if err != nil {
				test.AssertErrorWraps(t, err, &prob)
				test.AssertEquals(t, prob.Type, tc.expectedProbType)
				test.AssertContains(t, prob.Detail, "Error validating contact(s)")
				test.AssertContains(t, prob.Detail, tc.expectedProbDetail)
			} else {
				t.Errorf("expected %s type problem for %q, got nil",
					tc.expectedProbType, strings.Join(tc.contacts, ","))
			}
		})
	}
}

func TestRejectedIdentifier(t *testing.T) {
	t.Parallel()

	// When a single malformed name is provided, we correctly reject it.
	idents := []acme.Identifier{
		{Type: "dns", Value: "яџ–Х6яяdь}"},
	}
	_, err := authAndIssue(nil, nil, idents, true, "")
	test.AssertError(t, err, "issuance should fail for one malformed name")
	var prob acme.Problem
	test.AssertErrorWraps(t, err, &prob)
	test.AssertEquals(t, prob.Type, "urn:ietf:params:acme:error:rejectedIdentifier")
	test.AssertContains(t, prob.Detail, "Domain name contains an invalid character")

	// When multiple malformed names are provided, we correctly reject all of
	// them and reflect this in suberrors. This test ensures that the way we
	// encode these errors across the gRPC boundary is resilient to non-ascii
	// characters.
	idents = []acme.Identifier{
		{Type: "dns", Value: "o-"},
		{Type: "dns", Value: "ш№Ў"},
		{Type: "dns", Value: "р±y"},
		{Type: "dns", Value: "яџ–Х6яя"},
		{Type: "dns", Value: "яџ–Х6яя`ь"},
	}
	_, err = authAndIssue(nil, nil, idents, true, "")
	test.AssertError(t, err, "issuance should fail for multiple malformed names")
	test.AssertErrorWraps(t, err, &prob)
	test.AssertEquals(t, prob.Type, "urn:ietf:params:acme:error:rejectedIdentifier")
	test.AssertContains(t, prob.Detail, "Domain name contains an invalid character")
	test.AssertContains(t, prob.Detail, "and 4 more problems")
}

// TestBadSignatureAlgorithm tests that supplying an unacceptable value for the
// "alg" field of the JWS Protected Header results in a problem document with
// the set of acceptable "alg" values listed in a custom extension field named
// "algorithms". Creating a request with an unacceptable "alg" field requires
// us to do some shenanigans.
func TestBadSignatureAlgorithm(t *testing.T) {
	t.Parallel()

	client, err := makeClient()
	if err != nil {
		t.Fatal("creating test client")
	}

	header, err := json.Marshal(&struct {
		Alg   string `json:"alg"`
		KID   string `json:"kid"`
		Nonce string `json:"nonce"`
		URL   string `json:"url"`
	}{
		Alg:   string(jose.RS512), // This is the important bit; RS512 is unacceptable.
		KID:   client.Account.URL,
		Nonce: "deadbeef", // This nonce would fail, but that check comes after the alg check.
		URL:   client.Directory().NewAccount,
	})
	if err != nil {
		t.Fatalf("creating JWS protected header: %s", err)
	}
	protected := base64.RawURLEncoding.EncodeToString(header)

	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"onlyReturnExisting": true}`))
	hash := crypto.SHA512.New()
	hash.Write([]byte(protected + "." + payload))
	sig, err := client.Account.PrivateKey.Sign(rand.Reader, hash.Sum(nil), crypto.SHA512)
	if err != nil {
		t.Fatalf("creating fake signature: %s", err)
	}

	data, err := json.Marshal(&struct {
		Protected string `json:"protected"`
		Payload   string `json:"payload"`
		Signature string `json:"signature"`
	}{
		Protected: protected,
		Payload:   payload,
		Signature: base64.RawURLEncoding.EncodeToString(sig),
	})

	req, err := http.NewRequest(http.MethodPost, client.Directory().NewAccount, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("creating HTTP request: %s", err)
	}
	req.Header.Set("Content-Type", "application/jose+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("making HTTP request: %s", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading HTTP response: %s", err)
	}

	var prob struct {
		Type       string                    `json:"type"`
		Detail     string                    `json:"detail"`
		Status     int                       `json:"status"`
		Algorithms []jose.SignatureAlgorithm `json:"algorithms"`
	}
	err = json.Unmarshal(body, &prob)
	if err != nil {
		t.Fatalf("parsing HTTP response: %s", err)
	}

	if prob.Type != "urn:ietf:params:acme:error:badSignatureAlgorithm" {
		t.Errorf("problem document has wrong type: want badSignatureAlgorithm, got %s", prob.Type)
	}
	if prob.Status != http.StatusBadRequest {
		t.Errorf("problem document has wrong status: want 400, got %d", prob.Status)
	}
	if len(prob.Algorithms) == 0 {
		t.Error("problem document MUST contain acceptable algorithms, got none")
	}
}

// TestOrderFinalizeEarly tests that finalizing an order before it is fully
// authorized results in an orderNotReady error.
func TestOrderFinalizeEarly(t *testing.T) {
	t.Parallel()

	client, err := makeClient()
	if err != nil {
		t.Fatalf("creating acme client: %s", err)
	}

	idents := []acme.Identifier{{Type: "dns", Value: randomDomain(t)}}

	order, err := client.Client.NewOrder(client.Account, idents)
	if err != nil {
		t.Fatalf("creating order: %s", err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %s", err)
	}
	csr, err := makeCSR(key, idents, false)
	if err != nil {
		t.Fatalf("generating CSR: %s", err)
	}

	order, err = client.Client.FinalizeOrder(client.Account, order, csr)
	if err == nil {
		t.Fatal("expected finalize to fail, but got success")
	}
	var prob acme.Problem
	ok := errors.As(err, &prob)
	if !ok {
		t.Fatalf("expected error to be of type acme.Problem, got: %T", err)
	}
	if prob.Type != "urn:ietf:params:acme:error:orderNotReady" {
		t.Errorf("expected problem type 'urn:ietf:params:acme:error:orderNotReady', got: %s", prob.Type)
	}
	if order.Status != "pending" {
		t.Errorf("expected order status to be pending, got: %s", order.Status)
	}
}
