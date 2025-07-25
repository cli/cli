package grpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmhodges/clock"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/balancer/roundrobin"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/letsencrypt/boulder/grpc/test_proto"
	"github.com/letsencrypt/boulder/metrics"
	"github.com/letsencrypt/boulder/test"
	"github.com/letsencrypt/boulder/web"
)

var fc = clock.NewFake()

func testHandler(_ context.Context, i interface{}) (interface{}, error) {
	if i != nil {
		return nil, errors.New("")
	}
	fc.Sleep(time.Second)
	return nil, nil
}

func testInvoker(_ context.Context, method string, _, _ interface{}, _ *grpc.ClientConn, opts ...grpc.CallOption) error {
	switch method {
	case "-service-brokeTest":
		return errors.New("")
	case "-service-requesterCanceledTest":
		return status.Error(1, context.Canceled.Error())
	}
	fc.Sleep(time.Second)
	return nil
}

func TestServerInterceptor(t *testing.T) {
	serverMetrics, err := newServerMetrics(metrics.NoopRegisterer)
	test.AssertNotError(t, err, "creating server metrics")
	si := newServerMetadataInterceptor(serverMetrics, clock.NewFake())

	md := metadata.New(map[string]string{clientRequestTimeKey: "0"})
	ctxWithMetadata := metadata.NewIncomingContext(context.Background(), md)

	_, err = si.Unary(context.Background(), nil, nil, testHandler)
	test.AssertError(t, err, "si.intercept didn't fail with a context missing metadata")

	_, err = si.Unary(ctxWithMetadata, nil, nil, testHandler)
	test.AssertError(t, err, "si.intercept didn't fail with a nil grpc.UnaryServerInfo")

	_, err = si.Unary(ctxWithMetadata, nil, &grpc.UnaryServerInfo{FullMethod: "-service-test"}, testHandler)
	test.AssertNotError(t, err, "si.intercept failed with a non-nil grpc.UnaryServerInfo")

	_, err = si.Unary(ctxWithMetadata, 0, &grpc.UnaryServerInfo{FullMethod: "brokeTest"}, testHandler)
	test.AssertError(t, err, "si.intercept didn't fail when handler returned a error")
}

func TestClientInterceptor(t *testing.T) {
	clientMetrics, err := newClientMetrics(metrics.NoopRegisterer)
	test.AssertNotError(t, err, "creating client metrics")
	ci := clientMetadataInterceptor{
		timeout: time.Second,
		metrics: clientMetrics,
		clk:     clock.NewFake(),
	}

	err = ci.Unary(context.Background(), "-service-test", nil, nil, nil, testInvoker)
	test.AssertNotError(t, err, "ci.intercept failed with a non-nil grpc.UnaryServerInfo")

	err = ci.Unary(context.Background(), "-service-brokeTest", nil, nil, nil, testInvoker)
	test.AssertError(t, err, "ci.intercept didn't fail when handler returned a error")
}

