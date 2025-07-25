package wfe2

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/go-jose/go-jose/v4"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc/status"

	"github.com/letsencrypt/boulder/core"
	berrors "github.com/letsencrypt/boulder/errors"
	"github.com/letsencrypt/boulder/goodkey"
	"github.com/letsencrypt/boulder/grpc"
	nb "github.com/letsencrypt/boulder/grpc/noncebalancer"
	"github.com/letsencrypt/boulder/nonce"
	noncepb "github.com/letsencrypt/boulder/nonce/proto"
	sapb "github.com/letsencrypt/boulder/sa/proto"
	"github.com/letsencrypt/boulder/web"
)

const (
	// POST requests with a JWS body must have the following Content-Type header
	expectedJWSContentType = "application/jose+json"

	maxRequestSize = 50000
)

func sigAlgorithmForKey(key *jose.JSONWebKey) (jose.SignatureAlgorithm, error) {
	switch k := key.Key.(type) {
	case *rsa.PublicKey:
		return jose.RS256, nil
	case *ecdsa.PublicKey:
		switch k.Params().Name {
		case "P-256":
			return jose.ES256, nil
		case "P-384":
			return jose.ES384, nil
		case "P-521":
			return jose.ES512, nil
		}
	}
	return "", berrors.BadPublicKeyError("JWK contains unsupported key type (expected RSA, or ECDSA P-256, P-384, or P-521)")
}

// getSupportedAlgs returns a sorted slice of joseSignatureAlgorithm's from a
// map of boulder allowed signature algorithms. We use a function for this to
// ensure that the source-of-truth slice can never be modified.
func getSupportedAlgs() []jose.SignatureAlgorithm {
	return []jose.SignatureAlgorithm{
		jose.RS256,
		jose.ES256,
		jose.ES384,
		jose.ES512,
	}
}

// Check that (1) there is a suitable algorithm for the provided key based on its
// Golang type, (2) the Algorithm field on the JWK is either absent, or matches
// that algorithm, and (3) the Algorithm field on the JWK is present and matches
// that algorithm.
func checkAlgorithm(key *jose.JSONWebKey, header jose.Header) error {
	sigHeaderAlg := jose.SignatureAlgorithm(header.Algorithm)
	if !slices.Contains(getSupportedAlgs(), sigHeaderAlg) {
		return berrors.BadSignatureAlgorithmError(
			"JWS signature header contains unsupported algorithm %q, expected one of %s",
			header.Algorithm, getSupportedAlgs(),
		)
	}

	expectedAlg, err := sigAlgorithmForKey(key)
	if err != nil {
		return err
	}
	if sigHeaderAlg != expectedAlg {
		return berrors.MalformedError("JWS signature header algorithm %q does not match expected algorithm %q for JWK", sigHeaderAlg, string(expectedAlg))
	}
	if key.Algorithm != "" && key.Algorithm != string(expectedAlg) {
		return berrors.MalformedError("JWK key header algorithm %q does not match expected algorithm %q for JWK", key.Algorithm, string(expectedAlg))
	}
	return nil
}

// jwsAuthType represents whether a given POST request is authenticated using
// a JWS with an embedded JWK (v1 ACME style, new-account, revoke-cert) or an
// embedded Key ID (v2 AMCE style) or an unsupported/unknown auth type.
type jwsAuthType int

const (
	embeddedJWK jwsAuthType = iota
	embeddedKeyID
	invalidAuthType
)

// checkJWSAuthType examines the protected headers from a bJSONWebSignature to
// determine if the request being authenticated by the JWS is identified using
// an embedded JWK or an embedded key ID. If no signatures are present, or
// mutually exclusive authentication types are specified at the same time, a
// error is returned. checkJWSAuthType is separate from enforceJWSAuthType so
// that endpoints that need to handle both embedded JWK and embedded key ID
// requests can determine which type of request they have and act accordingly
// (e.g. acme v2 cert revocation).
func checkJWSAuthType(header jose.Header) (jwsAuthType, error) {
	// There must not be a Key ID *and* an embedded JWK
	if header.KeyID != "" && header.JSONWebKey != nil {
		return invalidAuthType, berrors.MalformedError("jwk and kid header fields are mutually exclusive")
	} else if header.KeyID != "" {
		return embeddedKeyID, nil
	} else if header.JSONWebKey != nil {
		return embeddedJWK, nil
	}

	return invalidAuthType, nil
}

