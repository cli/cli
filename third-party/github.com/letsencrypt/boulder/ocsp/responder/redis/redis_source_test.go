package redis

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/jmhodges/clock"
	"github.com/letsencrypt/boulder/core"
	"github.com/letsencrypt/boulder/log"
	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/ocsp/responder"
	ocsp_test "github.com/letsencrypt/boulder/ocsp/test"
	"github.com/letsencrypt/boulder/rocsp"
	"github.com/letsencrypt/boulder/test"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/crypto/ocsp"
)

// notFoundRedis is a mock *rocsp.WritingClient that (a) returns "not found"
// for all GetResponse, and (b) sends all StoreResponse serial numbers to
// a channel. The latter is necessary because the code under test calls
// StoreResponse from a goroutine, so we need something to synchronize back to
// the testing goroutine.
// For tests where you do not expect StoreResponse to be called, set the chan
// to nil so sends will panic.
type notFoundRedis struct {
	serialStored chan *big.Int
}

func (nfr *notFoundRedis) GetResponse(ctx context.Context, serial string) ([]byte, error) {
	return nil, rocsp.ErrRedisNotFound
}

func (nfr *notFoundRedis) StoreResponse(ctx context.Context, resp *ocsp.Response) error {
	nfr.serialStored <- resp.SerialNumber
	return nil
}

type recordingSigner struct {
	serialRequested *big.Int
}

func (rs *recordingSigner) Response(ctx context.Context, req *ocsp.Request) (*responder.Response, error) {
	if rs.serialRequested != nil {
		panic("signed twice")
	}
	rs.serialRequested = req.SerialNumber
	// Return a fake response with only serial number filled, because that's
	// all the test cares about.
	return &responder.Response{Response: &ocsp.Response{
		SerialNumber: req.SerialNumber,
	}}, nil
}

func TestNotFound(t *testing.T) {
	recordingSigner := recordingSigner{}
	src, err := NewRedisSource(nil, &recordingSigner, time.Second, clock.NewFake(), metrics.NoopRegisterer, log.NewMock(), 1)
	test.AssertNotError(t, err, "making source")
	notFoundRedis := &notFoundRedis{make(chan *big.Int)}
	src.client = notFoundRedis

	serial := big.NewInt(987654321)
	_, err = src.Response(context.Background(), &ocsp.Request{
		SerialNumber: serial,
	})
	test.AssertNotError(t, err, "signing response when not found")
	if recordingSigner.serialRequested.Cmp(serial) != 0 {
		t.Errorf("issued signing request for serial %x; expected %x", recordingSigner.serialRequested, serial)
	}
	stored := <-notFoundRedis.serialStored
	if stored == nil {
		t.Fatalf("response was never stored")
	}
	if stored.Cmp(serial) != 0 {
		t.Errorf("stored response for serial %x; expected %x", notFoundRedis.serialStored, serial)
	}
}

type panicSource struct{}

func (ps panicSource) Response(ctx context.Context, req *ocsp.Request) (*responder.Response, error) {
	panic("shouldn't happen")
}

type errorRedis struct{}

func (er errorRedis) GetResponse(ctx context.Context, serial string) ([]byte, error) {
	return nil, errors.New("the enzabulators florbled")
}

func (er errorRedis) StoreResponse(ctx context.Context, resp *ocsp.Response) error {
	return nil
}

// When the initial Redis lookup returns an error, we should
// proceed with live signing.
func TestQueryError(t *testing.T) {
	serial := big.NewInt(314159)
	thisUpdate := time.Now().Truncate(time.Second).UTC()
	resp, _, err := ocsp_test.FakeResponse(ocsp.Response{
		SerialNumber: serial,
		Status:       ocsp.Good,
		ThisUpdate:   thisUpdate,
	})
	test.AssertNotError(t, err, "making fake response")
	source := echoSource{resp: resp}

	src, err := NewRedisSource(nil, source, time.Second, clock.NewFake(), metrics.NoopRegisterer, log.NewMock(), 1)
	test.AssertNotError(t, err, "making source")
	src.client = errorRedis{}

	receivedResp, err := src.Response(context.Background(), &ocsp.Request{
		SerialNumber: serial,
	})
	test.AssertNotError(t, err, "expected no error when Redis errored")
	test.AssertDeepEquals(t, resp.Raw, receivedResp.Raw)
	test.AssertMetricWithLabelsEquals(t, src.counter, prometheus.Labels{"result": "lookup_error"}, 1)
}

type garbleRedis struct{}

func (er garbleRedis) GetResponse(ctx context.Context, serial string) ([]byte, error) {
	return []byte("not a valid OCSP response, I can tell by the pixels"), nil
}

func (er garbleRedis) StoreResponse(ctx context.Context, resp *ocsp.Response) error {
	panic("shouldn't happen")
}