// TestWaitForReadyTrue configures a gRPC client with waitForReady: true and
// sends a request to a backend that is unavailable. It ensures that the
// request doesn't error out until the timeout is reached, i.e. that
// FailFast is set to false.
// https://github.com/grpc/grpc/blob/main/doc/wait-for-ready.md
func TestWaitForReadyTrue(t *testing.T) {
	clientMetrics, err := newClientMetrics(metrics.NoopRegisterer)
	test.AssertNotError(t, err, "creating client metrics")
	ci := &clientMetadataInterceptor{
		timeout:      100 * time.Millisecond,
		metrics:      clientMetrics,
		clk:          clock.NewFake(),
		waitForReady: true,
	}
	conn, err := grpc.NewClient("localhost:19876", // random, probably unused port
		grpc.WithDefaultServiceConfig(fmt.Sprintf(`{"loadBalancingConfig": [{"%s":{}}]}`, roundrobin.Name)),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(ci.Unary))
	if err != nil {
		t.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := test_proto.NewChillerClient(conn)

	start := time.Now()
	_, err = c.Chill(context.Background(), &test_proto.Time{Duration: durationpb.New(time.Second)})
	if err == nil {
		t.Errorf("Successful Chill when we expected failure.")
	}
	if time.Since(start) < 90*time.Millisecond {
		t.Errorf("Chill failed fast, when WaitForReady should be enabled.")
	}
}

// TestWaitForReadyFalse configures a gRPC client with waitForReady: false and
// sends a request to a backend that is unavailable, and ensures that the request
// errors out promptly.
func TestWaitForReadyFalse(t *testing.T) {
	clientMetrics, err := newClientMetrics(metrics.NoopRegisterer)
	test.AssertNotError(t, err, "creating client metrics")
	ci := &clientMetadataInterceptor{
		timeout:      time.Second,
		metrics:      clientMetrics,
		clk:          clock.NewFake(),
		waitForReady: false,
	}
	conn, err := grpc.NewClient("localhost:19876", // random, probably unused port
		grpc.WithDefaultServiceConfig(fmt.Sprintf(`{"loadBalancingConfig": [{"%s":{}}]}`, roundrobin.Name)),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(ci.Unary))
	if err != nil {
		t.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	c := test_proto.NewChillerClient(conn)

	start := time.Now()
	_, err = c.Chill(context.Background(), &test_proto.Time{Duration: durationpb.New(time.Second)})
	if err == nil {
		t.Errorf("Successful Chill when we expected failure.")
	}
	if time.Since(start) > 200*time.Millisecond {
		t.Errorf("Chill failed slow, when WaitForReady should be disabled.")
	}
}

// testTimeoutServer is used to implement TestTimeouts, and will attempt to sleep for
// the given amount of time (unless it hits a timeout or cancel).
type testTimeoutServer struct {
	test_proto.UnimplementedChillerServer
}

// Chill implements ChillerServer.Chill
func (s *testTimeoutServer) Chill(ctx context.Context, in *test_proto.Time) (*test_proto.Time, error) {
	start := time.Now()
	// Sleep for either the requested amount of time, or the context times out or
	// is canceled.
	select {
	case <-time.After(in.Duration.AsDuration() * time.Nanosecond):
		spent := time.Since(start) / time.Nanosecond
		return &test_proto.Time{Duration: durationpb.New(spent)}, nil
	case <-ctx.Done():
		return nil, errors.New("unique error indicating that the server's shortened context timed itself out")
	}
}

func TestTimeouts(t *testing.T) {
	server := new(testTimeoutServer)
	client, _, stop := setup(t, server, clock.NewFake())
	defer stop()

	testCases := []struct {
		timeout             time.Duration
		expectedErrorPrefix string
	}{
		{250 * time.Millisecond, "rpc error: code = Unknown desc = unique error indicating that the server's shortened context timed itself out"},
		{100 * time.Millisecond, "Chiller.Chill timed out after 0 ms"},
		{10 * time.Millisecond, "Chiller.Chill timed out after 0 ms"},
	}
	for _, tc := range testCases {
		t.Run(tc.timeout.String(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), tc.timeout)
			defer cancel()
			_, err := client.Chill(ctx, &test_proto.Time{Duration: durationpb.New(time.Second)})
			if err == nil {
				t.Fatal("Got no error, expected a timeout")
			}
			if !strings.HasPrefix(err.Error(), tc.expectedErrorPrefix) {
				t.Errorf("Wrong error. Got %s, expected %s", err.Error(), tc.expectedErrorPrefix)
			}
		})
	}
}

func TestRequestTimeTagging(t *testing.T) {
	server := new(testTimeoutServer)
	serverMetrics, err := newServerMetrics(metrics.NoopRegisterer)
	test.AssertNotError(t, err, "creating server metrics")
	client, _, stop := setup(t, server, serverMetrics)
	defer stop()

	// Make an RPC request with the ChillerClient with a timeout higher than the
	// requested ChillerServer delay so that the RPC completes normally
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := client.Chill(ctx, &test_proto.Time{Duration: durationpb.New(time.Second * 5)}); err != nil {
		t.Fatalf("Unexpected error calling Chill RPC: %s", err)
	}

	// There should be one histogram sample in the serverInterceptor rpcLag stat
	test.AssertMetricWithLabelsEquals(t, serverMetrics.rpcLag, prometheus.Labels{}, 1)
}

