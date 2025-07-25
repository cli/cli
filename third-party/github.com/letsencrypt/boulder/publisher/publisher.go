package publisher

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	ct "github.com/google/certificate-transparency-go"
	ctClient "github.com/google/certificate-transparency-go/client"
	"github.com/google/certificate-transparency-go/jsonclient"
	cttls "github.com/google/certificate-transparency-go/tls"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/letsencrypt/boulder/core"
	"github.com/letsencrypt/boulder/issuance"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/metrics"
	pubpb "github.com/letsencrypt/boulder/publisher/proto"
)

// Log contains the CT client for a particular CT log
type Log struct {
	logID  string
	uri    string
	client *ctClient.LogClient
}

// cacheKey is a comparable type for use as a key within a logCache. It holds
// both the log URI and its log_id (base64 encoding of its pubkey), so that
// the cache won't interfere if the RA decides that a log's URI or pubkey has
// changed.
type cacheKey struct {
	uri    string
	pubkey string
}

// logCache contains a cache of *Log's that are constructed as required by
// `SubmitToSingleCT`
type logCache struct {
	sync.RWMutex
	logs map[cacheKey]*Log
}

// AddLog adds a *Log to the cache by constructing the statName, client and
// verifier for the given uri & base64 public key.
func (c *logCache) AddLog(uri, b64PK, userAgent string, logger blog.Logger) (*Log, error) {
	// Lock the mutex for reading to check the cache
	c.RLock()
	log, present := c.logs[cacheKey{uri, b64PK}]
	c.RUnlock()

	// If we have already added this log, give it back
	if present {
		return log, nil
	}

	// Lock the mutex for writing to add to the cache
	c.Lock()
	defer c.Unlock()

	// Construct a Log, add it to the cache, and return it to the caller
	log, err := NewLog(uri, b64PK, userAgent, logger)
	if err != nil {
		return nil, err
	}
	c.logs[cacheKey{uri, b64PK}] = log
	return log, nil
}

// Len returns the number of logs in the logCache
func (c *logCache) Len() int {
	c.RLock()
	defer c.RUnlock()
	return len(c.logs)
}

type logAdaptor struct {
	blog.Logger
}

func (la logAdaptor) Printf(s string, args ...interface{}) {
	la.Logger.Infof(s, args...)
}

// NewLog returns an initialized Log struct
func NewLog(uri, b64PK, userAgent string, logger blog.Logger) (*Log, error) {
	url, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	url.Path = strings.TrimSuffix(url.Path, "/")

	derPK, err := base64.StdEncoding.DecodeString(b64PK)
	if err != nil {
		return nil, err
	}

	opts := jsonclient.Options{
		Logger:       logAdaptor{logger},
		PublicKeyDER: derPK,
		UserAgent:    userAgent,
	}
	httpClient := &http.Client{
		// We set the HTTP client timeout to about half of what we expect
		// the gRPC timeout to be set to. This allows us to retry the
		// request at least twice in the case where the server we are
		// talking to is simply hanging indefinitely.
		Timeout: time.Minute*2 + time.Second*30,
		// We provide a new Transport for each Client so that different logs don't
		// share a connection pool. This shouldn't matter, but we occasionally see a
		// strange bug where submission to all logs hangs for about fifteen minutes.
		// One possibility is that there is a strange bug in the locking on
		// connection pools (possibly triggered by timed-out TCP connections). If
		// that's the case, separate connection pools should prevent cross-log impact.
		// We set some fields like TLSHandshakeTimeout to the values from
		// DefaultTransport because the zero value for these fields means
		// "unlimited," which would be bad.
		Transport: &http.Transport{
			MaxIdleConns:        http.DefaultTransport.(*http.Transport).MaxIdleConns,
			MaxIdleConnsPerHost: http.DefaultTransport.(*http.Transport).MaxIdleConns,
			IdleConnTimeout:     http.DefaultTransport.(*http.Transport).IdleConnTimeout,
			TLSHandshakeTimeout: http.DefaultTransport.(*http.Transport).TLSHandshakeTimeout,
			// In Boulder Issue 3821[0] we found that HTTP/2 support was causing hard
			// to diagnose intermittent freezes in CT submission. Disabling HTTP/2 with
			// an environment variable resolved the freezes but is not a stable fix.
			//
			// Per the Go `http` package docs we can make this change persistent by
			// changing the `http.Transport` config:
			//   "Programs that must disable HTTP/2 can do so by setting
			//   Transport.TLSNextProto (for clients) or Server.TLSNextProto (for
			//   servers) to a non-nil, empty map"
			//
			// [0]: https://github.com/letsencrypt/boulder/issues/3821
			TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{},
		},
	}
	client, err := ctClient.New(url.String(), httpClient, opts)
	if err != nil {
		return nil, fmt.Errorf("making CT client: %s", err)
	}

	return &Log{
		logID:  b64PK,
		uri:    url.String(),
		client: client,
	}, nil
}

