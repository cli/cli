package ca

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/crypto/ocsp"

	capb "github.com/letsencrypt/boulder/ca/proto"
	"github.com/letsencrypt/boulder/core"
	berrors "github.com/letsencrypt/boulder/errors"
	"github.com/letsencrypt/boulder/issuance"
	blog "github.com/letsencrypt/boulder/log"
)

// ocspImpl provides a backing implementation for the OCSP gRPC service.
type ocspImpl struct {
	capb.UnsafeOCSPGeneratorServer
	issuers      map[issuance.NameID]*issuance.Issuer
	ocspLifetime time.Duration
	ocspLogQueue *ocspLogQueue
	log          blog.Logger
	metrics      *caMetrics
	clk          clock.Clock
}

var _ capb.OCSPGeneratorServer = (*ocspImpl)(nil)

func NewOCSPImpl(
	issuers []*issuance.Issuer,
	ocspLifetime time.Duration,
	ocspLogMaxLength int,
	ocspLogPeriod time.Duration,
	logger blog.Logger,
	stats prometheus.Registerer,
	metrics *caMetrics,
	clk clock.Clock,
) (*ocspImpl, error) {
	issuersByNameID := make(map[issuance.NameID]*issuance.Issuer, len(issuers))
	for _, issuer := range issuers {
		issuersByNameID[issuer.NameID()] = issuer
	}

	if ocspLifetime < 8*time.Hour || ocspLifetime > 7*24*time.Hour {
		return nil, fmt.Errorf("invalid OCSP lifetime %q", ocspLifetime)
	}

	var ocspLogQueue *ocspLogQueue
	if ocspLogMaxLength > 0 {
		ocspLogQueue = newOCSPLogQueue(ocspLogMaxLength, ocspLogPeriod, stats, logger)
	}

	oi := &ocspImpl{
		issuers:      issuersByNameID,
		ocspLifetime: ocspLifetime,
		ocspLogQueue: ocspLogQueue,
		log:          logger,
		metrics:      metrics,
		clk:          clk,
	}
	return oi, nil
}

// LogOCSPLoop collects OCSP generation log events into bundles, and logs
// them periodically.
func (oi *ocspImpl) LogOCSPLoop() {
	if oi.ocspLogQueue != nil {
		oi.ocspLogQueue.loop()
	}
}

// Stop asks this ocspImpl to shut down. It must be called after the
// corresponding RPC service is shut down and there are no longer any inflight
// RPCs. It will attempt to drain any logging queues (which may block), and will
// return only when done.
func (oi *ocspImpl) Stop() {
	if oi.ocspLogQueue != nil {
		oi.ocspLogQueue.stop()
	}
}

// GenerateOCSP produces a new OCSP response and returns it
func (oi *ocspImpl) GenerateOCSP(ctx context.Context, req *capb.GenerateOCSPRequest) (*capb.OCSPResponse, error) {
	// req.Status, req.Reason, and req.RevokedAt are often 0, for non-revoked certs.
	if core.IsAnyNilOrZero(req, req.Serial, req.IssuerID) {
		return nil, berrors.InternalServerError("Incomplete generate OCSP request")
	}

	serialInt, err := core.StringToSerial(req.Serial)
	if err != nil {
		return nil, err
	}
	serial := serialInt

	issuer, ok := oi.issuers[issuance.NameID(req.IssuerID)]
	if !ok {
		return nil, fmt.Errorf("unrecognized issuer ID %d", req.IssuerID)
	}

	now := oi.clk.Now().Truncate(time.Minute)
	tbsResponse := ocsp.Response{
		Status:       ocspStatusToCode[req.Status],
		SerialNumber: serial,
		ThisUpdate:   now,
		NextUpdate:   now.Add(oi.ocspLifetime - time.Second),
	}
	if tbsResponse.Status == ocsp.Revoked {
		tbsResponse.RevokedAt = req.RevokedAt.AsTime()
		tbsResponse.RevocationReason = int(req.Reason)
	}

	if oi.ocspLogQueue != nil {
		oi.ocspLogQueue.enqueue(serial.Bytes(), now, tbsResponse.Status, tbsResponse.RevocationReason)
	}

	ocspResponse, err := ocsp.CreateResponse(issuer.Cert.Certificate, issuer.Cert.Certificate, tbsResponse, issuer.Signer)
	if err == nil {
		oi.metrics.signatureCount.With(prometheus.Labels{"purpose": "ocsp", "issuer": issuer.Name()}).Inc()
	} else {
		oi.metrics.noteSignError(err)
	}
	return &capb.OCSPResponse{Response: ocspResponse}, err
}