// enforceJWSAuthType enforces that the protected headers from a
// bJSONWebSignature have the provided auth type. If there is an error
// determining the auth type or if it is not the expected auth type then a
// error is returned.
func (wfe *WebFrontEndImpl) enforceJWSAuthType(
	header jose.Header,
	expectedAuthType jwsAuthType) error {
	// Check the auth type for the provided JWS
	authType, err := checkJWSAuthType(header)
	if err != nil {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSAuthTypeInvalid"}).Inc()
		return err
	}
	// If the auth type isn't the one expected return a sensible error based on
	// what was expected
	if authType != expectedAuthType {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSAuthTypeWrong"}).Inc()
		switch expectedAuthType {
		case embeddedKeyID:
			return berrors.MalformedError("No Key ID in JWS header")
		case embeddedJWK:
			return berrors.MalformedError("No embedded JWK in JWS header")
		}
	}
	return nil
}

// validPOSTRequest checks a *http.Request to ensure it has the headers
// a well-formed ACME POST request has, and to ensure there is a body to
// process.
func (wfe *WebFrontEndImpl) validPOSTRequest(request *http.Request) error {
	// All POSTs should have an accompanying Content-Length header
	if _, present := request.Header["Content-Length"]; !present {
		wfe.stats.httpErrorCount.With(prometheus.Labels{"type": "ContentLengthRequired"}).Inc()
		return berrors.MalformedError("missing Content-Length header")
	}

	// Per 6.2 ALL POSTs should have the correct JWS Content-Type for flattened
	// JSON serialization.
	if _, present := request.Header["Content-Type"]; !present {
		wfe.stats.httpErrorCount.With(prometheus.Labels{"type": "NoContentType"}).Inc()
		return berrors.MalformedError("No Content-Type header on POST. Content-Type must be %q", expectedJWSContentType)
	}
	if contentType := request.Header.Get("Content-Type"); contentType != expectedJWSContentType {
		wfe.stats.httpErrorCount.With(prometheus.Labels{"type": "WrongContentType"}).Inc()
		return berrors.MalformedError("Invalid Content-Type header on POST. Content-Type must be %q", expectedJWSContentType)
	}

	// Per 6.4.1 "Replay-Nonce" clients should not send a Replay-Nonce header in
	// the HTTP request, it needs to be part of the signed JWS request body
	if _, present := request.Header["Replay-Nonce"]; present {
		wfe.stats.httpErrorCount.With(prometheus.Labels{"type": "ReplayNonceOutsideJWS"}).Inc()
		return berrors.MalformedError("HTTP requests should NOT contain Replay-Nonce header. Use JWS nonce field")
	}

	// All POSTs should have a non-nil body
	if request.Body == nil {
		wfe.stats.httpErrorCount.With(prometheus.Labels{"type": "NoPOSTBody"}).Inc()
		return berrors.MalformedError("No body on POST")
	}

	return nil
}

// nonceWellFormed checks a JWS' Nonce header to ensure it is well-formed,
// otherwise a bad nonce error is returned. This avoids unnecessary RPCs to
// the nonce redemption service.
func nonceWellFormed(nonceHeader string, prefixLen int) error {
	errBadNonce := berrors.BadNonceError("JWS has an invalid anti-replay nonce: %q", nonceHeader)
	if len(nonceHeader) <= prefixLen {
		// Nonce header was an unexpected length because there is either:
		// 1) no nonce, or
		// 2) no nonce material after the prefix.
		return errBadNonce
	}
	body, err := base64.RawURLEncoding.DecodeString(nonceHeader[prefixLen:])
	if err != nil {
		// Nonce was not valid base64url.
		return errBadNonce
	}
	if len(body) != nonce.NonceLen {
		// Nonce was an unexpected length.
		return errBadNonce
	}
	return nil
}

