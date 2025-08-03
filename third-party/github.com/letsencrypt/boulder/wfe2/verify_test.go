package wfe2

import (
	"context"
	"crypto"
	"crypto/dsa"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/letsencrypt/boulder/core"
	corepb "github.com/letsencrypt/boulder/core/proto"
	berrors "github.com/letsencrypt/boulder/errors"
	"github.com/letsencrypt/boulder/goodkey"
	bgrpc "github.com/letsencrypt/boulder/grpc"
	"github.com/letsencrypt/boulder/grpc/noncebalancer"
	noncepb "github.com/letsencrypt/boulder/nonce/proto"
	sapb "github.com/letsencrypt/boulder/sa/proto"
	"github.com/letsencrypt/boulder/test"
	"github.com/letsencrypt/boulder/web"

	"github.com/go-jose/go-jose/v4"
	"google.golang.org/grpc"
)

// sigAlgForKey uses `signatureAlgorithmForKey` but fails immediately using the
// testing object if the sig alg is unknown.
func sigAlgForKey(t *testing.T, key interface{}) jose.SignatureAlgorithm {
	var sigAlg jose.SignatureAlgorithm
	var err error
	// Gracefully handle the case where a non-pointer public key is given where
	// sigAlgorithmForKey always wants a pointer. It may be tempting to try and do
	// `sigAlgorithmForKey(&jose.JSONWebKey{Key: &key})` without a type switch but this produces
	// `*interface {}` and not the desired `*rsa.PublicKey` or `*ecdsa.PublicKey`.
	switch k := key.(type) {
	case rsa.PublicKey:
		sigAlg, err = sigAlgorithmForKey(&jose.JSONWebKey{Key: &k})
	case ecdsa.PublicKey:
		sigAlg, err = sigAlgorithmForKey(&jose.JSONWebKey{Key: &k})
	default:
		sigAlg, err = sigAlgorithmForKey(&jose.JSONWebKey{Key: k})
	}
	test.Assert(t, err == nil, fmt.Sprintf("Error getting signature algorithm for key %#v", key))
	return sigAlg
}

// keyAlgForKey returns a JWK key algorithm based on the provided private key.
// Only ECDSA and RSA private keys are supported.
func keyAlgForKey(t *testing.T, key interface{}) string {
	switch key.(type) {
	case *rsa.PrivateKey, rsa.PrivateKey:
		return "RSA"
	case *ecdsa.PrivateKey, ecdsa.PrivateKey:
		return "ECDSA"
	}
	t.Fatalf("Can't figure out keyAlgForKey: %#v", key)
	return ""
}

// pubKeyForKey returns the public key of an RSA/ECDSA private key provided as
// argument.
func pubKeyForKey(t *testing.T, privKey interface{}) interface{} {
	switch k := privKey.(type) {
	case *rsa.PrivateKey:
		return k.PublicKey
	case *ecdsa.PrivateKey:
		return k.PublicKey
	}
	t.Fatalf("Unable to get public key for private key %#v", privKey)
	return nil
}

// requestSigner offers methods to sign requests that will be accepted by a
// specific WFE in unittests. It is only valid for the lifetime of a single
// unittest.
type requestSigner struct {
	t            *testing.T
	nonceService jose.NonceSource
}

// embeddedJWK creates a JWS for a given request body with an embedded JWK
// corresponding to the private key provided. The URL and nonce extra headers
// are set based on the additional arguments. A computed JWS, the corresponding
// embedded JWK and the JWS in serialized string form are returned.
func (rs requestSigner) embeddedJWK(
	privateKey interface{},
	url string,
	req string) (*jose.JSONWebSignature, *jose.JSONWebKey, string) {
	// if no key is provided default to test1KeyPrivatePEM
	var publicKey interface{}
	if privateKey == nil {
		signer := loadKey(rs.t, []byte(test1KeyPrivatePEM))
		privateKey = signer
		publicKey = signer.Public()
	} else {
		publicKey = pubKeyForKey(rs.t, privateKey)
	}

	signerKey := jose.SigningKey{
		Key:       privateKey,
		Algorithm: sigAlgForKey(rs.t, publicKey),
	}

	opts := &jose.SignerOptions{
		NonceSource: rs.nonceService,
		EmbedJWK:    true,
	}
	if url != "" {
		opts.ExtraHeaders = map[jose.HeaderKey]interface{}{
			"url": url,
		}
	}

	signer, err := jose.NewSigner(signerKey, opts)
	test.AssertNotError(rs.t, err, "Failed to make signer")

	jws, err := signer.Sign([]byte(req))
	test.AssertNotError(rs.t, err, "Failed to sign req")

	body := jws.FullSerialize()
	parsedJWS, err := jose.ParseSigned(body, getSupportedAlgs())
	test.AssertNotError(rs.t, err, "Failed to parse generated JWS")

	return parsedJWS, parsedJWS.Signatures[0].Header.JSONWebKey, body
}

// signRequestKeyID creates a JWS for a given request body with key ID specified
// based on the ID number provided. The URL and nonce extra headers
// are set based on the additional arguments. A computed JWS, the corresponding
// embedded JWK and the JWS in serialized string form are returned.
func (rs requestSigner) byKeyID(
	keyID int64,
	privateKey interface{},
	url string,
	req string) (*jose.JSONWebSignature, *jose.JSONWebKey, string) {
	// if no key is provided default to test1KeyPrivatePEM
	if privateKey == nil {
		privateKey = loadKey(rs.t, []byte(test1KeyPrivatePEM))
	}

	jwk := &jose.JSONWebKey{
		Key:       privateKey,
		Algorithm: keyAlgForKey(rs.t, privateKey),
		KeyID:     fmt.Sprintf("http://localhost/acme/acct/%d", keyID),
	}

	signerKey := jose.SigningKey{
		Key:       jwk,
		Algorithm: jose.RS256,
	}

	opts := &jose.SignerOptions{
		NonceSource: rs.nonceService,
		ExtraHeaders: map[jose.HeaderKey]interface{}{
			"url": url,
		},
	}

	signer, err := jose.NewSigner(signerKey, opts)
	test.AssertNotError(rs.t, err, "Failed to make signer")
	jws, err := signer.Sign([]byte(req))
	test.AssertNotError(rs.t, err, "Failed to sign req")

	body := jws.FullSerialize()
	parsedJWS, err := jose.ParseSigned(body, getSupportedAlgs())
	test.AssertNotError(rs.t, err, "Failed to parse generated JWS")

	return parsedJWS, jwk, body
}

// missingNonce returns an otherwise well-signed request that is missing its
// nonce.
func (rs requestSigner) missingNonce() *jose.JSONWebSignature {
	privateKey := loadKey(rs.t, []byte(test1KeyPrivatePEM))
	jwk := &jose.JSONWebKey{
		Key:       privateKey,
		Algorithm: keyAlgForKey(rs.t, privateKey),
		KeyID:     "http://localhost/acme/acct/1",
	}
	signerKey := jose.SigningKey{
		Key:       jwk,
		Algorithm: jose.RS256,
	}

	opts := &jose.SignerOptions{
		ExtraHeaders: map[jose.HeaderKey]interface{}{
			"url": "https://example.com/acme/foo",
		},
	}

	signer, err := jose.NewSigner(signerKey, opts)
	test.AssertNotError(rs.t, err, "Failed to make signer")
	jws, err := signer.Sign([]byte(""))
	test.AssertNotError(rs.t, err, "Failed to sign req")

	return jws
}

// invalidNonce returns an otherwise well-signed request with an invalid nonce.
func (rs requestSigner) invalidNonce() *jose.JSONWebSignature {
	privateKey := loadKey(rs.t, []byte(test1KeyPrivatePEM))
	jwk := &jose.JSONWebKey{
		Key:       privateKey,
		Algorithm: keyAlgForKey(rs.t, privateKey),
		KeyID:     "http://localhost/acme/acct/1",
	}
	signerKey := jose.SigningKey{
		Key:       jwk,
		Algorithm: jose.RS256,
	}

	opts := &jose.SignerOptions{
		NonceSource: badNonceProvider{},
		ExtraHeaders: map[jose.HeaderKey]interface{}{
			"url": "https://example.com/acme/foo",
		},
	}

	signer, err := jose.NewSigner(signerKey, opts)
	test.AssertNotError(rs.t, err, "Failed to make signer")
	jws, err := signer.Sign([]byte(""))
	test.AssertNotError(rs.t, err, "Failed to sign req")

	body := jws.FullSerialize()
	parsedJWS, err := jose.ParseSigned(body, getSupportedAlgs())
	test.AssertNotError(rs.t, err, "Failed to parse generated JWS")

	return parsedJWS
}

// malformedNonce returns an otherwise well-signed request with a malformed
// nonce.
func (rs requestSigner) malformedNonce() *jose.JSONWebSignature {
	privateKey := loadKey(rs.t, []byte(test1KeyPrivatePEM))
	jwk := &jose.JSONWebKey{
		Key:       privateKey,
		Algorithm: keyAlgForKey(rs.t, privateKey),
		KeyID:     "http://localhost/acme/acct/1",
	}
	signerKey := jose.SigningKey{
		Key:       jwk,
		Algorithm: jose.RS256,
	}

	opts := &jose.SignerOptions{
		NonceSource: badNonceProvider{malformed: true},
		ExtraHeaders: map[jose.HeaderKey]interface{}{
			"url": "https://example.com/acme/foo",
		},
	}

	signer, err := jose.NewSigner(signerKey, opts)
	test.AssertNotError(rs.t, err, "Failed to make signer")
	jws, err := signer.Sign([]byte(""))
	test.AssertNotError(rs.t, err, "Failed to sign req")

	body := jws.FullSerialize()
	parsedJWS, err := jose.ParseSigned(body, getSupportedAlgs())
	test.AssertNotError(rs.t, err, "Failed to parse generated JWS")

	return parsedJWS
}

