/*
This code was originally forked from https://github.com/cloudflare/cfssl/blob/1a911ca1b1d6e899bf97dcfa4a14b38db0d31134/ocsp/responder.go

Copyright (c) 2014 CloudFlare Inc.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions
are met:

Redistributions of source code must retain the above copyright notice,
this list of conditions and the following disclaimer.

Redistributions in binary form must reproduce the above copyright notice,
this list of conditions and the following disclaimer in the documentation
and/or other materials provided with the distribution.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED
TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR
PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF
LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING
NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS
SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
*/

// Package responder implements an OCSP HTTP responder based on a generic
// storage backend.
package responder

import (
	"context"
	"crypto"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"time"

	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/crypto/ocsp"

	"github.com/letsencrypt/boulder/core"
	blog "github.com/letsencrypt/boulder/log"
)

// ErrNotFound indicates the request OCSP response was not found. It is used to
// indicate that the responder should reply with unauthorizedErrorResponse.
var ErrNotFound = errors.New("request OCSP Response not found")

// errOCSPResponseExpired indicates that the nextUpdate field of the requested
// OCSP response occurred in the past and an HTTP status code of 533 should be
// returned to the caller.
var errOCSPResponseExpired = errors.New("OCSP response is expired")

var responseTypeToString = map[ocsp.ResponseStatus]string{
	ocsp.Success:           "Success",
	ocsp.Malformed:         "Malformed",
	ocsp.InternalError:     "InternalError",
	ocsp.TryLater:          "TryLater",
	ocsp.SignatureRequired: "SignatureRequired",
	ocsp.Unauthorized:      "Unauthorized",
}

// A Responder object provides an HTTP wrapper around a Source.
type Responder struct {
	Source        Source
	timeout       time.Duration
	responseTypes *prometheus.CounterVec
	responseAges  prometheus.Histogram
	requestSizes  prometheus.Histogram
	sampleRate    int
	clk           clock.Clock
	log           blog.Logger
}

// NewResponder instantiates a Responder with the give Source.
func NewResponder(source Source, timeout time.Duration, stats prometheus.Registerer, logger blog.Logger, sampleRate int) *Responder {
	requestSizes := prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "ocsp_request_sizes",
			Help:    "Size of OCSP requests",
			Buckets: []float64{1, 100, 200, 400, 800, 1200, 2000, 5000, 10000},
		},
	)
	stats.MustRegister(requestSizes)

	// Set up 12-hour-wide buckets, measured in seconds.
	buckets := make([]float64, 14)
	for i := range buckets {
		buckets[i] = 43200 * float64(i)
	}
	responseAges := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "ocsp_response_ages",
		Help:    "How old are the OCSP responses when we serve them. Must stay well below 84 hours.",
		Buckets: buckets,
	})
	stats.MustRegister(responseAges)

	responseTypes := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ocsp_responses",
			Help: "Number of OCSP responses returned by type",
		},
		[]string{"type"},
	)
	stats.MustRegister(responseTypes)

	return &Responder{
		Source:        source,
		timeout:       timeout,
		responseTypes: responseTypes,
		responseAges:  responseAges,
		requestSizes:  requestSizes,
		clk:           clock.New(),
		log:           logger,
		sampleRate:    sampleRate,
	}
}

type logEvent struct {
	IP       string        `json:"ip,omitempty"`
	UA       string        `json:"ua,omitempty"`
	Method   string        `json:"method,omitempty"`
	Path     string        `json:"path,omitempty"`
	Body     string        `json:"body,omitempty"`
	Received time.Time     `json:"received,omitempty"`
	Took     time.Duration `json:"took,omitempty"`
	Headers  http.Header   `json:"headers,omitempty"`

	Serial         string `json:"serial,omitempty"`
	IssuerKeyHash  string `json:"issuerKeyHash,omitempty"`
	IssuerNameHash string `json:"issuerNameHash,omitempty"`
	HashAlg        string `json:"hashAlg,omitempty"`
}

// hashToString contains mappings for the only hash functions
// x/crypto/ocsp supports
var hashToString = map[crypto.Hash]string{
	crypto.SHA1:   "SHA1",
	crypto.SHA256: "SHA256",
	crypto.SHA384: "SHA384",
	crypto.SHA512: "SHA512",
}

func SampledError(log blog.Logger, sampleRate int, format string, a ...interface{}) {
	if sampleRate > 0 && rand.Intn(sampleRate) == 0 {
		log.Errf(format, a...)
	}
}