// validNonce checks a JWS' Nonce header to ensure it is one that the
// nonceService knows about, otherwise a bad nonce error is returned.
// NOTE: this function assumes the JWS has already been verified with the
// correct public key.
func (wfe *WebFrontEndImpl) validNonce(ctx context.Context, header jose.Header) error {
	if len(header.Nonce) == 0 {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSMissingNonce"}).Inc()
		return berrors.BadNonceError("JWS has no anti-replay nonce")
	}

	err := nonceWellFormed(header.Nonce, nonce.PrefixLen)
	if err != nil {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSMalformedNonce"}).Inc()
		return err
	}

	// Populate the context with the nonce prefix and HMAC key. These are
	// used by a custom gRPC balancer, known as "noncebalancer", to route
	// redemption RPCs to the backend that originally issued the nonce.
	ctx = context.WithValue(ctx, nonce.PrefixCtxKey{}, header.Nonce[:nonce.PrefixLen])
	ctx = context.WithValue(ctx, nonce.HMACKeyCtxKey{}, wfe.rncKey)

	resp, err := wfe.rnc.Redeem(ctx, &noncepb.NonceMessage{Nonce: header.Nonce})
	if err != nil {
		rpcStatus, ok := status.FromError(err)
		if !ok || rpcStatus != nb.ErrNoBackendsMatchPrefix {
			return fmt.Errorf("failed to redeem nonce: %w", err)
		}

		// ErrNoBackendsMatchPrefix suggests that the nonce backend, which
		// issued this nonce, is presently unreachable or unrecognized by
		// this WFE. As this is a transient failure, the client should retry
		// their request with a fresh nonce.
		resp = &noncepb.ValidMessage{Valid: false}
		wfe.stats.nonceNoMatchingBackendCount.Inc()
	}

	if !resp.Valid {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSInvalidNonce"}).Inc()
		return berrors.BadNonceError("JWS has an invalid anti-replay nonce: %q", header.Nonce)
	}
	return nil
}

// validPOSTURL checks the JWS' URL header against the expected URL based on the
// HTTP request. This prevents a JWS intended for one endpoint being replayed
// against a different endpoint. If the URL isn't present, is invalid, or
// doesn't match the HTTP request a error is returned.
func (wfe *WebFrontEndImpl) validPOSTURL(
	request *http.Request,
	header jose.Header) error {
	extraHeaders := header.ExtraHeaders
	// Check that there is at least one Extra Header
	if len(extraHeaders) == 0 {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSNoExtraHeaders"}).Inc()
		return berrors.MalformedError("JWS header parameter 'url' required")
	}
	// Try to read a 'url' Extra Header as a string
	headerURL, ok := extraHeaders[jose.HeaderKey("url")].(string)
	if !ok || len(headerURL) == 0 {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSMissingURL"}).Inc()
		return berrors.MalformedError("JWS header parameter 'url' required")
	}
	// Compute the URL we expect to be in the JWS based on the HTTP request
	expectedURL := url.URL{
		Scheme: requestProto(request),
		Host:   request.Host,
		Path:   request.RequestURI,
	}
	// Check that the URL we expect is the one that was found in the signed JWS
	// header
	if expectedURL.String() != headerURL {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSMismatchedURL"}).Inc()
		return berrors.MalformedError("JWS header parameter 'url' incorrect. Expected %q got %q", expectedURL.String(), headerURL)
	}
	return nil
}

// matchJWSURLs checks two JWS' URL headers are equal. This is used during key
// rollover to check that the inner JWS URL matches the outer JWS URL. If the
// JWS URLs do not match a error is returned.
func (wfe *WebFrontEndImpl) matchJWSURLs(outer, inner jose.Header) error {
	// Verify that the outer JWS has a non-empty URL header. This is strictly
	// defensive since the expectation is that endpoints using `matchJWSURLs`
	// have received at least one of their JWS from calling validPOSTForAccount(),
	// which checks the outer JWS has the expected URL header before processing
	// the inner JWS.
	outerURL, ok := outer.ExtraHeaders[jose.HeaderKey("url")].(string)
	if !ok || len(outerURL) == 0 {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "KeyRolloverOuterJWSNoURL"}).Inc()
		return berrors.MalformedError("Outer JWS header parameter 'url' required")
	}

	// Verify the inner JWS has a non-empty URL header.
	innerURL, ok := inner.ExtraHeaders[jose.HeaderKey("url")].(string)
	if !ok || len(innerURL) == 0 {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "KeyRolloverInnerJWSNoURL"}).Inc()
		return berrors.MalformedError("Inner JWS header parameter 'url' required")
	}

	// Verify that the outer URL matches the inner URL
	if outerURL != innerURL {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "KeyRolloverMismatchedURLs"}).Inc()
		return berrors.MalformedError("Outer JWS 'url' value %q does not match inner JWS 'url' value %q", outerURL, innerURL)
	}

	return nil
}

// bJSONWebSignature is a new distinct type which embeds the
// *jose.JSONWebSignature concrete type. Callers must never create their own
// bJSONWebSignature. Instead they should rely upon wfe.parseJWS instead.
type bJSONWebSignature struct {
	*jose.JSONWebSignature
}