// shortNonce returns an otherwise well-signed request with a nonce shorter than
// the prefix length.
func (rs requestSigner) shortNonce() *jose.JSONWebSignature {
	privateKey := loadKey(rs.t, []byte(test1KeyPrivatePEM))
	jwk := &jose.JSONWebKey{
		Key:       privateKey,
		Algorithm: keyAlgForKey(rs.t, privateKey),
		KeyID:     "http://localhost/acme/acct/1",
	}
	signerKey := jose.SigningKey{
		Key:       jwk,
		Algorithm: jose.RS256,
	}

	opts := &jose.SignerOptions{
		NonceSource: badNonceProvider{shortNonce: true},
		ExtraHeaders: map[jose.HeaderKey]interface{}{
			"url": "https://example.com/acme/foo",
		},
	}

	signer, err := jose.NewSigner(signerKey, opts)
	test.AssertNotError(rs.t, err, "Failed to make signer")
	jws, err := signer.Sign([]byte(""))
	test.AssertNotError(rs.t, err, "Failed to sign req")

	body := jws.FullSerialize()
	parsedJWS, err := jose.ParseSigned(body, getSupportedAlgs())
	test.AssertNotError(rs.t, err, "Failed to parse generated JWS")

	return parsedJWS
}

func TestRejectsNone(t *testing.T) {
	noneJWSBody := `
		{
			"header": {
				"alg": "none",
				"jwk": {
					"kty": "RSA",
					"n": "vrjT",
					"e": "AQAB"
				}
			},
			"payload": "aGkK",
  		"signature": "ghTIjrhiRl2pQ09vAkUUBbF5KziJdhzOTB-okM9SPRzU8Hyj0W1H5JA1Zoc-A-LuJGNAtYYHWqMw1SeZbT0l9FHcbMPeWDaJNkHS9jz5_g_Oyol8vcrWur2GDtB2Jgw6APtZKrbuGATbrF7g41Wijk6Kk9GXDoCnlfOQOhHhsrFFcWlCPLG-03TtKD6EBBoVBhmlp8DRLs7YguWRZ6jWNaEX-1WiRntBmhLqoqQFtvZxCBw_PRuaRw_RZBd1x2_BNYqEdOmVNC43UHMSJg3y_3yrPo905ur09aUTscf-C_m4Sa4M0FuDKn3bQ_pFrtz-aCCq6rcTIyxYpDqNvHMT2Q"
		}
	`
	_, err := jose.ParseSigned(noneJWSBody, getSupportedAlgs())
	test.AssertError(t, err, "Should not have been able to parse 'none' algorithm")
}

func TestRejectsHS256(t *testing.T) {
	hs256JWSBody := `
		{
			"header": {
				"alg": "HS256",
				"jwk": {
					"kty": "RSA",
					"n": "vrjT",
					"e": "AQAB"
				}
			},
			"payload": "aGkK",
  		"signature": "ghTIjrhiRl2pQ09vAkUUBbF5KziJdhzOTB-okM9SPRzU8Hyj0W1H5JA1Zoc-A-LuJGNAtYYHWqMw1SeZbT0l9FHcbMPeWDaJNkHS9jz5_g_Oyol8vcrWur2GDtB2Jgw6APtZKrbuGATbrF7g41Wijk6Kk9GXDoCnlfOQOhHhsrFFcWlCPLG-03TtKD6EBBoVBhmlp8DRLs7YguWRZ6jWNaEX-1WiRntBmhLqoqQFtvZxCBw_PRuaRw_RZBd1x2_BNYqEdOmVNC43UHMSJg3y_3yrPo905ur09aUTscf-C_m4Sa4M0FuDKn3bQ_pFrtz-aCCq6rcTIyxYpDqNvHMT2Q"
		}
	`

	_, err := jose.ParseSigned(hs256JWSBody, getSupportedAlgs())
	fmt.Println(err)
	test.AssertError(t, err, "Parsed hs256JWSBody, but should not have")
}

func TestCheckAlgorithm(t *testing.T) {
	testCases := []struct {
		key         jose.JSONWebKey
		jws         jose.JSONWebSignature
		expectedErr string
	}{
		{
			jose.JSONWebKey{},
			jose.JSONWebSignature{
				Signatures: []jose.Signature{
					{
						Header: jose.Header{
							Algorithm: "RS256",
						},
					},
				},
			},
			"JWK contains unsupported key type (expected RSA, or ECDSA P-256, P-384, or P-521)",
		},
		{
			jose.JSONWebKey{
				Algorithm: "HS256",
				Key:       &rsa.PublicKey{},
			},
			jose.JSONWebSignature{
				Signatures: []jose.Signature{
					{
						Header: jose.Header{
							Algorithm: "HS256",
						},
					},
				},
			},
			"JWS signature header contains unsupported algorithm \"HS256\", expected one of [RS256 ES256 ES384 ES512]",
		},
		{
			jose.JSONWebKey{
				Algorithm: "ES256",
				Key:       &dsa.PublicKey{},
			},
			jose.JSONWebSignature{
				Signatures: []jose.Signature{
					{
						Header: jose.Header{
							Algorithm: "ES512",
						},
					},
				},
			},
			"JWK contains unsupported key type (expected RSA, or ECDSA P-256, P-384, or P-521)",
		},
		{
			jose.JSONWebKey{
				Algorithm: "RS256",
				Key:       &rsa.PublicKey{},
			},
			jose.JSONWebSignature{
				Signatures: []jose.Signature{
					{
						Header: jose.Header{
							Algorithm: "ES512",
						},
					},
				},
			},
			"JWS signature header algorithm \"ES512\" does not match expected algorithm \"RS256\" for JWK",
		},
		{
			jose.JSONWebKey{
				Algorithm: "HS256",
				Key:       &rsa.PublicKey{},
			},
			jose.JSONWebSignature{
				Signatures: []jose.Signature{
					{
						Header: jose.Header{
							Algorithm: "RS256",
						},
					},
				},
			},
			"JWK key header algorithm \"HS256\" does not match expected algorithm \"RS256\" for JWK",
		},
	}
	for i, tc := range testCases {
		err := checkAlgorithm(&tc.key, tc.jws.Signatures[0].Header)
		if tc.expectedErr != "" && err.Error() != tc.expectedErr {
			t.Errorf("TestCheckAlgorithm %d: Expected %q, got %q", i, tc.expectedErr, err)
		}
	}
}

func TestCheckAlgorithmSuccess(t *testing.T) {
	jwsRS256 := &jose.JSONWebSignature{
		Signatures: []jose.Signature{
			{
				Header: jose.Header{
					Algorithm: "RS256",
				},
			},
		},
	}
	goodJSONWebKeyRS256 := &jose.JSONWebKey{
		Algorithm: "RS256",
		Key:       &rsa.PublicKey{},
	}
	err := checkAlgorithm(goodJSONWebKeyRS256, jwsRS256.Signatures[0].Header)
	test.AssertNotError(t, err, "RS256 key: Expected nil error")

	badJSONWebKeyRS256 := &jose.JSONWebKey{
		Algorithm: "ObviouslyWrongButNotZeroValue",
		Key:       &rsa.PublicKey{},
	}
	err = checkAlgorithm(badJSONWebKeyRS256, jwsRS256.Signatures[0].Header)
	test.AssertError(t, err, "RS256 key: Expected nil error")
	test.AssertContains(t, err.Error(), "JWK key header algorithm \"ObviouslyWrongButNotZeroValue\" does not match expected algorithm \"RS256\" for JWK")

	jwsES256 := &jose.JSONWebSignature{
		Signatures: []jose.Signature{
			{
				Header: jose.Header{
					Algorithm: "ES256",
				},
			},
		},
	}
	goodJSONWebKeyES256 := &jose.JSONWebKey{
		Algorithm: "ES256",
		Key: &ecdsa.PublicKey{
			Curve: elliptic.P256(),
		},
	}
	err = checkAlgorithm(goodJSONWebKeyES256, jwsES256.Signatures[0].Header)
	test.AssertNotError(t, err, "ES256 key: Expected nil error")

	badJSONWebKeyES256 := &jose.JSONWebKey{
		Algorithm: "ObviouslyWrongButNotZeroValue",
		Key: &ecdsa.PublicKey{
			Curve: elliptic.P256(),
		},
	}
	err = checkAlgorithm(badJSONWebKeyES256, jwsES256.Signatures[0].Header)
	test.AssertError(t, err, "ES256 key: Expected nil error")
	test.AssertContains(t, err.Error(), "JWK key header algorithm \"ObviouslyWrongButNotZeroValue\" does not match expected algorithm \"ES256\" for JWK")
}

