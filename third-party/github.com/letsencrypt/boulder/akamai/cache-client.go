package akamai

import (
	"bytes"
	"crypto/hmac"
	"crypto/md5" //nolint: gosec // MD5 is required by the Akamai API.
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/crypto/ocsp"

	"github.com/letsencrypt/boulder/core"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/metrics"
)

const (
	timestampFormat = "20060102T15:04:05-0700"
	v3PurgePath     = "/ccu/v3/delete/url/"
	v3PurgeTagPath  = "/ccu/v3/delete/tag/"
)

var (
	// ErrAllRetriesFailed indicates that all purge submission attempts have
	// failed.
	ErrAllRetriesFailed = errors.New("all attempts to submit purge request failed")

	// errFatal is returned by the purge method of CachePurgeClient to indicate
	// that it failed for a reason that cannot be remediated by retrying the
	// request.
	errFatal = errors.New("fatal error")
)

type v3PurgeRequest struct {
	Objects []string `json:"objects"`
}

type purgeResponse struct {
	HTTPStatus       int    `json:"httpStatus"`
	Detail           string `json:"detail"`
	EstimatedSeconds int    `json:"estimatedSeconds"`
	PurgeID          string `json:"purgeId"`
}

// CachePurgeClient talks to the Akamai CCU REST API. It is safe to make
// concurrent requests using this client.
type CachePurgeClient struct {
	client       *http.Client
	apiEndpoint  string
	apiHost      string
	apiScheme    string
	clientToken  string
	clientSecret string
	accessToken  string
	v3Network    string
	retries      int
	retryBackoff time.Duration
	log          blog.Logger
	purgeLatency prometheus.Histogram
	purges       *prometheus.CounterVec
	clk          clock.Clock
}

// NewCachePurgeClient performs some basic validation of supplied configuration
// and returns a newly constructed CachePurgeClient.
func NewCachePurgeClient(
	baseURL,
	clientToken,
	secret,
	accessToken,
	network string,
	retries int,
	retryBackoff time.Duration,
	log blog.Logger, scope prometheus.Registerer,
) (*CachePurgeClient, error) {
	if network != "production" && network != "staging" {
		return nil, fmt.Errorf("'V3Network' must be \"staging\" or \"production\", got %q", network)
	}

	endpoint, err := url.Parse(strings.TrimSuffix(baseURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("failed to parse 'BaseURL' as a URL: %s", err)
	}

	purgeLatency := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "ccu_purge_latency",
		Help:    "Histogram of latencies of CCU purges",
		Buckets: metrics.InternetFacingBuckets,
	})
	scope.MustRegister(purgeLatency)

	purges := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ccu_purges",
		Help: "A counter of CCU purges labelled by the result",
	}, []string{"type"})
	scope.MustRegister(purges)

	return &CachePurgeClient{
		client:       new(http.Client),
		apiEndpoint:  endpoint.String(),
		apiHost:      endpoint.Host,
		apiScheme:    strings.ToLower(endpoint.Scheme),
		clientToken:  clientToken,
		clientSecret: secret,
		accessToken:  accessToken,
		v3Network:    network,
		retries:      retries,
		retryBackoff: retryBackoff,
		log:          log,
		clk:          clock.New(),
		purgeLatency: purgeLatency,
		purges:       purges,
	}, nil
}

// makeAuthHeader constructs a special Akamai authorization header. This header
// is used to identify clients to Akamai's EdgeGrid APIs. For a more detailed
// description of the generation process see their docs:
// https://developer.akamai.com/introduction/Client_Auth.html
func (cpc *CachePurgeClient) makeAuthHeader(body []byte, apiPath string, nonce string) string {
	// The akamai API is very time sensitive (recommending reliance on a stratum 2
	// or better time source). Additionally, timestamps MUST be in UTC.
	timestamp := cpc.clk.Now().UTC().Format(timestampFormat)
	header := fmt.Sprintf(
		"EG1-HMAC-SHA256 client_token=%s;access_token=%s;timestamp=%s;nonce=%s;",
		cpc.clientToken,
		cpc.accessToken,
		timestamp,
		nonce,
	)
	bodyHash := sha256.Sum256(body)
	tbs := fmt.Sprintf(
		"%s\t%s\t%s\t%s\t%s\t%s\t%s",
		"POST",
		cpc.apiScheme,
		cpc.apiHost,
		apiPath,
		// Signed headers are not required for this request type.
		"",
		base64.StdEncoding.EncodeToString(bodyHash[:]),
		header,
	)
	cpc.log.Debugf("To-be-signed Akamai EdgeGrid authentication %q", tbs)

	h := hmac.New(sha256.New, signingKey(cpc.clientSecret, timestamp))
	h.Write([]byte(tbs))
	return fmt.Sprintf(
		"%ssignature=%s",
		header,
		base64.StdEncoding.EncodeToString(h.Sum(nil)),
	)
}