// parseJWS extracts a JSONWebSignature from a byte slice. If there is an error
// reading the JWS or it is unacceptable (e.g. too many/too few signatures,
// presence of unprotected headers) a error is returned, otherwise a
// *bJSONWebSignature is returned.
func (wfe *WebFrontEndImpl) parseJWS(body []byte) (*bJSONWebSignature, error) {
	// Parse the raw JWS JSON to check that:
	// * the unprotected Header field is not being used.
	// * the "signatures" member isn't present, just "signature".
	//
	// This must be done prior to `jose.parseSigned` since it will strip away
	// these headers.
	var unprotected struct {
		Header     map[string]string
		Signatures []interface{}
	}
	err := json.Unmarshal(body, &unprotected)
	if err != nil {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSUnmarshalFailed"}).Inc()
		return nil, berrors.MalformedError("Parse error reading JWS")
	}

	// ACME v2 never uses values from the unprotected JWS header. Reject JWS that
	// include unprotected headers.
	if unprotected.Header != nil {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSUnprotectedHeaders"}).Inc()
		return nil, berrors.MalformedError(
			"JWS \"header\" field not allowed. All headers must be in \"protected\" field")
	}

	// ACME v2 never uses the "signatures" array of JSON serialized JWS, just the
	// mandatory "signature" field. Reject JWS that include the "signatures" array.
	if len(unprotected.Signatures) > 0 {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSMultiSig"}).Inc()
		return nil, berrors.MalformedError(
			"JWS \"signatures\" field not allowed. Only the \"signature\" field should contain a signature")
	}

	// Parse the JWS using go-jose and enforce that the expected one non-empty
	// signature is present in the parsed JWS.
	bodyStr := string(body)
	parsedJWS, err := jose.ParseSigned(bodyStr, getSupportedAlgs())
	if err != nil {
		var unexpectedSignAlgoErr *jose.ErrUnexpectedSignatureAlgorithm
		if errors.As(err, &unexpectedSignAlgoErr) {
			wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSAlgorithmCheckFailed"}).Inc()
			return nil, berrors.BadSignatureAlgorithmError(
				"JWS signature header contains unsupported algorithm %q, expected one of %s",
				unexpectedSignAlgoErr.Got,
				getSupportedAlgs(),
			)
		}

		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSParseError"}).Inc()
		return nil, berrors.MalformedError("Parse error reading JWS")
	}
	if len(parsedJWS.Signatures) > 1 {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSTooManySignatures"}).Inc()
		return nil, berrors.MalformedError("Too many signatures in POST body")
	}
	if len(parsedJWS.Signatures) == 0 {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSNoSignatures"}).Inc()
		return nil, berrors.MalformedError("POST JWS not signed")
	}
	if len(parsedJWS.Signatures) == 1 && len(parsedJWS.Signatures[0].Signature) == 0 {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSEmptySignature"}).Inc()
		return nil, berrors.MalformedError("POST JWS not signed")
	}

	return &bJSONWebSignature{parsedJWS}, nil
}

// parseJWSRequest extracts a bJSONWebSignature from an HTTP POST request's body using parseJWS.
func (wfe *WebFrontEndImpl) parseJWSRequest(request *http.Request) (*bJSONWebSignature, error) {
	// Verify that the POST request has the expected headers
	if err := wfe.validPOSTRequest(request); err != nil {
		return nil, err
	}

	// Read the POST request body's bytes. validPOSTRequest has already checked
	// that the body is non-nil
	bodyBytes, err := io.ReadAll(http.MaxBytesReader(nil, request.Body, maxRequestSize))
	if err != nil {
		if err.Error() == "http: request body too large" {
			return nil, berrors.UnauthorizedError("request body too large")
		}
		wfe.stats.httpErrorCount.With(prometheus.Labels{"type": "UnableToReadReqBody"}).Inc()
		return nil, errors.New("unable to read request body")
	}

	jws, err := wfe.parseJWS(bodyBytes)
	if err != nil {
		return nil, err
	}

	return jws, nil
}

