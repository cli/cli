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
	"github.com/letsencrypt/boulder/probs"
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
	return "", errors.New("JWK contains unsupported key type (expected RSA, or ECDSA P-256, P-384, or P-521)")
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
		return fmt.Errorf(
			"JWS signature header contains unsupported algorithm %q, expected one of %s",
			header.Algorithm, getSupportedAlgs(),
		)
	}

	expectedAlg, err := sigAlgorithmForKey(key)
	if err != nil {
		return err
	}
	if sigHeaderAlg != expectedAlg {
		return fmt.Errorf("JWS signature header algorithm %q does not match expected algorithm %q for JWK", sigHeaderAlg, string(expectedAlg))
	}
	if key.Algorithm != "" && key.Algorithm != string(expectedAlg) {
		return fmt.Errorf("JWK key header algorithm %q does not match expected algorithm %q for JWK", key.Algorithm, string(expectedAlg))
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
// problem is returned. checkJWSAuthType is separate from enforceJWSAuthType so
// that endpoints that need to handle both embedded JWK and embedded key ID
// requests can determine which type of request they have and act accordingly
// (e.g. acme v2 cert revocation).
func checkJWSAuthType(header jose.Header) (jwsAuthType, *probs.ProblemDetails) {
	// There must not be a Key ID *and* an embedded JWK
	if header.KeyID != "" && header.JSONWebKey != nil {
		return invalidAuthType, probs.Malformed(
			"jwk and kid header fields are mutually exclusive")
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
// problem is returned.
func (wfe *WebFrontEndImpl) enforceJWSAuthType(
	header jose.Header,
	expectedAuthType jwsAuthType) *probs.ProblemDetails {
	// Check the auth type for the provided JWS
	authType, prob := checkJWSAuthType(header)
	if prob != nil {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSAuthTypeInvalid"}).Inc()
		return prob
	}
	// If the auth type isn't the one expected return a sensible problem based on
	// what was expected
	if authType != expectedAuthType {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSAuthTypeWrong"}).Inc()
		switch expectedAuthType {
		case embeddedKeyID:
			return probs.Malformed("No Key ID in JWS header")
		case embeddedJWK:
			return probs.Malformed("No embedded JWK in JWS header")
		}
	}
	return nil
}

// validPOSTRequest checks a *http.Request to ensure it has the headers
// a well-formed ACME POST request has, and to ensure there is a body to
// process.
func (wfe *WebFrontEndImpl) validPOSTRequest(request *http.Request) *probs.ProblemDetails {
	// All POSTs should have an accompanying Content-Length header
	if _, present := request.Header["Content-Length"]; !present {
		wfe.stats.httpErrorCount.With(prometheus.Labels{"type": "ContentLengthRequired"}).Inc()
		return probs.ContentLengthRequired()
	}

	// Per 6.2 ALL POSTs should have the correct JWS Content-Type for flattened
	// JSON serialization.
	if _, present := request.Header["Content-Type"]; !present {
		wfe.stats.httpErrorCount.With(prometheus.Labels{"type": "NoContentType"}).Inc()
		return probs.InvalidContentType(fmt.Sprintf("No Content-Type header on POST. Content-Type must be %q",
			expectedJWSContentType))
	}
	if contentType := request.Header.Get("Content-Type"); contentType != expectedJWSContentType {
		wfe.stats.httpErrorCount.With(prometheus.Labels{"type": "WrongContentType"}).Inc()
		return probs.InvalidContentType(fmt.Sprintf("Invalid Content-Type header on POST. Content-Type must be %q",
			expectedJWSContentType))
	}

	// Per 6.4.1 "Replay-Nonce" clients should not send a Replay-Nonce header in
	// the HTTP request, it needs to be part of the signed JWS request body
	if _, present := request.Header["Replay-Nonce"]; present {
		wfe.stats.httpErrorCount.With(prometheus.Labels{"type": "ReplayNonceOutsideJWS"}).Inc()
		return probs.Malformed("HTTP requests should NOT contain Replay-Nonce header. Use JWS nonce field")
	}

	// All POSTs should have a non-nil body
	if request.Body == nil {
		wfe.stats.httpErrorCount.With(prometheus.Labels{"type": "NoPOSTBody"}).Inc()
		return probs.Malformed("No body on POST")
	}

	return nil
}

// nonceWellFormed checks a JWS' Nonce header to ensure it is well-formed,
// otherwise a bad nonce problem is returned. This avoids unnecessary RPCs to
// the nonce redemption service.
func nonceWellFormed(nonceHeader string, prefixLen int) *probs.ProblemDetails {
	errBadNonce := probs.BadNonce(fmt.Sprintf("JWS has an invalid anti-replay nonce: %q", nonceHeader))
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
// nonceService knows about, otherwise a bad nonce problem is returned.
// NOTE: this function assumes the JWS has already been verified with the
// correct public key.
func (wfe *WebFrontEndImpl) validNonce(ctx context.Context, header jose.Header) *probs.ProblemDetails {
	if len(header.Nonce) == 0 {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSMissingNonce"}).Inc()
		return probs.BadNonce("JWS has no anti-replay nonce")
	}

	prob := nonceWellFormed(header.Nonce, nonce.PrefixLen)
	if prob != nil {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSMalformedNonce"}).Inc()
		return prob
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
			return web.ProblemDetailsForError(err, "failed to redeem nonce")
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
		return probs.BadNonce(fmt.Sprintf("JWS has an invalid anti-replay nonce: %q", header.Nonce))
	}
	return nil
}

// validPOSTURL checks the JWS' URL header against the expected URL based on the
// HTTP request. This prevents a JWS intended for one endpoint being replayed
// against a different endpoint. If the URL isn't present, is invalid, or
// doesn't match the HTTP request a problem is returned.
func (wfe *WebFrontEndImpl) validPOSTURL(
	request *http.Request,
	header jose.Header) *probs.ProblemDetails {
	extraHeaders := header.ExtraHeaders
	// Check that there is at least one Extra Header
	if len(extraHeaders) == 0 {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSNoExtraHeaders"}).Inc()
		return probs.Malformed("JWS header parameter 'url' required")
	}
	// Try to read a 'url' Extra Header as a string
	headerURL, ok := extraHeaders[jose.HeaderKey("url")].(string)
	if !ok || len(headerURL) == 0 {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSMissingURL"}).Inc()
		return probs.Malformed("JWS header parameter 'url' required")
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
		return probs.Malformed(fmt.Sprintf(
			"JWS header parameter 'url' incorrect. Expected %q got %q",
			expectedURL.String(), headerURL))
	}
	return nil
}

// matchJWSURLs checks two JWS' URL headers are equal. This is used during key
// rollover to check that the inner JWS URL matches the outer JWS URL. If the
// JWS URLs do not match a problem is returned.
func (wfe *WebFrontEndImpl) matchJWSURLs(outer, inner jose.Header) *probs.ProblemDetails {
	// Verify that the outer JWS has a non-empty URL header. This is strictly
	// defensive since the expectation is that endpoints using `matchJWSURLs`
	// have received at least one of their JWS from calling validPOSTForAccount(),
	// which checks the outer JWS has the expected URL header before processing
	// the inner JWS.
	outerURL, ok := outer.ExtraHeaders[jose.HeaderKey("url")].(string)
	if !ok || len(outerURL) == 0 {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "KeyRolloverOuterJWSNoURL"}).Inc()
		return probs.Malformed("Outer JWS header parameter 'url' required")
	}

	// Verify the inner JWS has a non-empty URL header.
	innerURL, ok := inner.ExtraHeaders[jose.HeaderKey("url")].(string)
	if !ok || len(innerURL) == 0 {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "KeyRolloverInnerJWSNoURL"}).Inc()
		return probs.Malformed("Inner JWS header parameter 'url' required")
	}

	// Verify that the outer URL matches the inner URL
	if outerURL != innerURL {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "KeyRolloverMismatchedURLs"}).Inc()
		return probs.Malformed(fmt.Sprintf(
			"Outer JWS 'url' value %q does not match inner JWS 'url' value %q",
			outerURL, innerURL))
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
// presence of unprotected headers) a problem is returned, otherwise a
// *bJSONWebSignature is returned.
func (wfe *WebFrontEndImpl) parseJWS(body []byte) (*bJSONWebSignature, *probs.ProblemDetails) {
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
		return nil, probs.Malformed("Parse error reading JWS")
	}

	// ACME v2 never uses values from the unprotected JWS header. Reject JWS that
	// include unprotected headers.
	if unprotected.Header != nil {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSUnprotectedHeaders"}).Inc()
		return nil, probs.Malformed(
			"JWS \"header\" field not allowed. All headers must be in \"protected\" field")
	}

	// ACME v2 never uses the "signatures" array of JSON serialized JWS, just the
	// mandatory "signature" field. Reject JWS that include the "signatures" array.
	if len(unprotected.Signatures) > 0 {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSMultiSig"}).Inc()
		return nil, probs.Malformed(
			"JWS \"signatures\" field not allowed. Only the \"signature\" field should contain a signature")
	}

	// Parse the JWS using go-jose and enforce that the expected one non-empty
	// signature is present in the parsed JWS.
	bodyStr := string(body)
	parsedJWS, err := jose.ParseSigned(bodyStr, getSupportedAlgs())
	if err != nil {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSParseError"}).Inc()
		return nil, probs.Malformed("Parse error reading JWS")
	}
	if len(parsedJWS.Signatures) > 1 {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSTooManySignatures"}).Inc()
		return nil, probs.Malformed("Too many signatures in POST body")
	}
	if len(parsedJWS.Signatures) == 0 {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSNoSignatures"}).Inc()
		return nil, probs.Malformed("POST JWS not signed")
	}
	if len(parsedJWS.Signatures) == 1 && len(parsedJWS.Signatures[0].Signature) == 0 {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSEmptySignature"}).Inc()
		return nil, probs.Malformed("POST JWS not signed")
	}

	return &bJSONWebSignature{parsedJWS}, nil
}

