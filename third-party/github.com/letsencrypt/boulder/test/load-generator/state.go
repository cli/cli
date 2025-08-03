package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-jose/go-jose/v4"

	"github.com/letsencrypt/boulder/test/load-generator/acme"
	"github.com/letsencrypt/challtestsrv"
)

// account is an ACME v2 account resource. It does not have a `jose.Signer`
// because we need to set the Signer options per-request with the URL being
// POSTed and must construct it on the fly from the `key`. Accounts are
// protected by a `sync.Mutex` that must be held for updates (see
// `account.Update`).
type account struct {
	key             *ecdsa.PrivateKey
	id              string
	finalizedOrders []string
	certs           []string
	mu              sync.Mutex
}

// update locks an account resource's mutex and sets the `finalizedOrders` and
// `certs` fields to the provided values.
func (acct *account) update(finalizedOrders, certs []string) {
	acct.mu.Lock()
	defer acct.mu.Unlock()

	acct.finalizedOrders = append(acct.finalizedOrders, finalizedOrders...)
	acct.certs = append(acct.certs, certs...)
}

type acmeCache struct {
	// The current V2 account (may be nil for legacy load generation)
	acct *account
	// Pending orders waiting for authorization challenge validation
	pendingOrders []*OrderJSON
	// Fulfilled orders in a valid status waiting for finalization
	fulfilledOrders []string
	// Finalized orders that have certificates
	finalizedOrders []string

	// A list of URLs for issued certificates
	certs []string
	// The nonce source for JWS signature nonce headers
	ns *nonceSource
}

// signEmbeddedV2Request signs the provided request data using the acmeCache's
// account's private key. The provided URL is set as a protected header per ACME
// v2 JWS standards. The resulting JWS contains an **embedded** JWK - this makes
// this function primarily applicable to new account requests where no key ID is
// known.
func (c *acmeCache) signEmbeddedV2Request(data []byte, url string) (*jose.JSONWebSignature, error) {
	// Create a signing key for the account's private key
	signingKey := jose.SigningKey{
		Key:       c.acct.key,
		Algorithm: jose.ES256,
	}
	// Create a signer, setting the URL protected header
	signer, err := jose.NewSigner(signingKey, &jose.SignerOptions{
		NonceSource: c.ns,
		EmbedJWK:    true,
		ExtraHeaders: map[jose.HeaderKey]interface{}{
			"url": url,
		},
	})
	if err != nil {
		return nil, err
	}

	// Sign the data with the signer
	signed, err := signer.Sign(data)
	if err != nil {
		return nil, err
	}
	return signed, nil
}

// signKeyIDV2Request signs the provided request data using the acmeCache's
// account's private key. The provided URL is set as a protected header per ACME
// v2 JWS standards. The resulting JWS contains a Key ID header that is
// populated using the acmeCache's account's ID. This is the default JWS signing
// style for ACME v2 requests and should be used everywhere but where the key ID
// is unknown (e.g. new-account requests where an account doesn't exist yet).
func (c *acmeCache) signKeyIDV2Request(data []byte, url string) (*jose.JSONWebSignature, error) {
	// Create a JWK with the account's private key and key ID
	jwk := &jose.JSONWebKey{
		Key:       c.acct.key,
		Algorithm: "ECDSA",
		KeyID:     c.acct.id,
	}

	// Create a signing key with the JWK
	signerKey := jose.SigningKey{
		Key:       jwk,
		Algorithm: jose.ES256,
	}

	// Ensure the signer's nonce source and URL header will be set
	opts := &jose.SignerOptions{
		NonceSource: c.ns,
		ExtraHeaders: map[jose.HeaderKey]interface{}{
			"url": url,
		},
	}

	// Construct the signer with the configured options
	signer, err := jose.NewSigner(signerKey, opts)
	if err != nil {
		return nil, err
	}

	// Sign the data with the signer
	signed, err := signer.Sign(data)
	if err != nil {
		return nil, err
	}
	return signed, nil
}

type RateDelta struct {
	Inc    int64
	Period time.Duration
}

