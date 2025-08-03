package main

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	mrand "math/rand/v2"
	"net/http"
	"time"

	"github.com/go-jose/go-jose/v4"
	"golang.org/x/crypto/ocsp"

	"github.com/letsencrypt/boulder/core"
	"github.com/letsencrypt/boulder/identifier"
	"github.com/letsencrypt/boulder/probs"
	"github.com/letsencrypt/boulder/test/load-generator/acme"
)

var (
	// stringToOperation maps a configured plan action to a function that can
	// operate on a state/context.
	stringToOperation = map[string]func(*State, *acmeCache) error{
		"newAccount":        newAccount,
		"getAccount":        getAccount,
		"newOrder":          newOrder,
		"fulfillOrder":      fulfillOrder,
		"finalizeOrder":     finalizeOrder,
		"revokeCertificate": revokeCertificate,
	}
)

// OrderJSON is used because it's awkward to work with core.Order or corepb.Order
// when the API returns a different object than either of these types can represent without
// converting field values. The WFE uses an unexported `orderJSON` type for the
// API results that contain an order. We duplicate it here instead of moving it
// somewhere exported for this one utility.
type OrderJSON struct {
	// The URL field isn't returned by the API, we populate it manually with the
	// `Location` header.
	URL            string
	Status         core.AcmeStatus            `json:"status"`
	Expires        time.Time                  `json:"expires"`
	Identifiers    identifier.ACMEIdentifiers `json:"identifiers"`
	Authorizations []string                   `json:"authorizations"`
	Finalize       string                     `json:"finalize"`
	Certificate    string                     `json:"certificate,omitempty"`
	Error          *probs.ProblemDetails      `json:"error,omitempty"`
}

// getAccount takes a randomly selected v2 account from `state.accts` and puts it
// into `c.acct`. The context `nonceSource` is also populated as convenience.
func getAccount(s *State, c *acmeCache) error {
	s.rMu.RLock()
	defer s.rMu.RUnlock()

	// There must be an existing v2 account in the state
	if len(s.accts) == 0 {
		return errors.New("no accounts to return")
	}

	// Select a random account from the state and put it into the context
	c.acct = s.accts[mrand.IntN(len(s.accts))]
	c.ns = &nonceSource{s: s}
	return nil
}

// newAccount puts a V2 account into the provided context. If the state provided
// has too many accounts already (based on `state.NumAccts` and `state.maxRegs`)
// then `newAccount` puts an existing account from the state into the context,
// otherwise it creates a new account and puts it into both the state and the
// context.
func newAccount(s *State, c *acmeCache) error {
	// Check the max regs and if exceeded, just return an existing account instead
	// of creating a new one.
	if s.maxRegs != 0 && s.numAccts() >= s.maxRegs {
		return getAccount(s, c)
	}

	// Create a random signing key
	signKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	c.acct = &account{
		key: signKey,
	}
	c.ns = &nonceSource{s: s}

	// Prepare an account registration message body
	reqBody := struct {
		ToSAgreed bool `json:"termsOfServiceAgreed"`
		Contact   []string
	}{
		ToSAgreed: true,
	}
	// Set the account contact email if configured
	if s.email != "" {
		reqBody.Contact = []string{fmt.Sprintf("mailto:%s", s.email)}
	}
	reqBodyStr, err := json.Marshal(&reqBody)
	if err != nil {
		return err
	}

	// Sign the new account registration body using a JWS with an embedded JWK
	// because we do not have a key ID from the server yet.
	newAccountURL := s.directory.EndpointURL(acme.NewAccountEndpoint)
	jws, err := c.signEmbeddedV2Request(reqBodyStr, newAccountURL)
	if err != nil {
		return err
	}
	bodyBuf := []byte(jws.FullSerialize())

	resp, err := s.post(
		newAccountURL,
		bodyBuf,
		c.ns,
		string(acme.NewAccountEndpoint),
		http.StatusCreated)
	if err != nil {
		return fmt.Errorf("%s, post failed: %s", newAccountURL, err)
	}
	defer resp.Body.Close()

	// Populate the context account's key ID with the Location header returned by
	// the server
	locHeader := resp.Header.Get("Location")
	if locHeader == "" {
		return fmt.Errorf("%s, bad response - no Location header with account ID", newAccountURL)
	}
	c.acct.id = locHeader

	// Add the account to the state
	s.addAccount(c.acct)
	return nil
}