// parseJWSRequest extracts a bJSONWebSignature from an HTTP POST request's body using parseJWS.
func (wfe *WebFrontEndImpl) parseJWSRequest(request *http.Request) (*bJSONWebSignature, *probs.ProblemDetails) {
	// Verify that the POST request has the expected headers
	if prob := wfe.validPOSTRequest(request); prob != nil {
		return nil, prob
	}

	// Read the POST request body's bytes. validPOSTRequest has already checked
	// that the body is non-nil
	bodyBytes, err := io.ReadAll(http.MaxBytesReader(nil, request.Body, maxRequestSize))
	if err != nil {
		if err.Error() == "http: request body too large" {
			return nil, probs.Unauthorized("request body too large")
		}
		wfe.stats.httpErrorCount.With(prometheus.Labels{"type": "UnableToReadReqBody"}).Inc()
		return nil, probs.ServerInternal("unable to read request body")
	}

	jws, prob := wfe.parseJWS(bodyBytes)
	if prob != nil {
		return nil, prob
	}

	return jws, nil
}

// extractJWK extracts a JWK from the protected headers of a bJSONWebSignature
// or returns a problem. It expects that the JWS is using the embedded JWK style
// of authentication and does not contain an embedded Key ID. Callers should
// have acquired the headers from a bJSONWebSignature returned by parseJWS to
// ensure it has the correct number of signatures present.
func (wfe *WebFrontEndImpl) extractJWK(header jose.Header) (*jose.JSONWebKey, *probs.ProblemDetails) {
	// extractJWK expects the request to be using an embedded JWK auth type and
	// to not contain the mutually exclusive KeyID.
	if prob := wfe.enforceJWSAuthType(header, embeddedJWK); prob != nil {
		return nil, prob
	}

	// We can be sure that JSONWebKey is != nil because we have already called
	// enforceJWSAuthType()
	key := header.JSONWebKey

	// If the key isn't considered valid by go-jose return a problem immediately
	if !key.Valid() {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWKInvalid"}).Inc()
		return nil, probs.Malformed("Invalid JWK in JWS header")
	}

	return key, nil
}