func TestValidPOSTRequest(t *testing.T) {
	wfe, _, _ := setupWFE(t)

	dummyContentLength := []string{"pretty long, idk, maybe a nibble or two?"}

	testCases := []struct {
		Name               string
		Headers            map[string][]string
		Body               *string
		HTTPStatus         int
		ErrorDetail        string
		ErrorStatType      string
		EnforceContentType bool
	}{
		// POST requests without a Content-Length should produce a problem
		{
			Name:          "POST without a Content-Length header",
			Headers:       nil,
			HTTPStatus:    http.StatusLengthRequired,
			ErrorDetail:   "missing Content-Length header",
			ErrorStatType: "ContentLengthRequired",
		},
		// POST requests with a Replay-Nonce header should produce a problem
		{
			Name: "POST with a Replay-Nonce HTTP header",
			Headers: map[string][]string{
				"Content-Length": dummyContentLength,
				"Replay-Nonce":   {"ima-misplaced-nonce"},
				"Content-Type":   {expectedJWSContentType},
			},
			HTTPStatus:    http.StatusBadRequest,
			ErrorDetail:   "HTTP requests should NOT contain Replay-Nonce header. Use JWS nonce field",
			ErrorStatType: "ReplayNonceOutsideJWS",
		},
		// POST requests without a body should produce a problem
		{
			Name: "POST with an empty POST body",
			Headers: map[string][]string{
				"Content-Length": dummyContentLength,
				"Content-Type":   {expectedJWSContentType},
			},
			HTTPStatus:    http.StatusBadRequest,
			ErrorDetail:   "No body on POST",
			ErrorStatType: "NoPOSTBody",
		},
		{
			Name: "POST without a Content-Type header",
			Headers: map[string][]string{
				"Content-Length": dummyContentLength,
			},
			HTTPStatus: http.StatusUnsupportedMediaType,
			ErrorDetail: fmt.Sprintf(
				"No Content-Type header on POST. Content-Type must be %q",
				expectedJWSContentType),
			ErrorStatType:      "NoContentType",
			EnforceContentType: true,
		},
		{
			Name: "POST with an invalid Content-Type header",
			Headers: map[string][]string{
				"Content-Length": dummyContentLength,
				"Content-Type":   {"fresh.and.rare"},
			},
			HTTPStatus: http.StatusUnsupportedMediaType,
			ErrorDetail: fmt.Sprintf(
				"Invalid Content-Type header on POST. Content-Type must be %q",
				expectedJWSContentType),
			ErrorStatType:      "WrongContentType",
			EnforceContentType: true,
		},
	}

	for _, tc := range testCases {
		input := &http.Request{
			Method: "POST",
			URL:    mustParseURL("/"),
			Header: tc.Headers,
		}
		t.Run(tc.Name, func(t *testing.T) {
			err := wfe.validPOSTRequest(input)
			test.AssertError(t, err, "No error returned for invalid POST")
			test.AssertErrorIs(t, err, berrors.Malformed)
			test.AssertContains(t, err.Error(), tc.ErrorDetail)
			test.AssertMetricWithLabelsEquals(
				t, wfe.stats.httpErrorCount, prometheus.Labels{"type": tc.ErrorStatType}, 1)
		})
	}
}

func TestEnforceJWSAuthType(t *testing.T) {
	wfe, _, signer := setupWFE(t)

	testKeyIDJWS, _, _ := signer.byKeyID(1, nil, "", "")
	testEmbeddedJWS, _, _ := signer.embeddedJWK(nil, "", "")

	// A hand crafted JWS that has both a Key ID and an embedded JWK
	conflictJWSBody := `
{
  "header": {
    "alg": "RS256", 
    "jwk": {
      "e": "AQAB", 
      "kty": "RSA", 
      "n": "ppbqGaMFnnq9TeMUryR6WW4Lr5WMgp46KlBXZkNaGDNQoifWt6LheeR5j9MgYkIFU7Z8Jw5-bpJzuBeEVwb-yHGh4Umwo_qKtvAJd44iLjBmhBSxq-OSe6P5hX1LGCByEZlYCyoy98zOtio8VK_XyS5VoOXqchCzBXYf32ksVUTrtH1jSlamKHGz0Q0pRKIsA2fLqkE_MD3jP6wUDD6ExMw_tKYLx21lGcK41WSrRpDH-kcZo1QdgCy2ceNzaliBX1eHmKG0-H8tY4tPQudk-oHQmWTdvUIiHO6gSKMGDZNWv6bq74VTCsRfUEAkuWhqUhgRSGzlvlZ24wjHv5Qdlw"
    }
  }, 
  "protected": "eyJub25jZSI6ICJibTl1WTJVIiwgInVybCI6ICJodHRwOi8vbG9jYWxob3N0L3Rlc3QiLCAia2lkIjogInRlc3RrZXkifQ", 
  "payload": "Zm9v", 
  "signature": "ghTIjrhiRl2pQ09vAkUUBbF5KziJdhzOTB-okM9SPRzU8Hyj0W1H5JA1Zoc-A-LuJGNAtYYHWqMw1SeZbT0l9FHcbMPeWDaJNkHS9jz5_g_Oyol8vcrWur2GDtB2Jgw6APtZKrbuGATbrF7g41Wijk6Kk9GXDoCnlfOQOhHhsrFFcWlCPLG-03TtKD6EBBoVBhmlp8DRLs7YguWRZ6jWNaEX-1WiRntBmhLqoqQFtvZxCBw_PRuaRw_RZBd1x2_BNYqEdOmVNC43UHMSJg3y_3yrPo905ur09aUTscf-C_m4Sa4M0FuDKn3bQ_pFrtz-aCCq6rcTIyxYpDqNvHMT2Q"
}
`

	conflictJWS, err := jose.ParseSigned(conflictJWSBody, getSupportedAlgs())
	if err != nil {
		t.Fatal("Unable to parse conflict JWS")
	}

	testCases := []struct {
		Name          string
		JWS           *jose.JSONWebSignature
		AuthType      jwsAuthType
		WantErrType   berrors.ErrorType
		WantErrDetail string
		WantStatType  string
	}{
		{
			Name:          "Key ID and embedded JWS",
			JWS:           conflictJWS,
			AuthType:      invalidAuthType,
			WantErrType:   berrors.Malformed,
			WantErrDetail: "jwk and kid header fields are mutually exclusive",
			WantStatType:  "JWSAuthTypeInvalid",
		},
		{
			Name:          "Key ID when expected is embedded JWK",
			JWS:           testKeyIDJWS,
			AuthType:      embeddedJWK,
			WantErrType:   berrors.Malformed,
			WantErrDetail: "No embedded JWK in JWS header",
			WantStatType:  "JWSAuthTypeWrong",
		},
		{
			Name:          "Embedded JWK when expected is Key ID",
			JWS:           testEmbeddedJWS,
			AuthType:      embeddedKeyID,
			WantErrType:   berrors.Malformed,
			WantErrDetail: "No Key ID in JWS header",
			WantStatType:  "JWSAuthTypeWrong",
		},
		{
			Name:     "Key ID when expected is KeyID",
			JWS:      testKeyIDJWS,
			AuthType: embeddedKeyID,
		},
		{
			Name:     "Embedded JWK when expected is embedded JWK",
			JWS:      testEmbeddedJWS,
			AuthType: embeddedJWK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			wfe.stats.joseErrorCount.Reset()
			in := tc.JWS.Signatures[0].Header

			gotErr := wfe.enforceJWSAuthType(in, tc.AuthType)
			if tc.WantErrDetail == "" {
				if gotErr != nil {
					t.Fatalf("enforceJWSAuthType(%#v, %#v) = %#v, want nil", in, tc.AuthType, gotErr)
				}
			} else {
				berr, ok := gotErr.(*berrors.BoulderError)
				if !ok {
					t.Fatalf("enforceJWSAuthType(%#v, %#v) returned %T, want BoulderError", in, tc.AuthType, gotErr)
				}
				if berr.Type != tc.WantErrType {
					t.Errorf("enforceJWSAuthType(%#v, %#v) = %#v, want %#v", in, tc.AuthType, berr.Type, tc.WantErrType)
				}
				if !strings.Contains(berr.Detail, tc.WantErrDetail) {
					t.Errorf("enforceJWSAuthType(%#v, %#v) = %q, want %q", in, tc.AuthType, berr.Detail, tc.WantErrDetail)
				}
				test.AssertMetricWithLabelsEquals(
					t, wfe.stats.joseErrorCount, prometheus.Labels{"type": tc.WantStatType}, 1)
			}
		})
	}
}

type badNonceProvider struct {
	malformed  bool
	shortNonce bool
}

func (b badNonceProvider) Nonce() (string, error) {
	if b.malformed {
		return "im-a-nonce", nil
	}
	if b.shortNonce {
		// A nonce length of 4 is considered "short" because there is no nonce
		// material to be redeemed after the prefix. Derived prefixes are 8
		// characters and static prefixes are 4 characters.
		return "woww", nil
	}
	return "mlolmlol3ov77I5Ui-cdaY_k8IcjK58FvbG0y_BCRrx5rGQ8rjA", nil
}

func TestValidNonce(t *testing.T) {
	wfe, _, signer := setupWFE(t)

	goodJWS, _, _ := signer.embeddedJWK(nil, "", "")

	testCases := []struct {
		Name          string
		JWS           *jose.JSONWebSignature
		WantErrType   berrors.ErrorType
		WantErrDetail string
		WantStatType  string
	}{
		{
			Name:          "No nonce in JWS",
			JWS:           signer.missingNonce(),
			WantErrType:   berrors.BadNonce,
			WantErrDetail: "JWS has no anti-replay nonce",
			WantStatType:  "JWSMissingNonce",
		},
		{
			Name:          "Malformed nonce in JWS",
			JWS:           signer.malformedNonce(),
			WantErrType:   berrors.BadNonce,
			WantErrDetail: "JWS has an invalid anti-replay nonce: \"im-a-nonce\"",
			WantStatType:  "JWSMalformedNonce",
		},
		{
			Name:          "Canned nonce shorter than prefixLength in JWS",
			JWS:           signer.shortNonce(),
			WantErrType:   berrors.BadNonce,
			WantErrDetail: "JWS has an invalid anti-replay nonce: \"woww\"",
			WantStatType:  "JWSMalformedNonce",
		},
		{
			Name:          "Invalid nonce in JWS (test/config-next)",
			JWS:           signer.invalidNonce(),
			WantErrType:   berrors.BadNonce,
			WantErrDetail: "JWS has an invalid anti-replay nonce: \"mlolmlol3ov77I5Ui-cdaY_k8IcjK58FvbG0y_BCRrx5rGQ8rjA\"",
			WantStatType:  "JWSInvalidNonce",
		},
		{
			Name: "Valid nonce in JWS",
			JWS:  goodJWS,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			in := tc.JWS.Signatures[0].Header
			wfe.stats.joseErrorCount.Reset()

			gotErr := wfe.validNonce(context.Background(), in)
			if tc.WantErrDetail == "" {
				if gotErr != nil {
					t.Fatalf("validNonce(%#v) = %#v, want nil", in, gotErr)
				}
			} else {
				berr, ok := gotErr.(*berrors.BoulderError)
				if !ok {
					t.Fatalf("validNonce(%#v) returned %T, want BoulderError", in, gotErr)
				}
				if berr.Type != tc.WantErrType {
					t.Errorf("validNonce(%#v) = %#v, want %#v", in, berr.Type, tc.WantErrType)
				}
				if !strings.Contains(berr.Detail, tc.WantErrDetail) {
					t.Errorf("validNonce(%#v) = %q, want %q", in, berr.Detail, tc.WantErrDetail)
				}
				test.AssertMetricWithLabelsEquals(
					t, wfe.stats.joseErrorCount, prometheus.Labels{"type": tc.WantStatType}, 1)
			}
		})
	}
}

