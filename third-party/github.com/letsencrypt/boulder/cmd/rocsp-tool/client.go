package notmain

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"sync/atomic"
	"time"

	"github.com/jmhodges/clock"
	"golang.org/x/crypto/ocsp"
	"google.golang.org/protobuf/types/known/timestamppb"

	capb "github.com/letsencrypt/boulder/ca/proto"
	"github.com/letsencrypt/boulder/core"
	"github.com/letsencrypt/boulder/db"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/rocsp"
	"github.com/letsencrypt/boulder/sa"
	"github.com/letsencrypt/boulder/test/ocsp/helper"
)

type client struct {
	redis         *rocsp.RWClient
	db            *db.WrappedMap // optional
	ocspGenerator capb.OCSPGeneratorClient
	clk           clock.Clock
	scanBatchSize int
	logger        blog.Logger
}

// processResult represents the result of attempting to sign and store status
// for a single certificateStatus ID. If `err` is non-nil, it indicates the
// attempt failed.
type processResult struct {
	id  int64
	err error
}

func getStartingID(ctx context.Context, clk clock.Clock, db *db.WrappedMap) (int64, error) {
	// To scan the DB efficiently, we want to select only currently-valid certificates. There's a
	// handy expires index, but for selecting a large set of rows, using the primary key will be
	// more efficient. So first we find a good id to start with, then scan from there. Note: since
	// AUTO_INCREMENT can skip around a bit, we add padding to ensure we get all currently-valid
	// certificates.
	startTime := clk.Now().Add(-24 * time.Hour)
	var minID *int64
	err := db.QueryRowContext(
		ctx,
		"SELECT MIN(id) FROM certificateStatus WHERE notAfter >= ?",
		startTime,
	).Scan(&minID)
	if err != nil {
		return 0, fmt.Errorf("selecting minID: %w", err)
	}
	if minID == nil {
		return 0, fmt.Errorf("no entries in certificateStatus (where notAfter >= %s)", startTime)
	}
	return *minID, nil
}

func (cl *client) loadFromDB(ctx context.Context, speed ProcessingSpeed, startFromID int64) error {
	prevID := startFromID
	var err error
	if prevID == 0 {
		prevID, err = getStartingID(ctx, cl.clk, cl.db)
		if err != nil {
			return fmt.Errorf("getting starting ID: %w", err)
		}
	}

	// Find the current maximum id in certificateStatus. We do this because the table is always
	// growing. If we scanned until we saw a batch with no rows, we would scan forever.
	var maxID *int64
	err = cl.db.QueryRowContext(
		ctx,
		"SELECT MAX(id) FROM certificateStatus",
	).Scan(&maxID)
	if err != nil {
		return fmt.Errorf("selecting maxID: %w", err)
	}
	if maxID == nil {
		return fmt.Errorf("no entries in certificateStatus")
	}

	// Limit the rate of reading rows.
	frequency := time.Duration(float64(time.Second) / float64(time.Duration(speed.RowsPerSecond)))
	// a set of all inflight certificate statuses, indexed by their `ID`.
	inflightIDs := newInflight()
	statusesToSign := cl.scanFromDB(ctx, prevID, *maxID, frequency, inflightIDs)

	results := make(chan processResult, speed.ParallelSigns)
	var runningSigners int32
	for range speed.ParallelSigns {
		atomic.AddInt32(&runningSigners, 1)
		go cl.signAndStoreResponses(ctx, statusesToSign, results, &runningSigners)
	}

	var successCount, errorCount int64

	for result := range results {
		inflightIDs.remove(result.id)
		if result.err != nil {
			errorCount++
			if errorCount < 10 ||
				(errorCount < 1000 && rand.IntN(1000) < 100) ||
				(errorCount < 100000 && rand.IntN(1000) < 10) ||
				(rand.IntN(1000) < 1) {
				cl.logger.Errf("error: %s", result.err)
			}
		} else {
			successCount++
		}

		total := successCount + errorCount
		if total < 10 ||
			(total < 1000 && rand.IntN(1000) < 100) ||
			(total < 100000 && rand.IntN(1000) < 10) ||
			(rand.IntN(1000) < 1) {
			cl.logger.Infof("stored %d responses, %d errors", successCount, errorCount)
		}
	}

	cl.logger.Infof("done. processed %d successes and %d errors\n", successCount, errorCount)
	if inflightIDs.len() != 0 {
		return fmt.Errorf("inflightIDs non-empty! has %d items, lowest %d", inflightIDs.len(), inflightIDs.min())
	}

	return nil
}

// scanFromDB scans certificateStatus rows from the DB, starting with `minID`, and writes them to
// its output channel at a maximum frequency of `frequency`. When it's read all available rows, it
// closes its output channel and exits.
// If there is an error, it logs the error, closes its output channel, and exits.
func (cl *client) scanFromDB(ctx context.Context, prevID int64, maxID int64, frequency time.Duration, inflightIDs *inflight) <-chan *sa.CertStatusMetadata {
	statusesToSign := make(chan *sa.CertStatusMetadata)
	go func() {
		defer close(statusesToSign)

		var err error
		currentMin := prevID
		for currentMin < maxID {
			currentMin, err = cl.scanFromDBOneBatch(ctx, currentMin, frequency, statusesToSign, inflightIDs)
			if err != nil {
				cl.logger.Infof("error scanning rows: %s", err)
			}
		}
	}()
	return statusesToSign
}

