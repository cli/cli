package updater

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"time"

	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/crypto/ocsp"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	capb "github.com/letsencrypt/boulder/ca/proto"
	"github.com/letsencrypt/boulder/core"
	"github.com/letsencrypt/boulder/core/proto"
	"github.com/letsencrypt/boulder/crl"
	cspb "github.com/letsencrypt/boulder/crl/storer/proto"
	"github.com/letsencrypt/boulder/issuance"
	blog "github.com/letsencrypt/boulder/log"
	sapb "github.com/letsencrypt/boulder/sa/proto"
)

type crlUpdater struct {
	issuers        map[issuance.NameID]*issuance.Certificate
	numShards      int
	shardWidth     time.Duration
	lookbackPeriod time.Duration
	updatePeriod   time.Duration
	updateTimeout  time.Duration
	maxParallelism int
	maxAttempts    int

	cacheControl  string
	expiresMargin time.Duration

	temporallyShardedPrefixes []string

	sa sapb.StorageAuthorityClient
	ca capb.CRLGeneratorClient
	cs cspb.CRLStorerClient

	tickHistogram  *prometheus.HistogramVec
	updatedCounter *prometheus.CounterVec

	log blog.Logger
	clk clock.Clock
}

func NewUpdater(
	issuers []*issuance.Certificate,
	numShards int,
	shardWidth time.Duration,
	lookbackPeriod time.Duration,
	updatePeriod time.Duration,
	updateTimeout time.Duration,
	maxParallelism int,
	maxAttempts int,
	cacheControl string,
	expiresMargin time.Duration,
	temporallyShardedPrefixes []string,
	sa sapb.StorageAuthorityClient,
	ca capb.CRLGeneratorClient,
	cs cspb.CRLStorerClient,
	stats prometheus.Registerer,
	log blog.Logger,
	clk clock.Clock,
) (*crlUpdater, error) {
	issuersByNameID := make(map[issuance.NameID]*issuance.Certificate, len(issuers))
	for _, issuer := range issuers {
		issuersByNameID[issuer.NameID()] = issuer
	}

	if numShards < 1 {
		return nil, fmt.Errorf("must have positive number of shards, got: %d", numShards)
	}

	if updatePeriod >= 24*time.Hour {
		return nil, fmt.Errorf("must update CRLs at least every 24 hours, got: %s", updatePeriod)
	}

	if updateTimeout >= updatePeriod {
		return nil, fmt.Errorf("update timeout must be less than period: %s !< %s", updateTimeout, updatePeriod)
	}

	if lookbackPeriod < 2*updatePeriod {
		return nil, fmt.Errorf("lookbackPeriod must be at least 2x updatePeriod: %s !< 2 * %s", lookbackPeriod, updatePeriod)
	}

	if maxParallelism <= 0 {
		maxParallelism = 1
	}

	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	tickHistogram := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "crl_updater_ticks",
		Help:    "A histogram of crl-updater tick latencies labeled by issuer and result",
		Buckets: []float64{0.01, 0.2, 0.5, 1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000},
	}, []string{"issuer", "result"})
	stats.MustRegister(tickHistogram)

	updatedCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "crl_updater_generated",
		Help: "A counter of CRL generation calls labeled by result",
	}, []string{"issuer", "result"})
	stats.MustRegister(updatedCounter)

	return &crlUpdater{
		issuersByNameID,
		numShards,
		shardWidth,
		lookbackPeriod,
		updatePeriod,
		updateTimeout,
		maxParallelism,
		maxAttempts,
		cacheControl,
		expiresMargin,
		temporallyShardedPrefixes,
		sa,
		ca,
		cs,
		tickHistogram,
		updatedCounter,
		log,
		clk,
	}, nil
}