// extractJWK extracts a JWK from the protected headers of a bJSONWebSignature
// or returns a error. It expects that the JWS is using the embedded JWK style
// of authentication and does not contain an embedded Key ID. Callers should
// have acquired the headers from a bJSONWebSignature returned by parseJWS to
// ensure it has the correct number of signatures present.
func (wfe *WebFrontEndImpl) extractJWK(header jose.Header) (*jose.JSONWebKey, error) {
	// extractJWK expects the request to be using an embedded JWK auth type and
	// to not contain the mutually exclusive KeyID.
	if err := wfe.enforceJWSAuthType(header, embeddedJWK); err != nil {
		return nil, err
	}

	// We can be sure that JSONWebKey is != nil because we have already called
	// enforceJWSAuthType()
	key := header.JSONWebKey

	// If the key isn't considered valid by go-jose return a error immediately
	if !key.Valid() {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWKInvalid"}).Inc()
		return nil, berrors.MalformedError("Invalid JWK in JWS header")
	}

	return key, nil
}

// acctIDFromURL extracts the numeric int64 account ID from a ACMEv1 or ACMEv2
// account URL. If the acctURL has an invalid URL or the account ID in the
// acctURL is non-numeric a MalformedError is returned.
func (wfe *WebFrontEndImpl) acctIDFromURL(acctURL string, request *http.Request) (int64, error) {
	// For normal ACME v2 accounts we expect the account URL has a prefix composed
	// of the Host header and the acctPath.
	expectedURLPrefix := web.RelativeEndpoint(request, acctPath)

	// Process the acctURL to find only the trailing numeric account ID. Both the
	// expected URL prefix and a legacy URL prefix are permitted in order to allow
	// ACME v1 clients to use legacy accounts with unmodified account URLs for V2
	// requests.
	var accountIDStr string
	if strings.HasPrefix(acctURL, expectedURLPrefix) {
		accountIDStr = strings.TrimPrefix(acctURL, expectedURLPrefix)
	} else if strings.HasPrefix(acctURL, wfe.LegacyKeyIDPrefix) {
		accountIDStr = strings.TrimPrefix(acctURL, wfe.LegacyKeyIDPrefix)
	} else {
		return 0, berrors.MalformedError("KeyID header contained an invalid account URL: %q", acctURL)
	}

	// Convert the raw account ID string to an int64 for use with the SA's
	// GetRegistration RPC
	accountID, err := strconv.ParseInt(accountIDStr, 10, 64)
	if err != nil {
		return 0, berrors.MalformedError("Malformed account ID in KeyID header URL: %q", acctURL)
	}
	return accountID, nil
}

// lookupJWK finds a JWK associated with the Key ID present in the provided
// headers, returning the JWK and a pointer to the associated account, or a
// error. It expects that the JWS header is using the embedded Key ID style of
// authentication and does not contain an embedded JWK. Callers should have
// acquired headers from a bJSONWebSignature.
func (wfe *WebFrontEndImpl) lookupJWK(
	header jose.Header,
	ctx context.Context,
	request *http.Request,
	logEvent *web.RequestEvent) (*jose.JSONWebKey, *core.Registration, error) {
	// We expect the request to be using an embedded Key ID auth type and to not
	// contain the mutually exclusive embedded JWK.
	if err := wfe.enforceJWSAuthType(header, embeddedKeyID); err != nil {
		return nil, nil, err
	}

	accountURL := header.KeyID
	accountID, err := wfe.acctIDFromURL(accountURL, request)
	if err != nil {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSInvalidKeyID"}).Inc()
		return nil, nil, err
	}

	// Try to find the account for this account ID
	account, err := wfe.accountGetter.GetRegistration(ctx, &sapb.RegistrationID{Id: accountID})
	if err != nil {
		// If the account isn't found, return a suitable error
		if errors.Is(err, berrors.NotFound) {
			wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSKeyIDNotFound"}).Inc()
			return nil, nil, berrors.AccountDoesNotExistError("Account %q not found", accountURL)
		}

		// If there was an error and it isn't a "Not Found" error, return
		// a ServerInternal error since this is unexpected.
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSKeyIDLookupFailed"}).Inc()
		// Add an error to the log event with the internal error message
		logEvent.AddError("calling SA.GetRegistration: %s", err)
		return nil, nil, berrors.InternalServerError("Error retrieving account %q: %s", accountURL, err)
	}

	// Verify the account is not deactivated
	if core.AcmeStatus(account.Status) != core.StatusValid {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSKeyIDAccountInvalid"}).Inc()
		return nil, nil, berrors.UnauthorizedError("Account is not valid, has status %q", account.Status)
	}

	// Update the logEvent with the account information and return the JWK
	logEvent.Requester = account.Id

	acct, err := grpc.PbToRegistration(account)
	if err != nil {
		return nil, nil, fmt.Errorf("error unmarshalling account %q: %w", accountURL, err)
	}
	return acct.Key, &acct, nil
}

