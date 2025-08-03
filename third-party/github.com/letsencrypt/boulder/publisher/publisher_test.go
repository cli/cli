package publisher

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	ct "github.com/google/certificate-transparency-go"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/letsencrypt/boulder/core"
	"github.com/letsencrypt/boulder/issuance"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/metrics"
	pubpb "github.com/letsencrypt/boulder/publisher/proto"
	"github.com/letsencrypt/boulder/test"
)

var log = blog.UseMock()
var ctx = context.Background()

func getPort(srvURL string) (int, error) {
	url, err := url.Parse(srvURL)
	if err != nil {
		return 0, err
	}
	_, portString, err := net.SplitHostPort(url.Host)
	if err != nil {
		return 0, err
	}
	port, err := strconv.ParseInt(portString, 10, 64)
	if err != nil {
		return 0, err
	}
	return int(port), nil
}

type testLogSrv struct {
	*httptest.Server
	submissions int64
}

func logSrv(k *ecdsa.PrivateKey) *testLogSrv {
	testLog := &testLogSrv{}
	m := http.NewServeMux()
	m.HandleFunc("/ct/", func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		var jsonReq ctSubmissionRequest
		err := decoder.Decode(&jsonReq)
		if err != nil {
			return
		}
		precert := false
		if r.URL.Path == "/ct/v1/add-pre-chain" {
			precert = true
		}
		sct := CreateTestingSignedSCT(jsonReq.Chain, k, precert, time.Now())
		fmt.Fprint(w, string(sct))
		atomic.AddInt64(&testLog.submissions, 1)
	})

	testLog.Server = httptest.NewUnstartedServer(m)
	testLog.Server.Start()
	return testLog
}

// lyingLogSrv always signs SCTs with the timestamp it was given.
func lyingLogSrv(k *ecdsa.PrivateKey, timestamp time.Time) *testLogSrv {
	testLog := &testLogSrv{}
	m := http.NewServeMux()
	m.HandleFunc("/ct/", func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		var jsonReq ctSubmissionRequest
		err := decoder.Decode(&jsonReq)
		if err != nil {
			return
		}
		precert := false
		if r.URL.Path == "/ct/v1/add-pre-chain" {
			precert = true
		}
		sct := CreateTestingSignedSCT(jsonReq.Chain, k, precert, timestamp)
		fmt.Fprint(w, string(sct))
		atomic.AddInt64(&testLog.submissions, 1)
	})

	testLog.Server = httptest.NewUnstartedServer(m)
	testLog.Server.Start()
	return testLog
}

func errorBodyLogSrv() *httptest.Server {
	m := http.NewServeMux()
	m.HandleFunc("/ct/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("well this isn't good now is it."))
	})

	server := httptest.NewUnstartedServer(m)
	server.Start()
	return server
}

func setup(t *testing.T) (*Impl, *x509.Certificate, *ecdsa.PrivateKey) {
	// Load chain: R3 <- Root DST
	chain1, err := issuance.LoadChain([]string{
		"../test/hierarchy/int-r3-cross.cert.pem",
		"../test/hierarchy/root-dst.cert.pem",
	})
	test.AssertNotError(t, err, "failed to load chain1.")

	// Load chain: R3 <- Root X1
	chain2, err := issuance.LoadChain([]string{
		"../test/hierarchy/int-r3.cert.pem",
		"../test/hierarchy/root-x1.cert.pem",
	})
	test.AssertNotError(t, err, "failed to load chain2.")

	// Load chain: E1 <- Root X2
	chain3, err := issuance.LoadChain([]string{
		"../test/hierarchy/int-e1.cert.pem",
		"../test/hierarchy/root-x2.cert.pem",
	})
	test.AssertNotError(t, err, "failed to load chain3.")

	// Create an example issuerNameID to CT bundle mapping
	issuerBundles := map[issuance.NameID][]ct.ASN1Cert{
		chain1[0].NameID(): GetCTBundleForChain(chain1),
		chain2[0].NameID(): GetCTBundleForChain(chain2),
		chain3[0].NameID(): GetCTBundleForChain(chain3),
	}
	pub := New(
		issuerBundles,
		"test-user-agent/1.0",
		log,
		metrics.NoopRegisterer)

	// Load leaf certificate
	leaf, err := core.LoadCert("../test/hierarchy/ee-r3.cert.pem")
	test.AssertNotError(t, err, "unable to load leaf certificate.")

	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "Couldn't generate test key")

	return pub, leaf, k
}