// scanFromDBOneBatch scans up to `cl.scanBatchSize` rows from certificateStatus, in order, and
// writes them to `output`. When done, it returns the highest `id` it saw during the scan.
// We do this in batches because if we tried to scan the whole table in a single query, MariaDB
// would terminate the query after a certain amount of data transferred.
func (cl *client) scanFromDBOneBatch(ctx context.Context, prevID int64, frequency time.Duration, output chan<- *sa.CertStatusMetadata, inflightIDs *inflight) (int64, error) {
	rowTicker := time.NewTicker(frequency)

	clauses := "WHERE id > ? ORDER BY id LIMIT ?"
	params := []interface{}{prevID, cl.scanBatchSize}

	selector, err := db.NewMappedSelector[sa.CertStatusMetadata](cl.db)
	if err != nil {
		return -1, fmt.Errorf("initializing db map: %w", err)
	}

	rows, err := selector.QueryContext(ctx, clauses, params...)
	if err != nil {
		return -1, fmt.Errorf("scanning certificateStatus: %w", err)
	}

	var scanned int
	var previousID int64
	err = rows.ForEach(func(row *sa.CertStatusMetadata) error {
		<-rowTicker.C

		status, err := rows.Get()
		if err != nil {
			return fmt.Errorf("scanning row %d (previous ID %d): %w", scanned, previousID, err)
		}
		scanned++
		inflightIDs.add(status.ID)
		// Emit a log line every 100000 rows. For our current ~215M rows, that
		// will emit about 2150 log lines. This probably strikes a good balance
		// between too spammy and having a reasonably frequent checkpoint.
		if scanned%100000 == 0 {
			cl.logger.Infof("scanned %d certificateStatus rows. minimum inflight ID %d", scanned, inflightIDs.min())
		}
		output <- status
		previousID = status.ID
		return nil
	})
	if err != nil {
		return -1, err
	}

	return previousID, nil
}

// signAndStoreResponses consumes cert statuses on its input channel and writes them to its output
// channel. Before returning, it atomically decrements the provided runningSigners int. If the
// result is 0, indicating this was the last running signer, it closes its output channel.
func (cl *client) signAndStoreResponses(ctx context.Context, input <-chan *sa.CertStatusMetadata, output chan processResult, runningSigners *int32) {
	defer func() {
		if atomic.AddInt32(runningSigners, -1) <= 0 {
			close(output)
		}
	}()
	for status := range input {
		ocspReq := &capb.GenerateOCSPRequest{
			Serial:    status.Serial,
			IssuerID:  status.IssuerID,
			Status:    string(status.Status),
			Reason:    int32(status.RevokedReason), //nolint: gosec // Revocation reasons are guaranteed to be small, no risk of overflow.
			RevokedAt: timestamppb.New(status.RevokedDate),
		}
		result, err := cl.ocspGenerator.GenerateOCSP(ctx, ocspReq)
		if err != nil {
			output <- processResult{id: status.ID, err: err}
			continue
		}
		resp, err := ocsp.ParseResponse(result.Response, nil)
		if err != nil {
			output <- processResult{id: status.ID, err: err}
			continue
		}

		err = cl.redis.StoreResponse(ctx, resp)
		if err != nil {
			output <- processResult{id: status.ID, err: err}
		} else {
			output <- processResult{id: status.ID, err: nil}
		}
	}
}

type expiredError struct {
	serial string
	ago    time.Duration
}

func (e expiredError) Error() string {
	return fmt.Sprintf("response for %s expired %s ago", e.serial, e.ago)
}

func (cl *client) storeResponsesFromFiles(ctx context.Context, files []string) error {
	for _, respFile := range files {
		respBytes, err := os.ReadFile(respFile)
		if err != nil {
			return fmt.Errorf("reading response file %q: %w", respFile, err)
		}
		err = cl.storeResponse(ctx, respBytes)
		if err != nil {
			return err
		}
	}
	return nil
}

func (cl *client) storeResponse(ctx context.Context, respBytes []byte) error {
	resp, err := ocsp.ParseResponse(respBytes, nil)
	if err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	serial := core.SerialToString(resp.SerialNumber)

	if resp.NextUpdate.Before(cl.clk.Now()) {
		return expiredError{
			serial: serial,
			ago:    cl.clk.Now().Sub(resp.NextUpdate),
		}
	}

	cl.logger.Infof("storing response for %s, generated %s, ttl %g hours",
		serial,
		resp.ThisUpdate,
		time.Until(resp.NextUpdate).Hours(),
	)

	err = cl.redis.StoreResponse(ctx, resp)
	if err != nil {
		return fmt.Errorf("storing response: %w", err)
	}

	retrievedResponse, err := cl.redis.GetResponse(ctx, serial)
	if err != nil {
		return fmt.Errorf("getting response: %w", err)
	}

	parsedRetrievedResponse, err := ocsp.ParseResponse(retrievedResponse, nil)
	if err != nil {
		return fmt.Errorf("parsing retrieved response: %w", err)
	}
	cl.logger.Infof("retrieved %s", helper.PrettyResponse(parsedRetrievedResponse))
	return nil
}