// validJWSForKey checks a provided JWS for a given HTTP request validates
// correctly using the provided JWK. If the JWS verifies the protected payload
// is returned. The key/JWS algorithms are verified and
// the JWK is checked against the keyPolicy before any signature validation is
// done. If the JWS signature validates correctly then the JWS nonce value
// and the JWS URL are verified to ensure that they are correct.
func (wfe *WebFrontEndImpl) validJWSForKey(
	ctx context.Context,
	jws *bJSONWebSignature,
	jwk *jose.JSONWebKey,
	request *http.Request) ([]byte, error) {
	err := checkAlgorithm(jwk, jws.Signatures[0].Header)
	if err != nil {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSAlgorithmCheckFailed"}).Inc()
		return nil, err
	}

	// Verify the JWS signature with the public key.
	// NOTE: It might seem insecure for the WFE to be trusted to verify
	// client requests, i.e., that the verification should be done at the
	// RA.  However the WFE is the RA's only view of the outside world
	// *anyway*, so it could always lie about what key was used by faking
	// the signature itself.
	payload, err := jws.Verify(jwk)
	if err != nil {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSVerifyFailed"}).Inc()
		return nil, berrors.MalformedError("JWS verification error")
	}

	// Check that the JWS contains a correct Nonce header
	if err := wfe.validNonce(ctx, jws.Signatures[0].Header); err != nil {
		return nil, err
	}

	// Check that the HTTP request URL matches the URL in the signed JWS
	if err := wfe.validPOSTURL(request, jws.Signatures[0].Header); err != nil {
		return nil, err
	}

	// In the WFE1 package the check for the request URL required unmarshalling
	// the payload JSON to check the "resource" field of the protected JWS body.
	// This caught invalid JSON early and so we preserve this check by explicitly
	// trying to unmarshal the payload (when it is non-empty to allow POST-as-GET
	// behaviour) as part of the verification and failing early if it isn't valid JSON.
	var parsedBody struct{}
	err = json.Unmarshal(payload, &parsedBody)
	if string(payload) != "" && err != nil {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSBodyUnmarshalFailed"}).Inc()
		return nil, berrors.MalformedError("Request payload did not parse as JSON")
	}

	return payload, nil
}

// validJWSForAccount checks that a given JWS is valid and verifies with the
// public key associated to a known account specified by the JWS Key ID. If the
// JWS is valid (e.g. the JWS is well formed, verifies with the JWK stored for the
// specified key ID, specifies the correct URL, and has a valid nonce) then
// `validJWSForAccount` returns the validated JWS body, the parsed
// JSONWebSignature, and a pointer to the JWK's associated account. If any of
// these conditions are not met or an error occurs only a error is returned.
func (wfe *WebFrontEndImpl) validJWSForAccount(
	jws *bJSONWebSignature,
	request *http.Request,
	ctx context.Context,
	logEvent *web.RequestEvent) ([]byte, *bJSONWebSignature, *core.Registration, error) {
	// Lookup the account and JWK for the key ID that authenticated the JWS
	pubKey, account, err := wfe.lookupJWK(jws.Signatures[0].Header, ctx, request, logEvent)
	if err != nil {
		return nil, nil, nil, err
	}

	// Verify the JWS with the JWK from the SA
	payload, err := wfe.validJWSForKey(ctx, jws, pubKey, request)
	if err != nil {
		return nil, nil, nil, err
	}

	return payload, jws, account, nil
}

// validPOSTForAccount checks that a given POST request has a valid JWS
// using `validJWSForAccount`. If valid, the authenticated JWS body and the
// registration that authenticated the body are returned. Otherwise a error is
// returned. The returned JWS body may be empty if the request is a POST-as-GET
// request.
func (wfe *WebFrontEndImpl) validPOSTForAccount(
	request *http.Request,
	ctx context.Context,
	logEvent *web.RequestEvent) ([]byte, *bJSONWebSignature, *core.Registration, error) {
	// Parse the JWS from the POST request
	jws, err := wfe.parseJWSRequest(request)
	if err != nil {
		return nil, nil, nil, err
	}
	return wfe.validJWSForAccount(jws, request, ctx, logEvent)
}