// ocspLogQueue accumulates OCSP logging events and writes several of them
// in a single log line. This reduces the number of log lines and bytes,
// which would otherwise be quite high. As of Jan 2021 we do approximately
// 550 rps of OCSP generation events. We can turn that into about 5.5 rps
// of log lines if we accumulate 100 entries per line, which amounts to about
// 3900 bytes per log line.
// Summary of log line usage:
// serial in hex: 36 bytes, separator characters: 2 bytes, status: 1 byte
// If maxLogLen is less than the length of a single log item, generate
// one log line for every item.
type ocspLogQueue struct {
	// Maximum length, in bytes, of a single log line.
	maxLogLen int
	// Maximum amount of time between OCSP logging events.
	period time.Duration
	queue  chan ocspLog
	// This allows the stop() function to block until we've drained the queue.
	wg     sync.WaitGroup
	depth  prometheus.Gauge
	logger blog.Logger
	clk    clock.Clock
}

type ocspLog struct {
	serial []byte
	time   time.Time
	status int
	reason int
}

func newOCSPLogQueue(
	maxLogLen int,
	period time.Duration,
	stats prometheus.Registerer,
	logger blog.Logger,
) *ocspLogQueue {
	depth := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ocsp_log_queue_depth",
			Help: "Number of OCSP generation log entries waiting to be written",
		})
	stats.MustRegister(depth)
	olq := ocspLogQueue{
		maxLogLen: maxLogLen,
		period:    period,
		queue:     make(chan ocspLog),
		wg:        sync.WaitGroup{},
		depth:     depth,
		logger:    logger,
		clk:       clock.New(),
	}
	olq.wg.Add(1)
	return &olq
}

func (olq *ocspLogQueue) enqueue(serial []byte, time time.Time, status, reason int) {
	olq.queue <- ocspLog{
		serial: append([]byte{}, serial...),
		time:   time,
		status: status,
		reason: reason,
	}
}

// To ensure we don't go over the max log line length, use a safety margin
// equal to the expected length of an entry.
const ocspSingleLogEntryLen = 39

// loop consumes events from the queue channel, batches them up, and
// logs them in batches of maxLogLen / 39, or every `period`,
// whichever comes first.
func (olq *ocspLogQueue) loop() {
	defer olq.wg.Done()
	done := false
	for !done {
		var builder strings.Builder
		deadline := olq.clk.After(olq.period)
	inner:
		for {
			olq.depth.Set(float64(len(olq.queue)))
			select {
			case ol, ok := <-olq.queue:
				if !ok {
					// Channel was closed, finish.
					done = true
					break inner
				}
				reasonStr := "_"
				if ol.status == ocsp.Revoked {
					reasonStr = fmt.Sprintf("%d", ol.reason)
				}
				fmt.Fprintf(&builder, "%x:%s,", ol.serial, reasonStr)
			case <-deadline:
				break inner
			}
			if builder.Len()+ocspSingleLogEntryLen >= olq.maxLogLen {
				break
			}
		}
		if builder.Len() > 0 {
			olq.logger.AuditInfof("OCSP signed: %s", builder.String())
		}
	}
}

// stop the loop, and wait for it to finish. This must be called only after
// it's guaranteed that nothing will call enqueue again (for instance, after
// the OCSPGenerator and CertificateAuthority services are shut down with
// no RPCs in flight). Otherwise, enqueue will panic.
// If this is called without previously starting a goroutine running `.loop()`,
// it will block forever.
func (olq *ocspLogQueue) stop() {
	close(olq.queue)
	olq.wg.Wait()
}

// OCSPGenerator is an interface which exposes both the auto-generated gRPC
// methods and our special-purpose log queue start and stop methods, so that
// they can be called from main without exporting the ocspImpl type.
type OCSPGenerator interface {
	capb.OCSPGeneratorServer
	LogOCSPLoop()
	Stop()
}