type ctSubmissionRequest struct {
	Chain []string `json:"chain"`
}

type pubMetrics struct {
	submissionLatency *prometheus.HistogramVec
	probeLatency      *prometheus.HistogramVec
	errorCount        *prometheus.CounterVec
}

func initMetrics(stats prometheus.Registerer) *pubMetrics {
	submissionLatency := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ct_submission_time_seconds",
			Help:    "Time taken to submit a certificate to a CT log",
			Buckets: metrics.InternetFacingBuckets,
		},
		[]string{"log", "type", "status", "http_status"},
	)
	stats.MustRegister(submissionLatency)

	probeLatency := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ct_probe_time_seconds",
			Help:    "Time taken to probe a CT log",
			Buckets: metrics.InternetFacingBuckets,
		},
		[]string{"log", "status"},
	)
	stats.MustRegister(probeLatency)

	errorCount := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ct_errors_count",
			Help: "Count of errors by type",
		},
		[]string{"log", "type"},
	)
	stats.MustRegister(errorCount)

	return &pubMetrics{submissionLatency, probeLatency, errorCount}
}

// Impl defines a Publisher
type Impl struct {
	pubpb.UnsafePublisherServer
	log           blog.Logger
	userAgent     string
	issuerBundles map[issuance.NameID][]ct.ASN1Cert
	ctLogsCache   logCache
	metrics       *pubMetrics
}

var _ pubpb.PublisherServer = (*Impl)(nil)

// New creates a Publisher that will submit certificates
// to requested CT logs
func New(
	bundles map[issuance.NameID][]ct.ASN1Cert,
	userAgent string,
	logger blog.Logger,
	stats prometheus.Registerer,
) *Impl {
	return &Impl{
		issuerBundles: bundles,
		userAgent:     userAgent,
		ctLogsCache: logCache{
			logs: make(map[cacheKey]*Log),
		},
		log:     logger,
		metrics: initMetrics(stats),
	}
}

// SubmitToSingleCTWithResult will submit the certificate represented by certDER
// to the CT log specified by log URL and public key (base64) and return the SCT
// to the caller.
func (pub *Impl) SubmitToSingleCTWithResult(ctx context.Context, req *pubpb.Request) (*pubpb.Result, error) {
	if core.IsAnyNilOrZero(req.Der, req.LogURL, req.LogPublicKey, req.Kind) {
		return nil, errors.New("incomplete gRPC request message")
	}

	cert, err := x509.ParseCertificate(req.Der)
	if err != nil {
		pub.log.AuditErrf("Failed to parse certificate: %s", err)
		return nil, err
	}

	chain := []ct.ASN1Cert{{Data: req.Der}}
	id := issuance.IssuerNameID(cert)
	issuerBundle, ok := pub.issuerBundles[id]
	if !ok {
		err := fmt.Errorf("No issuerBundle matching issuerNameID: %d", int64(id))
		pub.log.AuditErrf("Failed to submit certificate to CT log: %s", err)
		return nil, err
	}
	chain = append(chain, issuerBundle...)

	// Add a log URL/pubkey to the cache, if already present the
	// existing *Log will be returned, otherwise one will be constructed, added
	// and returned.
	ctLog, err := pub.ctLogsCache.AddLog(req.LogURL, req.LogPublicKey, pub.userAgent, pub.log)
	if err != nil {
		pub.log.AuditErrf("Making Log: %s", err)
		return nil, err
	}

	sct, err := pub.singleLogSubmit(ctx, chain, req.Kind, ctLog)
	if err != nil {
		if core.IsCanceled(err) {
			return nil, err
		}
		var body string
		var rspErr jsonclient.RspError
		if errors.As(err, &rspErr) && rspErr.StatusCode < 500 {
			body = string(rspErr.Body)
		}
		pub.log.AuditErrf("Failed to submit certificate to CT log at %s: %s Body=%q",
			ctLog.uri, err, body)
		return nil, err
	}

	sctBytes, err := cttls.Marshal(*sct)
	if err != nil {
		return nil, err
	}
	return &pubpb.Result{Sct: sctBytes}, nil
}