// randDomain generates a random(-ish) domain name as a subdomain of the
// provided base domain.
func randDomain(base string) string {
	// This approach will cause some repeat domains but not enough to make rate
	// limits annoying!
	var bytes [3]byte
	_, _ = rand.Read(bytes[:])
	return hex.EncodeToString(bytes[:]) + base
}

// newOrder creates a new pending order object for a random set of domains using
// the context's account.
func newOrder(s *State, c *acmeCache) error {
	// Pick a random number of names within the constraints of the maxNamesPerCert
	// parameter
	orderSize := 1 + mrand.IntN(s.maxNamesPerCert-1)
	// Generate that many random domain names. There may be some duplicates, we
	// don't care. The ACME server will collapse those down for us, how handy!
	dnsNames := identifier.ACMEIdentifiers{}
	for range orderSize {
		dnsNames = append(dnsNames, identifier.NewDNS(randDomain(s.domainBase)))
	}

	// create the new order request object
	initOrder := struct {
		Identifiers identifier.ACMEIdentifiers
	}{
		Identifiers: dnsNames,
	}
	initOrderStr, err := json.Marshal(&initOrder)
	if err != nil {
		return err
	}

	// Sign the new order request with the context account's key/key ID
	newOrderURL := s.directory.EndpointURL(acme.NewOrderEndpoint)
	jws, err := c.signKeyIDV2Request(initOrderStr, newOrderURL)
	if err != nil {
		return err
	}
	bodyBuf := []byte(jws.FullSerialize())

	resp, err := s.post(
		newOrderURL,
		bodyBuf,
		c.ns,
		string(acme.NewOrderEndpoint),
		http.StatusCreated)
	if err != nil {
		return fmt.Errorf("%s, post failed: %s", newOrderURL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%s, bad response: %s", newOrderURL, body)
	}

	// Unmarshal the Order object
	var orderJSON OrderJSON
	err = json.Unmarshal(body, &orderJSON)
	if err != nil {
		return err
	}

	// Populate the URL of the order from the Location header
	orderURL := resp.Header.Get("Location")
	if orderURL == "" {
		return fmt.Errorf("%s, bad response - no Location header with order ID", newOrderURL)
	}
	orderJSON.URL = orderURL

	// Store the pending order in the context
	c.pendingOrders = append(c.pendingOrders, &orderJSON)
	return nil
}

// popPendingOrder *removes* a random pendingOrder from the context, returning
// it.
func popPendingOrder(c *acmeCache) *OrderJSON {
	orderIndex := mrand.IntN(len(c.pendingOrders))
	order := c.pendingOrders[orderIndex]
	c.pendingOrders = append(c.pendingOrders[:orderIndex], c.pendingOrders[orderIndex+1:]...)
	return order
}

// getAuthorization fetches an authorization by GET-ing the provided URL. It
// records the latency and result of the GET operation in the state.
func getAuthorization(s *State, c *acmeCache, url string) (*core.Authorization, error) {
	latencyTag := "/acme/authz/{ID}"
	resp, err := postAsGet(s, c, url, latencyTag)
	// If there was an error, note the state and return
	if err != nil {
		return nil, fmt.Errorf("%s bad response: %s", url, err)
	}

	// Read the response body
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Unmarshal an authorization from the HTTP response body
	var authz core.Authorization
	err = json.Unmarshal(body, &authz)
	if err != nil {
		return nil, fmt.Errorf("%s response: %s", url, body)
	}
	// The Authorization ID is not set in the response so we populate it using the
	// URL
	authz.ID = url
	return &authz, nil
}

// completeAuthorization processes a provided authorization by solving its
// HTTP-01 challenge using the context's account and the state's challenge
// server. Aftering POSTing the authorization's HTTP-01 challenge the
// authorization will be polled waiting for a state change.
func completeAuthorization(authz *core.Authorization, s *State, c *acmeCache) error {
	// Skip if the authz isn't pending
	if authz.Status != core.StatusPending {
		return nil
	}

	// Find a challenge to solve from the pending authorization using the
	// challenge selection strategy from the load-generator state.
	chalToSolve, err := s.challStrat.PickChallenge(authz)
	if err != nil {
		return err
	}

	// Compute the key authorization from the context account's key
	jwk := &jose.JSONWebKey{Key: &c.acct.key.PublicKey}
	thumbprint, err := jwk.Thumbprint(crypto.SHA256)
	if err != nil {
		return err
	}
	authStr := fmt.Sprintf("%s.%s", chalToSolve.Token, base64.RawURLEncoding.EncodeToString(thumbprint))

	// Add the challenge response to the state's test server and defer a clean-up.
	switch chalToSolve.Type {
	case core.ChallengeTypeHTTP01:
		s.challSrv.AddHTTPOneChallenge(chalToSolve.Token, authStr)
		defer s.challSrv.DeleteHTTPOneChallenge(chalToSolve.Token)
	case core.ChallengeTypeDNS01:
		// Compute the digest of the key authorization
		h := sha256.New()
		h.Write([]byte(authStr))
		authorizedKeysDigest := base64.RawURLEncoding.EncodeToString(h.Sum(nil))
		domain := "_acme-challenge." + authz.Identifier.Value + "."
		s.challSrv.AddDNSOneChallenge(domain, authorizedKeysDigest)
		defer s.challSrv.DeleteDNSOneChallenge(domain)
	case core.ChallengeTypeTLSALPN01:
		s.challSrv.AddTLSALPNChallenge(authz.Identifier.Value, authStr)
		defer s.challSrv.DeleteTLSALPNChallenge(authz.Identifier.Value)
	default:
		return fmt.Errorf("challenge strategy picked challenge with unknown type: %q", chalToSolve.Type)
	}

	// Prepare the Challenge POST body
	jws, err := c.signKeyIDV2Request([]byte(`{}`), chalToSolve.URL)
	if err != nil {
		return err
	}
	requestPayload := []byte(jws.FullSerialize())

	resp, err := s.post(
		chalToSolve.URL,
		requestPayload,
		c.ns,
		"/acme/challenge/{ID}", // We want all challenge POST latencies to be grouped
		http.StatusOK,
	)
	if err != nil {
		return err
	}

	// Read the response body and cleanup when finished
	defer resp.Body.Close()
	_, err = io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Poll the authorization waiting for the challenge response to be recorded in
	// a change of state. The polling may sleep and retry a few times if required
	err = pollAuthorization(authz, s, c)
	if err != nil {
		return err
	}

	// The challenge is completed, the authz is valid
	return nil
}

// pollAuthorization GETs a provided authorization up to three times, sleeping
// in between attempts, waiting for the status of the returned authorization to
// be valid. If the status is invalid, or if three GETs do not produce the
// correct authorization state an error is returned. If no error is returned
// then the authorization is valid and ready.
func pollAuthorization(authz *core.Authorization, s *State, c *acmeCache) error {
	authzURL := authz.ID
	for range 3 {
		// Fetch the authz by its URL
		authz, err := getAuthorization(s, c, authzURL)
		if err != nil {
			return nil
		}
		// If the authz is invalid, abort with an error
		if authz.Status == "invalid" {
			return fmt.Errorf("Authorization %q failed challenge and is status invalid", authzURL)
		}
		// If the authz is valid, return with no error - the authz is ready to go!
		if authz.Status == "valid" {
			return nil
		}
		// Otherwise sleep and try again
		time.Sleep(3 * time.Second)
	}
	return fmt.Errorf("Timed out polling authorization %q", authzURL)
}

// fulfillOrder processes a pending order from the context, completing each
// authorization's HTTP-01 challenge using the context's account, and finally
// placing the now-ready-to-be-finalized order into the context's list of
// fulfilled orders.
func fulfillOrder(s *State, c *acmeCache) error {
	// There must be at least one pending order in the context to fulfill
	if len(c.pendingOrders) == 0 {
		return errors.New("no pending orders to fulfill")
	}

	// Get an order to fulfill from the context
	order := popPendingOrder(c)

	// Each of its authorizations need to be processed
	for _, url := range order.Authorizations {
		// Fetch the authz by its URL
		authz, err := getAuthorization(s, c, url)
		if err != nil {
			return err
		}

		// Complete the authorization by solving a challenge
		err = completeAuthorization(authz, s, c)
		if err != nil {
			return err
		}
	}

	// Once all of the authorizations have been fulfilled the order is fulfilled
	// and ready for future finalization.
	c.fulfilledOrders = append(c.fulfilledOrders, order.URL)
	return nil
}

// getOrder GETs an order by URL, returning an OrderJSON object. It tracks the
// latency of the GET operation in the provided state.
func getOrder(s *State, c *acmeCache, url string) (*OrderJSON, error) {
	latencyTag := "/acme/order/{ID}"
	// POST-as-GET the order URL
	resp, err := postAsGet(s, c, url, latencyTag)
	// If there was an error, track that result
	if err != nil {
		return nil, fmt.Errorf("%s bad response: %s", url, err)
	}
	// Read the response body
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s, bad response: %s", url, body)
	}

	// Unmarshal the Order object from the response body
	var orderJSON OrderJSON
	err = json.Unmarshal(body, &orderJSON)
	if err != nil {
		return nil, err
	}

	// Populate the order's URL based on the URL we fetched it from
	orderJSON.URL = url
	return &orderJSON, nil
}