// noBackendsNonceRedeemer is a nonce redeemer that always returns an error
// indicating that the prefix matches no known nonce provider.
type noBackendsNonceRedeemer struct{}

func (n noBackendsNonceRedeemer) Redeem(ctx context.Context, _ *noncepb.NonceMessage, opts ...grpc.CallOption) (*noncepb.ValidMessage, error) {
	return nil, noncebalancer.ErrNoBackendsMatchPrefix.Err()
}

func TestValidNonce_NoMatchingBackendFound(t *testing.T) {
	wfe, _, signer := setupWFE(t)
	goodJWS, _, _ := signer.embeddedJWK(nil, "", "")
	wfe.rnc = noBackendsNonceRedeemer{}

	// A valid JWS with a nonce whose prefix matches no known nonce provider should
	// result in a BadNonceProblem.
	err := wfe.validNonce(context.Background(), goodJWS.Signatures[0].Header)
	test.AssertError(t, err, "Expected error for valid nonce with no backend")
	test.AssertErrorIs(t, err, berrors.BadNonce)
	test.AssertContains(t, err.Error(), "JWS has an invalid anti-replay nonce")
	test.AssertMetricWithLabelsEquals(t, wfe.stats.nonceNoMatchingBackendCount, prometheus.Labels{}, 1)
}

func (rs requestSigner) signExtraHeaders(
	headers map[jose.HeaderKey]interface{}) (*jose.JSONWebSignature, string) {
	privateKey := loadKey(rs.t, []byte(test1KeyPrivatePEM))

	signerKey := jose.SigningKey{
		Key:       privateKey,
		Algorithm: sigAlgForKey(rs.t, privateKey.Public()),
	}

	opts := &jose.SignerOptions{
		NonceSource:  rs.nonceService,
		EmbedJWK:     true,
		ExtraHeaders: headers,
	}

	signer, err := jose.NewSigner(signerKey, opts)
	test.AssertNotError(rs.t, err, "Failed to make signer")

	jws, err := signer.Sign([]byte(""))
	test.AssertNotError(rs.t, err, "Failed to sign req")

	body := jws.FullSerialize()
	parsedJWS, err := jose.ParseSigned(body, getSupportedAlgs())
	test.AssertNotError(rs.t, err, "Failed to parse generated JWS")

	return parsedJWS, body
}

func TestValidPOSTURL(t *testing.T) {
	wfe, _, signer := setupWFE(t)

	// A JWS and HTTP request with no extra headers
	noHeadersJWS, noHeadersJWSBody := signer.signExtraHeaders(nil)
	noHeadersRequest := makePostRequestWithPath("test-path", noHeadersJWSBody)

	// A JWS and HTTP request with extra headers, but no "url" extra header
	noURLHeaders := map[jose.HeaderKey]interface{}{
		"nifty": "swell",
	}
	noURLHeaderJWS, noURLHeaderJWSBody := signer.signExtraHeaders(noURLHeaders)
	noURLHeaderRequest := makePostRequestWithPath("test-path", noURLHeaderJWSBody)

	// A JWS and HTTP request with a mismatched HTTP URL to JWS "url" header
	wrongURLHeaders := map[jose.HeaderKey]interface{}{
		"url": "foobar",
	}
	wrongURLHeaderJWS, wrongURLHeaderJWSBody := signer.signExtraHeaders(wrongURLHeaders)
	wrongURLHeaderRequest := makePostRequestWithPath("test-path", wrongURLHeaderJWSBody)

	correctURLHeaderJWS, _, correctURLHeaderJWSBody := signer.embeddedJWK(nil, "http://localhost/test-path", "")
	correctURLHeaderRequest := makePostRequestWithPath("test-path", correctURLHeaderJWSBody)

	testCases := []struct {
		Name          string
		JWS           *jose.JSONWebSignature
		Request       *http.Request
		WantErrType   berrors.ErrorType
		WantErrDetail string
		WantStatType  string
	}{
		{
			Name:          "No extra headers in JWS",
			JWS:           noHeadersJWS,
			Request:       noHeadersRequest,
			WantErrType:   berrors.Malformed,
			WantErrDetail: "JWS header parameter 'url' required",
			WantStatType:  "JWSNoExtraHeaders",
		},
		{
			Name:          "No URL header in JWS",
			JWS:           noURLHeaderJWS,
			Request:       noURLHeaderRequest,
			WantErrType:   berrors.Malformed,
			WantErrDetail: "JWS header parameter 'url' required",
			WantStatType:  "JWSMissingURL",
		},
		{
			Name:          "Wrong URL header in JWS",
			JWS:           wrongURLHeaderJWS,
			Request:       wrongURLHeaderRequest,
			WantErrType:   berrors.Malformed,
			WantErrDetail: "JWS header parameter 'url' incorrect. Expected \"http://localhost/test-path\" got \"foobar\"",
			WantStatType:  "JWSMismatchedURL",
		},
		{
			Name:    "Correct URL header in JWS",
			JWS:     correctURLHeaderJWS,
			Request: correctURLHeaderRequest,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			in := tc.JWS.Signatures[0].Header
			tc.Request.Header.Add("Content-Type", expectedJWSContentType)
			wfe.stats.joseErrorCount.Reset()

			got := wfe.validPOSTURL(tc.Request, in)
			if tc.WantErrDetail == "" {
				if got != nil {
					t.Fatalf("validPOSTURL(%#v) = %#v, want nil", in, got)
				}
			} else {
				berr, ok := got.(*berrors.BoulderError)
				if !ok {
					t.Fatalf("validPOSTURL(%#v) returned %T, want BoulderError", in, got)
				}
				if berr.Type != tc.WantErrType {
					t.Errorf("validPOSTURL(%#v) = %#v, want %#v", in, berr.Type, tc.WantErrType)
				}
				if !strings.Contains(berr.Detail, tc.WantErrDetail) {
					t.Errorf("validPOSTURL(%#v) = %q, want %q", in, berr.Detail, tc.WantErrDetail)
				}
				test.AssertMetricWithLabelsEquals(
					t, wfe.stats.joseErrorCount, prometheus.Labels{"type": tc.WantStatType}, 1)
			}
		})
	}
}

func (rs requestSigner) multiSigJWS() (*jose.JSONWebSignature, string) {
	privateKeyA := loadKey(rs.t, []byte(test1KeyPrivatePEM))
	privateKeyB := loadKey(rs.t, []byte(test2KeyPrivatePEM))

	signerKeyA := jose.SigningKey{
		Key:       privateKeyA,
		Algorithm: sigAlgForKey(rs.t, privateKeyA.Public()),
	}

	signerKeyB := jose.SigningKey{
		Key:       privateKeyB,
		Algorithm: sigAlgForKey(rs.t, privateKeyB.Public()),
	}

	opts := &jose.SignerOptions{
		NonceSource: rs.nonceService,
		EmbedJWK:    true,
	}

	signer, err := jose.NewMultiSigner([]jose.SigningKey{signerKeyA, signerKeyB}, opts)
	test.AssertNotError(rs.t, err, "Failed to make multi signer")

	jws, err := signer.Sign([]byte(""))
	test.AssertNotError(rs.t, err, "Failed to sign req")

	body := jws.FullSerialize()
	parsedJWS, err := jose.ParseSigned(body, getSupportedAlgs())
	test.AssertNotError(rs.t, err, "Failed to parse generated JWS")

	return parsedJWS, body
}

