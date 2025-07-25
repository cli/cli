package akamai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jmhodges/clock"

	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/test"
)

func TestMakeAuthHeader(t *testing.T) {
	log := blog.NewMock()
	stats := metrics.NoopRegisterer
	cpc, err := NewCachePurgeClient(
		"https://akaa-baseurl-xxxxxxxxxxx-xxxxxxxxxxxxx.luna.akamaiapis.net",
		"akab-client-token-xxx-xxxxxxxxxxxxxxxx",
		"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx=",
		"akab-access-token-xxx-xxxxxxxxxxxxxxxx",
		"production",
		2,
		time.Second,
		log,
		stats,
	)
	test.AssertNotError(t, err, "Failed to create cache purge client")
	fc := clock.NewFake()
	cpc.clk = fc
	wantedTimestamp, err := time.Parse(timestampFormat, "20140321T19:34:21+0000")
	test.AssertNotError(t, err, "Failed to parse timestamp")
	fc.Set(wantedTimestamp)

	expectedHeader := "EG1-HMAC-SHA256 client_token=akab-client-token-xxx-xxxxxxxxxxxxxxxx;access_token=akab-access-token-xxx-xxxxxxxxxxxxxxxx;timestamp=20140321T19:34:21+0000;nonce=nonce-xx-xxxx-xxxx-xxxx-xxxxxxxxxxxx;signature=hXm4iCxtpN22m4cbZb4lVLW5rhX8Ca82vCFqXzSTPe4="
	authHeader := cpc.makeAuthHeader(
		[]byte("datadatadatadatadatadatadatadata"),
		"/testapi/v1/t3",
		"nonce-xx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
	)
	test.AssertEquals(t, authHeader, expectedHeader)
}

type akamaiServer struct {
	responseCode int
	*httptest.Server
}