func TestParseError(t *testing.T) {
	src, err := NewRedisSource(nil, panicSource{}, time.Second, clock.NewFake(), metrics.NoopRegisterer, log.NewMock(), 1)
	test.AssertNotError(t, err, "making source")
	src.client = garbleRedis{}

	_, err = src.Response(context.Background(), &ocsp.Request{
		SerialNumber: big.NewInt(314159),
	})
	test.AssertError(t, err, "expected error when Redis returned junk")
	if errors.Is(err, rocsp.ErrRedisNotFound) {
		t.Errorf("incorrect error value ErrRedisNotFound; expected general error")
	}
}

func TestSignError(t *testing.T) {
	src, err := NewRedisSource(nil, errorSource{}, time.Second, clock.NewFake(), metrics.NoopRegisterer, log.NewMock(), 1)
	test.AssertNotError(t, err, "making source")
	src.client = &notFoundRedis{nil}

	_, err = src.Response(context.Background(), &ocsp.Request{
		SerialNumber: big.NewInt(2718),
	})
	test.AssertError(t, err, "Expected error when signer errored")
}

// staleRedis is a mock *rocsp.WritingClient that (a) returns response with a
// fixed ThisUpdate for all GetResponse, and (b) sends all StoreResponse serial
// numbers to a channel. The latter is necessary because the code under test
// calls StoreResponse from a goroutine, so we need something to synchronize
// back to the testing goroutine.
type staleRedis struct {
	serialStored chan *big.Int
	thisUpdate   time.Time
}

func (sr *staleRedis) GetResponse(ctx context.Context, serial string) ([]byte, error) {
	serInt, err := core.StringToSerial(serial)
	if err != nil {
		return nil, err
	}
	resp, _, err := ocsp_test.FakeResponse(ocsp.Response{
		SerialNumber: serInt,
		ThisUpdate:   sr.thisUpdate,
	})
	if err != nil {
		return nil, err
	}
	return resp.Raw, nil
}

func (sr *staleRedis) StoreResponse(ctx context.Context, resp *ocsp.Response) error {
	sr.serialStored <- resp.SerialNumber
	return nil
}

func TestStale(t *testing.T) {
	recordingSigner := recordingSigner{}
	clk := clock.NewFake()
	src, err := NewRedisSource(nil, &recordingSigner, time.Second, clk, metrics.NoopRegisterer, log.NewMock(), 1)
	test.AssertNotError(t, err, "making source")
	staleRedis := &staleRedis{
		serialStored: make(chan *big.Int),
		thisUpdate:   clk.Now().Add(-time.Hour),
	}
	src.client = staleRedis

	serial := big.NewInt(8675309)
	_, err = src.Response(context.Background(), &ocsp.Request{
		SerialNumber: serial,
	})
	test.AssertNotError(t, err, "signing response when not found")
	if recordingSigner.serialRequested == nil {
		t.Fatalf("signing source was never called")
	}
	if recordingSigner.serialRequested.Cmp(serial) != 0 {
		t.Errorf("issued signing request for serial %x; expected %x", recordingSigner.serialRequested, serial)
	}
	stored := <-staleRedis.serialStored
	if stored == nil {
		t.Fatalf("response was never stored")
	}
	if stored.Cmp(serial) != 0 {
		t.Errorf("stored response for serial %x; expected %x", staleRedis.serialStored, serial)
	}
}

// notFoundSigner is a Source that always returns NotFound.
type notFoundSigner struct{}

func (nfs notFoundSigner) Response(ctx context.Context, req *ocsp.Request) (*responder.Response, error) {
	return nil, responder.ErrNotFound
}

func TestCertificateNotFound(t *testing.T) {
	src, err := NewRedisSource(nil, notFoundSigner{}, time.Second, clock.NewFake(), metrics.NoopRegisterer, log.NewMock(), 1)
	test.AssertNotError(t, err, "making source")
	notFoundRedis := &notFoundRedis{nil}
	src.client = notFoundRedis

	_, err = src.Response(context.Background(), &ocsp.Request{
		SerialNumber: big.NewInt(777777777),
	})
	if !errors.Is(err, responder.ErrNotFound) {
		t.Errorf("expected NotFound error, got %s", err)
	}
}

func TestNoServeStale(t *testing.T) {
	clk := clock.NewFake()
	src, err := NewRedisSource(nil, errorSource{}, time.Second, clk, metrics.NoopRegisterer, log.NewMock(), 1)
	test.AssertNotError(t, err, "making source")
	staleRedis := &staleRedis{
		serialStored: nil,
		thisUpdate:   clk.Now().Add(-time.Hour),
	}
	src.client = staleRedis

	serial := big.NewInt(111111)
	_, err = src.Response(context.Background(), &ocsp.Request{
		SerialNumber: serial,
	})
	test.AssertError(t, err, "expected to error when signer was down")
}