// updateShardWithRetry calls updateShard repeatedly (with exponential backoff
// between attempts) until it succeeds or the max number of attempts is reached.
func (cu *crlUpdater) updateShardWithRetry(ctx context.Context, atTime time.Time, issuerNameID issuance.NameID, shardIdx int, chunks []chunk) error {
	deadline := cu.clk.Now().Add(cu.updateTimeout)
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	if chunks == nil {
		// Compute the shard map and relevant chunk boundaries, if not supplied.
		// Batch mode supplies this to avoid duplicate computation.
		shardMap, err := cu.getShardMappings(ctx, atTime)
		if err != nil {
			return fmt.Errorf("computing shardmap: %w", err)
		}
		chunks = shardMap[shardIdx%cu.numShards]
	}

	_, err := cu.sa.LeaseCRLShard(ctx, &sapb.LeaseCRLShardRequest{
		IssuerNameID: int64(issuerNameID),
		MinShardIdx:  int64(shardIdx),
		MaxShardIdx:  int64(shardIdx),
		Until:        timestamppb.New(deadline.Add(time.Minute)),
	})
	if err != nil {
		return fmt.Errorf("leasing shard: %w", err)
	}

	crlID := crl.Id(issuerNameID, shardIdx, crl.Number(atTime))

	for i := range cu.maxAttempts {
		// core.RetryBackoff always returns 0 when its first argument is zero.
		sleepTime := core.RetryBackoff(i, time.Second, time.Minute, 2)
		if i != 0 {
			cu.log.Errf(
				"Generating CRL failed, will retry in %vs: id=[%s] err=[%s]",
				sleepTime.Seconds(), crlID, err)
		}
		cu.clk.Sleep(sleepTime)

		err = cu.updateShard(ctx, atTime, issuerNameID, shardIdx, chunks)
		if err == nil {
			break
		}
	}
	if err != nil {
		return err
	}

	// Notify the database that that we're done.
	_, err = cu.sa.UpdateCRLShard(ctx, &sapb.UpdateCRLShardRequest{
		IssuerNameID: int64(issuerNameID),
		ShardIdx:     int64(shardIdx),
		ThisUpdate:   timestamppb.New(atTime),
	})
	if err != nil {
		return fmt.Errorf("updating db metadata: %w", err)
	}

	return nil
}

type crlStream interface {
	Recv() (*proto.CRLEntry, error)
}

// reRevoked returns the later of the two entries, only if the latter represents a valid
// re-revocation of the former (reason == KeyCompromise).
func reRevoked(a *proto.CRLEntry, b *proto.CRLEntry) (*proto.CRLEntry, error) {
	first, second := a, b
	if b.RevokedAt.AsTime().Before(a.RevokedAt.AsTime()) {
		first, second = b, a
	}
	if first.Reason != ocsp.KeyCompromise && second.Reason == ocsp.KeyCompromise {
		return second, nil
	}
	// The RA has logic to prevent re-revocation for any reason other than KeyCompromise,
	// so this should be impossible. The best we can do is error out.
	return nil, fmt.Errorf("certificate %s was revoked with reason %d at %s and re-revoked with invalid reason %d at %s",
		first.Serial, first.Reason, first.RevokedAt.AsTime(), second.Reason, second.RevokedAt.AsTime())
}

// addFromStream pulls `proto.CRLEntry` objects from a stream, adding them to the crlEntries map.
//
// Consolidates duplicates and checks for internal consistency of the results.
// If allowedSerialPrefixes is non-empty, only serials with that one-byte prefix (two hex-encoded
// bytes) will be accepted.
//
// Returns the number of entries received from the stream, regardless of whether they were accepted.
func addFromStream(crlEntries map[string]*proto.CRLEntry, stream crlStream, allowedSerialPrefixes []string) (int, error) {
	var count int
	for {
		entry, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return 0, fmt.Errorf("retrieving entry from SA: %w", err)
		}
		count++
		serialPrefix := entry.Serial[0:2]
		if len(allowedSerialPrefixes) > 0 && !slices.Contains(allowedSerialPrefixes, serialPrefix) {
			continue
		}
		previousEntry := crlEntries[entry.Serial]
		if previousEntry == nil {
			crlEntries[entry.Serial] = entry
			continue
		}
		if previousEntry.Reason == entry.Reason &&
			previousEntry.RevokedAt.AsTime().Equal(entry.RevokedAt.AsTime()) {
			continue
		}

		// There's a tiny possibility a certificate was re-revoked for KeyCompromise and
		// we got a different view of it from temporal sharding vs explicit sharding.
		// Prefer the re-revoked CRL entry, which must be the one with KeyCompromise.
		second, err := reRevoked(entry, previousEntry)
		if err != nil {
			return 0, err
		}
		crlEntries[entry.Serial] = second
	}
	return count, nil
}