// acctIDFromURL extracts the numeric int64 account ID from a ACMEv1 or ACMEv2
// account URL. If the acctURL has an invalid URL or the account ID in the
// acctURL is non-numeric a MalformedProblem is returned.
func (wfe *WebFrontEndImpl) acctIDFromURL(acctURL string, request *http.Request) (int64, *probs.ProblemDetails) {
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
		return 0, probs.Malformed(
			fmt.Sprintf("KeyID header contained an invalid account URL: %q", acctURL))
	}

	// Convert the raw account ID string to an int64 for use with the SA's
	// GetRegistration RPC
	accountID, err := strconv.ParseInt(accountIDStr, 10, 64)
	if err != nil {
		return 0, probs.Malformed("Malformed account ID in KeyID header URL: %q", acctURL)
	}
	return accountID, nil
}

// lookupJWK finds a JWK associated with the Key ID present in the provided
// headers, returning the JWK and a pointer to the associated account, or a
// problem. It expects that the JWS header is using the embedded Key ID style of
// authentication and does not contain an embedded JWK. Callers should have
// acquired headers from a bJSONWebSignature.
func (wfe *WebFrontEndImpl) lookupJWK(
	header jose.Header,
	ctx context.Context,
	request *http.Request,
	logEvent *web.RequestEvent) (*jose.JSONWebKey, *core.Registration, *probs.ProblemDetails) {
	// We expect the request to be using an embedded Key ID auth type and to not
	// contain the mutually exclusive embedded JWK.
	if prob := wfe.enforceJWSAuthType(header, embeddedKeyID); prob != nil {
		return nil, nil, prob
	}

	accountURL := header.KeyID
	accountID, prob := wfe.acctIDFromURL(accountURL, request)
	if prob != nil {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSInvalidKeyID"}).Inc()
		return nil, nil, prob
	}

	// Try to find the account for this account ID
	account, err := wfe.accountGetter.GetRegistration(ctx, &sapb.RegistrationID{Id: accountID})
	if err != nil {
		// If the account isn't found, return a suitable problem
		if errors.Is(err, berrors.NotFound) {
			wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSKeyIDNotFound"}).Inc()
			return nil, nil, probs.AccountDoesNotExist(fmt.Sprintf(
				"Account %q not found", accountURL))
		}

		// If there was an error and it isn't a "Not Found" error, return
		// a ServerInternal problem since this is unexpected.
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSKeyIDLookupFailed"}).Inc()
		// Add an error to the log event with the internal error message
		logEvent.AddError("calling SA.GetRegistration: %s", err)
		return nil, nil, web.ProblemDetailsForError(err, fmt.Sprintf("Error retrieving account %q", accountURL))
	}

	// Verify the account is not deactivated
	if core.AcmeStatus(account.Status) != core.StatusValid {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSKeyIDAccountInvalid"}).Inc()
		return nil, nil, probs.Unauthorized(
			fmt.Sprintf("Account is not valid, has status %q", account.Status))
	}

	// Update the logEvent with the account information and return the JWK
	logEvent.Requester = account.Id

	acct, err := grpc.PbToRegistration(account)
	if err != nil {
		return nil, nil, probs.ServerInternal(fmt.Sprintf(
			"Error unmarshalling account %q", accountURL))
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
	request *http.Request) ([]byte, *probs.ProblemDetails) {
	err := checkAlgorithm(jwk, jws.Signatures[0].Header)
	if err != nil {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWSAlgorithmCheckFailed"}).Inc()
		return nil, probs.BadSignatureAlgorithm(err.Error())
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
		return nil, probs.Malformed("JWS verification error")
	}

	// Check that the JWS contains a correct Nonce header
	if prob := wfe.validNonce(ctx, jws.Signatures[0].Header); prob != nil {
		return nil, prob
	}

	// Check that the HTTP request URL matches the URL in the signed JWS
	if prob := wfe.validPOSTURL(request, jws.Signatures[0].Header); prob != nil {
		return nil, prob
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
		return nil, probs.Malformed("Request payload did not parse as JSON")
	}

	return payload, nil
}

// validJWSForAccount checks that a given JWS is valid and verifies with the
// public key associated to a known account specified by the JWS Key ID. If the
// JWS is valid (e.g. the JWS is well formed, verifies with the JWK stored for the
// specified key ID, specifies the correct URL, and has a valid nonce) then
// `validJWSForAccount` returns the validated JWS body, the parsed
// JSONWebSignature, and a pointer to the JWK's associated account. If any of
// these conditions are not met or an error occurs only a problem is returned.
func (wfe *WebFrontEndImpl) validJWSForAccount(
	jws *bJSONWebSignature,
	request *http.Request,
	ctx context.Context,
	logEvent *web.RequestEvent) ([]byte, *bJSONWebSignature, *core.Registration, *probs.ProblemDetails) {
	// Lookup the account and JWK for the key ID that authenticated the JWS
	pubKey, account, prob := wfe.lookupJWK(jws.Signatures[0].Header, ctx, request, logEvent)
	if prob != nil {
		return nil, nil, nil, prob
	}

	// Verify the JWS with the JWK from the SA
	payload, prob := wfe.validJWSForKey(ctx, jws, pubKey, request)
	if prob != nil {
		return nil, nil, nil, prob
	}

	return payload, jws, account, nil
}

// validPOSTForAccount checks that a given POST request has a valid JWS
// using `validJWSForAccount`. If valid, the authenticated JWS body and the
// registration that authenticated the body are returned. Otherwise a problem is
// returned. The returned JWS body may be empty if the request is a POST-as-GET
// request.
func (wfe *WebFrontEndImpl) validPOSTForAccount(
	request *http.Request,
	ctx context.Context,
	logEvent *web.RequestEvent) ([]byte, *bJSONWebSignature, *core.Registration, *probs.ProblemDetails) {
	// Parse the JWS from the POST request
	jws, prob := wfe.parseJWSRequest(request)
	if prob != nil {
		return nil, nil, nil, prob
	}
	return wfe.validJWSForAccount(jws, request, ctx, logEvent)
}

// validPOSTAsGETForAccount checks that a given POST request is valid using
// `validPOSTForAccount`. It additionally validates that the JWS request payload
// is empty, indicating that it is a POST-as-GET request per ACME draft 15+
// section 6.3 "GET and POST-as-GET requests". If a non empty payload is
// provided in the JWS the invalidPOSTAsGETErr problem is returned. This
// function is useful only for endpoints that do not need to handle both POSTs
// with a body and POST-as-GET requests (e.g. Order, Certificate).
func (wfe *WebFrontEndImpl) validPOSTAsGETForAccount(
	request *http.Request,
	ctx context.Context,
	logEvent *web.RequestEvent) (*core.Registration, *probs.ProblemDetails) {
	// Call validPOSTForAccount to verify the JWS and extract the body.
	body, _, reg, prob := wfe.validPOSTForAccount(request, ctx, logEvent)
	if prob != nil {
		return nil, prob
	}
	// Verify the POST-as-GET payload is empty
	if string(body) != "" {
		return nil, probs.Malformed("POST-as-GET requests must have an empty payload")
	}
	// To make log analysis easier we choose to elevate the pseudo ACME HTTP
	// method "POST-as-GET" to the logEvent's Method, replacing the
	// http.MethodPost value.
	logEvent.Method = "POST-as-GET"
	return reg, prob
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
// an error occurs only a problem is returned.
// Note that this function does *not* enforce that the JWK abides by our goodkey
// policies. This is because this method is used by the RevokeCertificate path,
// which must allow JWKs which are signed by blocklisted (i.e. already revoked
// due to compromise) keys, in case multiple clients attempt to revoke the same
// cert.
func (wfe *WebFrontEndImpl) validSelfAuthenticatedJWS(
	ctx context.Context,
	jws *bJSONWebSignature,
	request *http.Request) ([]byte, *jose.JSONWebKey, *probs.ProblemDetails) {
	// Extract the embedded JWK from the parsed protected JWS' headers
	pubKey, prob := wfe.extractJWK(jws.Signatures[0].Header)
	if prob != nil {
		return nil, nil, prob
	}

	// Verify the JWS with the embedded JWK
	payload, prob := wfe.validJWSForKey(ctx, jws, pubKey, request)
	if prob != nil {
		return nil, nil, prob
	}

	return payload, pubKey, nil
}

// validSelfAuthenticatedPOST checks that a given POST request has a valid JWS
// using `validSelfAuthenticatedJWS`. It enforces that the JWK abides by our
// goodkey policies (key algorithm, length, blocklist, etc).
func (wfe *WebFrontEndImpl) validSelfAuthenticatedPOST(
	ctx context.Context,
	request *http.Request) ([]byte, *jose.JSONWebKey, *probs.ProblemDetails) {
	// Parse the JWS from the POST request
	jws, prob := wfe.parseJWSRequest(request)
	if prob != nil {
		return nil, nil, prob
	}

	// Extract and validate the embedded JWK from the parsed JWS
	payload, pubKey, prob := wfe.validSelfAuthenticatedJWS(ctx, jws, request)
	if prob != nil {
		return nil, nil, prob
	}

	// If the key doesn't meet the GoodKey policy return a problem
	err := wfe.keyPolicy.GoodKey(ctx, pubKey.Key)
	if err != nil {
		if errors.Is(err, goodkey.ErrBadKey) {
			wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "JWKRejectedByGoodKey"}).Inc()
			return nil, nil, probs.BadPublicKey(err.Error())
		}
		return nil, nil, probs.ServerInternal("error checking key quality")
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
// otherwise a problem is returned. The caller is left to verify
// whether the new key is appropriate (e.g. isn't being used by another existing
// account) and that the account field of the rollover object matches the
// account that verified the outer JWS.
func (wfe *WebFrontEndImpl) validKeyRollover(
	ctx context.Context,
	outerJWS *bJSONWebSignature,
	innerJWS *bJSONWebSignature,
	oldKey *jose.JSONWebKey) (*rolloverOperation, *probs.ProblemDetails) {

	// Extract the embedded JWK from the inner JWS' protected headers
	innerJWK, prob := wfe.extractJWK(innerJWS.Signatures[0].Header)
	if prob != nil {
		return nil, prob
	}

	// If the key doesn't meet the GoodKey policy return a problem immediately
	err := wfe.keyPolicy.GoodKey(ctx, innerJWK.Key)
	if err != nil {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "KeyRolloverJWKRejectedByGoodKey"}).Inc()
		return nil, probs.BadPublicKey(err.Error())
	}

	// Check that the public key and JWS algorithms match expected
	err = checkAlgorithm(innerJWK, innerJWS.Signatures[0].Header)
	if err != nil {
		return nil, probs.Malformed(err.Error())
	}

	// Verify the inner JWS signature with the public key from the embedded JWK.
	// NOTE(@cpu): We do not use `wfe.validJWSForKey` here because the inner JWS
	// of a key rollover operation is special (e.g. has no nonce, doesn't have an
	// HTTP request to match the URL to)
	innerPayload, err := innerJWS.Verify(innerJWK)
	if err != nil {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "KeyRolloverJWSVerifyFailed"}).Inc()
		return nil, probs.Malformed("Inner JWS does not verify with embedded JWK")
	}
	// NOTE(@cpu): we do not stomp the web.RequestEvent's payload here since that is set
	// from the outerJWS in validPOSTForAccount and contains the inner JWS and inner
	// payload already.

	// Verify that the outer and inner JWS protected URL headers match
	if prob := wfe.matchJWSURLs(outerJWS.Signatures[0].Header, innerJWS.Signatures[0].Header); prob != nil {
		return nil, prob
	}

	var req rolloverRequest
	if json.Unmarshal(innerPayload, &req) != nil {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "KeyRolloverUnmarshalFailed"}).Inc()
		return nil, probs.Malformed(
			"Inner JWS payload did not parse as JSON key rollover object")
	}

	// If there's no oldkey specified fail before trying to use
	// core.PublicKeyEqual on a nil argument.
	if req.OldKey.Key == nil {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "KeyRolloverWrongOldKey"}).Inc()
		return nil, probs.Malformed("Inner JWS does not contain old key field matching current account key")
	}

	// We must validate that the inner JWS' rollover request specifies the correct
	// oldKey.
	if keysEqual, err := core.PublicKeysEqual(req.OldKey.Key, oldKey.Key); err != nil {
		return nil, probs.Malformed("Unable to compare new and old keys: %s", err.Error())
	} else if !keysEqual {
		wfe.stats.joseErrorCount.With(prometheus.Labels{"type": "KeyRolloverWrongOldKey"}).Inc()
		return nil, probs.Malformed("Inner JWS does not contain old key field matching current account key")
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