// validPOSTAsGETForAccount checks that a given POST request is valid using
// `validPOSTForAccount`. It additionally validates that the JWS request payload
// is empty, indicating that it is a POST-as-GET request per ACME draft 15+
// section 6.3 "GET and POST-as-GET requests". If a non empty payload is
// provided in the JWS the invalidPOSTAsGETErr error is returned. This
// function is useful only for endpoints that do not need to handle both POSTs
// with a body and POST-as-GET requests (e.g. Order, Certificate).
func (wfe *WebFrontEndImpl) validPOSTAsGETForAccount(
	request *http.Request,
	ctx context.Context,
	logEvent *web.RequestEvent) (*core.Registration, error) {
	// Call validPOSTForAccount to verify the JWS and extract the body.
	body, _, reg, err := wfe.validPOSTForAccount(request, ctx, logEvent)
	if err != nil {
		return nil, err
	}
	// Verify the POST-as-GET payload is empty
	if string(body) != "" {
		return nil, berrors.MalformedError("POST-as-GET requests must have an empty payload")
	}
	// To make log analysis easier we choose to elevate the pseudo ACME HTTP
	// method "POST-as-GET" to the logEvent's Method, replacing the
	// http.MethodPost value.
	logEvent.Method = "POST-as-GET"
	return reg, err
}

// validSelfAuthenticatedJWS checks that a given JWS verifies with the JWK
// embedded in the JWS itself (e.g. self-authenticated). This type of JWS
// is only used for creating new accounts or revoking a certificate by signing
// the request with the private key corresponding to the certificate's public
// key and embedding that public key in the JWS. All other request should be
// validated using `validJWSforAccount`.
// If the JWS validates (e.g. the JWS is well formed, verifies with the JWK
// embedded in it, has the correct URL, and includes a valid nonce) then
// `validSelfAuthenticatedJWS` returns the validated JWS body and the JWK that
// was embedded in the JWS. Otherwise if the valid JWS conditions are not met or
// an error occurs only a error is returned.
// Note that this function does *not* enforce that the JWK abides by our goodkey
// policies. This is because this method is used by the RevokeCertificate path,
// which must allow JWKs which are signed by blocklisted (i.e. already revoked
// due to compromise) keys, in case multiple clients attempt to revoke the same
// cert.
func (wfe *WebFrontEndImpl) validSelfAuthenticatedJWS(
	ctx context.Context,
	jws *bJSONWebSignature,
	request *http.Request) ([]byte, *jose.JSONWebKey, error) {
	// Extract the embedded JWK from the parsed protected JWS' headers
	pubKey, err := wfe.extractJWK(jws.Signatures[0].Header)
	if err != nil {
		return nil, nil, err
	}

	// Verify the JWS with the embedded JWK
	payload, err := wfe.validJWSForKey(ctx, jws, pubKey, request)
	if err != nil {
		return nil, nil, err
	}

	return payload, pubKey, nil
}

// validSelfAuthenticatedPOST checks that a given POST request has a valid JWS
// using `validSelfAuthenticatedJWS`. It enforces that the JWK abides by our
// goodkey policies (key algorithm, length, blocklist, etc).
func (wfe *WebFrontEndImpl) validSelfAuthenticatedPOST(
	ctx context.Context,
	request *http.Request) ([]byte, *jose.JSONWebKey, error) {
	// Parse the JWS from the POST request
	jws, err := wfe.parseJWSRequest(request)
	if err != nil {
		return nil, nil, err
	}

	// Extract and validate the embedded JWK from the parsed JWS
	payload, pubKey, err := wfe.validSelfAuthenticatedJWS(ctx, jws, request)
	if err != nil {
		return nil, nil, err
	}

	// If the key doesn't meet the GoodKey policy return a error
	err = wfe.keyPolicy.GoodKey(ctx, pubKey.Key)
	if err != nil {
		if errors.Is(err, goodkey.ErrBadKey) {
			wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWKRejectedByGoodKey"}).Inc()
			return nil, nil, berrors.BadPublicKeyError("invalid request signing key: %s", err.Error())
		}
		return nil, nil, berrors.InternalServerError("internal error while checking JWK: %s", err)
	}

	return payload, pubKey, nil
}

// rolloverRequest is a client request to change the key for the account ID
// provided from the specified old key to a new key (the embedded JWK in the
// inner JWS).
type rolloverRequest struct {
	OldKey  jose.JSONWebKey
	Account string
}

