package redis

import (
	"context"
	"errors"
	"reflect"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/crypto/ocsp"

	"github.com/letsencrypt/boulder/core"
	"github.com/letsencrypt/boulder/db"
	berrors "github.com/letsencrypt/boulder/errors"
	blog "github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/ocsp/responder"
	"github.com/letsencrypt/boulder/sa"
	sapb "github.com/letsencrypt/boulder/sa/proto"
)

// dbSelector is a limited subset of the db.WrappedMap interface to allow for
// easier mocking of mysql operations in tests.
type dbSelector interface {
	SelectOne(ctx context.Context, holder interface{}, query string, args ...interface{}) error
}

// rocspSourceInterface expands on responder.Source by adding a private signAndSave method.
// This allows checkedRedisSource to trigger a live signing if the DB disagrees with Redis.
type rocspSourceInterface interface {
	Response(ctx context.Context, req *ocsp.Request) (*responder.Response, error)
	signAndSave(ctx context.Context, req *ocsp.Request, cause signAndSaveCause) (*responder.Response, error)
}

// checkedRedisSource implements the Source interface. It relies on two
// underlying datastores to provide its OCSP responses: a rocspSourceInterface
// (a Source that can also signAndSave new responses) to provide the responses
// themselves, and the database to double-check that those responses match the
// authoritative revocation status stored in the db.
// TODO(#6285): Inline the rocspSourceInterface into this type.
// TODO(#6295): Remove the dbMap after all deployments use the SA instead.
type checkedRedisSource struct {
	base    rocspSourceInterface
	dbMap   dbSelector
	sac     sapb.StorageAuthorityReadOnlyClient
	counter *prometheus.CounterVec
	log     blog.Logger
}

// NewCheckedRedisSource builds a source that queries both the DB and Redis, and confirms
// the value in Redis matches the DB.
func NewCheckedRedisSource(base *redisSource, dbMap dbSelector, sac sapb.StorageAuthorityReadOnlyClient, stats prometheus.Registerer, log blog.Logger) (*checkedRedisSource, error) {
	if base == nil {
		return nil, errors.New("base was nil")
	}

	// We have to use reflect here because these arguments are interfaces, and
	// thus checking for nil the normal way doesn't work reliably, because they
	// may be non-nil interfaces whose inner value is still nil, i.e. "boxed nil".
	// But using reflect here is okay, because we only expect this constructor to
	// be called once per process.
	if (reflect.TypeOf(sac) == nil || reflect.ValueOf(sac).IsNil()) &&
		(reflect.TypeOf(dbMap) == nil || reflect.ValueOf(dbMap).IsNil()) {
		return nil, errors.New("either SA gRPC or direct DB connection must be provided")
	}

	return newCheckedRedisSource(base, dbMap, sac, stats, log), nil
}

// newCheckedRedisSource is an internal-only constructor that takes a private interface as a parameter.
// We call this from tests and from NewCheckedRedisSource.
func newCheckedRedisSource(base rocspSourceInterface, dbMap dbSelector, sac sapb.StorageAuthorityReadOnlyClient, stats prometheus.Registerer, log blog.Logger) *checkedRedisSource {
	counter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "checked_rocsp_responses",
		Help: "Count of OCSP requests/responses from checkedRedisSource, by result",
	}, []string{"result"})
	stats.MustRegister(counter)

	return &checkedRedisSource{
		base:    base,
		dbMap:   dbMap,
		sac:     sac,
		counter: counter,
		log:     log,
	}
}

// Response implements the responder.Source interface. It looks up the requested OCSP
// response in the redis cluster and looks up the corresponding status in the DB. If
// the status disagrees with what redis says, it signs a fresh response and serves it.
func (src *checkedRedisSource) Response(ctx context.Context, req *ocsp.Request) (*responder.Response, error) {
	serialString := core.SerialToString(req.SerialNumber)

	var wg sync.WaitGroup
	wg.Add(2)
	var dbStatus *sapb.RevocationStatus
	var redisResult *responder.Response
	var redisErr, dbErr error
	go func() {
		defer wg.Done()
		if src.sac != nil {
			dbStatus, dbErr = src.sac.GetRevocationStatus(ctx, &sapb.Serial{Serial: serialString})
		} else {
			dbStatus, dbErr = sa.SelectRevocationStatus(ctx, src.dbMap, serialString)
		}
	}()
	go func() {
		defer wg.Done()
		redisResult, redisErr = src.base.Response(ctx, req)
	}()
	wg.Wait()

	if dbErr != nil {
		// If the DB says "not found", the certificate either doesn't exist or has
		// expired and been removed from the DB. We don't need to check the Redis error.
		if db.IsNoRows(dbErr) || errors.Is(dbErr, berrors.NotFound) {
			src.counter.WithLabelValues("not_found").Inc()
			return nil, responder.ErrNotFound
		}

		src.counter.WithLabelValues("db_error").Inc()
		return nil, dbErr
	}

	if redisErr != nil {
		src.counter.WithLabelValues("redis_error").Inc()
		return nil, redisErr
	}

	// If the DB status matches the status returned from the Redis pipeline, all is good.
	if agree(dbStatus, redisResult.Response) {
		src.counter.WithLabelValues("success").Inc()
		return redisResult, nil
	}

	// Otherwise, the DB is authoritative. Trigger a fresh signing.
	freshResult, err := src.base.signAndSave(ctx, req, causeMismatch)
	if err != nil {
		src.counter.WithLabelValues("revocation_re_sign_error").Inc()
		return nil, err
	}

	if agree(dbStatus, freshResult.Response) {
		src.counter.WithLabelValues("revocation_re_sign_success").Inc()
		return freshResult, nil
	}

	// This could happen for instance with replication lag, or if the
	// RA was talking to a different DB.
	src.counter.WithLabelValues("revocation_re_sign_mismatch").Inc()
	return nil, errors.New("freshly signed status did not match DB")

}

// agree returns true if the contents of the redisResult ocsp.Response agree with what's in the DB.
func agree(dbStatus *sapb.RevocationStatus, redisResult *ocsp.Response) bool {
	return dbStatus.Status == int64(redisResult.Status) &&
		dbStatus.RevokedReason == int64(redisResult.RevocationReason) &&
		dbStatus.RevokedDate.AsTime().Equal(redisResult.RevokedAt)
}