func TestParseJWSRequest(t *testing.T) {
	wfe, _, signer := setupWFE(t)

	_, tooManySigsJWSBody := signer.multiSigJWS()

	_, _, validJWSBody := signer.embeddedJWK(nil, "http://localhost/test-path", "")
	validJWSRequest := makePostRequestWithPath("test-path", validJWSBody)

	missingSigsJWSBody := `{"payload":"Zm9x","protected":"eyJhbGciOiJSUzI1NiIsImp3ayI6eyJrdHkiOiJSU0EiLCJuIjoicW5BUkxyVDdYejRnUmNLeUxkeWRtQ3ItZXk5T3VQSW1YNFg0MHRoazNvbjI2RmtNem5SM2ZSanM2NmVMSzdtbVBjQlo2dU9Kc2VVUlU2d0FhWk5tZW1vWXgxZE12cXZXV0l5aVFsZUhTRDdROHZCcmhSNnVJb080akF6SlpSLUNoelp1U0R0N2lITi0zeFVWc3B1NVhHd1hVX01WSlpzaFR3cDRUYUZ4NWVsSElUX09iblR2VE9VM1hoaXNoMDdBYmdaS21Xc1ZiWGg1cy1DcklpY1U0T2V4SlBndW5XWl9ZSkp1ZU9LbVR2bkxsVFY0TXpLUjJvWmxCS1oyN1MwLVNmZFZfUUR4X3lkbGU1b01BeUtWdGxBVjM1Y3lQTUlzWU53Z1VHQkNkWV8yVXppNWVYMGxUYzdNUFJ3ejZxUjFraXAtaTU5VmNHY1VRZ3FIVjZGeXF3IiwiZSI6IkFRQUIifSwia2lkIjoiIiwibm9uY2UiOiJyNHpuenZQQUVwMDlDN1JwZUtYVHhvNkx3SGwxZVBVdmpGeXhOSE1hQnVvIiwidXJsIjoiaHR0cDovL2xvY2FsaG9zdC9hY21lL25ldy1yZWcifQ"}`
	missingSigsJWSRequest := makePostRequestWithPath("test-path", missingSigsJWSBody)

	unprotectedHeadersJWSBody := `
{
  "header": {
    "alg": "RS256",
    "kid": "unprotected key id"
  },
  "protected": "eyJub25jZSI6ICJibTl1WTJVIiwgInVybCI6ICJodHRwOi8vbG9jYWxob3N0L3Rlc3QiLCAia2lkIjogInRlc3RrZXkifQ", 
  "payload": "Zm9v",
  "signature": "PKWWclRsiHF4bm-nmpxDez6Y_3Mdtu263YeYklbGYt1EiMOLiKY_dr_EqhUUKAKEWysFLO-hQLXVU7kVkHeYWQFFOA18oFgcZgkSF2Pr3DNZrVj9e2gl0eZ2i2jk6X5GYPt1lIfok_DrL92wrxEKGcrmxqXXGm0JgP6Al2VGapKZK2HaYbCHoGvtzNmzUX9rC21sKewq5CquJRvTmvQp5bmU7Q9KeafGibFr0jl6IA3W5LBGgf6xftuUtEVEbKmKaKtaG7tXsQH1mIVOPUZZoLWz9sWJSFLmV0QSXm3ZHV0DrOhLfcADbOCoQBMeGdseBQZuUO541A3BEKGv2Aikjw"
}
`

	wrongSignaturesFieldJWSBody := `
{
  "protected": "eyJub25jZSI6ICJibTl1WTJVIiwgInVybCI6ICJodHRwOi8vbG9jYWxob3N0L3Rlc3QiLCAia2lkIjogInRlc3RrZXkifQ", 
  "payload": "Zm9v",
  "signatures": ["PKWWclRsiHF4bm-nmpxDez6Y_3Mdtu263YeYklbGYt1EiMOLiKY_dr_EqhUUKAKEWysFLO-hQLXVU7kVkHeYWQFFOA18oFgcZgkSF2Pr3DNZrVj9e2gl0eZ2i2jk6X5GYPt1lIfok_DrL92wrxEKGcrmxqXXGm0JgP6Al2VGapKZK2HaYbCHoGvtzNmzUX9rC21sKewq5CquJRvTmvQp5bmU7Q9KeafGibFr0jl6IA3W5LBGgf6xftuUtEVEbKmKaKtaG7tXsQH1mIVOPUZZoLWz9sWJSFLmV0QSXm3ZHV0DrOhLfcADbOCoQBMeGdseBQZuUO541A3BEKGv2Aikjw"]
}
`
	wrongSignatureTypeJWSBody := `
{
  "protected": "eyJhbGciOiJIUzI1NiJ9",
  "payload" : "IiI",
  "signature" : "5WiUupHzCWfpJza6EMteSxMDY8_6xIV7HnKaUqmykIQ"
}
`

	testCases := []struct {
		Name          string
		Request       *http.Request
		WantErrType   berrors.ErrorType
		WantErrDetail string
		WantStatType  string
	}{
		{
			Name: "Invalid POST request",
			// No Content-Length, something that validPOSTRequest should be flagging
			Request: &http.Request{
				Method: "POST",
				URL:    mustParseURL("/"),
			},
			WantErrType:   berrors.Malformed,
			WantErrDetail: "missing Content-Length header",
		},
		{
			Name:          "Invalid JWS in POST body",
			Request:       makePostRequestWithPath("test-path", `{`),
			WantErrType:   berrors.Malformed,
			WantErrDetail: "Parse error reading JWS",
			WantStatType:  "JWSUnmarshalFailed",
		},
		{
			Name:          "Too few signatures in JWS",
			Request:       missingSigsJWSRequest,
			WantErrType:   berrors.Malformed,
			WantErrDetail: "POST JWS not signed",
			WantStatType:  "JWSEmptySignature",
		},
		{
			Name:          "Too many signatures in JWS",
			Request:       makePostRequestWithPath("test-path", tooManySigsJWSBody),
			WantErrType:   berrors.Malformed,
			WantErrDetail: "JWS \"signatures\" field not allowed. Only the \"signature\" field should contain a signature",
			WantStatType:  "JWSMultiSig",
		},
		{
			Name:          "Unprotected JWS headers",
			Request:       makePostRequestWithPath("test-path", unprotectedHeadersJWSBody),
			WantErrType:   berrors.Malformed,
			WantErrDetail: "JWS \"header\" field not allowed. All headers must be in \"protected\" field",
			WantStatType:  "JWSUnprotectedHeaders",
		},
		{
			Name:          "Unsupported signatures field in JWS",
			Request:       makePostRequestWithPath("test-path", wrongSignaturesFieldJWSBody),
			WantErrType:   berrors.Malformed,
			WantErrDetail: "JWS \"signatures\" field not allowed. Only the \"signature\" field should contain a signature",
			WantStatType:  "JWSMultiSig",
		},
		{
			Name:          "JWS with an invalid algorithm",
			Request:       makePostRequestWithPath("test-path", wrongSignatureTypeJWSBody),
			WantErrType:   berrors.BadSignatureAlgorithm,
			WantErrDetail: "JWS signature header contains unsupported algorithm \"HS256\", expected one of [RS256 ES256 ES384 ES512]",
			WantStatType:  "JWSAlgorithmCheckFailed",
		},
		{
			Name:    "Valid JWS in POST request",
			Request: validJWSRequest,
		},
		{
			Name:          "POST body too large",
			Request:       makePostRequestWithPath("test-path", fmt.Sprintf(`{"a":"%s"}`, strings.Repeat("a", 50000))),
			WantErrType:   berrors.Unauthorized,
			WantErrDetail: "request body too large",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			wfe.stats.joseErrorCount.Reset()

			_, gotErr := wfe.parseJWSRequest(tc.Request)
			if tc.WantErrDetail == "" {
				if gotErr != nil {
					t.Fatalf("parseJWSRequest(%#v) = %#v, want nil", tc.Request, gotErr)
				}
			} else {
				berr, ok := gotErr.(*berrors.BoulderError)
				if !ok {
					t.Fatalf("parseJWSRequest(%#v) returned %T, want BoulderError", tc.Request, gotErr)
				}
				if berr.Type != tc.WantErrType {
					t.Errorf("parseJWSRequest(%#v) = %#v, want %#v", tc.Request, berr.Type, tc.WantErrType)
				}
				if !strings.Contains(berr.Detail, tc.WantErrDetail) {
					t.Errorf("parseJWSRequest(%#v) = %q, want %q", tc.Request, berr.Detail, tc.WantErrDetail)
				}
				if tc.WantStatType != "" {
					test.AssertMetricWithLabelsEquals(
						t, wfe.stats.joseErrorCount, prometheus.Labels{"type": tc.WantStatType}, 1)
				}
			}
		})
	}
}

func TestExtractJWK(t *testing.T) {
	wfe, _, signer := setupWFE(t)

	keyIDJWS, _, _ := signer.byKeyID(1, nil, "", "")
	goodJWS, goodJWK, _ := signer.embeddedJWK(nil, "", "")

	testCases := []struct {
		Name          string
		JWS           *jose.JSONWebSignature
		WantKey       *jose.JSONWebKey
		WantErrType   berrors.ErrorType
		WantErrDetail string
	}{
		{
			Name:          "JWS with wrong auth type (Key ID vs embedded JWK)",
			JWS:           keyIDJWS,
			WantErrType:   berrors.Malformed,
			WantErrDetail: "No embedded JWK in JWS header",
		},
		{
			Name:    "Valid JWS with embedded JWK",
			JWS:     goodJWS,
			WantKey: goodJWK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			in := tc.JWS.Signatures[0].Header

			gotKey, gotErr := wfe.extractJWK(in)
			if tc.WantErrDetail == "" {
				if gotErr != nil {
					t.Fatalf("extractJWK(%#v) = %#v, want nil", in, gotKey)
				}
				test.AssertMarshaledEquals(t, gotKey, tc.WantKey)
			} else {
				berr, ok := gotErr.(*berrors.BoulderError)
				if !ok {
					t.Fatalf("extractJWK(%#v) returned %T, want BoulderError", in, gotErr)
				}
				if berr.Type != tc.WantErrType {
					t.Errorf("extractJWK(%#v) = %#v, want %#v", in, berr.Type, tc.WantErrType)
				}
				if !strings.Contains(berr.Detail, tc.WantErrDetail) {
					t.Errorf("extractJWK(%#v) = %q, want %q", in, berr.Detail, tc.WantErrDetail)
				}
			}
		})
	}
}

func (rs requestSigner) specifyKeyID(keyID string) (*jose.JSONWebSignature, string) {
	privateKey := loadKey(rs.t, []byte(test1KeyPrivatePEM))

	if keyID == "" {
		keyID = "this is an invalid non-numeric key ID"
	}

	jwk := &jose.JSONWebKey{
		Key:       privateKey,
		Algorithm: "RSA",
		KeyID:     keyID,
	}

	signerKey := jose.SigningKey{
		Key:       jwk,
		Algorithm: jose.RS256,
	}

	opts := &jose.SignerOptions{
		NonceSource: rs.nonceService,
		ExtraHeaders: map[jose.HeaderKey]interface{}{
			"url": "http://localhost",
		},
	}

	signer, err := jose.NewSigner(signerKey, opts)
	test.AssertNotError(rs.t, err, "Failed to make signer")

	jws, err := signer.Sign([]byte(""))
	test.AssertNotError(rs.t, err, "Failed to sign req")

	body := jws.FullSerialize()
	parsedJWS, err := jose.ParseSigned(body, getSupportedAlgs())
	test.AssertNotError(rs.t, err, "Failed to parse generated JWS")

	return parsedJWS, body
}