// signingKey makes a signing key by HMAC'ing the timestamp
// using a client secret as the key.
func signingKey(clientSecret string, timestamp string) []byte {
	h := hmac.New(sha256.New, []byte(clientSecret))
	h.Write([]byte(timestamp))
	key := make([]byte, base64.StdEncoding.EncodedLen(32))
	base64.StdEncoding.Encode(key, h.Sum(nil))
	return key
}

// PurgeTags constructs and dispatches a request to purge a batch of Tags.
func (cpc *CachePurgeClient) PurgeTags(tags []string) error {
	purgeReq := v3PurgeRequest{
		Objects: tags,
	}
	endpoint := fmt.Sprintf("%s%s%s", cpc.apiEndpoint, v3PurgeTagPath, cpc.v3Network)
	return cpc.authedRequest(endpoint, purgeReq)
}

// purgeURLs constructs and dispatches a request to purge a batch of URLs.
func (cpc *CachePurgeClient) purgeURLs(urls []string) error {
	purgeReq := v3PurgeRequest{
		Objects: urls,
	}
	endpoint := fmt.Sprintf("%s%s%s", cpc.apiEndpoint, v3PurgePath, cpc.v3Network)
	return cpc.authedRequest(endpoint, purgeReq)
}

// authedRequest POSTs the JSON marshaled purge request to the provided endpoint
// along with an Akamai authorization header.
func (cpc *CachePurgeClient) authedRequest(endpoint string, body v3PurgeRequest) error {
	reqBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("%s: %w", err, errFatal)
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return fmt.Errorf("%s: %w", err, errFatal)
	}

	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("while parsing %q as URL: %s: %w", endpoint, err, errFatal)
	}

	authorization := cpc.makeAuthHeader(reqBody, endpointURL.Path, core.RandomString(16))
	req.Header.Set("Authorization", authorization)
	req.Header.Set("Content-Type", "application/json")
	cpc.log.Debugf("POSTing to endpoint %q (header %q) (body %q)", endpoint, authorization, reqBody)

	start := cpc.clk.Now()
	resp, err := cpc.client.Do(req)
	cpc.purgeLatency.Observe(cpc.clk.Since(start).Seconds())
	if err != nil {
		return fmt.Errorf("while POSTing to endpoint %q: %w", endpointURL, err)
	}
	defer resp.Body.Close()

	if resp.Body == nil {
		return fmt.Errorf("response body was empty from URL %q", resp.Request.URL)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Success for a request to purge a URL or Cache tag is 'HTTP 201'.
	// https://techdocs.akamai.com/purge-cache/reference/delete-url
	// https://techdocs.akamai.com/purge-cache/reference/delete-tag
	if resp.StatusCode != http.StatusCreated {
		switch resp.StatusCode {
		// https://techdocs.akamai.com/purge-cache/reference/403
		case http.StatusForbidden:
			return fmt.Errorf("client not authorized to make requests for URL %q: %w", resp.Request.URL, errFatal)

		// https://techdocs.akamai.com/purge-cache/reference/504
		case http.StatusGatewayTimeout:
			return fmt.Errorf("server timed out, got HTTP %d (body %q) for URL %q", resp.StatusCode, respBody, resp.Request.URL)

		// https://techdocs.akamai.com/purge-cache/reference/429
		case http.StatusTooManyRequests:
			return fmt.Errorf("exceeded request count rate limit, got HTTP %d (body %q) for URL %q", resp.StatusCode, respBody, resp.Request.URL)

		// https://techdocs.akamai.com/purge-cache/reference/413
		case http.StatusRequestEntityTooLarge:
			return fmt.Errorf("exceeded request size rate limit, got HTTP %d (body %q) for URL %q", resp.StatusCode, respBody, resp.Request.URL)
		default:
			return fmt.Errorf("received HTTP %d (body %q) for URL %q", resp.StatusCode, respBody, resp.Request.URL)
		}
	}

	var purgeInfo purgeResponse
	err = json.Unmarshal(respBody, &purgeInfo)
	if err != nil {
		return fmt.Errorf("while unmarshalling body %q from URL %q as JSON: %w", respBody, resp.Request.URL, err)
	}

	// Ensure the unmarshaled body concurs with the status of the response
	// received.
	if purgeInfo.HTTPStatus != http.StatusCreated {
		if purgeInfo.HTTPStatus == http.StatusForbidden {
			return fmt.Errorf("client not authorized to make requests to URL %q: %w", resp.Request.URL, errFatal)
		}
		return fmt.Errorf("unmarshaled HTTP %d (body %q) from URL %q", purgeInfo.HTTPStatus, respBody, resp.Request.URL)
	}

	cpc.log.AuditInfof("Purge request sent successfully (ID %s) (body %s). Purge expected in %ds",
		purgeInfo.PurgeID, reqBody, purgeInfo.EstimatedSeconds)
	return nil
}