type Plan struct {
	Runtime time.Duration
	Rate    int64
	Delta   *RateDelta
}

type respCode struct {
	code int
	num  int
}

// State holds *all* the stuff
type State struct {
	domainBase      string
	email           string
	maxRegs         int
	maxNamesPerCert int
	realIP          string
	certKey         *ecdsa.PrivateKey

	operations []func(*State, *acmeCache) error

	rMu sync.RWMutex

	// accts holds V2 account objects
	accts []*account

	challSrv    *challtestsrv.ChallSrv
	callLatency latencyWriter

	directory  *acme.Directory
	challStrat acme.ChallengeStrategy
	httpClient *http.Client

	revokeChance float32

	reqTotal  int64
	respCodes map[int]*respCode
	cMu       sync.Mutex

	wg *sync.WaitGroup
}

type rawAccount struct {
	FinalizedOrders []string `json:"finalizedOrders"`
	Certs           []string `json:"certs"`
	ID              string   `json:"id"`
	RawKey          []byte   `json:"rawKey"`
}

type snapshot struct {
	Accounts []rawAccount
}

func (s *State) numAccts() int {
	s.rMu.RLock()
	defer s.rMu.RUnlock()
	return len(s.accts)
}

// Snapshot will save out generated accounts
func (s *State) Snapshot(filename string) error {
	fmt.Printf("[+] Saving accounts to %s\n", filename)
	snap := snapshot{}
	for _, acct := range s.accts {
		k, err := x509.MarshalECPrivateKey(acct.key)
		if err != nil {
			return err
		}
		snap.Accounts = append(snap.Accounts, rawAccount{
			Certs:           acct.certs,
			FinalizedOrders: acct.finalizedOrders,
			ID:              acct.id,
			RawKey:          k,
		})
	}
	cont, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	return os.WriteFile(filename, cont, os.ModePerm)
}

// Restore previously generated accounts
func (s *State) Restore(filename string) error {
	fmt.Printf("[+] Loading accounts from %q\n", filename)
	// NOTE(@cpu): Using os.O_CREATE here explicitly to create the file if it does
	// not exist.
	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return err
	}

	content, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	// If the file's content is the empty string it was probably just created.
	// Avoid an unmarshaling error by assuming an empty file is an empty snapshot.
	if string(content) == "" {
		content = []byte("{}")
	}

	snap := snapshot{}
	err = json.Unmarshal(content, &snap)
	if err != nil {
		return err
	}
	for _, a := range snap.Accounts {
		key, err := x509.ParseECPrivateKey(a.RawKey)
		if err != nil {
			continue
		}
		s.accts = append(s.accts, &account{
			key:             key,
			id:              a.ID,
			finalizedOrders: a.FinalizedOrders,
			certs:           a.Certs,
		})
	}
	return nil
}

// New returns a pointer to a new State struct or an error
func New(
	directoryURL string,
	domainBase string,
	realIP string,
	maxRegs, maxNamesPerCert int,
	latencyPath string,
	userEmail string,
	operations []string,
	challStrat string,
	revokeChance float32) (*State, error) {
	certKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	directory, err := acme.NewDirectory(directoryURL)
	if err != nil {
		return nil, err
	}
	strategy, err := acme.NewChallengeStrategy(challStrat)
	if err != nil {
		return nil, err
	}
	if revokeChance > 1 {
		return nil, errors.New("revokeChance must be between 0.0 and 1.0")
	}
	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout: 5 * time.Second,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // CDN bypass can cause validation failures
			},
			MaxIdleConns:    500,
			IdleConnTimeout: 90 * time.Second,
		},
		Timeout: 10 * time.Second,
	}
	latencyFile, err := newLatencyFile(latencyPath)
	if err != nil {
		return nil, err
	}
	s := &State{
		httpClient:      httpClient,
		directory:       directory,
		challStrat:      strategy,
		certKey:         certKey,
		domainBase:      domainBase,
		callLatency:     latencyFile,
		wg:              new(sync.WaitGroup),
		realIP:          realIP,
		maxRegs:         maxRegs,
		maxNamesPerCert: maxNamesPerCert,
		email:           userEmail,
		respCodes:       make(map[int]*respCode),
		revokeChance:    revokeChance,
	}

	// convert operations strings to methods
	for _, opName := range operations {
		op, present := stringToOperation[opName]
		if !present {
			return nil, fmt.Errorf("unknown operation %q", opName)
		}
		s.operations = append(s.operations, op)
	}

	return s, nil
}