func (rs Responder) sampledError(format string, a ...interface{}) {
	SampledError(rs.log, rs.sampleRate, format, a...)
}

// ServeHTTP is a Responder that can process both GET and POST requests. The
// mapping from an OCSP request to an OCSP response is done by the Source; the
// Responder simply decodes the request, and passes back whatever response is
// provided by the source.
// The Responder will set these headers:
//
//	Cache-Control: "max-age=(response.NextUpdate-now), public, no-transform, must-revalidate",
//	Last-Modified: response.ThisUpdate,
//	Expires: response.NextUpdate,
//	ETag: the SHA256 hash of the response, and
//	Content-Type: application/ocsp-response.
//
// Note: The caller must use http.StripPrefix to strip any path components
// (including '/') on GET requests.
// Do not use this responder in conjunction with http.NewServeMux, because the
// default handler will try to canonicalize path components by changing any
// strings of repeated '/' into a single '/', which will break the base64
// encoding.
func (rs Responder) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	// We specifically ignore request.Context() because we would prefer for clients
	// to not be able to cancel our operations in arbitrary places. Instead we
	// start a new context, and apply timeouts in our various RPCs.
	ctx := context.WithoutCancel(request.Context())
	request = request.WithContext(ctx)

	if rs.timeout != 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, rs.timeout)
		defer cancel()
	}

	le := logEvent{
		IP:       request.RemoteAddr,
		UA:       request.UserAgent(),
		Method:   request.Method,
		Path:     request.URL.Path,
		Received: time.Now(),
	}

	defer func() {
		le.Headers = response.Header()
		le.Took = time.Since(le.Received)
		jb, err := json.Marshal(le)
		if err != nil {
			// we log this error at the debug level as if we aren't at that level anyway
			// we shouldn't really care about marshalling the log event object
			rs.log.Debugf("failed to marshal log event object: %s", err)
			return
		}
		rs.log.Debugf("Received request: %s", string(jb))
	}()
	// By default we set a 'max-age=0, no-cache' Cache-Control header, this
	// is only returned to the client if a valid authorized OCSP response
	// is not found or an error is returned. If a response if found the header
	// will be altered to contain the proper max-age and modifiers.
	response.Header().Add("Cache-Control", "max-age=0, no-cache")
	// Read response from request
	var requestBody []byte
	var err error
	switch request.Method {
	case "GET":
		base64Request, err := url.QueryUnescape(request.URL.Path)
		if err != nil {
			rs.log.Debugf("Error decoding URL: %s", request.URL.Path)
			rs.responseTypes.With(prometheus.Labels{"type": responseTypeToString[ocsp.Malformed]}).Inc()
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		// url.QueryUnescape not only unescapes %2B escaping, but it additionally
		// turns the resulting '+' into a space, which makes base64 decoding fail.
		// So we go back afterwards and turn ' ' back into '+'. This means we
		// accept some malformed input that includes ' ' or %20, but that's fine.
		base64RequestBytes := []byte(base64Request)
		for i := range base64RequestBytes {
			if base64RequestBytes[i] == ' ' {
				base64RequestBytes[i] = '+'
			}
		}
		// In certain situations a UA may construct a request that has a double
		// slash between the host name and the base64 request body due to naively
		// constructing the request URL. In that case strip the leading slash
		// so that we can still decode the request.
		if len(base64RequestBytes) > 0 && base64RequestBytes[0] == '/' {
			base64RequestBytes = base64RequestBytes[1:]
		}
		requestBody, err = base64.StdEncoding.DecodeString(string(base64RequestBytes))
		if err != nil {
			rs.log.Debugf("Error decoding base64 from URL: %s", string(base64RequestBytes))
			response.WriteHeader(http.StatusBadRequest)
			rs.responseTypes.With(prometheus.Labels{"type": responseTypeToString[ocsp.Malformed]}).Inc()
			return
		}
	case "POST":
		requestBody, err = io.ReadAll(http.MaxBytesReader(nil, request.Body, 10000))
		if err != nil {
			rs.log.Errf("Problem reading body of POST: %s", err)
			response.WriteHeader(http.StatusBadRequest)
			rs.responseTypes.With(prometheus.Labels{"type": responseTypeToString[ocsp.Malformed]}).Inc()
			return
		}
		rs.requestSizes.Observe(float64(len(requestBody)))
	default:
		response.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	b64Body := base64.StdEncoding.EncodeToString(requestBody)
	rs.log.Debugf("Received OCSP request: %s", b64Body)
	if request.Method == http.MethodPost {
		le.Body = b64Body
	}

	// All responses after this point will be OCSP.
	// We could check for the content type of the request, but that
	// seems unnecessariliy restrictive.
	response.Header().Add("Content-Type", "application/ocsp-response")

	// Parse response as an OCSP request
	// XXX: This fails if the request contains the nonce extension.
	//      We don't intend to support nonces anyway, but maybe we
	//      should return unauthorizedRequest instead of malformed.
	ocspRequest, err := ocsp.ParseRequest(requestBody)
	if err != nil {
		rs.log.Debugf("Error decoding request body: %s", b64Body)
		response.WriteHeader(http.StatusBadRequest)
		response.Write(ocsp.MalformedRequestErrorResponse)
		rs.responseTypes.With(prometheus.Labels{"type": responseTypeToString[ocsp.Malformed]}).Inc()
		return
	}
	le.Serial = fmt.Sprintf("%x", ocspRequest.SerialNumber.Bytes())
	le.IssuerKeyHash = fmt.Sprintf("%x", ocspRequest.IssuerKeyHash)
	le.IssuerNameHash = fmt.Sprintf("%x", ocspRequest.IssuerNameHash)
	le.HashAlg = hashToString[ocspRequest.HashAlgorithm]

	// Look up OCSP response from source
	ocspResponse, err := rs.Source.Response(ctx, ocspRequest)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			response.Write(ocsp.UnauthorizedErrorResponse)
			rs.responseTypes.With(prometheus.Labels{"type": responseTypeToString[ocsp.Unauthorized]}).Inc()
			return
		} else if errors.Is(err, errOCSPResponseExpired) {
			rs.sampledError("Requested ocsp response is expired: serial %x, request body %s",
				ocspRequest.SerialNumber, b64Body)
			// HTTP StatusCode - unassigned
			response.WriteHeader(533)
			response.Write(ocsp.InternalErrorErrorResponse)
			rs.responseTypes.With(prometheus.Labels{"type": responseTypeToString[ocsp.Unauthorized]}).Inc()
			return
		}
		rs.sampledError("Error retrieving response for request: serial %x, request body %s, error: %s",
			ocspRequest.SerialNumber, b64Body, err)
		response.WriteHeader(http.StatusInternalServerError)
		response.Write(ocsp.InternalErrorErrorResponse)
		rs.responseTypes.With(prometheus.Labels{"type": responseTypeToString[ocsp.InternalError]}).Inc()
		return
	}

	// Write OCSP response
	response.Header().Add("Last-Modified", ocspResponse.ThisUpdate.Format(time.RFC1123))
	response.Header().Add("Expires", ocspResponse.NextUpdate.Format(time.RFC1123))
	now := rs.clk.Now()
	var maxAge int
	if now.Before(ocspResponse.NextUpdate) {
		maxAge = int(ocspResponse.NextUpdate.Sub(now) / time.Second)
	} else {
		// TODO(#530): we want max-age=0 but this is technically an authorized OCSP response
		//             (despite being stale) and 5019 forbids attaching no-cache
		maxAge = 0
	}
	response.Header().Set(
		"Cache-Control",
		fmt.Sprintf(
			"max-age=%d, public, no-transform, must-revalidate",
			maxAge,
		),
	)
	responseHash := sha256.Sum256(ocspResponse.Raw)
	response.Header().Add("ETag", fmt.Sprintf("\"%X\"", responseHash))

	serialString := core.SerialToString(ocspResponse.SerialNumber)
	if len(serialString) > 2 {
		// Set a cache tag that is equal to the last two bytes of the serial.
		// We expect that to be randomly distributed, so each tag should map to
		// about 1/256 of our responses.
		response.Header().Add("Edge-Cache-Tag", serialString[len(serialString)-2:])
	}

	// RFC 7232 says that a 304 response must contain the above
	// headers if they would also be sent for a 200 for the same
	// request, so we have to wait until here to do this
	if etag := request.Header.Get("If-None-Match"); etag != "" {
		if etag == fmt.Sprintf("\"%X\"", responseHash) {
			response.WriteHeader(http.StatusNotModified)
			return
		}
	}
	response.WriteHeader(http.StatusOK)
	response.Write(ocspResponse.Raw)
	rs.responseAges.Observe(rs.clk.Now().Sub(ocspResponse.ThisUpdate).Seconds())
	rs.responseTypes.With(prometheus.Labels{"type": responseTypeToString[ocsp.Success]}).Inc()
}
