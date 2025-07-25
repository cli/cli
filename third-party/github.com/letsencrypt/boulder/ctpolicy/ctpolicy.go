package ctpolicy

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/letsencrypt/boulder/core"
	"github.com/letsencrypt/boulder/ctpolicy/loglist"
	berrors "github.com/letsencrypt/boulder/errors"
	blog "github.com/letsencrypt/boulder/log"
	pubpb "github.com/letsencrypt/boulder/publisher/proto"
)

const (
	succeeded = "succeeded"
	failed    = "failed"
)

// CTPolicy is used to hold information about SCTs required from various
// groupings
type CTPolicy struct {
	pub              pubpb.PublisherClient
	sctLogs          loglist.List
	infoLogs         loglist.List
	finalLogs        loglist.List
	stagger          time.Duration
	log              blog.Logger
	winnerCounter    *prometheus.CounterVec
	shardExpiryGauge *prometheus.GaugeVec
}

// New creates a new CTPolicy struct
func New(pub pubpb.PublisherClient, sctLogs loglist.List, infoLogs loglist.List, finalLogs loglist.List, stagger time.Duration, log blog.Logger, stats prometheus.Registerer) *CTPolicy {
	winnerCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sct_winner",
			Help: "Counter of logs which are selected for sct submission, by log URL and result (succeeded or failed).",
		},
		[]string{"url", "result"},
	)
	stats.MustRegister(winnerCounter)

	shardExpiryGauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ct_shard_expiration_seconds",
			Help: "CT shard end_exclusive field expressed as Unix epoch time, by operator and logID.",
		},
		[]string{"operator", "logID"},
	)
	stats.MustRegister(shardExpiryGauge)

	for _, log := range sctLogs {
		if log.EndExclusive.IsZero() {
			// Handles the case for non-temporally sharded logs too.
			shardExpiryGauge.WithLabelValues(log.Operator, log.Name).Set(float64(0))
		} else {
			shardExpiryGauge.WithLabelValues(log.Operator, log.Name).Set(float64(log.EndExclusive.Unix()))
		}
	}

	return &CTPolicy{
		pub:              pub,
		sctLogs:          sctLogs,
		infoLogs:         infoLogs,
		finalLogs:        finalLogs,
		stagger:          stagger,
		log:              log,
		winnerCounter:    winnerCounter,
		shardExpiryGauge: shardExpiryGauge,
	}
}

type result struct {
	log loglist.Log
	sct []byte
	err error
}

// GetSCTs retrieves exactly two SCTs from the total collection of configured
// log groups, with at most one SCT coming from each group. It expects that all
// logs run by a single operator (e.g. Google) are in the same group, to
// guarantee that SCTs from logs in different groups do not end up coming from
// the same operator. As such, it enforces Google's current CT Policy, which
// requires that certs have two SCTs from logs run by different operators.
func (ctp *CTPolicy) GetSCTs(ctx context.Context, cert core.CertDER, expiration time.Time) (core.SCTDERs, error) {
	// We'll cancel this sub-context when we have the two SCTs we need, to cause
	// any other ongoing submission attempts to quit.
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// This closure will be called in parallel once for each log.
	getOne := func(i int, l loglist.Log) ([]byte, error) {
		// Sleep a little bit to stagger our requests to the later logs. Use `i-1`
		// to compute the stagger duration so that the first two logs (indices 0
		// and 1) get negative or zero (i.e. instant) sleep durations. If the
		// context gets cancelled (most likely because we got enough SCTs from other
		// logs already) before the sleep is complete, quit instead.
		select {
		case <-subCtx.Done():
			return nil, subCtx.Err()
		case <-time.After(time.Duration(i-1) * ctp.stagger):
		}

		sct, err := ctp.pub.SubmitToSingleCTWithResult(ctx, &pubpb.Request{
			LogURL:       l.Url,
			LogPublicKey: base64.StdEncoding.EncodeToString(l.Key),
			Der:          cert,
			Kind:         pubpb.SubmissionType_sct,
		})
		if err != nil {
			return nil, fmt.Errorf("ct submission to %q (%q) failed: %w", l.Name, l.Url, err)
		}

		return sct.Sct, nil
	}

	// Identify the set of candidate logs whose temporal interval includes this
	// cert's expiry. Randomize the order of the logs so that we're not always
	// trying to submit to the same two.
	logs := ctp.sctLogs.ForTime(expiration).Permute()

	// Kick off a collection of goroutines to try to submit the precert to each
	// log. Ensure that the results channel has a buffer equal to the number of
	// goroutines we're kicking off, so that they're all guaranteed to be able to
	// write to it and exit without blocking and leaking.
	resChan := make(chan result, len(logs))
	for i, log := range logs {
		go func(i int, l loglist.Log) {
			sctDER, err := getOne(i, l)
			resChan <- result{log: l, sct: sctDER, err: err}
		}(i, log)
	}

	go ctp.submitPrecertInformational(cert, expiration)

	// Finally, collect SCTs and/or errors from our results channel. We know that
	// we can collect len(logs) results from the channel because every goroutine
	// is guaranteed to write one result (either sct or error) to the channel.
	results := make([]result, 0)
	errs := make([]string, 0)
	for range len(logs) {
		res := <-resChan
		if res.err != nil {
			errs = append(errs, res.err.Error())
			ctp.winnerCounter.WithLabelValues(res.log.Url, failed).Inc()
			continue
		}
		results = append(results, res)
		ctp.winnerCounter.WithLabelValues(res.log.Url, succeeded).Inc()

		scts := compliantSet(results)
		if scts != nil {
			return scts, nil
		}
	}

	// If we made it to the end of that loop, that means we never got two SCTs
	// to return. Error out instead.
	if ctx.Err() != nil {
		// We timed out (the calling function returned and canceled our context),
		// thereby causing all of our getOne sub-goroutines to be cancelled.
		return nil, berrors.MissingSCTsError("failed to get 2 SCTs before ctx finished: %s", ctx.Err())
	}
	return nil, berrors.MissingSCTsError("failed to get 2 SCTs, got %d error(s): %s", len(errs), strings.Join(errs, "; "))
}