// Run runs the WFE load-generator
func (s *State) Run(
	ctx context.Context,
	httpOneAddrs []string,
	tlsALPNOneAddrs []string,
	dnsAddrs []string,
	fakeDNS string,
	p Plan) error {
	// Create a new challenge server binding the requested addrs.
	challSrv, err := challtestsrv.New(challtestsrv.Config{
		HTTPOneAddrs:    httpOneAddrs,
		TLSALPNOneAddrs: tlsALPNOneAddrs,
		DNSOneAddrs:     dnsAddrs,
		// Use a logger that has a load-generator prefix
		Log: log.New(os.Stdout, "load-generator challsrv - ", log.LstdFlags),
	})
	// Setup the challenge server to return the mock "fake DNS" IP address
	challSrv.SetDefaultDNSIPv4(fakeDNS)
	// Disable returning any AAAA records.
	challSrv.SetDefaultDNSIPv6("")

	if err != nil {
		return err
	}
	// Save the challenge server in the state
	s.challSrv = challSrv

	// Start the Challenge server in its own Go routine
	go s.challSrv.Run()

	if p.Delta != nil {
		go func() {
			for {
				time.Sleep(p.Delta.Period)
				atomic.AddInt64(&p.Rate, p.Delta.Inc)
			}
		}()
	}

	// Run sending loop
	stop := make(chan bool, 1)
	fmt.Println("[+] Beginning execution plan")
	i := int64(0)
	go func() {
		for {
			start := time.Now()
			select {
			case <-stop:
				return
			default:
				s.wg.Add(1)
				go s.sendCall()
				atomic.AddInt64(&i, 1)
			}
			sf := time.Duration(time.Second.Nanoseconds()/atomic.LoadInt64(&p.Rate)) - time.Since(start)
			time.Sleep(sf)
		}
	}()
	go func() {
		lastTotal := int64(0)
		lastReqTotal := int64(0)
		for {
			time.Sleep(time.Second)
			curTotal := atomic.LoadInt64(&i)
			curReqTotal := atomic.LoadInt64(&s.reqTotal)
			fmt.Printf(
				"%s Action rate: %d/s [expected: %d/s], Request rate: %d/s, Responses: [%s]\n",
				time.Now().Format(time.DateTime),
				curTotal-lastTotal,
				atomic.LoadInt64(&p.Rate),
				curReqTotal-lastReqTotal,
				s.respCodeString(),
			)
			lastTotal = curTotal
			lastReqTotal = curReqTotal
		}
	}()

	select {
	case <-time.After(p.Runtime):
		fmt.Println("[+] Execution plan finished")
	case <-ctx.Done():
		fmt.Println("[!] Execution plan cancelled")
	}
	stop <- true
	fmt.Println("[+] Waiting for pending flows to finish before killing challenge server")
	s.wg.Wait()
	fmt.Println("[+] Shutting down challenge server")
	s.challSrv.Shutdown()
	return nil
}

// HTTP utils

func (s *State) addRespCode(code int) {
	s.cMu.Lock()
	defer s.cMu.Unlock()
	code = code / 100
	if e, ok := s.respCodes[code]; ok {
		e.num++
	} else if !ok {
		s.respCodes[code] = &respCode{code, 1}
	}
}

// codes is a convenience type for holding copies of the state object's
// `respCodes` field of `map[int]*respCode`. Unlike the state object the
// respCodes are copied by value and not held as pointers. The codes type allows
// sorting the response codes for output.
type codes []respCode