// pollOrderForCert polls a provided order, waiting for the status to change to
// valid such that a certificate URL for the order is known. Three attempts are
// made to check the order status, sleeping 3s between each. If these attempts
// expire without the status becoming valid an error is returned.
func pollOrderForCert(order *OrderJSON, s *State, c *acmeCache) (*OrderJSON, error) {
	for range 3 {
		// Fetch the order by its URL
		order, err := getOrder(s, c, order.URL)
		if err != nil {
			return nil, err
		}
		// If the order is invalid, fail
		if order.Status == "invalid" {
			return nil, fmt.Errorf("Order %q failed and is status invalid", order.URL)
		}
		// If the order is valid, return with no error - the authz is ready to go!
		if order.Status == "valid" {
			return order, nil
		}
		// Otherwise sleep and try again
		time.Sleep(3 * time.Second)
	}
	return nil, fmt.Errorf("Timed out polling order %q", order.URL)
}

// popFulfilledOrder **removes** a fulfilled order from the context, returning
// it. Fulfilled orders have all of their authorizations satisfied.
func popFulfilledOrder(c *acmeCache) string {
	orderIndex := mrand.IntN(len(c.fulfilledOrders))
	order := c.fulfilledOrders[orderIndex]
	c.fulfilledOrders = append(c.fulfilledOrders[:orderIndex], c.fulfilledOrders[orderIndex+1:]...)
	return order
}