func TestClockSkew(t *testing.T) {
	// Create two separate clocks for the client and server
	serverClk := clock.NewFake()
	serverClk.Set(time.Now())
	clientClk := clock.NewFake()
	clientClk.Set(time.Now())

	_, serverPort, stop := setup(t, &testTimeoutServer{}, serverClk)
	defer stop()

	clientMetrics, err := newClientMetrics(metrics.NoopRegisterer)
	test.AssertNotError(t, err, "creating client metrics")
	ci := &clientMetadataInterceptor{
		timeout: 30 * time.Second,
		metrics: clientMetrics,
		clk:     clientClk,
	}
	conn, err := grpc.NewClient(net.JoinHostPort("localhost", strconv.Itoa(serverPort)),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(ci.Unary))
	if err != nil {
		t.Fatalf("did not connect: %v", err)
	}

	client := test_proto.NewChillerClient(conn)

	// Create a context with plenty of timeout
	ctx, cancel := context.WithDeadline(context.Background(), clientClk.Now().Add(10*time.Second))
	defer cancel()

	// Attempt a gRPC request which should succeed
	_, err = client.Chill(ctx, &test_proto.Time{Duration: durationpb.New(100 * time.Millisecond)})
	test.AssertNotError(t, err, "should succeed with no skew")

	// Skew the client clock forward and the request should fail due to skew
	clientClk.Add(time.Hour)
	_, err = client.Chill(ctx, &test_proto.Time{Duration: durationpb.New(100 * time.Millisecond)})
	test.AssertError(t, err, "should fail with positive client skew")
	test.AssertContains(t, err.Error(), "very different time")

	// Skew the server clock forward and the request should fail due to skew
	serverClk.Add(2 * time.Hour)
	_, err = client.Chill(ctx, &test_proto.Time{Duration: durationpb.New(100 * time.Millisecond)})
	test.AssertError(t, err, "should fail with negative client skew")
	test.AssertContains(t, err.Error(), "very different time")
}

// blockedServer implements a ChillerServer with a Chill method that:
//  1. Calls Done() on the received waitgroup when receiving an RPC
//  2. Blocks the RPC on the roadblock waitgroup
//
// This is used by TestInFlightRPCStat to test that the gauge for in-flight RPCs
// is incremented and decremented as expected.
type blockedServer struct {
	test_proto.UnimplementedChillerServer
	roadblock, received sync.WaitGroup
}

// Chill implements ChillerServer.Chill
func (s *blockedServer) Chill(_ context.Context, _ *test_proto.Time) (*test_proto.Time, error) {
	// Note that a client RPC arrived
	s.received.Done()
	// Wait for the roadblock to be cleared
	s.roadblock.Wait()
	// Return a dummy spent value to adhere to the chiller protocol
	return &test_proto.Time{Duration: durationpb.New(time.Millisecond)}, nil
}

func TestInFlightRPCStat(t *testing.T) {
	// Create a new blockedServer to act as a ChillerServer
	server := &blockedServer{}

	metrics, err := newClientMetrics(metrics.NoopRegisterer)
	test.AssertNotError(t, err, "creating client metrics")

	client, _, stop := setup(t, server, metrics)
	defer stop()

	// Increment the roadblock waitgroup - this will cause all chill RPCs to
	// the server to block until we call Done()!
	server.roadblock.Add(1)

	// Increment the sentRPCs waitgroup - we use this to find out when all the
	// RPCs we want to send have been received and we can count the in-flight
	// gauge
	numRPCs := 5
	server.received.Add(numRPCs)

	// Fire off a few RPCs. They will block on the blockedServer's roadblock wg
	for range numRPCs {
		go func() {
			// Ignore errors, just chilllll.
			_, _ = client.Chill(context.Background(), &test_proto.Time{})
		}()
	}

	// wait until all of the client RPCs have been sent and are blocking. We can
	// now check the gauge.
	server.received.Wait()

	// Specify the labels for the RPCs we're interested in
	labels := prometheus.Labels{
		"service": "Chiller",
		"method":  "Chill",
	}

	// We expect the inFlightRPCs gauge for the Chiller.Chill RPCs to be equal to numRPCs.
	test.AssertMetricWithLabelsEquals(t, metrics.inFlightRPCs, labels, float64(numRPCs))

	// Unblock the blockedServer to let all of the Chiller.Chill RPCs complete
	server.roadblock.Done()
	// Sleep for a little bit to let all the RPCs complete
	time.Sleep(1 * time.Second)

	// Check the gauge value again
	test.AssertMetricWithLabelsEquals(t, metrics.inFlightRPCs, labels, 0)
}