// rolloverOperation is a struct representing a requested rollover operation
// from the specified old key to the new key for the given account ID.
type rolloverOperation struct {
	rolloverRequest
	NewKey jose.JSONWebKey
}

// validKeyRollover checks if the innerJWS is a valid key rollover operation
// given the outer JWS that carried it. It is assumed that the outerJWS has
// already been validated per the normal ACME process using `validPOSTForAccount`.
// It is *critical* this is the case since `validKeyRollover` does not check the
// outerJWS signature. This function checks that:
// 1) the inner JWS is valid and well formed
// 2) the inner JWS has the same "url" header as the outer JWS
// 3) the inner JWS is self-authenticated with an embedded JWK
//
// This function verifies that the inner JWS' body is a rolloverRequest instance
// that specifies the correct oldKey. The returned rolloverOperation's NewKey
// field will be set to the JWK from the inner JWS.
//
// If the request is valid a *rolloverOperation object is returned,
// otherwise a error is returned. The caller is left to verify
// whether the new key is appropriate (e.g. isn't being used by another existing
// account) and that the account field of the rollover object matches the
// account that verified the outer JWS.
func (wfe *WebFrontEndImpl) validKeyRollover(
	ctx context.Context,
	outerJWS *bJSONWebSignature,
	innerJWS *bJSONWebSignature,
	oldKey *jose.JSONWebKey) (*rolloverOperation, error) {

	// Extract the embedded JWK from the inner JWS' protected headers
	innerJWK, err := wfe.extractJWK(innerJWS.Signatures[0].Header)
	if err != nil {
		return nil, err
	}

	// If the key doesn't meet the GoodKey policy return a error immediately
	err = wfe.keyPolicy.GoodKey(ctx, innerJWK.Key)
	if err != nil {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "KeyRolloverJWKRejectedByGoodKey"}).Inc()
		return nil, berrors.BadPublicKeyError("invalid request signing key: %s", err.Error())
	}

	// Check that the public key and JWS algorithms match expected
	err = checkAlgorithm(innerJWK, innerJWS.Signatures[0].Header)
	if err != nil {
		return nil, err
	}

	// Verify the inner JWS signature with the public key from the embedded JWK.
	// NOTE(@cpu): We do not use `wfe.validJWSForKey` here because the inner JWS
	// of a key rollover operation is special (e.g. has no nonce, doesn't have an
	// HTTP request to match the URL to)
	innerPayload, err := innerJWS.Verify(innerJWK)
	if err != nil {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "KeyRolloverJWSVerifyFailed"}).Inc()
		return nil, berrors.MalformedError("Inner JWS does not verify with embedded JWK")
	}
	// NOTE(@cpu): we do not stomp the web.RequestEvent's payload here since that is set
	// from the outerJWS in validPOSTForAccount and contains the inner JWS and inner
	// payload already.

	// Verify that the outer and inner JWS protected URL headers match
	if err := wfe.matchJWSURLs(outerJWS.Signatures[0].Header, innerJWS.Signatures[0].Header); err != nil {
		return nil, err
	}

	var req rolloverRequest
	if json.Unmarshal(innerPayload, &req) != nil {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "KeyRolloverUnmarshalFailed"}).Inc()
		return nil, berrors.MalformedError("Inner JWS payload did not parse as JSON key rollover object")
	}

	// If there's no oldkey specified fail before trying to use
	// core.PublicKeyEqual on a nil argument.
	if req.OldKey.Key == nil {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "KeyRolloverWrongOldKey"}).Inc()
		return nil, berrors.MalformedError("Inner JWS does not contain old key field matching current account key")
	}

	// We must validate that the inner JWS' rollover request specifies the correct
	// oldKey.
	keysEqual, err := core.PublicKeysEqual(req.OldKey.Key, oldKey.Key)
	if err != nil {
		return nil, berrors.MalformedError("Unable to compare new and old keys: %s", err.Error())
	}
	if !keysEqual {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "KeyRolloverWrongOldKey"}).Inc()
		return nil, berrors.MalformedError("Inner JWS does not contain old key field matching current account key")
	}

	// Return a rolloverOperation populated with the validated old JWK, the
	// requested account, and the new JWK extracted from the inner JWS.
	return &rolloverOperation{
		rolloverRequest: rolloverRequest{
			OldKey:  *oldKey,
			Account: req.Account,
		},
		NewKey: *innerJWK,
	}, nil
}