func addLog(t *testing.T, port int, pubKey *ecdsa.PublicKey) *Log {
	uri := fmt.Sprintf("http://localhost:%d", port)
	der, err := x509.MarshalPKIXPublicKey(pubKey)
	test.AssertNotError(t, err, "Failed to marshal key")
	newLog, err := NewLog(uri, base64.StdEncoding.EncodeToString(der), "test-user-agent/1.0", log)
	test.AssertNotError(t, err, "Couldn't create log")
	test.AssertEquals(t, newLog.uri, fmt.Sprintf("http://localhost:%d", port))
	return newLog
}

func makePrecert(k *ecdsa.PrivateKey) (map[issuance.NameID][]ct.ASN1Cert, []byte, error) {
	rootTmpl := x509.Certificate{
		SerialNumber:          big.NewInt(0),
		Subject:               pkix.Name{CommonName: "root"},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	rootBytes, err := x509.CreateCertificate(rand.Reader, &rootTmpl, &rootTmpl, k.Public(), k)
	if err != nil {
		return nil, nil, err
	}
	root, err := x509.ParseCertificate(rootBytes)
	if err != nil {
		return nil, nil, err
	}
	precertTmpl := x509.Certificate{
		SerialNumber: big.NewInt(0),
		ExtraExtensions: []pkix.Extension{
			{Id: asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 11129, 2, 4, 3}, Critical: true, Value: []byte{0x05, 0x00}},
		},
	}
	precert, err := x509.CreateCertificate(rand.Reader, &precertTmpl, root, k.Public(), k)
	if err != nil {
		return nil, nil, err
	}
	precertX509, err := x509.ParseCertificate(precert)
	if err != nil {
		return nil, nil, err
	}
	precertIssuerNameID := issuance.IssuerNameID(precertX509)
	bundles := map[issuance.NameID][]ct.ASN1Cert{
		precertIssuerNameID: {
			ct.ASN1Cert{Data: rootBytes},
		},
	}
	return bundles, precert, err
}

func TestTimestampVerificationFuture(t *testing.T) {
	pub, _, k := setup(t)

	server := lyingLogSrv(k, time.Now().Add(time.Hour))
	defer server.Close()
	port, err := getPort(server.URL)
	test.AssertNotError(t, err, "Failed to get test server port")
	testLog := addLog(t, port, &k.PublicKey)

	// Precert
	issuerBundles, precert, err := makePrecert(k)
	test.AssertNotError(t, err, "Failed to create test leaf")
	pub.issuerBundles = issuerBundles

	_, err = pub.SubmitToSingleCTWithResult(ctx, &pubpb.Request{
		LogURL:       testLog.uri,
		LogPublicKey: testLog.logID,
		Der:          precert,
		Kind:         pubpb.SubmissionType_sct,
	})
	if err == nil {
		t.Fatal("Expected error for lying log server, got none")
	}
	if !strings.HasPrefix(err.Error(), "SCT Timestamp was too far in the future") {
		t.Fatalf("Got wrong error: %s", err)
	}
}

func TestTimestampVerificationPast(t *testing.T) {
	pub, _, k := setup(t)

	server := lyingLogSrv(k, time.Now().Add(-time.Hour))
	defer server.Close()
	port, err := getPort(server.URL)
	test.AssertNotError(t, err, "Failed to get test server port")
	testLog := addLog(t, port, &k.PublicKey)

	// Precert
	issuerBundles, precert, err := makePrecert(k)
	test.AssertNotError(t, err, "Failed to create test leaf")

	pub.issuerBundles = issuerBundles

	_, err = pub.SubmitToSingleCTWithResult(ctx, &pubpb.Request{
		LogURL:       testLog.uri,
		LogPublicKey: testLog.logID,
		Der:          precert,
		Kind:         pubpb.SubmissionType_sct,
	})
	if err == nil {
		t.Fatal("Expected error for lying log server, got none")
	}
	if !strings.HasPrefix(err.Error(), "SCT Timestamp was too far in the past") {
		t.Fatalf("Got wrong error: %s", err)
	}
}