func (c codes) Len() int {
	return len(c)
}

func (c codes) Less(i, j int) bool {
	return c[i].code < c[j].code
}

func (c codes) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

func (s *State) respCodeString() string {
	s.cMu.Lock()
	list := codes{}
	for _, v := range s.respCodes {
		list = append(list, *v)
	}
	s.cMu.Unlock()
	sort.Sort(list)
	counts := []string{}
	for _, v := range list {
		counts = append(counts, fmt.Sprintf("%dxx: %d", v.code, v.num))
	}
	return strings.Join(counts, ", ")
}

var userAgent = "boulder load-generator -- heyo ^_^"

func (s *State) post(
	url string,
	payload []byte,
	ns *nonceSource,
	latencyTag string,
	expectedCode int) (*http.Response, error) {
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Add("X-Real-IP", s.realIP)
	req.Header.Add("User-Agent", userAgent)
	req.Header.Add("Content-Type", "application/jose+json")
	atomic.AddInt64(&s.reqTotal, 1)
	started := time.Now()
	resp, err := s.httpClient.Do(req)
	finished := time.Now()
	state := "error"
	// Defer logging the latency and result
	defer func() {
		s.callLatency.Add(latencyTag, started, finished, state)
	}()
	if err != nil {
		return nil, err
	}
	go s.addRespCode(resp.StatusCode)
	if newNonce := resp.Header.Get("Replay-Nonce"); newNonce != "" {
		ns.addNonce(newNonce)
	}
	if resp.StatusCode != expectedCode {
		return nil, fmt.Errorf("POST %q returned HTTP status %d, expected %d",
			url, resp.StatusCode, expectedCode)
	}
	state = "good"
	return resp, nil
}

type nonceSource struct {
	mu        sync.Mutex
	noncePool []string
	s         *State
}

func (ns *nonceSource) getNonce() (string, error) {
	nonceURL := ns.s.directory.EndpointURL(acme.NewNonceEndpoint)
	latencyTag := string(acme.NewNonceEndpoint)
	started := time.Now()
	resp, err := ns.s.httpClient.Head(nonceURL)
	finished := time.Now()
	state := "error"
	defer func() {
		ns.s.callLatency.Add(fmt.Sprintf("HEAD %s", latencyTag),
			started, finished, state)
	}()
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if nonce := resp.Header.Get("Replay-Nonce"); nonce != "" {
		state = "good"
		return nonce, nil
	}
	return "", errors.New("'Replay-Nonce' header not supplied")
}

// Nonce satisfies the interface jose.NonceSource, should probably actually be per context but ¯\_(ツ)_/¯ for now
func (ns *nonceSource) Nonce() (string, error) {
	ns.mu.Lock()
	if len(ns.noncePool) == 0 {
		ns.mu.Unlock()
		return ns.getNonce()
	}
	defer ns.mu.Unlock()
	nonce := ns.noncePool[0]
	if len(ns.noncePool) > 1 {
		ns.noncePool = ns.noncePool[1:]
	} else {
		ns.noncePool = []string{}
	}
	return nonce, nil
}

func (ns *nonceSource) addNonce(nonce string) {
	ns.mu.Lock()
	defer ns.mu.Unlock()
	ns.noncePool = append(ns.noncePool, nonce)
}

// addAccount adds the provided account to the state's list of accts
func (s *State) addAccount(acct *account) {
	s.rMu.Lock()
	defer s.rMu.Unlock()

	s.accts = append(s.accts, acct)
}

func (s *State) sendCall() {
	defer s.wg.Done()
	c := &acmeCache{}

	for _, op := range s.operations {
		err := op(s, c)
		if err != nil {
			method := runtime.FuncForPC(reflect.ValueOf(op).Pointer()).Name()
			fmt.Printf("[FAILED] %s: %s\n", method, err)
			break
		}
	}
	// If the acmeCache's V2 account isn't nil, update it based on the cache's
	// finalizedOrders and certs.
	if c.acct != nil {
		c.acct.update(c.finalizedOrders, c.certs)
	}
}