func TestLookupJWK(t *testing.T) {
	wfe, _, signer := setupWFE(t)

	embeddedJWS, _, embeddedJWSBody := signer.embeddedJWK(nil, "", "")
	invalidKeyIDJWS, invalidKeyIDJWSBody := signer.specifyKeyID("https://acme-99.lettuceencrypt.org/acme/reg/1")
	// ID 100 is mocked to return a non-missing error from sa.GetRegistration
	errorIDJWS, _, errorIDJWSBody := signer.byKeyID(100, nil, "", "")
	// ID 102 is mocked to return an account does not exist error from sa.GetRegistration
	missingIDJWS, _, missingIDJWSBody := signer.byKeyID(102, nil, "", "")
	// ID 3 is mocked to return a deactivated account from sa.GetRegistration
	deactivatedIDJWS, _, deactivatedIDJWSBody := signer.byKeyID(3, nil, "", "")

	wfe.LegacyKeyIDPrefix = "https://acme-v00.lettuceencrypt.org/acme/reg/"
	legacyKeyIDJWS, legacyKeyIDJWSBody := signer.specifyKeyID(wfe.LegacyKeyIDPrefix + "1")

	nonNumericKeyIDJWS, nonNumericKeyIDJWSBody := signer.specifyKeyID(wfe.LegacyKeyIDPrefix + "abcd")

	validJWS, validKey, validJWSBody := signer.byKeyID(1, nil, "", "")
	validAccountPB, _ := wfe.sa.GetRegistration(context.Background(), &sapb.RegistrationID{Id: 1})
	validAccount, _ := bgrpc.PbToRegistration(validAccountPB)

	// good key, log event requester is set

	testCases := []struct {
		Name          string
		JWS           *jose.JSONWebSignature
		Request       *http.Request
		WantJWK       *jose.JSONWebKey
		WantAccount   *core.Registration
		WantErrType   berrors.ErrorType
		WantErrDetail string
		WantStatType  string
	}{
		{
			Name:          "JWS with wrong auth type (embedded JWK vs Key ID)",
			JWS:           embeddedJWS,
			Request:       makePostRequestWithPath("test-path", embeddedJWSBody),
			WantErrType:   berrors.Malformed,
			WantErrDetail: "No Key ID in JWS header",
			WantStatType:  "JWSAuthTypeWrong",
		},
		{
			Name:          "JWS with invalid key ID URL",
			JWS:           invalidKeyIDJWS,
			Request:       makePostRequestWithPath("test-path", invalidKeyIDJWSBody),
			WantErrType:   berrors.Malformed,
			WantErrDetail: "KeyID header contained an invalid account URL: \"https://acme-99.lettuceencrypt.org/acme/reg/1\"",
			WantStatType:  "JWSInvalidKeyID",
		},
		{
			Name:          "JWS with non-numeric account ID in key ID URL",
			JWS:           nonNumericKeyIDJWS,
			Request:       makePostRequestWithPath("test-path", nonNumericKeyIDJWSBody),
			WantErrType:   berrors.Malformed,
			WantErrDetail: "Malformed account ID in KeyID header URL: \"https://acme-v00.lettuceencrypt.org/acme/reg/abcd\"",
			WantStatType:  "JWSInvalidKeyID",
		},
		{
			Name:          "JWS with account ID that causes GetRegistration error",
			JWS:           errorIDJWS,
			Request:       makePostRequestWithPath("test-path", errorIDJWSBody),
			WantErrType:   berrors.InternalServer,
			WantErrDetail: "Error retrieving account \"http://localhost/acme/acct/100\"",
			WantStatType:  "JWSKeyIDLookupFailed",
		},
		{
			Name:          "JWS with account ID that doesn't exist",
			JWS:           missingIDJWS,
			Request:       makePostRequestWithPath("test-path", missingIDJWSBody),
			WantErrType:   berrors.AccountDoesNotExist,
			WantErrDetail: "Account \"http://localhost/acme/acct/102\" not found",
			WantStatType:  "JWSKeyIDNotFound",
		},
		{
			Name:          "JWS with account ID that is deactivated",
			JWS:           deactivatedIDJWS,
			Request:       makePostRequestWithPath("test-path", deactivatedIDJWSBody),
			WantErrType:   berrors.Unauthorized,
			WantErrDetail: "Account is not valid, has status \"deactivated\"",
			WantStatType:  "JWSKeyIDAccountInvalid",
		},
		{
			Name:        "Valid JWS with legacy account ID",
			JWS:         legacyKeyIDJWS,
			Request:     makePostRequestWithPath("test-path", legacyKeyIDJWSBody),
			WantJWK:     validKey,
			WantAccount: &validAccount,
		},
		{
			Name:        "Valid JWS with valid account ID",
			JWS:         validJWS,
			Request:     makePostRequestWithPath("test-path", validJWSBody),
			WantJWK:     validKey,
			WantAccount: &validAccount,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			wfe.stats.joseErrorCount.Reset()
			in := tc.JWS.Signatures[0].Header
			inputLogEvent := newRequestEvent()

			gotJWK, gotAcct, gotErr := wfe.lookupJWK(in, context.Background(), tc.Request, inputLogEvent)
			if tc.WantErrDetail == "" {
				if gotErr != nil {
					t.Fatalf("lookupJWK(%#v) = %#v, want nil", in, gotErr)
				}
				gotThumb, _ := gotJWK.Thumbprint(crypto.SHA256)
				wantThumb, _ := tc.WantJWK.Thumbprint(crypto.SHA256)
				if !slices.Equal(gotThumb, wantThumb) {
					t.Fatalf("lookupJWK(%#v) = %#v, want %#v", tc.Request, gotThumb, wantThumb)
				}
				test.AssertMarshaledEquals(t, gotAcct, tc.WantAccount)
				test.AssertEquals(t, inputLogEvent.Requester, gotAcct.ID)
			} else {
				var berr *berrors.BoulderError
				ok := errors.As(gotErr, &berr)
				if !ok {
					t.Fatalf("lookupJWK(%#v) returned %T, want BoulderError", in, gotErr)
				}
				if berr.Type != tc.WantErrType {
					t.Errorf("lookupJWK(%#v) = %#v, want %#v", in, berr.Type, tc.WantErrType)
				}
				if !strings.Contains(berr.Detail, tc.WantErrDetail) {
					t.Errorf("lookupJWK(%#v) = %q, want %q", in, berr.Detail, tc.WantErrDetail)
				}
				test.AssertMetricWithLabelsEquals(
					t, wfe.stats.joseErrorCount, prometheus.Labels{"type": tc.WantStatType}, 1)
			}
		})
	}
}