func (as *akamaiServer) sendResponse(w http.ResponseWriter, resp purgeResponse) {
	respBytes, err := json.Marshal(resp)
	if err != nil {
		fmt.Printf("Failed to marshal response body: %s\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(as.responseCode)
	w.Write(respBytes)
}

func (as *akamaiServer) purgeHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Objects []string
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		fmt.Printf("Failed to read request body: %s\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = CheckSignature("secret", as.URL, r, body)
	if err != nil {
		fmt.Printf("Error checking signature: %s\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = json.Unmarshal(body, &req)
	if err != nil {
		fmt.Printf("Failed to unmarshal request body: %s\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	resp := purgeResponse{
		HTTPStatus:       as.responseCode,
		Detail:           "?",
		EstimatedSeconds: 10,
		PurgeID:          "?",
	}

	fmt.Println(r.URL.Path, v3PurgePath)
	if strings.HasPrefix(r.URL.Path, v3PurgePath) {
		for _, testURL := range req.Objects {
			if !strings.HasPrefix(testURL, "http://") {
				resp.HTTPStatus = http.StatusForbidden
				break
			}
		}
	}
	as.sendResponse(w, resp)
}
func newAkamaiServer(code int) *akamaiServer {
	m := http.NewServeMux()
	as := akamaiServer{
		responseCode: code,
		Server:       httptest.NewServer(m),
	}
	m.HandleFunc(v3PurgePath, as.purgeHandler)
	m.HandleFunc(v3PurgeTagPath, as.purgeHandler)
	return &as
}

// TestV3Purge tests the Akamai CCU v3 purge API
func TestV3Purge(t *testing.T) {
	as := newAkamaiServer(http.StatusCreated)
	defer as.Close()

	// Client is a purge client with a "production" v3Network parameter
	client, err := NewCachePurgeClient(
		as.URL,
		"token",
		"secret",
		"accessToken",
		"production",
		3,
		time.Second,
		blog.NewMock(),
		metrics.NoopRegisterer,
	)
	test.AssertNotError(t, err, "Failed to create CachePurgeClient")
	client.clk = clock.NewFake()

	err = client.Purge([]string{"http://test.com"})
	test.AssertNotError(t, err, "Purge failed; expected 201 response")

	started := client.clk.Now()
	as.responseCode = http.StatusInternalServerError
	err = client.Purge([]string{"http://test.com"})
	test.AssertError(t, err, "Purge succeeded; expected 500 response")
	t.Log(client.clk.Since(started))
	// Given 3 retries, with a retry interval of 1 second, a growth factor of 1.3,
	// and a jitter of 0.2, the minimum amount of elapsed time is:
	// (1 * 0.8) + (1 * 1.3 * 0.8) + (1 * 1.3 * 1.3 * 0.8) = 3.192s
	test.Assert(t, client.clk.Since(started) > (time.Second*3), "Retries should've taken at least 3.192 seconds")

	started = client.clk.Now()
	as.responseCode = http.StatusCreated
	err = client.Purge([]string{"http:/test.com"})
	test.AssertError(t, err, "Purge succeeded; expected a 403 response from malformed URL")
	test.Assert(t, client.clk.Since(started) < time.Second, "Purge should've failed out immediately")
}

func TestPurgeTags(t *testing.T) {
	as := newAkamaiServer(http.StatusCreated)
	defer as.Close()

	// Client is a purge client with a "production" v3Network parameter
	client, err := NewCachePurgeClient(
		as.URL,
		"token",
		"secret",
		"accessToken",
		"production",
		3,
		time.Second,
		blog.NewMock(),
		metrics.NoopRegisterer,
	)
	test.AssertNotError(t, err, "Failed to create CachePurgeClient")
	fc := clock.NewFake()
	client.clk = fc

	err = client.PurgeTags([]string{"ff"})
	test.AssertNotError(t, err, "Purge failed; expected response 201")

	as.responseCode = http.StatusForbidden
	err = client.PurgeTags([]string{"http://test.com"})
	test.AssertError(t, err, "Purge succeeded; expected Forbidden response")
}

func TestNewCachePurgeClient(t *testing.T) {
	// Creating a new cache purge client with an invalid "network" parameter should error
	_, err := NewCachePurgeClient(
		"http://127.0.0.1:9000/",
		"token",
		"secret",
		"accessToken",
		"fake",
		3,
		time.Second,
		blog.NewMock(),
		metrics.NoopRegisterer,
	)
	test.AssertError(t, err, "NewCachePurgeClient with invalid network parameter didn't error")

	// Creating a new cache purge client with a valid "network" parameter shouldn't error
	_, err = NewCachePurgeClient(
		"http://127.0.0.1:9000/",
		"token",
		"secret",
		"accessToken",
		"staging",
		3,
		time.Second,
		blog.NewMock(),
		metrics.NoopRegisterer,
	)
	test.AssertNotError(t, err, "NewCachePurgeClient with valid network parameter errored")

	// Creating a new cache purge client with an invalid server URL parameter should error
	_, err = NewCachePurgeClient(
		"h&amp;ttp://whatever",
		"token",
		"secret",
		"accessToken",
		"staging",
		3,
		time.Second,
		blog.NewMock(),
		metrics.NoopRegisterer,
	)
	test.AssertError(t, err, "NewCachePurgeClient with invalid server url parameter didn't error")
}

func TestBigBatchPurge(t *testing.T) {
	log := blog.NewMock()

	as := newAkamaiServer(http.StatusCreated)

	client, err := NewCachePurgeClient(
		as.URL,
		"token",
		"secret",
		"accessToken",
		"production",
		3,
		time.Second,
		log,
		metrics.NoopRegisterer,
	)
	test.AssertNotError(t, err, "Failed to create CachePurgeClient")

	var urls []string
	for i := range 250 {
		urls = append(urls, fmt.Sprintf("http://test.com/%d", i))
	}

	err = client.Purge(urls)
	test.AssertNotError(t, err, "Purge failed.")
}

func TestReverseBytes(t *testing.T) {
	a := []byte{0, 1, 2, 3}
	test.AssertDeepEquals(t, reverseBytes(a), []byte{3, 2, 1, 0})
}

func TestGenerateOCSPCacheKeys(t *testing.T) {
	der := []byte{105, 239, 255}
	test.AssertDeepEquals(
		t,
		makeOCSPCacheURLs(der, "ocsp.invalid/"),
		[]string{
			"ocsp.invalid/?body-md5=d6101198a9d9f1f6",
			"ocsp.invalid/ae/",
			"ocsp.invalid/ae%2F%2F",
		},
	)
}