// updateShard processes a single shard. It computes the shard's boundaries, gets
// the list of revoked certs in that shard from the SA, gets the CA to sign the
// resulting CRL, and gets the crl-storer to upload it. It returns an error if
// any of these operations fail.
func (cu *crlUpdater) updateShard(ctx context.Context, atTime time.Time, issuerNameID issuance.NameID, shardIdx int, chunks []chunk) (err error) {
	if shardIdx <= 0 {
		return fmt.Errorf("invalid shard %d", shardIdx)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	crlID := crl.Id(issuerNameID, shardIdx, crl.Number(atTime))

	start := cu.clk.Now()
	defer func() {
		// This func closes over the named return value `err`, so can reference it.
		result := "success"
		if err != nil {
			result = "failed"
		}
		cu.tickHistogram.WithLabelValues(cu.issuers[issuerNameID].Subject.CommonName, result).Observe(cu.clk.Since(start).Seconds())
		cu.updatedCounter.WithLabelValues(cu.issuers[issuerNameID].Subject.CommonName, result).Inc()
	}()

	cu.log.Infof(
		"Generating CRL shard: id=[%s] numChunks=[%d]", crlID, len(chunks))

	// Deduplicate the CRL entries by serial number, since we can get the same certificate via
	// both temporal sharding (GetRevokedCerts) and explicit sharding (GetRevokedCertsByShard).
	crlEntries := make(map[string]*proto.CRLEntry)

	for _, chunk := range chunks {
		saStream, err := cu.sa.GetRevokedCerts(ctx, &sapb.GetRevokedCertsRequest{
			IssuerNameID:  int64(issuerNameID),
			ExpiresAfter:  timestamppb.New(chunk.start),
			ExpiresBefore: timestamppb.New(chunk.end),
			RevokedBefore: timestamppb.New(atTime),
		})
		if err != nil {
			return fmt.Errorf("GetRevokedCerts: %w", err)
		}

		n, err := addFromStream(crlEntries, saStream, cu.temporallyShardedPrefixes)
		if err != nil {
			return fmt.Errorf("streaming GetRevokedCerts: %w", err)
		}

		cu.log.Infof(
			"Queried SA for CRL shard: id=[%s] expiresAfter=[%s] expiresBefore=[%s] numEntries=[%d]",
			crlID, chunk.start, chunk.end, n)
	}

	// Query for unexpired certificates, with padding to ensure that revoked certificates show
	// up in at least one CRL, even if they expire between revocation and CRL generation.
	expiresAfter := cu.clk.Now().Add(-cu.lookbackPeriod)

	saStream, err := cu.sa.GetRevokedCertsByShard(ctx, &sapb.GetRevokedCertsByShardRequest{
		IssuerNameID:  int64(issuerNameID),
		ShardIdx:      int64(shardIdx),
		ExpiresAfter:  timestamppb.New(expiresAfter),
		RevokedBefore: timestamppb.New(atTime),
	})
	if err != nil {
		return fmt.Errorf("GetRevokedCertsByShard: %w", err)
	}

	n, err := addFromStream(crlEntries, saStream, nil)
	if err != nil {
		return fmt.Errorf("streaming GetRevokedCertsByShard: %w", err)
	}

	cu.log.Infof(
		"Queried SA by CRL shard number: id=[%s] shardIdx=[%d] numEntries=[%d]", crlID, shardIdx, n)

	// Send the full list of CRL Entries to the CA.
	caStream, err := cu.ca.GenerateCRL(ctx)
	if err != nil {
		return fmt.Errorf("connecting to CA: %w", err)
	}

	err = caStream.Send(&capb.GenerateCRLRequest{
		Payload: &capb.GenerateCRLRequest_Metadata{
			Metadata: &capb.CRLMetadata{
				IssuerNameID: int64(issuerNameID),
				ThisUpdate:   timestamppb.New(atTime),
				ShardIdx:     int64(shardIdx),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("sending CA metadata: %w", err)
	}

	for _, entry := range crlEntries {
		err = caStream.Send(&capb.GenerateCRLRequest{
			Payload: &capb.GenerateCRLRequest_Entry{
				Entry: entry,
			},
		})
		if err != nil {
			return fmt.Errorf("sending entry to CA: %w", err)
		}
	}

	err = caStream.CloseSend()
	if err != nil {
		return fmt.Errorf("closing CA request stream: %w", err)
	}

	// Receive the full bytes of the signed CRL from the CA.
	crlLen := 0
	crlHash := sha256.New()
	var crlChunks [][]byte
	for {
		out, err := caStream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("receiving CRL bytes: %w", err)
		}

		crlLen += len(out.Chunk)
		crlHash.Write(out.Chunk)
		crlChunks = append(crlChunks, out.Chunk)
	}

	// Send the full bytes of the signed CRL to the Storer.
	csStream, err := cu.cs.UploadCRL(ctx)
	if err != nil {
		return fmt.Errorf("connecting to CRLStorer: %w", err)
	}

	err = csStream.Send(&cspb.UploadCRLRequest{
		Payload: &cspb.UploadCRLRequest_Metadata{
			Metadata: &cspb.CRLMetadata{
				IssuerNameID: int64(issuerNameID),
				Number:       atTime.UnixNano(),
				ShardIdx:     int64(shardIdx),
				CacheControl: cu.cacheControl,
				Expires:      timestamppb.New(atTime.Add(cu.updatePeriod).Add(cu.expiresMargin)),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("sending CRLStorer metadata: %w", err)
	}

	for _, chunk := range crlChunks {
		err = csStream.Send(&cspb.UploadCRLRequest{
			Payload: &cspb.UploadCRLRequest_CrlChunk{
				CrlChunk: chunk,
			},
		})
		if err != nil {
			return fmt.Errorf("uploading CRL bytes: %w", err)
		}
	}

	_, err = csStream.CloseAndRecv()
	if err != nil {
		return fmt.Errorf("closing CRLStorer upload stream: %w", err)
	}

	cu.log.Infof(
		"Generated CRL shard: id=[%s] size=[%d] hash=[%x]",
		crlID, crlLen, crlHash.Sum(nil))

	return nil
}

// anchorTime is used as a universal starting point against which other times
// can be compared. This time must be less than 290 years (2^63-1 nanoseconds)
// in the past, to ensure that Go's time.Duration can represent that difference.
// The significance of 2015-06-04 11:04:38 UTC is left as an exercise to the
// reader.
func anchorTime() time.Time {
	return time.Date(2015, time.June, 04, 11, 04, 38, 0, time.UTC)
}

// chunk represents a fixed slice of time during which some certificates
// presumably expired or will expire. Its non-unique index indicates which shard
// it will be mapped to. The start boundary is inclusive, the end boundary is
// exclusive.
type chunk struct {
	start time.Time
	end   time.Time
	Idx   int
}

// shardMap is a mapping of shard indices to the set of chunks which should be
// included in that shard. Under most circumstances there is a one-to-one
// mapping, but certain configuration (such as having very narrow shards, or
// having a very long lookback period) can result in more than one chunk being
// mapped to a single shard.
type shardMap [][]chunk

// getShardMappings determines which chunks are currently relevant, based on
// the current time, the configured lookbackPeriod, and the farthest-future
// certificate expiration in the database. It then maps all of those chunks to
// their corresponding shards, and returns that mapping.
//
// The idea here is that shards should be stable. Picture a timeline, divided
// into chunks. Number those chunks from 0 (starting at the anchor time) up to
// numShards, then repeat the cycle when you run out of numbers:
//
//	chunk:  0     1     2     3     4     0     1     2     3     4     0
//	     |-----|-----|-----|-----|-----|-----|-----|-----|-----|-----|-----...
//	     ^-anchorTime
//
// The total time window we care about goes from atTime-lookbackPeriod, forward
// through the time of the farthest-future notAfter date found in the database.
// The lookbackPeriod must be larger than the updatePeriod, to ensure that any
// certificates which were both revoked *and* expired since the last time we
// issued CRLs get included in this generation. Because these times are likely
// to fall in the middle of chunks, we include the whole chunks surrounding
// those times in our output CRLs:
//
//	included chunk:     4     0     1     2     3     4     0     1
//	      ...--|-----|-----|-----|-----|-----|-----|-----|-----|-----|-----...
//	atTime-lookbackPeriod-^   ^-atTime                lastExpiry-^
//
// Because this total period of time may include multiple chunks with the same
// number, we then coalesce these chunks into a single shard. Ideally, this
// will never happen: it should only happen if the lookbackPeriod is very
// large, or if the shardWidth is small compared to the lastExpiry (such that
// numShards * shardWidth is less than lastExpiry - atTime). In this example,
// shards 0, 1, and 4 all get the contents of two chunks mapped to them, while
// shards 2 and 3 get only one chunk each.
//
//	included chunk:     4     0     1     2     3     4     0     1
//	      ...--|-----|-----|-----|-----|-----|-----|-----|-----|-----|-----...
//	                    │     │     │     │     │     │     │     │
//	shard 0: <────────────────┘─────────────────────────────┘     │
//	shard 1: <──────────────────────┘─────────────────────────────┘
//	shard 2: <────────────────────────────┘     │     │
//	shard 3: <──────────────────────────────────┘     │
//	shard 4: <──────────┘─────────────────────────────┘
//
// Under this scheme, the shard to which any given certificate will be mapped is
// a function of only three things: that certificate's notAfter timestamp, the
// chunk width, and the number of shards.
func (cu *crlUpdater) getShardMappings(ctx context.Context, atTime time.Time) (shardMap, error) {
	res := make(shardMap, cu.numShards)

	// Get the farthest-future expiration timestamp to ensure we cover everything.
	lastExpiry, err := cu.sa.GetMaxExpiration(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}

	// Find the id number and boundaries of the earliest chunk we care about.
	first := atTime.Add(-cu.lookbackPeriod)
	c, err := GetChunkAtTime(cu.shardWidth, cu.numShards, first)
	if err != nil {
		return nil, err
	}

	// Iterate over chunks until we get completely beyond the farthest-future
	// expiration.
	for c.start.Before(lastExpiry.AsTime()) {
		res[c.Idx] = append(res[c.Idx], c)
		c = chunk{
			start: c.end,
			end:   c.end.Add(cu.shardWidth),
			Idx:   (c.Idx + 1) % cu.numShards,
		}
	}

	return res, nil
}

// GetChunkAtTime returns the chunk whose boundaries contain the given time.
// It is exported so that it can be used by both the crl-updater and the RA
// as we transition from dynamic to static shard mappings.
func GetChunkAtTime(shardWidth time.Duration, numShards int, atTime time.Time) (chunk, error) {
	// Compute the amount of time between the current time and the anchor time.
	timeSinceAnchor := atTime.Sub(anchorTime())
	if timeSinceAnchor == time.Duration(math.MaxInt64) || timeSinceAnchor < 0 {
		return chunk{}, errors.New("shard boundary math broken: anchor time too far away")
	}

	// Determine how many full chunks fit within that time, and from that the
	// index number of the desired chunk.
	chunksSinceAnchor := timeSinceAnchor.Nanoseconds() / shardWidth.Nanoseconds()
	chunkIdx := int(chunksSinceAnchor) % numShards

	// Determine the boundaries of the chunk.
	timeSinceChunk := time.Duration(timeSinceAnchor.Nanoseconds() % shardWidth.Nanoseconds())
	left := atTime.Add(-timeSinceChunk)
	right := left.Add(shardWidth)

	return chunk{left, right, chunkIdx}, nil
}