func TestValidJWSForKey(t *testing.T) {
	wfe, _, signer := setupWFE(t)

	payload := `{ "test": "payload" }`
	testURL := "http://localhost/test"
	goodJWS, goodJWK, _ := signer.embeddedJWK(nil, testURL, payload)

	// badSigJWSBody is a JWS that has had the payload changed by 1 byte to break the signature
	badSigJWSBody := `{"payload":"Zm9x","protected":"eyJhbGciOiJSUzI1NiIsImp3ayI6eyJrdHkiOiJSU0EiLCJuIjoicW5BUkxyVDdYejRnUmNLeUxkeWRtQ3ItZXk5T3VQSW1YNFg0MHRoazNvbjI2RmtNem5SM2ZSanM2NmVMSzdtbVBjQlo2dU9Kc2VVUlU2d0FhWk5tZW1vWXgxZE12cXZXV0l5aVFsZUhTRDdROHZCcmhSNnVJb080akF6SlpSLUNoelp1U0R0N2lITi0zeFVWc3B1NVhHd1hVX01WSlpzaFR3cDRUYUZ4NWVsSElUX09iblR2VE9VM1hoaXNoMDdBYmdaS21Xc1ZiWGg1cy1DcklpY1U0T2V4SlBndW5XWl9ZSkp1ZU9LbVR2bkxsVFY0TXpLUjJvWmxCS1oyN1MwLVNmZFZfUUR4X3lkbGU1b01BeUtWdGxBVjM1Y3lQTUlzWU53Z1VHQkNkWV8yVXppNWVYMGxUYzdNUFJ3ejZxUjFraXAtaTU5VmNHY1VRZ3FIVjZGeXF3IiwiZSI6IkFRQUIifSwia2lkIjoiIiwibm9uY2UiOiJyNHpuenZQQUVwMDlDN1JwZUtYVHhvNkx3SGwxZVBVdmpGeXhOSE1hQnVvIiwidXJsIjoiaHR0cDovL2xvY2FsaG9zdC9hY21lL25ldy1yZWcifQ","signature":"jcTdxSygm_cvD7KbXqsxgnoPApCTSkV4jolToSOd2ciRkg5W7Yl0ZKEEKwOc-dYIbQiwGiDzisyPCicwWsOUA1WSqHylKvZ3nxSMc6KtwJCW2DaOqcf0EEjy5VjiZJUrOt2c-r6b07tbn8sfOJKwlF2lsOeGi4s-rtvvkeQpAU-AWauzl9G4bv2nDUeCviAZjHx_PoUC-f9GmZhYrbDzAvXZ859ktM6RmMeD0OqPN7bhAeju2j9Gl0lnryZMtq2m0J2m1ucenQBL1g4ZkP1JiJvzd2cAz5G7Ftl2YeJJyWhqNd3qq0GVOt1P11s8PTGNaSoM0iR9QfUxT9A6jxARtg"}`
	badJWS, err := jose.ParseSigned(badSigJWSBody, getSupportedAlgs())
	test.AssertNotError(t, err, "error loading badSigJWS body")

	// wrongAlgJWS is a JWS that has an invalid "HS256" algorithm in its header
	wrongAlgJWS := &jose.JSONWebSignature{
		Signatures: []jose.Signature{
			{
				Header: jose.Header{
					Algorithm: "HS256",
				},
			},
		},
	}

	// A JWS and HTTP request with a mismatched HTTP URL to JWS "url" header
	wrongURLHeaders := map[jose.HeaderKey]interface{}{
		"url": "foobar",
	}
	wrongURLHeaderJWS, _ := signer.signExtraHeaders(wrongURLHeaders)

	// badJSONJWS has a valid signature over a body that is not valid JSON
	badJSONJWS, _, _ := signer.embeddedJWK(nil, testURL, `{`)

	testCases := []struct {
		Name          string
		JWS           bJSONWebSignature
		JWK           *jose.JSONWebKey
		Body          string
		WantErrType   berrors.ErrorType
		WantErrDetail string
		WantStatType  string
	}{
		{
			Name:          "JWS with an invalid algorithm",
			JWS:           bJSONWebSignature{wrongAlgJWS},
			JWK:           goodJWK,
			WantErrType:   berrors.BadSignatureAlgorithm,
			WantErrDetail: "JWS signature header contains unsupported algorithm \"HS256\", expected one of [RS256 ES256 ES384 ES512]",
			WantStatType:  "JWSAlgorithmCheckFailed",
		},
		{
			Name:          "JWS with an invalid nonce (test/config-next)",
			JWS:           bJSONWebSignature{signer.invalidNonce()},
			JWK:           goodJWK,
			WantErrType:   berrors.BadNonce,
			WantErrDetail: "JWS has an invalid anti-replay nonce: \"mlolmlol3ov77I5Ui-cdaY_k8IcjK58FvbG0y_BCRrx5rGQ8rjA\"",
			WantStatType:  "JWSInvalidNonce",
		},
		{
			Name:          "JWS with broken signature",
			JWS:           bJSONWebSignature{badJWS},
			JWK:           badJWS.Signatures[0].Header.JSONWebKey,
			WantErrType:   berrors.Malformed,
			WantErrDetail: "JWS verification error",
			WantStatType:  "JWSVerifyFailed",
		},
		{
			Name:          "JWS with incorrect URL",
			JWS:           bJSONWebSignature{wrongURLHeaderJWS},
			JWK:           wrongURLHeaderJWS.Signatures[0].Header.JSONWebKey,
			WantErrType:   berrors.Malformed,
			WantErrDetail: "JWS header parameter 'url' incorrect. Expected \"http://localhost/test\" got \"foobar\"",
			WantStatType:  "JWSMismatchedURL",
		},
		{
			Name:          "Valid JWS with invalid JSON in the protected body",
			JWS:           bJSONWebSignature{badJSONJWS},
			JWK:           goodJWK,
			WantErrType:   berrors.Malformed,
			WantErrDetail: "Request payload did not parse as JSON",
			WantStatType:  "JWSBodyUnmarshalFailed",
		},
		{
			Name: "Good JWS and JWK",
			JWS:  bJSONWebSignature{goodJWS},
			JWK:  goodJWK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			wfe.stats.joseErrorCount.Reset()
			request := makePostRequestWithPath("test", tc.Body)

			gotPayload, gotErr := wfe.validJWSForKey(context.Background(), &tc.JWS, tc.JWK, request)
			if tc.WantErrDetail == "" {
				if gotErr != nil {
					t.Fatalf("validJWSForKey(%#v, %#v, %#v) = %#v, want nil", tc.JWS, tc.JWK, request, gotErr)
				}
				if string(gotPayload) != payload {
					t.Fatalf("validJWSForKey(%#v, %#v, %#v) = %q, want %q", tc.JWS, tc.JWK, request, string(gotPayload), payload)
				}
			} else {
				berr, ok := gotErr.(*berrors.BoulderError)
				if !ok {
					t.Fatalf("validJWSForKey(%#v, %#v, %#v) returned %T, want BoulderError", tc.JWS, tc.JWK, request, gotErr)
				}
				if berr.Type != tc.WantErrType {
					t.Errorf("validJWSForKey(%#v, %#v, %#v) = %#v, want %#v", tc.JWS, tc.JWK, request, berr.Type, tc.WantErrType)
				}
				if !strings.Contains(berr.Detail, tc.WantErrDetail) {
					t.Errorf("validJWSForKey(%#v, %#v, %#v) = %q, want %q", tc.JWS, tc.JWK, request, berr.Detail, tc.WantErrDetail)
				}
				test.AssertMetricWithLabelsEquals(
					t, wfe.stats.joseErrorCount, prometheus.Labels{"type": tc.WantStatType}, 1)
			}
		})
	}
}

func TestValidPOSTForAccount(t *testing.T) {
	wfe, _, signer := setupWFE(t)

	validJWS, _, validJWSBody := signer.byKeyID(1, nil, "http://localhost/test", `{"test":"passed"}`)
	validAccountPB, _ := wfe.sa.GetRegistration(context.Background(), &sapb.RegistrationID{Id: 1})
	validAccount, _ := bgrpc.PbToRegistration(validAccountPB)

	// ID 102 is mocked to return missing
	_, _, missingJWSBody := signer.byKeyID(102, nil, "http://localhost/test", "{}")

	// ID 3 is mocked to return deactivated
	key3 := loadKey(t, []byte(test3KeyPrivatePEM))
	_, _, deactivatedJWSBody := signer.byKeyID(3, key3, "http://localhost/test", "{}")

	_, _, embeddedJWSBody := signer.embeddedJWK(nil, "http://localhost/test", `{"test":"passed"}`)

	testCases := []struct {
		Name          string
		Request       *http.Request
		WantPayload   string
		WantAcct      *core.Registration
		WantJWS       *jose.JSONWebSignature
		WantErrType   berrors.ErrorType
		WantErrDetail string
		WantStatType  string
	}{
		{
			Name:          "Invalid JWS",
			Request:       makePostRequestWithPath("test", "foo"),
			WantErrType:   berrors.Malformed,
			WantErrDetail: "Parse error reading JWS",
			WantStatType:  "JWSUnmarshalFailed",
		},
		{
			Name:          "Embedded Key JWS",
			Request:       makePostRequestWithPath("test", embeddedJWSBody),
			WantErrType:   berrors.Malformed,
			WantErrDetail: "No Key ID in JWS header",
			WantStatType:  "JWSAuthTypeWrong",
		},
		{
			Name:          "JWS signed by account that doesn't exist",
			Request:       makePostRequestWithPath("test", missingJWSBody),
			WantErrType:   berrors.AccountDoesNotExist,
			WantErrDetail: "Account \"http://localhost/acme/acct/102\" not found",
			WantStatType:  "JWSKeyIDNotFound",
		},
		{
			Name:          "JWS signed by account that's deactivated",
			Request:       makePostRequestWithPath("test", deactivatedJWSBody),
			WantErrType:   berrors.Unauthorized,
			WantErrDetail: "Account is not valid, has status \"deactivated\"",
			WantStatType:  "JWSKeyIDAccountInvalid",
		},
		{
			Name:        "Valid JWS for account",
			Request:     makePostRequestWithPath("test", validJWSBody),
			WantPayload: `{"test":"passed"}`,
			WantAcct:    &validAccount,
			WantJWS:     validJWS,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			wfe.stats.joseErrorCount.Reset()
			inputLogEvent := newRequestEvent()

			gotPayload, gotJWS, gotAcct, gotErr := wfe.validPOSTForAccount(tc.Request, context.Background(), inputLogEvent)
			if tc.WantErrDetail == "" {
				if gotErr != nil {
					t.Fatalf("validPOSTForAccount(%#v) = %#v, want nil", tc.Request, gotErr)
				}
				if string(gotPayload) != tc.WantPayload {
					t.Fatalf("validPOSTForAccount(%#v) = %q, want %q", tc.Request, string(gotPayload), tc.WantPayload)
				}
				test.AssertMarshaledEquals(t, gotJWS, tc.WantJWS)
				test.AssertMarshaledEquals(t, gotAcct, tc.WantAcct)
			} else {
				berr, ok := gotErr.(*berrors.BoulderError)
				if !ok {
					t.Fatalf("validPOSTForAccount(%#v) returned %T, want BoulderError", tc.Request, gotErr)
				}
				if berr.Type != tc.WantErrType {
					t.Errorf("validPOSTForAccount(%#v) = %#v, want %#v", tc.Request, berr.Type, tc.WantErrType)
				}
				if !strings.Contains(berr.Detail, tc.WantErrDetail) {
					t.Errorf("validPOSTForAccount(%#v) = %q, want %q", tc.Request, berr.Detail, tc.WantErrDetail)
				}
				test.AssertMetricWithLabelsEquals(
					t, wfe.stats.joseErrorCount, prometheus.Labels{"type": tc.WantStatType}, 1)
			}
		})
	}
}

// TestValidPOSTAsGETForAccount tests POST-as-GET processing. Because
// wfe.validPOSTAsGETForAccount calls `wfe.validPOSTForAccount` to do all
// processing except the empty body test we do not duplicate the
// `TestValidPOSTForAccount` testcases here.
func TestValidPOSTAsGETForAccount(t *testing.T) {
	wfe, _, signer := setupWFE(t)

	// an invalid POST-as-GET request contains a non-empty payload. In this case
	// we test with the empty JSON payload ("{}")
	_, _, invalidPayloadRequest := signer.byKeyID(1, nil, "http://localhost/test", "{}")
	// a valid POST-as-GET request contains an empty payload.
	_, _, validRequest := signer.byKeyID(1, nil, "http://localhost/test", "")

	testCases := []struct {
		Name          string
		Request       *http.Request
		WantErrType   berrors.ErrorType
		WantErrDetail string
		WantLogEvent  web.RequestEvent
	}{
		{
			Name:          "Non-empty JWS payload",
			Request:       makePostRequestWithPath("test", invalidPayloadRequest),
			WantErrType:   berrors.Malformed,
			WantErrDetail: "POST-as-GET requests must have an empty payload",
			WantLogEvent:  web.RequestEvent{},
		},
		{
			Name:    "Valid POST-as-GET",
			Request: makePostRequestWithPath("test", validRequest),
			WantLogEvent: web.RequestEvent{
				Method: "POST-as-GET",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			ev := newRequestEvent()
			_, gotErr := wfe.validPOSTAsGETForAccount(tc.Request, context.Background(), ev)
			if tc.WantErrDetail == "" {
				if gotErr != nil {
					t.Fatalf("validPOSTAsGETForAccount(%#v) = %#v, want nil", tc.Request, gotErr)
				}
			} else {
				berr, ok := gotErr.(*berrors.BoulderError)
				if !ok {
					t.Fatalf("validPOSTAsGETForAccount(%#v) returned %T, want BoulderError", tc.Request, gotErr)
				}
				if berr.Type != tc.WantErrType {
					t.Errorf("validPOSTAsGETForAccount(%#v) = %#v, want %#v", tc.Request, berr.Type, tc.WantErrType)
				}
				if !strings.Contains(berr.Detail, tc.WantErrDetail) {
					t.Errorf("validPOSTAsGETForAccount(%#v) = %q, want %q", tc.Request, berr.Detail, tc.WantErrDetail)
				}
			}
			test.AssertMarshaledEquals(t, *ev, tc.WantLogEvent)
		})
	}
}