func (pub *Impl) singleLogSubmit(
	ctx context.Context,
	chain []ct.ASN1Cert,
	kind pubpb.SubmissionType,
	ctLog *Log,
) (*ct.SignedCertificateTimestamp, error) {
	submissionMethod := ctLog.client.AddChain
	if kind == pubpb.SubmissionType_sct || kind == pubpb.SubmissionType_info {
		submissionMethod = ctLog.client.AddPreChain
	}

	start := time.Now()
	sct, err := submissionMethod(ctx, chain)
	took := time.Since(start).Seconds()
	if err != nil {
		status := "error"
		if core.IsCanceled(err) {
			status = "canceled"
		}
		httpStatus := ""
		var rspError ctClient.RspError
		if errors.As(err, &rspError) && rspError.StatusCode != 0 {
			httpStatus = fmt.Sprintf("%d", rspError.StatusCode)
		}
		pub.metrics.submissionLatency.With(prometheus.Labels{
			"log":         ctLog.uri,
			"type":        kind.String(),
			"status":      status,
			"http_status": httpStatus,
		}).Observe(took)
		pub.metrics.errorCount.With(prometheus.Labels{
			"log":  ctLog.uri,
			"type": kind.String(),
		}).Inc()
		return nil, err
	}
	pub.metrics.submissionLatency.With(prometheus.Labels{
		"log":         ctLog.uri,
		"type":        kind.String(),
		"status":      "success",
		"http_status": "",
	}).Observe(took)

	threshold := uint64(time.Now().Add(time.Minute).UnixMilli()) //nolint: gosec // Current-ish timestamp is guaranteed to fit in a uint64
	if sct.Timestamp > threshold {
		return nil, fmt.Errorf("SCT Timestamp was too far in the future (%d > %d)", sct.Timestamp, threshold)
	}

	// For regular certificates, we could get an old SCT, but that shouldn't
	// happen for precertificates.
	threshold = uint64(time.Now().Add(-10 * time.Minute).UnixMilli()) //nolint: gosec // Current-ish timestamp is guaranteed to fit in a uint64
	if kind != pubpb.SubmissionType_final && sct.Timestamp < threshold {
		return nil, fmt.Errorf("SCT Timestamp was too far in the past (%d < %d)", sct.Timestamp, threshold)
	}

	return sct, nil
}

// CreateTestingSignedSCT is used by both the publisher tests and ct-test-serv, which is
// why it is exported. It creates a signed SCT based on the provided chain.
func CreateTestingSignedSCT(req []string, k *ecdsa.PrivateKey, precert bool, timestamp time.Time) []byte {
	chain := make([]ct.ASN1Cert, len(req))
	for i, str := range req {
		b, err := base64.StdEncoding.DecodeString(str)
		if err != nil {
			panic("cannot decode chain")
		}
		chain[i] = ct.ASN1Cert{Data: b}
	}

	// Generate the internal leaf entry for the SCT
	etype := ct.X509LogEntryType
	if precert {
		etype = ct.PrecertLogEntryType
	}
	leaf, err := ct.MerkleTreeLeafFromRawChain(chain, etype, 0)
	if err != nil {
		panic(fmt.Sprintf("failed to create leaf: %s", err))
	}

	// Sign the SCT
	rawKey, _ := x509.MarshalPKIXPublicKey(&k.PublicKey)
	logID := sha256.Sum256(rawKey)
	timestampMillis := uint64(timestamp.UnixMilli()) //nolint: gosec // Current-ish timestamp is guaranteed to fit in a uint64
	serialized, _ := ct.SerializeSCTSignatureInput(ct.SignedCertificateTimestamp{
		SCTVersion: ct.V1,
		LogID:      ct.LogID{KeyID: logID},
		Timestamp:  timestampMillis,
	}, ct.LogEntry{Leaf: *leaf})
	hashed := sha256.Sum256(serialized)
	var ecdsaSig struct {
		R, S *big.Int
	}
	ecdsaSig.R, ecdsaSig.S, _ = ecdsa.Sign(rand.Reader, k, hashed[:])
	sig, _ := asn1.Marshal(ecdsaSig)

	// The ct.SignedCertificateTimestamp object doesn't have the needed
	// `json` tags to properly marshal so we need to transform in into
	// a struct that does before we can send it off
	var jsonSCTObj struct {
		SCTVersion ct.Version `json:"sct_version"`
		ID         string     `json:"id"`
		Timestamp  uint64     `json:"timestamp"`
		Extensions string     `json:"extensions"`
		Signature  string     `json:"signature"`
	}
	jsonSCTObj.SCTVersion = ct.V1
	jsonSCTObj.ID = base64.StdEncoding.EncodeToString(logID[:])
	jsonSCTObj.Timestamp = timestampMillis
	ds := ct.DigitallySigned{
		Algorithm: cttls.SignatureAndHashAlgorithm{
			Hash:      cttls.SHA256,
			Signature: cttls.ECDSA,
		},
		Signature: sig,
	}
	jsonSCTObj.Signature, _ = ds.Base64String()

	jsonSCT, _ := json.Marshal(jsonSCTObj)
	return jsonSCT
}

// GetCTBundleForChain takes a slice of *issuance.Certificate(s)
// representing a certificate chain and returns a slice of
// ct.ASN1Cert(s) in the same order
func GetCTBundleForChain(chain []*issuance.Certificate) []ct.ASN1Cert {
	var ctBundle []ct.ASN1Cert
	for _, cert := range chain {
		ctBundle = append(ctBundle, ct.ASN1Cert{Data: cert.Raw})
	}
	return ctBundle
}