func TestServiceAuthChecker(t *testing.T) {
	ac := authInterceptor{
		map[string]map[string]struct{}{
			"package.ServiceName": {
				"allowed.client": {},
				"also.allowed":   {},
			},
		},
	}

	// No allowlist is a bad configuration.
	ctx := context.Background()
	err := ac.checkContextAuth(ctx, "/package.OtherService/Method/")
	test.AssertError(t, err, "checking empty allowlist")

	// Context with no peering information is disallowed.
	err = ac.checkContextAuth(ctx, "/package.ServiceName/Method/")
	test.AssertError(t, err, "checking un-peered context")

	// Context with no auth info is disallowed.
	ctx = peer.NewContext(ctx, &peer.Peer{})
	err = ac.checkContextAuth(ctx, "/package.ServiceName/Method/")
	test.AssertError(t, err, "checking peer with no auth")

	// Context with no verified chains is disallowed.
	ctx = peer.NewContext(ctx, &peer.Peer{
		AuthInfo: credentials.TLSInfo{
			State: tls.ConnectionState{},
		},
	})
	err = ac.checkContextAuth(ctx, "/package.ServiceName/Method/")
	test.AssertError(t, err, "checking TLS with no valid chains")

	// Context with cert with wrong name is disallowed.
	ctx = peer.NewContext(ctx, &peer.Peer{
		AuthInfo: credentials.TLSInfo{
			State: tls.ConnectionState{
				VerifiedChains: [][]*x509.Certificate{
					{
						&x509.Certificate{
							DNSNames: []string{
								"disallowed.client",
							},
						},
					},
				},
			},
		},
	})
	err = ac.checkContextAuth(ctx, "/package.ServiceName/Method/")
	test.AssertError(t, err, "checking disallowed cert")

	// Context with cert with good name is allowed.
	ctx = peer.NewContext(ctx, &peer.Peer{
		AuthInfo: credentials.TLSInfo{
			State: tls.ConnectionState{
				VerifiedChains: [][]*x509.Certificate{
					{
						&x509.Certificate{
							DNSNames: []string{
								"disallowed.client",
								"also.allowed",
							},
						},
					},
				},
			},
		},
	})
	err = ac.checkContextAuth(ctx, "/package.ServiceName/Method/")
	test.AssertNotError(t, err, "checking allowed cert")
}

// testUserAgentServer stores the last value it saw in the user agent field of its context.
type testUserAgentServer struct {
	test_proto.UnimplementedChillerServer

	lastSeenUA string
}

// Chill implements ChillerServer.Chill
func (s *testUserAgentServer) Chill(ctx context.Context, in *test_proto.Time) (*test_proto.Time, error) {
	s.lastSeenUA = web.UserAgent(ctx)
	return nil, nil
}

func TestUserAgentMetadata(t *testing.T) {
	server := new(testUserAgentServer)
	client, _, stop := setup(t, server)
	defer stop()

	testUA := "test UA"
	ctx := web.WithUserAgent(context.Background(), testUA)

	_, err := client.Chill(ctx, &test_proto.Time{})
	if err != nil {
		t.Fatalf("calling c.Chill: %s", err)
	}

	if server.lastSeenUA != testUA {
		t.Errorf("last seen User-Agent on server side was %q, want %q", server.lastSeenUA, testUA)
	}
}

// setup creates a server and client, returning the created client, the running server's port, and a stop function.
func setup(t *testing.T, server test_proto.ChillerServer, opts ...any) (test_proto.ChillerClient, int, func()) {
	clk := clock.NewFake()
	serverMetricsVal, err := newServerMetrics(metrics.NoopRegisterer)
	test.AssertNotError(t, err, "creating server metrics")
	clientMetricsVal, err := newClientMetrics(metrics.NoopRegisterer)
	test.AssertNotError(t, err, "creating client metrics")

	for _, opt := range opts {
		switch optTyped := opt.(type) {
		case clock.FakeClock:
			clk = optTyped
		case clientMetrics:
			clientMetricsVal = optTyped
		case serverMetrics:
			serverMetricsVal = optTyped
		default:
			t.Fatalf("setup called with unrecognize option %#v", t)
		}
	}
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	port := lis.Addr().(*net.TCPAddr).Port

	si := newServerMetadataInterceptor(serverMetricsVal, clk)
	s := grpc.NewServer(grpc.UnaryInterceptor(si.Unary))
	test_proto.RegisterChillerServer(s, server)

	go func() {
		start := time.Now()
		err := s.Serve(lis)
		if err != nil && !strings.HasSuffix(err.Error(), "use of closed network connection") {
			t.Logf("s.Serve: %v after %s", err, time.Since(start))
		}
	}()

	ci := &clientMetadataInterceptor{
		timeout: 30 * time.Second,
		metrics: clientMetricsVal,
		clk:     clock.NewFake(),
	}
	conn, err := grpc.NewClient(net.JoinHostPort("localhost", strconv.Itoa(port)),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(ci.Unary))
	if err != nil {
		t.Fatalf("did not connect: %v", err)
	}
	return test_proto.NewChillerClient(conn), port, s.Stop
}