func TestLogCache(t *testing.T) {
	cache := logCache{
		logs: make(map[cacheKey]*Log),
	}

	// Adding a log with an invalid base64 public key should error
	_, err := cache.AddLog("www.test.com", "1234", "test-user-agent/1.0", log)
	test.AssertError(t, err, "AddLog() with invalid base64 pk didn't error")

	// Adding a log with an invalid URI should error
	_, err = cache.AddLog(":", "", "test-user-agent/1.0", log)
	test.AssertError(t, err, "AddLog() with an invalid log URI didn't error")

	// Create one keypair & base 64 public key
	k1, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "ecdsa.GenerateKey() failed for k1")
	der1, err := x509.MarshalPKIXPublicKey(&k1.PublicKey)
	test.AssertNotError(t, err, "x509.MarshalPKIXPublicKey(der1) failed")
	k1b64 := base64.StdEncoding.EncodeToString(der1)

	// Create a second keypair & base64 public key
	k2, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	test.AssertNotError(t, err, "ecdsa.GenerateKey() failed for k2")
	der2, err := x509.MarshalPKIXPublicKey(&k2.PublicKey)
	test.AssertNotError(t, err, "x509.MarshalPKIXPublicKey(der2) failed")
	k2b64 := base64.StdEncoding.EncodeToString(der2)

	// Adding the first log should not produce an error
	l1, err := cache.AddLog("http://log.one.example.com", k1b64, "test-user-agent/1.0", log)
	test.AssertNotError(t, err, "cache.AddLog() failed for log 1")
	test.AssertEquals(t, cache.Len(), 1)
	test.AssertEquals(t, l1.uri, "http://log.one.example.com")
	test.AssertEquals(t, l1.logID, k1b64)

	// Adding it again should not produce any errors, or increase the Len()
	l1, err = cache.AddLog("http://log.one.example.com", k1b64, "test-user-agent/1.0", log)
	test.AssertNotError(t, err, "cache.AddLog() failed for second add of log 1")
	test.AssertEquals(t, cache.Len(), 1)
	test.AssertEquals(t, l1.uri, "http://log.one.example.com")
	test.AssertEquals(t, l1.logID, k1b64)

	// Adding a second log should not error and should increase the Len()
	l2, err := cache.AddLog("http://log.two.example.com", k2b64, "test-user-agent/1.0", log)
	test.AssertNotError(t, err, "cache.AddLog() failed for log 2")
	test.AssertEquals(t, cache.Len(), 2)
	test.AssertEquals(t, l2.uri, "http://log.two.example.com")
	test.AssertEquals(t, l2.logID, k2b64)
}

func TestLogErrorBody(t *testing.T) {
	pub, leaf, k := setup(t)

	srv := errorBodyLogSrv()
	defer srv.Close()
	port, err := getPort(srv.URL)
	test.AssertNotError(t, err, "Failed to get test server port")

	log.Clear()
	logURI := fmt.Sprintf("http://localhost:%d", port)
	pkDER, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
	test.AssertNotError(t, err, "Failed to marshal key")
	pkB64 := base64.StdEncoding.EncodeToString(pkDER)
	_, err = pub.SubmitToSingleCTWithResult(context.Background(), &pubpb.Request{
		LogURL:       logURI,
		LogPublicKey: pkB64,
		Der:          leaf.Raw,
		Kind:         pubpb.SubmissionType_final,
	})
	test.AssertError(t, err, "SubmitToSingleCTWithResult didn't fail")
	test.AssertEquals(t, len(log.GetAllMatching("well this isn't good now is it")), 1)
}

