// Package redis provides a Redis-based OCSP responder.
//
// This responder will first look for a response cached in Redis. If there is
// no response, or the response is too old, it will make a request to the RA
// for a freshly-signed response. If that succeeds, this responder will return
// the response to the user right away, while storing a copy to Redis in a
// separate goroutine.
//
// If the response was too old, but the request to the RA failed, this
// responder will serve the response anyhow. This allows for graceful
// degradation: it is better to serve a response that is 5 days old (outside
// the Baseline Requirements limits) than to serve no response at all.
// It's assumed that this will be wrapped in a responder.filterSource, which
// means that if a response is past its NextUpdate, we'll generate a 500.
package redis

import (
	"context"
	"errors"
	"time"

	"github.com/jmhodges/clock"
	"github.com/letsencrypt/boulder/core"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/ocsp/responder"
	"github.com/letsencrypt/boulder/rocsp"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/crypto/ocsp"

	berrors "github.com/letsencrypt/boulder/errors"
)

type rocspClient interface {
	GetResponse(ctx context.Context, serial string) ([]byte, error)
	StoreResponse(ctx context.Context, resp *ocsp.Response) error
}

type redisSource struct {
	client             rocspClient
	signer             responder.Source
	counter            *prometheus.CounterVec
	signAndSaveCounter *prometheus.CounterVec
	cachedResponseAges prometheus.Histogram
	clk                clock.Clock
	liveSigningPeriod  time.Duration
	// Error logs will be emitted at a rate of 1 in logSampleRate.
	// If logSampleRate is 0, no logs will be emitted.
	logSampleRate int
	// Note: this logger is not currently used, as all audit log events are from
	// the dbSource right now, but it should and will be used in the future.
	log blog.Logger
}

// NewRedisSource returns a responder.Source which will look up OCSP responses in a
// Redis table.
func NewRedisSource(
	client *rocsp.RWClient,
	signer responder.Source,
	liveSigningPeriod time.Duration,
	clk clock.Clock,
	stats prometheus.Registerer,
	log blog.Logger,
	logSampleRate int,
) (*redisSource, error) {
	counter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ocsp_redis_responses",
		Help: "Count of OCSP requests/responses by action taken by the redisSource",
	}, []string{"result"})
	stats.MustRegister(counter)

	signAndSaveCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "ocsp_redis_sign_and_save",
		Help: "Count of OCSP sign and save requests",
	}, []string{"cause", "result"})
	stats.MustRegister(signAndSaveCounter)

	// Set up 12-hour-wide buckets, measured in seconds.
	buckets := make([]float64, 14)
	for i := range buckets {
		buckets[i] = 43200 * float64(i)
	}

	cachedResponseAges := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "ocsp_redis_cached_response_ages",
		Help:    "How old are the cached OCSP responses when we successfully retrieve them.",
		Buckets: buckets,
	})
	stats.MustRegister(cachedResponseAges)

	var rocspReader rocspClient
	if client != nil {
		rocspReader = client
	}
	return &redisSource{
		client:             rocspReader,
		signer:             signer,
		counter:            counter,
		signAndSaveCounter: signAndSaveCounter,
		cachedResponseAges: cachedResponseAges,
		liveSigningPeriod:  liveSigningPeriod,
		clk:                clk,
		log:                log,
	}, nil
}

// Response implements the responder.Source interface. It looks up the requested OCSP
// response in the redis cluster.
func (src *redisSource) Response(ctx context.Context, req *ocsp.Request) (*responder.Response, error) {
	serialString := core.SerialToString(req.SerialNumber)

	respBytes, err := src.client.GetResponse(ctx, serialString)
	if err != nil {
		if errors.Is(err, rocsp.ErrRedisNotFound) {
			src.counter.WithLabelValues("not_found").Inc()
		} else {
			src.counter.WithLabelValues("lookup_error").Inc()
			responder.SampledError(src.log, src.logSampleRate, "looking for cached response: %s", err)
			// Proceed despite the error; when Redis is down we'd like to limp along with live signing
			// rather than returning an error to the client.
		}
		return src.signAndSave(ctx, req, causeNotFound)
	}

	resp, err := ocsp.ParseResponse(respBytes, nil)
	if err != nil {
		src.counter.WithLabelValues("parse_error").Inc()
		return nil, err
	}

	if src.isStale(resp) {
		src.counter.WithLabelValues("stale").Inc()
		freshResp, err := src.signAndSave(ctx, req, causeStale)
		// Note: we could choose to return the stale response (up to its actual
		// NextUpdate date), but if we pass the BR/root program limits, that
		// becomes a compliance problem; returning an error is an availability
		// problem and only becomes a compliance problem if we serve too many
		// of them for too long (the exact conditions are not clearly defined
		// by the BRs or root programs).
		if err != nil {
			return nil, err
		}
		return freshResp, nil
	}

	src.counter.WithLabelValues("success").Inc()
	return &responder.Response{Response: resp, Raw: respBytes}, nil
}

func (src *redisSource) isStale(resp *ocsp.Response) bool {
	age := src.clk.Since(resp.ThisUpdate)
	src.cachedResponseAges.Observe(age.Seconds())
	return age > src.liveSigningPeriod
}

type signAndSaveCause string

const (
	causeStale    signAndSaveCause = "stale"
	causeNotFound signAndSaveCause = "not_found"
	causeMismatch signAndSaveCause = "mismatch"
)

func (src *redisSource) signAndSave(ctx context.Context, req *ocsp.Request, cause signAndSaveCause) (*responder.Response, error) {
	resp, err := src.signer.Response(ctx, req)
	if errors.Is(err, responder.ErrNotFound) {
		src.signAndSaveCounter.WithLabelValues(string(cause), "certificate_not_found").Inc()
		return nil, responder.ErrNotFound
	} else if errors.Is(err, berrors.UnknownSerial) {
		// UnknownSerial is more interesting than NotFound, because it means we don't
		// have a record in the `serials` table, which is kept longer-term than the
		// `certificateStatus` table. That could mean someone is making up silly serial
		// numbers in their requests to us, or it could mean there's site on the internet
		// using a certificate that we don't have a record of in the `serials` table.
		src.signAndSaveCounter.WithLabelValues(string(cause), "unknown_serial").Inc()
		responder.SampledError(src.log, src.logSampleRate, "unknown serial: %s", core.SerialToString(req.SerialNumber))
		return nil, responder.ErrNotFound
	} else if err != nil {
		src.signAndSaveCounter.WithLabelValues(string(cause), "signing_error").Inc()
		return nil, err
	}
	src.signAndSaveCounter.WithLabelValues(string(cause), "signing_success").Inc()
	go func() {
		// We don't care about the error here, because if storing the response
		// fails, we'll just generate a new one on the next request.
		_ = src.client.StoreResponse(context.Background(), resp.Response)
	}()
	return resp, nil
}