// finalizeOrder removes a fulfilled order from the context and POSTs a CSR to
// the order's finalization URL. The CSR's key is set from the state's
// `certKey`. The order is then polled for the status to change to valid so that
// the certificate URL can be added to the context. The context's `certs` list
// is updated with the URL for the order's certificate.
func finalizeOrder(s *State, c *acmeCache) error {
	// There must be at least one fulfilled order in the context
	if len(c.fulfilledOrders) < 1 {
		return errors.New("No fulfilled orders in the context ready to be finalized")
	}

	// Pop a fulfilled order to process, and then GET its contents
	orderID := popFulfilledOrder(c)
	order, err := getOrder(s, c, orderID)
	if err != nil {
		return err
	}

	if order.Status != core.StatusReady {
		return fmt.Errorf("order %s was status %q, expected %q",
			orderID, order.Status, core.StatusReady)
	}

	// Mark down the finalization URL for the order
	finalizeURL := order.Finalize

	// Pull the values from the order identifiers for use in the CSR
	dnsNames := make([]string, len(order.Identifiers))
	for i, ident := range order.Identifiers {
		dnsNames[i] = ident.Value
	}

	// Create a CSR using the state's certKey
	csr, err := x509.CreateCertificateRequest(
		rand.Reader,
		&x509.CertificateRequest{DNSNames: dnsNames},
		s.certKey,
	)
	if err != nil {
		return err
	}

	// Create the finalization request body with the encoded CSR
	request := fmt.Sprintf(
		`{"csr":"%s"}`,
		base64.RawURLEncoding.EncodeToString(csr),
	)

	// Sign the request body with the context's account key/keyID
	jws, err := c.signKeyIDV2Request([]byte(request), finalizeURL)
	if err != nil {
		return err
	}
	requestPayload := []byte(jws.FullSerialize())

	resp, err := s.post(
		finalizeURL,
		requestPayload,
		c.ns,
		"/acme/order/finalize", // We want all order finalizations to be grouped.
		http.StatusOK,
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Read the body to ensure there isn't an error. We don't need the actual
	// contents.
	_, err = io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Poll the order waiting for the certificate to be ready
	completedOrder, err := pollOrderForCert(order, s, c)
	if err != nil {
		return err
	}

	// The valid order should have a certificate URL
	certURL := completedOrder.Certificate
	if certURL == "" {
		return fmt.Errorf("Order %q was finalized but has no cert URL", order.URL)
	}

	// Append the certificate URL into the context's list of certificates
	c.certs = append(c.certs, certURL)
	c.finalizedOrders = append(c.finalizedOrders, order.URL)
	return nil
}

// postAsGet performs a POST-as-GET request to the provided URL authenticated by
// the context's account. A HTTP status code other than StatusOK (200)
// in response to a POST-as-GET request is considered an error. The caller is
// responsible for closing the HTTP response body.
//
// See RFC 8555 Section 6.3 for more information on POST-as-GET requests.
func postAsGet(s *State, c *acmeCache, url string, latencyTag string) (*http.Response, error) {
	// Create the POST-as-GET request JWS
	jws, err := c.signKeyIDV2Request([]byte(""), url)
	if err != nil {
		return nil, err
	}
	requestPayload := []byte(jws.FullSerialize())

	return s.post(url, requestPayload, c.ns, latencyTag, http.StatusOK)
}

func popCertificate(c *acmeCache) string {
	certIndex := mrand.IntN(len(c.certs))
	certURL := c.certs[certIndex]
	c.certs = append(c.certs[:certIndex], c.certs[certIndex+1:]...)
	return certURL
}

func getCert(s *State, c *acmeCache, url string) ([]byte, error) {
	latencyTag := "/acme/cert/{serial}"
	resp, err := postAsGet(s, c, url, latencyTag)
	if err != nil {
		return nil, fmt.Errorf("%s bad response: %s", url, err)
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// revokeCertificate removes a certificate url from the context, retrieves it,
// and sends a revocation request for the certificate to the ACME server.
// The revocation request is signed with the account key rather than the certificate
// key.
func revokeCertificate(s *State, c *acmeCache) error {
	if len(c.certs) < 1 {
		return errors.New("No certificates in the context that can be revoked")
	}

	if r := mrand.Float32(); r > s.revokeChance {
		return nil
	}

	certURL := popCertificate(c)
	certPEM, err := getCert(s, c, certURL)
	if err != nil {
		return err
	}

	pemBlock, _ := pem.Decode(certPEM)
	revokeObj := struct {
		Certificate string
		Reason      int
	}{
		Certificate: base64.URLEncoding.EncodeToString(pemBlock.Bytes),
		Reason:      ocsp.Unspecified,
	}

	revokeJSON, err := json.Marshal(revokeObj)
	if err != nil {
		return err
	}
	revokeURL := s.directory.EndpointURL(acme.RevokeCertEndpoint)
	// TODO(roland): randomly use the certificate key to sign the request instead of
	// the account key
	jws, err := c.signKeyIDV2Request(revokeJSON, revokeURL)
	if err != nil {
		return err
	}
	requestPayload := []byte(jws.FullSerialize())

	resp, err := s.post(
		revokeURL,
		requestPayload,
		c.ns,
		"/acme/revoke-cert",
		http.StatusOK,
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, err = io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	return nil
}