// TestErrorMetrics checks that the ct_errors_count and
// ct_submission_time_seconds metrics are updated with the correct labels when
// the publisher encounters errors.
func TestErrorMetrics(t *testing.T) {
	pub, leaf, k := setup(t)

	pkDER, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
	test.AssertNotError(t, err, "Failed to marshal key")
	pkB64 := base64.StdEncoding.EncodeToString(pkDER)

	// Set up a bad server that will always produce errors.
	badSrv := errorBodyLogSrv()
	defer badSrv.Close()
	port, err := getPort(badSrv.URL)
	test.AssertNotError(t, err, "Failed to get test server port")
	logURI := fmt.Sprintf("http://localhost:%d", port)

	_, err = pub.SubmitToSingleCTWithResult(context.Background(), &pubpb.Request{
		LogURL:       logURI,
		LogPublicKey: pkB64,
		Der:          leaf.Raw,
		Kind:         pubpb.SubmissionType_sct,
	})
	test.AssertError(t, err, "SubmitToSingleCTWithResult didn't fail")
	test.AssertMetricWithLabelsEquals(t, pub.metrics.submissionLatency, prometheus.Labels{
		"log":         logURI,
		"type":        "sct",
		"status":      "error",
		"http_status": "400",
	}, 1)
	test.AssertMetricWithLabelsEquals(t, pub.metrics.errorCount, prometheus.Labels{
		"log":  logURI,
		"type": "sct",
	}, 1)

	_, err = pub.SubmitToSingleCTWithResult(context.Background(), &pubpb.Request{
		LogURL:       logURI,
		LogPublicKey: pkB64,
		Der:          leaf.Raw,
		Kind:         pubpb.SubmissionType_final,
	})
	test.AssertError(t, err, "SubmitToSingleCTWithResult didn't fail")
	test.AssertMetricWithLabelsEquals(t, pub.metrics.submissionLatency, prometheus.Labels{
		"log":         logURI,
		"type":        "final",
		"status":      "error",
		"http_status": "400",
	}, 1)
	test.AssertMetricWithLabelsEquals(t, pub.metrics.errorCount, prometheus.Labels{
		"log":  logURI,
		"type": "final",
	}, 1)

	_, err = pub.SubmitToSingleCTWithResult(context.Background(), &pubpb.Request{
		LogURL:       logURI,
		LogPublicKey: pkB64,
		Der:          leaf.Raw,
		Kind:         pubpb.SubmissionType_info,
	})
	test.AssertError(t, err, "SubmitToSingleCTWithResult didn't fail")
	test.AssertMetricWithLabelsEquals(t, pub.metrics.submissionLatency, prometheus.Labels{
		"log":         logURI,
		"type":        "info",
		"status":      "error",
		"http_status": "400",
	}, 1)
	test.AssertMetricWithLabelsEquals(t, pub.metrics.errorCount, prometheus.Labels{
		"log":  logURI,
		"type": "info",
	}, 1)
}

// TestSuccessMetrics checks that the ct_errors_count and
// ct_submission_time_seconds metrics are updated with the correct labels when
// the publisher succeeds.
func TestSuccessMetrics(t *testing.T) {
	pub, leaf, k := setup(t)

	pkDER, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
	test.AssertNotError(t, err, "Failed to marshal key")
	pkB64 := base64.StdEncoding.EncodeToString(pkDER)

	// Set up a working server that will succeed.
	workingSrv := logSrv(k)
	defer workingSrv.Close()
	port, err := getPort(workingSrv.URL)
	test.AssertNotError(t, err, "Failed to get test server port")
	logURI := fmt.Sprintf("http://localhost:%d", port)

	// Only the latency metric should be updated on a success.
	_, err = pub.SubmitToSingleCTWithResult(context.Background(), &pubpb.Request{
		LogURL:       logURI,
		LogPublicKey: pkB64,
		Der:          leaf.Raw,
		Kind:         pubpb.SubmissionType_final,
	})
	test.AssertNotError(t, err, "SubmitToSingleCTWithResult failed")
	test.AssertMetricWithLabelsEquals(t, pub.metrics.submissionLatency, prometheus.Labels{
		"log":         logURI,
		"type":        "final",
		"status":      "success",
		"http_status": "",
	}, 1)
	test.AssertMetricWithLabelsEquals(t, pub.metrics.errorCount, prometheus.Labels{
		"log":  logURI,
		"type": "final",
	}, 0)
}

func Test_GetCTBundleForChain(t *testing.T) {
	chain, err := issuance.LoadChain([]string{
		"../test/hierarchy/int-r3.cert.pem",
		"../test/hierarchy/root-x1.cert.pem",
	})
	test.AssertNotError(t, err, "Failed to load chain.")
	expect := []ct.ASN1Cert{{Data: chain[0].Raw}}
	type args struct {
		chain []*issuance.Certificate
	}
	tests := []struct {
		name string
		args args
		want []ct.ASN1Cert
	}{
		{"Create a ct bundle with a single intermediate", args{chain}, expect},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bundle := GetCTBundleForChain(tt.args.chain)
			test.AssertDeepEquals(t, bundle, tt.want)
		})
	}
}