// compliantSet returns a slice of SCTs which complies with all relevant CT Log
// Policy requirements, namely that the set of SCTs:
// - contain at least two SCTs, which
// - come from logs run by at least two different operators, and
// - contain at least one RFC6962-compliant (i.e. non-static/tiled) log.
//
// If no such set of SCTs exists, returns nil.
func compliantSet(results []result) core.SCTDERs {
	for _, first := range results {
		if first.err != nil {
			continue
		}
		for _, second := range results {
			if second.err != nil {
				continue
			}
			if first.log.Operator == second.log.Operator {
				// The two SCTs must come from different operators.
				continue
			}
			if first.log.Tiled && second.log.Tiled {
				// At least one must come from a non-tiled log.
				continue
			}
			return core.SCTDERs{first.sct, second.sct}
		}
	}
	return nil
}

// submitAllBestEffort submits the given certificate or precertificate to every
// log ("informational" for precerts, "final" for certs) configured in the policy.
// It neither waits for these submission to complete, nor tracks their success.
func (ctp *CTPolicy) submitAllBestEffort(blob core.CertDER, kind pubpb.SubmissionType, expiry time.Time) {
	logs := ctp.finalLogs
	if kind == pubpb.SubmissionType_info {
		logs = ctp.infoLogs
	}

	for _, log := range logs {
		if log.StartInclusive.After(expiry) || log.EndExclusive.Equal(expiry) || log.EndExclusive.Before(expiry) {
			continue
		}

		go func(log loglist.Log) {
			_, err := ctp.pub.SubmitToSingleCTWithResult(
				context.Background(),
				&pubpb.Request{
					LogURL:       log.Url,
					LogPublicKey: base64.StdEncoding.EncodeToString(log.Key),
					Der:          blob,
					Kind:         kind,
				},
			)
			if err != nil {
				ctp.log.Warningf("ct submission of cert to log %q failed: %s", log.Url, err)
			}
		}(log)
	}
}

// submitPrecertInformational submits precertificates to any configured
// "informational" logs, but does not care about success or returned SCTs.
func (ctp *CTPolicy) submitPrecertInformational(cert core.CertDER, expiration time.Time) {
	ctp.submitAllBestEffort(cert, pubpb.SubmissionType_info, expiration)
}

// SubmitFinalCert submits finalized certificates created from precertificates
// to any configured "final" logs, but does not care about success.
func (ctp *CTPolicy) SubmitFinalCert(cert core.CertDER, expiration time.Time) {
	ctp.submitAllBestEffort(cert, pubpb.SubmissionType_final, expiration)
}