type mockSADifferentStoredKey struct {
	sapb.StorageAuthorityReadOnlyClient
}

// mockSADifferentStoredKey has a GetRegistration that will always return an
// account with the test 2 key, no matter the provided ID
func (sa mockSADifferentStoredKey) GetRegistration(_ context.Context, _ *sapb.RegistrationID, _ ...grpc.CallOption) (*corepb.Registration, error) {
	return &corepb.Registration{
		Key:    []byte(test2KeyPublicJSON),
		Status: string(core.StatusValid),
	}, nil
}

func TestValidPOSTForAccountSwappedKey(t *testing.T) {
	wfe, _, signer := setupWFE(t)
	wfe.sa = &mockSADifferentStoredKey{}
	wfe.accountGetter = wfe.sa
	event := newRequestEvent()

	payload := `{"resource":"ima-payload"}`
	// Sign a request using test1key
	_, _, body := signer.byKeyID(1, nil, "http://localhost:4001/test", payload)
	request := makePostRequestWithPath("test", body)

	// Ensure that ValidPOSTForAccount produces an error since the
	// mockSADifferentStoredKey will return a different key than the one we used to
	// sign the request
	_, _, _, err := wfe.validPOSTForAccount(request, ctx, event)
	test.AssertError(t, err, "No error returned for request signed by wrong key")
	test.AssertErrorIs(t, err, berrors.Malformed)
	test.AssertContains(t, err.Error(), "JWS verification error")
}

func TestValidSelfAuthenticatedPOSTGoodKeyErrors(t *testing.T) {
	wfe, _, signer := setupWFE(t)

	timeoutErrCheckFunc := func(ctx context.Context, keyHash []byte) (bool, error) {
		return false, context.DeadlineExceeded
	}

	kp, err := goodkey.NewPolicy(nil, timeoutErrCheckFunc)
	test.AssertNotError(t, err, "making key policy")

	wfe.keyPolicy = kp

	_, _, validJWSBody := signer.embeddedJWK(nil, "http://localhost/test", `{"test":"passed"}`)
	request := makePostRequestWithPath("test", validJWSBody)

	_, _, err = wfe.validSelfAuthenticatedPOST(context.Background(), request)
	test.AssertErrorIs(t, err, berrors.InternalServer)

	badKeyCheckFunc := func(ctx context.Context, keyHash []byte) (bool, error) {
		return false, fmt.Errorf("oh no: %w", goodkey.ErrBadKey)
	}

	kp, err = goodkey.NewPolicy(nil, badKeyCheckFunc)
	test.AssertNotError(t, err, "making key policy")

	wfe.keyPolicy = kp

	_, _, validJWSBody = signer.embeddedJWK(nil, "http://localhost/test", `{"test":"passed"}`)
	request = makePostRequestWithPath("test", validJWSBody)

	_, _, err = wfe.validSelfAuthenticatedPOST(context.Background(), request)
	test.AssertErrorIs(t, err, berrors.BadPublicKey)
}

func TestValidSelfAuthenticatedPOST(t *testing.T) {
	wfe, _, signer := setupWFE(t)

	_, validKey, validJWSBody := signer.embeddedJWK(nil, "http://localhost/test", `{"test":"passed"}`)

	_, _, keyIDJWSBody := signer.byKeyID(1, nil, "http://localhost/test", `{"test":"passed"}`)

	testCases := []struct {
		Name          string
		Request       *http.Request
		WantPayload   string
		WantJWK       *jose.JSONWebKey
		WantErrType   berrors.ErrorType
		WantErrDetail string
		WantStatType  string
	}{
		{
			Name:          "Invalid JWS",
			Request:       makePostRequestWithPath("test", "foo"),
			WantErrType:   berrors.Malformed,
			WantErrDetail: "Parse error reading JWS",
			WantStatType:  "JWSUnmarshalFailed",
		},
		{
			Name:          "JWS with key ID",
			Request:       makePostRequestWithPath("test", keyIDJWSBody),
			WantErrType:   berrors.Malformed,
			WantErrDetail: "No embedded JWK in JWS header",
			WantStatType:  "JWSAuthTypeWrong",
		},
		{
			Name:        "Valid JWS",
			Request:     makePostRequestWithPath("test", validJWSBody),
			WantPayload: `{"test":"passed"}`,
			WantJWK:     validKey,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			wfe.stats.joseErrorCount.Reset()
			gotPayload, gotJWK, gotErr := wfe.validSelfAuthenticatedPOST(context.Background(), tc.Request)
			if tc.WantErrDetail == "" {
				if gotErr != nil {
					t.Fatalf("validSelfAuthenticatedPOST(%#v) = %#v, want nil", tc.Request, gotErr)
				}
				if string(gotPayload) != tc.WantPayload {
					t.Fatalf("validSelfAuthenticatedPOST(%#v) = %q, want %q", tc.Request, string(gotPayload), tc.WantPayload)
				}
				gotThumb, _ := gotJWK.Thumbprint(crypto.SHA256)
				wantThumb, _ := tc.WantJWK.Thumbprint(crypto.SHA256)
				if !slices.Equal(gotThumb, wantThumb) {
					t.Fatalf("validSelfAuthenticatedPOST(%#v) = %#v, want %#v", tc.Request, gotThumb, wantThumb)
				}
			} else {
				berr, ok := gotErr.(*berrors.BoulderError)
				if !ok {
					t.Fatalf("validSelfAuthenticatedPOST(%#v) returned %T, want BoulderError", tc.Request, gotErr)
				}
				if berr.Type != tc.WantErrType {
					t.Errorf("validSelfAuthenticatedPOST(%#v) = %#v, want %#v", tc.Request, berr.Type, tc.WantErrType)
				}
				if !strings.Contains(berr.Detail, tc.WantErrDetail) {
					t.Errorf("validSelfAuthenticatedPOST(%#v) = %q, want %q", tc.Request, berr.Detail, tc.WantErrDetail)
				}
				test.AssertMetricWithLabelsEquals(
					t, wfe.stats.joseErrorCount, prometheus.Labels{"type": tc.WantStatType}, 1)
			}
		})
	}
}

func TestMatchJWSURLs(t *testing.T) {
	wfe, _, signer := setupWFE(t)

	noURLJWS, _, _ := signer.embeddedJWK(nil, "", "")
	urlAJWS, _, _ := signer.embeddedJWK(nil, "example.com", "")
	urlBJWS, _, _ := signer.embeddedJWK(nil, "example.org", "")

	testCases := []struct {
		Name          string
		Outer         *jose.JSONWebSignature
		Inner         *jose.JSONWebSignature
		WantErrType   berrors.ErrorType
		WantErrDetail string
		WantStatType  string
	}{
		{
			Name:          "Outer JWS without URL",
			Outer:         noURLJWS,
			Inner:         urlAJWS,
			WantErrType:   berrors.Malformed,
			WantErrDetail: "Outer JWS header parameter 'url' required",
			WantStatType:  "KeyRolloverOuterJWSNoURL",
		},
		{
			Name:          "Inner JWS without URL",
			Outer:         urlAJWS,
			Inner:         noURLJWS,
			WantErrType:   berrors.Malformed,
			WantErrDetail: "Inner JWS header parameter 'url' required",
			WantStatType:  "KeyRolloverInnerJWSNoURL",
		},
		{
			Name:          "Inner and outer JWS without URL",
			Outer:         noURLJWS,
			Inner:         noURLJWS,
			WantErrType:   berrors.Malformed,
			WantErrDetail: "Outer JWS header parameter 'url' required",
			WantStatType:  "KeyRolloverOuterJWSNoURL",
		},
		{
			Name:          "Mismatched inner and outer JWS URLs",
			Outer:         urlAJWS,
			Inner:         urlBJWS,
			WantErrType:   berrors.Malformed,
			WantErrDetail: "Outer JWS 'url' value \"example.com\" does not match inner JWS 'url' value \"example.org\"",
			WantStatType:  "KeyRolloverMismatchedURLs",
		},
		{
			Name:  "Matching inner and outer JWS URLs",
			Outer: urlAJWS,
			Inner: urlAJWS,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			wfe.stats.joseErrorCount.Reset()
			outer := tc.Outer.Signatures[0].Header
			inner := tc.Inner.Signatures[0].Header

			gotErr := wfe.matchJWSURLs(outer, inner)
			if tc.WantErrDetail == "" {
				if gotErr != nil {
					t.Fatalf("matchJWSURLs(%#v, %#v) = %#v, want nil", outer, inner, gotErr)
				}
			} else {
				berr, ok := gotErr.(*berrors.BoulderError)
				if !ok {
					t.Fatalf("matchJWSURLs(%#v, %#v) returned %T, want BoulderError", outer, inner, gotErr)
				}
				if berr.Type != tc.WantErrType {
					t.Errorf("matchJWSURLs(%#v, %#v) = %#v, want %#v", outer, inner, berr.Type, tc.WantErrType)
				}
				if !strings.Contains(berr.Detail, tc.WantErrDetail) {
					t.Errorf("matchJWSURLs(%#v, %#v) = %q, want %q", outer, inner, berr.Detail, tc.WantErrDetail)
				}
				test.AssertMetricWithLabelsEquals(
					t, wfe.stats.joseErrorCount, prometheus.Labels{"type": tc.WantStatType}, 1)
			}
		})
	}
}
