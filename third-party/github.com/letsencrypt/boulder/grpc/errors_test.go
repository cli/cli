package grpc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/jmhodges/clock"
	berrors "github.com/letsencrypt/boulder/errors"
	"github.com/letsencrypt/boulder/grpc/test_proto"
	"github.com/letsencrypt/boulder/identifier"
	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/test"
)

type errorServer struct {
	test_proto.UnimplementedChillerServer
	err error
}

func (s *errorServer) Chill(_ context.Context, _ *test_proto.Time) (*test_proto.Time, error) {
	return nil, s.err
}

func TestErrorWrapping(t *testing.T) {
	serverMetrics, err := newServerMetrics(metrics.NoopRegisterer)
	test.AssertNotError(t, err, "creating server metrics")
	smi := newServerMetadataInterceptor(serverMetrics, clock.NewFake())
	clientMetrics, err := newClientMetrics(metrics.NoopRegisterer)
	test.AssertNotError(t, err, "creating client metrics")
	cmi := clientMetadataInterceptor{time.Second, clientMetrics, clock.NewFake(), true}
	srv := grpc.NewServer(grpc.UnaryInterceptor(smi.Unary))
	es := &errorServer{}
	test_proto.RegisterChillerServer(srv, es)
	lis, err := net.Listen("tcp", "127.0.0.1:")
	test.AssertNotError(t, err, "Failed to create listener")
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	conn, err := grpc.Dial(
		lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(cmi.Unary),
	)
	test.AssertNotError(t, err, "Failed to dial grpc test server")
	client := test_proto.NewChillerClient(conn)

	// RateLimitError with a RetryAfter of 500ms.
	expectRetryAfter := time.Millisecond * 500
	es.err = berrors.RateLimitError(expectRetryAfter, "yup")
	_, err = client.Chill(context.Background(), &test_proto.Time{})
	test.Assert(t, err != nil, fmt.Sprintf("nil error returned, expected: %s", err))
	test.AssertDeepEquals(t, err, es.err)
	var bErr *berrors.BoulderError
	ok := errors.As(err, &bErr)
	test.Assert(t, ok, "asserting error as boulder error")
	// Ensure we got a RateLimitError
	test.AssertErrorIs(t, bErr, berrors.RateLimit)
	// Ensure our RetryAfter is still 500ms.
	test.AssertEquals(t, bErr.RetryAfter, expectRetryAfter)

	test.AssertNil(t, wrapError(context.Background(), nil), "Wrapping nil should still be nil")
	test.AssertNil(t, unwrapError(nil, nil), "Unwrapping nil should still be nil")
}

// TestSubErrorWrapping tests that a boulder error with suberrors can be
// correctly wrapped and unwrapped across the RPC layer.
func TestSubErrorWrapping(t *testing.T) {
	serverMetrics, err := newServerMetrics(metrics.NoopRegisterer)
	test.AssertNotError(t, err, "creating server metrics")
	smi := newServerMetadataInterceptor(serverMetrics, clock.NewFake())
	clientMetrics, err := newClientMetrics(metrics.NoopRegisterer)
	test.AssertNotError(t, err, "creating client metrics")
	cmi := clientMetadataInterceptor{time.Second, clientMetrics, clock.NewFake(), true}
	srv := grpc.NewServer(grpc.UnaryInterceptor(smi.Unary))
	es := &errorServer{}
	test_proto.RegisterChillerServer(srv, es)
	lis, err := net.Listen("tcp", "127.0.0.1:")
	test.AssertNotError(t, err, "Failed to create listener")
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	conn, err := grpc.Dial(
		lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(cmi.Unary),
	)
	test.AssertNotError(t, err, "Failed to dial grpc test server")
	client := test_proto.NewChillerClient(conn)

	subErrors := []berrors.SubBoulderError{
		{
			Identifier: identifier.DNSIdentifier("chillserver.com"),
			BoulderError: &berrors.BoulderError{
				Type:   berrors.RejectedIdentifier,
				Detail: "2 ill 2 chill",
			},
		},
	}

	es.err = (&berrors.BoulderError{
		Type:   berrors.Malformed,
		Detail: "malformed chill req",
	}).WithSubErrors(subErrors)

	_, err = client.Chill(context.Background(), &test_proto.Time{})
	test.Assert(t, err != nil, fmt.Sprintf("nil error returned, expected: %s", err))
	test.AssertDeepEquals(t, err, es.err)
}