// Purge dispatches the provided URLs in a request to the Akamai Fast-Purge API.
// The request will be attempted cpc.retries number of times before giving up
// and returning ErrAllRetriesFailed.
func (cpc *CachePurgeClient) Purge(urls []string) error {
	successful := false
	for i := range cpc.retries + 1 {
		cpc.clk.Sleep(core.RetryBackoff(i, cpc.retryBackoff, time.Minute, 1.3))

		err := cpc.purgeURLs(urls)
		if err != nil {
			if errors.Is(err, errFatal) {
				cpc.purges.WithLabelValues("fatal failure").Inc()
				return err
			}
			cpc.log.AuditErrf("Akamai cache purge failed, retrying: %s", err)
			cpc.purges.WithLabelValues("retryable failure").Inc()
			continue
		}
		successful = true
		break
	}

	if !successful {
		cpc.purges.WithLabelValues("fatal failure").Inc()
		return ErrAllRetriesFailed
	}

	cpc.purges.WithLabelValues("success").Inc()
	return nil
}

// CheckSignature is exported for use in tests and akamai-test-srv.
func CheckSignature(secret string, url string, r *http.Request, body []byte) error {
	bodyHash := sha256.Sum256(body)
	bodyHashB64 := base64.StdEncoding.EncodeToString(bodyHash[:])

	authorization := r.Header.Get("Authorization")
	authValues := make(map[string]string)
	for _, v := range strings.Split(authorization, ";") {
		splitValue := strings.Split(v, "=")
		authValues[splitValue[0]] = splitValue[1]
	}
	headerTimestamp := authValues["timestamp"]
	splitHeader := strings.Split(authorization, "signature=")
	shortenedHeader, signature := splitHeader[0], splitHeader[1]
	hostPort := strings.Split(url, "://")[1]
	h := hmac.New(sha256.New, signingKey(secret, headerTimestamp))
	input := []byte(fmt.Sprintf("POST\thttp\t%s\t%s\t\t%s\t%s",
		hostPort,
		r.URL.Path,
		bodyHashB64,
		shortenedHeader,
	))
	h.Write(input)
	expectedSignature := base64.StdEncoding.EncodeToString(h.Sum(nil))
	if signature != expectedSignature {
		return fmt.Errorf("expected signature %q, got %q in %q",
			signature, authorization, expectedSignature)
	}
	return nil
}

func reverseBytes(b []byte) []byte {
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return b
}

// makeOCSPCacheURLs constructs the 3 URLs associated with each cached OCSP
// response.
func makeOCSPCacheURLs(req []byte, ocspServer string) []string {
	hash := md5.Sum(req)
	encReq := base64.StdEncoding.EncodeToString(req)
	return []string{
		// POST Cache Key: the format of this entry is the URL that was POSTed
		// to with a query string with the parameter 'body-md5' and the value of
		// the first two uint32s in little endian order in hex of the MD5 hash
		// of the OCSP request body.
		//
		// There is limited public documentation of this feature. However, this
		// entry is what triggers the Akamai cache behavior that allows Akamai to
		// identify POST based OCSP for purging. For more information, see:
		// https://techdocs.akamai.com/property-mgr/reference/v2020-03-04-cachepost
		// https://techdocs.akamai.com/property-mgr/docs/cache-post-responses
		fmt.Sprintf("%s?body-md5=%x%x", ocspServer, reverseBytes(hash[0:4]), reverseBytes(hash[4:8])),

		// URL (un-encoded): RFC 2560 and RFC 5019 state OCSP GET URLs 'MUST
		// properly url-encode the base64 encoded' request but a large enough
		// portion of tools do not properly do this (~10% of GET requests we
		// receive) such that we must purge both the encoded and un-encoded
		// URLs.
		//
		// Due to Akamai proxy/cache behavior which collapses '//' -> '/' we also
		// collapse double slashes in the un-encoded URL so that we properly purge
		// what is stored in the cache.
		fmt.Sprintf("%s%s", ocspServer, strings.Replace(encReq, "//", "/", -1)),

		// URL (encoded): this entry is the url-encoded GET URL used to request
		// OCSP as specified in RFC 2560 and RFC 5019.
		fmt.Sprintf("%s%s", ocspServer, url.QueryEscape(encReq)),
	}
}

// GeneratePurgeURLs generates akamai URLs that can be POSTed to in order to
// purge akamai's cache of the corresponding OCSP responses. The URLs encode
// the contents of the OCSP request, so this method constructs a full OCSP
// request.
func GeneratePurgeURLs(cert, issuer *x509.Certificate) ([]string, error) {
	req, err := ocsp.CreateRequest(cert, issuer, nil)
	if err != nil {
		return nil, err
	}

	// Create a GET and special Akamai POST style OCSP url for each endpoint in
	// cert.OCSPServer.
	urls := []string{}
	for _, ocspServer := range cert.OCSPServer {
		if !strings.HasSuffix(ocspServer, "/") {
			ocspServer += "/"
		}
		urls = append(urls, makeOCSPCacheURLs(req, ocspServer)...)
	}
	return urls, nil
}
